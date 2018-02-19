package vfsswift

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/swift"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
)

type swiftVFSV2 struct {
	vfs.Indexer
	vfs.DiskThresholder
	c             *swift.Connection
	container     string
	version       string
	dataContainer string
	mu            lock.ErrorRWLocker
	log           *logrus.Entry
}

const (
	swiftV2ContainerPrefixCozy = "cozy-v2-"
	swiftV2ContainerPrefixData = "data-v2-"
)

// NewV2 returns a vfs.VFS instance associated with the specified indexer and
// the swift storage url.
//
// This version implements a simpler layout where swift does not contain any
// hierarchy: meaning no index informations. This help with index incoherency
// and as many performance improvements regarding moving / renaming folders.
func NewV2(index vfs.Indexer, disk vfs.DiskThresholder, mu lock.ErrorRWLocker, domain string) (vfs.VFS, error) {
	if domain == "" {
		return nil, fmt.Errorf("vfsswift: specified domain is empty")
	}
	return &swiftVFSV2{
		Indexer:         index,
		DiskThresholder: disk,

		c:             config.GetSwiftConnection(),
		container:     swiftV2ContainerPrefixCozy + domain,
		version:       swiftV2ContainerPrefixCozy + domain + versionSuffix,
		dataContainer: swiftV2ContainerPrefixData + domain,
		mu:            mu,
		log:           logger.WithDomain(domain),
	}, nil
}

// MakeObjectName build the swift object name for a given file document. It
// creates a virtual subfolder by splitting the document ID, which should be 32
// bytes long, on the 27nth byte. This avoid having a flat hierarchy in swift with no bound
func MakeObjectName(docID string) string {
	if len(docID) != 32 {
		return docID
	}
	return docID[:22] + "/" + docID[22:27] + "/" + docID[27:]
}

func makeDocID(objName string) string {
	if len(objName) != 34 {
		return objName
	}
	return objName[:22] + objName[23:28] + objName[29:]
}

func (sfs *swiftVFSV2) InitFs() error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	if err := sfs.Indexer.InitIndex(); err != nil {
		return err
	}
	if err := sfs.c.VersionContainerCreate(sfs.container, sfs.version); err != nil {
		if err != swift.Forbidden {
			sfs.log.Errorf("[vfsswift] Could not create container %s: %s",
				sfs.container, err.Error())
			return err
		}
		sfs.log.Errorf("[vfsswift] Could not activate versioning for container %s: %s",
			sfs.container, err.Error())
		if err = sfs.c.ContainerDelete(sfs.version); err != nil {
			return err
		}
	}
	if err := sfs.c.ContainerCreate(sfs.dataContainer, nil); err != nil {
		sfs.log.Errorf("[vfsswift] Could not create container %s: %s",
			sfs.dataContainer, err.Error())
		return err
	}
	sfs.log.Infof("[vfsswift] Created container %s", sfs.container)
	return nil
}

func (sfs *swiftVFSV2) Delete() error {
	containerMeta := swift.Metadata{"to-be-deleted": "1"}.ContainerHeaders()
	sfs.log.Infof("[vfsswift] Marking containers %q, %q and %q as to-be-deleted",
		sfs.container, sfs.version, sfs.dataContainer)
	err1 := sfs.c.ContainerUpdate(sfs.container, containerMeta)
	err2 := sfs.c.ContainerUpdate(sfs.dataContainer, containerMeta)
	err3 := sfs.c.ContainerUpdate(sfs.version, containerMeta)
	if err1 != nil {
		sfs.log.Errorf("[vfsswift] Could not mark container %q as to-be-deleted: %s",
			sfs.container, err1)
		return err1
	}
	if err2 != nil {
		sfs.log.Errorf("[vfsswift] Could not mark container %q as to-be-deleted: %s",
			sfs.dataContainer, err2)
		return err2
	}
	if err3 != nil {
		sfs.log.Errorf("[vfsswift] Could not mark container %q as to-be-deleted: %s",
			sfs.version, err3)
		return err3
	}
	return nil
}

func (sfs *swiftVFSV2) CreateDir(doc *vfs.DirDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	exists, err := sfs.Indexer.DirChildExists(doc.DirID, doc.DocName)
	if err != nil {
		return err
	}
	if exists {
		return os.ErrExist
	}
	if doc.ID() == "" {
		return sfs.Indexer.CreateDirDoc(doc)
	}
	return sfs.Indexer.CreateNamedDirDoc(doc)
}

func (sfs *swiftVFSV2) CreateFile(newdoc, olddoc *vfs.FileDoc) (vfs.File, error) {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.Unlock()

	diskQuota := sfs.DiskQuota()

	var maxsize, newsize, oldsize int64
	newsize = newdoc.ByteSize
	if diskQuota > 0 {
		diskUsage, err := sfs.DiskUsage()
		if err != nil {
			return nil, err
		}
		if olddoc != nil {
			oldsize = olddoc.Size()
		}
		maxsize = diskQuota - diskUsage
		if maxsize > maxFileSize {
			maxsize = maxFileSize
		}
	} else {
		maxsize = maxFileSize
	}
	if maxsize <= 0 || (newsize >= 0 && (newsize-oldsize) > maxsize) {
		return nil, vfs.ErrFileTooBig
	}

	if olddoc != nil {
		newdoc.SetID(olddoc.ID())
		newdoc.SetRev(olddoc.Rev())
		newdoc.CreatedAt = olddoc.CreatedAt
	}

	newpath, err := sfs.Indexer.FilePath(newdoc)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(newpath, vfs.TrashDirName+"/") {
		return nil, vfs.ErrParentInTrash
	}

	// Avoid storing negative size in the index.
	if newdoc.ByteSize < 0 {
		newdoc.ByteSize = 0
	}

	if olddoc == nil {
		var exists bool
		exists, err = sfs.Indexer.DirChildExists(newdoc.DirID, newdoc.DocName)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, os.ErrExist
		}

		// When added to the index, the document is first considered hidden. This
		// flag will only be removed at the end of the upload when all its metadata
		// are known. See the Close() method.
		newdoc.Trashed = true

		if newdoc.ID() == "" {
			err = sfs.Indexer.CreateFileDoc(newdoc)
		} else {
			err = sfs.Indexer.CreateNamedFileDoc(newdoc)
		}
		if err != nil {
			return nil, err
		}
	}

	objName := MakeObjectName(newdoc.DocID)
	objMeta := swift.Metadata{
		"creation-name": newdoc.Name(),
		"created-at":    newdoc.CreatedAt.Format(time.RFC3339),
		"exec":          strconv.FormatBool(newdoc.Executable),
	}
	hash := hex.EncodeToString(newdoc.MD5Sum)
	f, err := sfs.c.ObjectCreate(
		sfs.container,
		objName,
		hash != "",
		hash,
		newdoc.Mime,
		objMeta.ObjectHeaders(),
	)
	if err != nil {
		return nil, err
	}
	return &swiftFileCreationV2{
		f:       f,
		fs:      sfs,
		w:       0,
		size:    newsize,
		name:    objName,
		meta:    vfs.NewMetaExtractor(newdoc),
		newdoc:  newdoc,
		olddoc:  olddoc,
		maxsize: maxsize,
	}, nil
}

func (sfs *swiftVFSV2) DestroyDirContent(doc *vfs.DirDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	return sfs.destroyDirContent(doc)
}

func (sfs *swiftVFSV2) DestroyDirAndContent(doc *vfs.DirDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	return sfs.destroyDirAndContent(doc)
}

func (sfs *swiftVFSV2) DestroyFile(doc *vfs.FileDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	return sfs.destroyFile(doc)
}

func (sfs *swiftVFSV2) destroyDirContent(doc *vfs.DirDoc) (err error) {
	iter := sfs.DirIterator(doc, nil)
	for {
		d, f, erri := iter.Next()
		if erri == vfs.ErrIteratorDone {
			return
		}
		if erri != nil {
			return erri
		}
		var errd error
		if d != nil {
			errd = sfs.destroyDirAndContent(d)
		} else {
			errd = sfs.destroyFile(f)
		}
		if errd != nil {
			err = multierror.Append(err, errd)
		}
	}
}

func (sfs *swiftVFSV2) destroyDirAndContent(doc *vfs.DirDoc) error {
	if err := sfs.destroyDirContent(doc); err != nil {
		return err
	}
	return sfs.Indexer.DeleteDirDoc(doc)
}

func (sfs *swiftVFSV2) destroyFile(doc *vfs.FileDoc) error {
	return sfs.Indexer.DeleteFileDoc(doc)
}

func (sfs *swiftVFSV2) OpenFile(doc *vfs.FileDoc) (vfs.File, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	objName := MakeObjectName(doc.DocID)
	f, _, err := sfs.c.ObjectOpen(sfs.container, objName, false, nil)
	if err == swift.ObjectNotFound {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	return &swiftFileOpenV2{f, nil}, nil
}

type fsckFile struct {
	file     *vfs.FileDoc
	fullpath string
}

func (sfs *swiftVFSV2) Fsck() ([]*vfs.FsckLog, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()

	root, err := sfs.Indexer.DirByID(consts.RootDirID)
	if err != nil {
		return nil, err
	}

	entries := make(map[string]fsckFile, 256)
	err = sfs.fsckWalk(root, entries)
	if err != nil {
		return nil, err
	}

	var logbook []*vfs.FsckLog

	err = sfs.c.ObjectsWalk(sfs.container, nil, func(opts *swift.ObjectsOpts) (interface{}, error) {
		var objs []swift.Object
		objs, err = sfs.c.Objects(sfs.container, opts)
		if err != nil {
			return nil, err
		}
		for _, obj := range objs {
			docID := makeDocID(obj.Name)
			f, ok := entries[docID]
			if !ok {
				var fileDoc *vfs.FileDoc
				var filePath string
				filePath, fileDoc, err = objectToFileDocV2(sfs.c, sfs.container, obj)
				if err != nil {
					return nil, err
				}
				logbook = append(logbook, &vfs.FsckLog{
					Type:     vfs.IndexMissing,
					IsFile:   true,
					FileDoc:  fileDoc,
					Filename: filePath,
				})
			} else {
				var md5sum []byte
				md5sum, err = hex.DecodeString(obj.Hash)
				if err != nil {
					return nil, err
				}
				if !bytes.Equal(md5sum, f.file.MD5Sum) {
					olddoc := f.file
					newdoc := olddoc.Clone().(*vfs.FileDoc)
					newdoc.MD5Sum = md5sum
					logbook = append(logbook, &vfs.FsckLog{
						Type:       vfs.ContentMismatch,
						IsFile:     true,
						FileDoc:    newdoc,
						OldFileDoc: olddoc,
						Filename:   f.fullpath,
					})
				}
				delete(entries, docID)
			}
		}
		return objs, err
	})
	if err != nil {
		return nil, err
	}

	// entries should contain only data that does not contain an associated
	// index.
	for docID, f := range entries {
		_, _, err = sfs.c.Object(sfs.container, docID)
		if err == swift.ObjectNotFound {
			logbook = append(logbook, &vfs.FsckLog{
				Type:     vfs.FileMissing,
				IsFile:   true,
				FileDoc:  f.file,
				Filename: f.fullpath,
			})
		} else if err != nil {
			return nil, err
		}
	}
	sort.Slice(logbook, func(i, j int) bool {
		return logbook[i].Filename < logbook[j].Filename
	})

	return logbook, nil
}

func (sfs *swiftVFSV2) fsckWalk(dir *vfs.DirDoc, entries map[string]fsckFile) error {
	iter := sfs.Indexer.DirIterator(dir, nil)
	for {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
			break
		}
		if err != nil {
			return err
		}
		if f != nil {
			fullpath := path.Join(dir.Fullpath, f.DocName)
			entries[f.DocID] = fsckFile{f, fullpath}
		} else if err = sfs.fsckWalk(d, entries); err != nil {
			return err
		}
	}
	return nil
}

// FsckPrune tries to fix the given list on inconsistencies in the VFS
func (sfs *swiftVFSV2) FsckPrune(logbook []*vfs.FsckLog, dryrun bool) {
	for _, entry := range logbook {
		vfs.FsckPrune(sfs, sfs.Indexer, entry, dryrun)
	}
}

// UpdateFileDoc calls the indexer UpdateFileDoc function and adds a few checks
// before actually calling this method:
//   - locks the filesystem for writing
//   - checks in case we have a move operation that the new path is available
//
// @override Indexer.UpdateFileDoc
func (sfs *swiftVFSV2) UpdateFileDoc(olddoc, newdoc *vfs.FileDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	if newdoc.DirID != olddoc.DirID || newdoc.DocName != olddoc.DocName {
		exists, err := sfs.Indexer.DirChildExists(newdoc.DirID, newdoc.DocName)
		if err != nil {
			return err
		}
		if exists {
			return os.ErrExist
		}
	}
	return sfs.Indexer.UpdateFileDoc(olddoc, newdoc)
}

// UdpdateDirDoc calls the indexer UdpdateDirDoc function and adds a few checks
// before actually calling this method:
//   - locks the filesystem for writing
//   - checks in case we have a move operation that the new path is available
//
// @override Indexer.UpdateDirDoc
func (sfs *swiftVFSV2) UpdateDirDoc(olddoc, newdoc *vfs.DirDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	if newdoc.DirID != olddoc.DirID || newdoc.DocName != olddoc.DocName {
		exists, err := sfs.Indexer.DirChildExists(newdoc.DirID, newdoc.DocName)
		if err != nil {
			return err
		}
		if exists {
			return os.ErrExist
		}
	}
	return sfs.Indexer.UpdateDirDoc(olddoc, newdoc)
}

func (sfs *swiftVFSV2) DirByID(fileID string) (*vfs.DirDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirByID(fileID)
}

func (sfs *swiftVFSV2) DirByPath(name string) (*vfs.DirDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirByPath(name)
}

func (sfs *swiftVFSV2) FileByID(fileID string) (*vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FileByID(fileID)
}

func (sfs *swiftVFSV2) FileByPath(name string) (*vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FileByPath(name)
}

func (sfs *swiftVFSV2) FilePath(doc *vfs.FileDoc) (string, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return "", lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FilePath(doc)
}

func (sfs *swiftVFSV2) DirOrFileByID(fileID string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirOrFileByID(fileID)
}

func (sfs *swiftVFSV2) DirOrFileByPath(name string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirOrFileByPath(name)
}

type swiftFileCreationV2 struct {
	f       *swift.ObjectCreateFile
	w       int64
	size    int64
	fs      *swiftVFSV2
	name    string
	err     error
	meta    *vfs.MetaExtractor
	newdoc  *vfs.FileDoc
	olddoc  *vfs.FileDoc
	maxsize int64
}

func (f *swiftFileCreationV2) Read(p []byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileCreationV2) ReadAt(p []byte, off int64) (int, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileCreationV2) Seek(offset int64, whence int) (int64, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileCreationV2) Write(p []byte) (int, error) {
	if f.meta != nil {
		if _, err := (*f.meta).Write(p); err != nil && err != io.ErrClosedPipe {
			(*f.meta).Abort(err)
			f.meta = nil
		}
	}

	n, err := f.f.Write(p)
	if err != nil {
		f.err = err
		return n, err
	}

	f.w += int64(n)
	if f.maxsize >= 0 && f.w > f.maxsize {
		f.err = vfs.ErrFileTooBig
		return n, f.err
	}

	if f.size >= 0 && f.w > f.size {
		f.err = vfs.ErrContentLengthMismatch
		return n, f.err
	}

	return n, nil
}

func (f *swiftFileCreationV2) Close() (err error) {
	defer func() {
		if err != nil {
			// Deleting the object should be secure since we use X-Versions-Location
			// on the container and the old object should be restored.
			f.fs.c.ObjectDelete(f.fs.container, f.name) // #nosec

			// If an error has occured that is not due to the index update, we should
			// delete the file from the index.
			_, isCouchErr := couchdb.IsCouchError(err)
			if !isCouchErr && f.olddoc == nil {
				f.fs.Indexer.DeleteFileDoc(f.newdoc) // #nosec
			}
		}
	}()

	if err = f.f.Close(); err != nil {
		if err == swift.ObjectCorrupted {
			err = vfs.ErrInvalidHash
		}
		if f.meta != nil {
			(*f.meta).Abort(err)
			f.meta = nil
		}
		if f.err == nil {
			f.err = err
		}
	}

	newdoc, olddoc, written := f.newdoc, f.olddoc, f.w
	if f.meta != nil {
		if errc := (*f.meta).Close(); errc == nil {
			newdoc.Metadata = (*f.meta).Result()
		}
	}

	if f.err != nil {
		return f.err
	}

	// The actual check of the optionally given md5 hash is handled by the swift
	// library.
	if newdoc.MD5Sum == nil {
		var headers swift.Headers
		var md5sum []byte
		headers, err = f.f.Headers()
		if err == nil {
			// Etags may be double-quoted
			eTag := headers["Etag"]
			if l := len(eTag); l >= 2 {
				if eTag[0] == '"' {
					eTag = eTag[1:]
				}
				if eTag[l-1] == '"' {
					eTag = eTag[:l-1]
				}
			}
			md5sum, err = hex.DecodeString(eTag)
			if err == nil {
				newdoc.MD5Sum = md5sum
			}
		}
	}

	if f.size < 0 {
		newdoc.ByteSize = written
	}

	if newdoc.ByteSize != written {
		return vfs.ErrContentLengthMismatch
	}

	// The document is already added to the index when closing the file creation
	// handler. When updating the content of the document with the final
	// informations (size, md5, ...) we can reuse the same document as olddoc.
	if olddoc == nil || !olddoc.Trashed {
		newdoc.Trashed = false
	}
	if olddoc == nil {
		olddoc = newdoc
	}
	lockerr := f.fs.mu.Lock()
	if lockerr != nil {
		return lockerr
	}
	defer f.fs.mu.Unlock()
	err = f.fs.Indexer.UpdateFileDoc(olddoc, newdoc)
	// If we reach a conflict error, the document has been modified while
	// uploading the content of the file.
	//
	// TODO: remove dep on couchdb, with a generalized conflict error for
	// UpdateFileDoc/UpdateDirDoc.
	if couchdb.IsConflictError(err) {
		resdoc, err := f.fs.Indexer.FileByID(olddoc.ID())
		if err != nil {
			return err
		}
		resdoc.Metadata = newdoc.Metadata
		resdoc.ByteSize = newdoc.ByteSize
		return f.fs.Indexer.UpdateFileDoc(resdoc, resdoc)
	}
	return
}

func (f *swiftFileCreationV2) CloseWithError(err error) error {
	if f.err == nil {
		f.err = err
	}
	return f.Close()
}

type swiftFileOpenV2 struct {
	f  *swift.ObjectOpenFile
	br *bytes.Reader
}

func (f *swiftFileOpenV2) Read(p []byte) (int, error) {
	return f.f.Read(p)
}

func (f *swiftFileOpenV2) ReadAt(p []byte, off int64) (int, error) {
	// TODO find something smarter than keeping the whole file in memory
	if f.br == nil {
		buf, err := ioutil.ReadAll(f.f)
		if err != nil {
			return 0, err
		}
		f.br = bytes.NewReader(buf)
	}
	return f.br.ReadAt(p, off)
}

func (f *swiftFileOpenV2) Seek(offset int64, whence int) (int64, error) {
	return f.f.Seek(offset, whence)
}

func (f *swiftFileOpenV2) Write(p []byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileOpenV2) Close() error {
	return f.f.Close()
}
func (f *swiftFileOpenV2) CloseWithError(err error) error {
	return f.Close()
}

func objectToFileDocV2(c *swift.Connection, container string, object swift.Object) (filePath string, fileDoc *vfs.FileDoc, err error) {
	var h swift.Headers
	_, h, err = c.Object(container, object.Name)
	if err != nil {
		return
	}
	md5sum, err := hex.DecodeString(object.Hash)
	if err != nil {
		return
	}
	objMeta := h.ObjectMetadata()
	name := objMeta["creation-name"]
	if name == "" {
		name = fmt.Sprintf("Unknown %s", utils.RandomString(10))
	}
	var cdate time.Time
	if v := objMeta["created-at"]; v != "" {
		cdate, _ = time.Parse(time.RFC3339, v)
	}
	if cdate.IsZero() {
		cdate = time.Now()
	}
	executable, _ := strconv.ParseBool(objMeta["exec"])
	mime, class := vfs.ExtractMimeAndClass(object.ContentType)
	filePath = path.Join(vfs.OrphansDirName, name)
	fileDoc, err = vfs.NewFileDoc(
		name,
		"",
		object.Bytes,
		md5sum,
		mime,
		class,
		cdate,
		executable,
		false,
		nil)
	return
}

var (
	_ vfs.VFS  = &swiftVFSV2{}
	_ vfs.File = &swiftFileCreationV2{}
	_ vfs.File = &swiftFileOpenV2{}
)
