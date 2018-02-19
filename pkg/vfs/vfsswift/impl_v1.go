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
	"strings"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/swift"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
)

const swiftV1ContainerPrefix = "cozy-"

const versionSuffix = "-version"
const maxFileSize = 5 << (3 * 10) // 5 GiB
const dirContentType = "directory"

type swiftVFS struct {
	vfs.Indexer
	vfs.DiskThresholder
	c         *swift.Connection
	container string
	version   string
	mu        lock.ErrorRWLocker
	log       *logrus.Entry
}

// New returns a vfs.VFS instance associated with the specified indexer and the
// swift storage url.
func New(index vfs.Indexer, disk vfs.DiskThresholder, mu lock.ErrorRWLocker, domain string) (vfs.VFS, error) {
	if domain == "" {
		return nil, fmt.Errorf("vfsswift: specified domain is empty")
	}
	return &swiftVFS{
		Indexer:         index,
		DiskThresholder: disk,

		c:         config.GetSwiftConnection(),
		container: swiftV1ContainerPrefix + domain,
		version:   swiftV1ContainerPrefix + domain + versionSuffix,
		mu:        mu,
		log:       logger.WithDomain(domain),
	}, nil
}

func (sfs *swiftVFS) InitFs() error {
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
	sfs.log.Infof("[vfsswift] Created container %s", sfs.container)
	return nil
}

func (sfs *swiftVFS) Delete() error {
	err := sfs.deleteContainer(sfs.version)
	if err != nil {
		sfs.log.Errorf("[vfsswift] Could not delete version container %s: %s",
			sfs.version, err.Error())
		return err
	}
	err = sfs.deleteContainer(sfs.container)
	if err != nil {
		sfs.log.Errorf("[vfsswift] Could not delete container %s: %s",
			sfs.container, err.Error())
		return err
	}
	sfs.log.Infof("[vfsswift] Deleted container %s", sfs.container)
	return nil
}

func (sfs *swiftVFS) deleteContainer(container string) error {
	_, _, err := sfs.c.Container(container)
	if err == swift.ContainerNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	objectNames, err := sfs.c.ObjectNamesAll(container, nil)
	if err != nil {
		return err
	}
	if len(objectNames) > 0 {
		_, err = sfs.c.BulkDelete(container, objectNames)
		if err != nil {
			return err
		}
	}
	return sfs.c.ContainerDelete(container)
}

func (sfs *swiftVFS) CreateDir(doc *vfs.DirDoc) error {
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
	objName := doc.DirID + "/" + doc.DocName
	f, err := sfs.c.ObjectCreate(sfs.container,
		objName,
		false,
		"",
		dirContentType,
		nil,
	)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if doc.ID() == "" {
		return sfs.Indexer.CreateDirDoc(doc)
	}
	return sfs.Indexer.CreateNamedDirDoc(doc)
}

func (sfs *swiftVFS) CreateFile(newdoc, olddoc *vfs.FileDoc) (vfs.File, error) {
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

	objName := newdoc.DirID + "/" + newdoc.DocName
	hash := hex.EncodeToString(newdoc.MD5Sum)
	f, err := sfs.c.ObjectCreate(
		sfs.container,
		objName,
		hash != "",
		hash,
		newdoc.Mime,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return &swiftFileCreation{
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

func (sfs *swiftVFS) DestroyDirContent(doc *vfs.DirDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	return sfs.destroyDirContent(doc)
}

func (sfs *swiftVFS) DestroyDirAndContent(doc *vfs.DirDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	return sfs.destroyDirAndContent(doc)
}

func (sfs *swiftVFS) DestroyFile(doc *vfs.FileDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	return sfs.destroyFile(doc)
}

func (sfs *swiftVFS) destroyDirContent(doc *vfs.DirDoc) error {
	iter := sfs.DirIterator(doc, nil)
	var errm error
	for {
		d, f, erri := iter.Next()
		if erri == vfs.ErrIteratorDone {
			return errm
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
			errm = multierror.Append(errm, errd)
		}
	}
}

func (sfs *swiftVFS) destroyDirAndContent(doc *vfs.DirDoc) error {
	err := sfs.destroyDirContent(doc)
	if err != nil {
		return err
	}
	err = sfs.c.ObjectDelete(sfs.container, doc.DirID+"/"+doc.DocName)
	if err != nil && err != swift.ObjectNotFound {
		return err
	}
	return sfs.Indexer.DeleteDirDoc(doc)
}

func (sfs *swiftVFS) destroyFile(doc *vfs.FileDoc) error {
	objName := doc.DirID + "/" + doc.DocName
	err := sfs.destroyFileVersions(objName)
	if err != nil {
		sfs.log.Errorf("[vfsswift] Could not delete version of %s: %s",
			objName, err.Error())
	}
	err = sfs.c.ObjectDelete(sfs.container, objName)
	if err != nil && err != swift.ObjectNotFound {
		return err
	}
	return sfs.Indexer.DeleteFileDoc(doc)
}

func (sfs *swiftVFS) destroyFileVersions(objName string) error {
	versionObjNames, err := sfs.c.VersionObjectList(sfs.version, objName)
	// could happened if the versionning could not be enabled, in which case we
	// do not propagate the error.
	if err == swift.ContainerNotFound || err == swift.ObjectNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if len(versionObjNames) > 0 {
		_, err = sfs.c.BulkDelete(sfs.version, versionObjNames)
		return err
	}
	return nil
}

func (sfs *swiftVFS) OpenFile(doc *vfs.FileDoc) (vfs.File, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	f, _, err := sfs.c.ObjectOpen(sfs.container, doc.DirID+"/"+doc.DocName, false, nil)
	if err == swift.ObjectNotFound {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	return &swiftFileOpen{f, nil}, nil
}

func (sfs *swiftVFS) Fsck() ([]*vfs.FsckLog, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	root, err := sfs.Indexer.DirByPath("/")
	if err != nil {
		return nil, err
	}
	var logbook []*vfs.FsckLog
	logbook, err = sfs.fsckWalk(root, logbook)
	if err != nil {
		return nil, err
	}
	sort.Slice(logbook, func(i, j int) bool {
		return logbook[i].Filename < logbook[j].Filename
	})
	return logbook, nil
}

func (sfs *swiftVFS) fsckWalk(dir *vfs.DirDoc, logbook []*vfs.FsckLog) ([]*vfs.FsckLog, error) {
	entries := make(map[string]struct{})
	iter := sfs.Indexer.DirIterator(dir, nil)
	for {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
			break
		}
		if err != nil {
			return nil, err
		}

		if f != nil {
			var info swift.Object
			fullpath := path.Join(dir.Fullpath, f.DocName)
			entries[f.DocName] = struct{}{}
			info, _, err = sfs.c.Object(sfs.container, f.DirID+"/"+f.DocName)
			if err == swift.ObjectNotFound {
				logbook = append(logbook, &vfs.FsckLog{
					Type:     vfs.FileMissing,
					IsFile:   true,
					FileDoc:  f,
					Filename: fullpath,
				})
			} else if err != nil {
				return nil, err
			} else if info.ContentType == dirContentType {
				var dirDoc *vfs.DirDoc
				name := path.Base(info.Name)
				dirDoc, err = vfs.NewDirDocWithParent(name, dir, nil)
				if err != nil {
					return nil, err
				}
				logbook = append(logbook, &vfs.FsckLog{
					Type:     vfs.TypeMismatch,
					IsFile:   true,
					DirDoc:   dirDoc,
					FileDoc:  f,
					Filename: fullpath,
				})
			}
		} else {
			entries[d.DocName] = struct{}{}
			if d.Fullpath == vfs.TrashDirName {
				continue
			}
			var info swift.Object
			info, _, err = sfs.c.Object(sfs.container, d.DirID+"/"+d.DocName)
			if err == swift.ObjectNotFound {
				logbook = append(logbook, &vfs.FsckLog{
					Type:     vfs.FileMissing,
					IsFile:   false,
					DirDoc:   d,
					Filename: d.Fullpath,
				})
			} else if err != nil {
				return nil, err
			} else if info.ContentType != dirContentType {
				var fileDoc *vfs.FileDoc
				_, fileDoc, err = objectToFileDocV1(dir, info)
				if err != nil {
					continue
				}
				logbook = append(logbook, &vfs.FsckLog{
					Type:     vfs.TypeMismatch,
					IsFile:   false,
					DirDoc:   d,
					FileDoc:  fileDoc,
					Filename: d.Fullpath,
				})
			} else {
				if logbook, err = sfs.fsckWalk(d, logbook); err != nil {
					return nil, err
				}
			}
		}
	}

	objects, err := sfs.c.ObjectsAll(sfs.container, &swift.ObjectsOpts{
		Path: dir.DocID,
	})
	if err != nil {
		return nil, err
	}
	for _, object := range objects {
		name := path.Base(object.Name)
		if _, ok := entries[name]; !ok {
			if object.Bytes == 0 {
				continue
			}
			filePath, fileDoc, err := objectToFileDocV1(dir, object)
			if err != nil {
				continue
			}
			logbook = append(logbook, &vfs.FsckLog{
				Type:     vfs.IndexMissing,
				IsFile:   true,
				FileDoc:  fileDoc,
				Filename: filePath,
			})
		}
	}

	return logbook, nil
}

func objectToFileDocV1(dir *vfs.DirDoc, object swift.Object) (filePath string, fileDoc *vfs.FileDoc, err error) {
	trashed := strings.HasPrefix(dir.Fullpath, vfs.TrashDirName)
	dirID := dir.DocID
	filePath = path.Join(dir.Fullpath, path.Base(object.Name))
	md5sum, err := hex.DecodeString(object.Hash)
	if err != nil {
		return
	}
	mime, class := vfs.ExtractMimeAndClass(object.ContentType)
	fileDoc, err = vfs.NewFileDoc(
		path.Base(object.Name),
		dirID,
		object.Bytes,
		md5sum,
		mime,
		class,
		object.LastModified,
		false,
		trashed,
		nil)
	return
}

// FsckPrune tries to fix the given list on inconsistencies in the VFS
func (sfs *swiftVFS) FsckPrune(logbook []*vfs.FsckLog, dryrun bool) {
	for _, entry := range logbook {
		switch entry.Type {
		case vfs.FileMissing, vfs.IndexMissing:
			vfs.FsckPrune(sfs, sfs.Indexer, entry, dryrun)
		case vfs.TypeMismatch:
			if entry.IsFile {
				// file on couchdb and directory on swift: we update the index to
				// remove the file index and create a directory one
				err := sfs.Indexer.DeleteFileDoc(entry.FileDoc)
				if err != nil {
					entry.PruneError = err
				}
				err = sfs.Indexer.CreateDirDoc(entry.DirDoc)
				if err != nil {
					entry.PruneError = err
				}
			} else {
				// directory on couchdb and file on swift: we keep the directory and
				// move the object into the orphan directory and create a new index
				// associated with it.
				orphanDir, err := vfs.Mkdir(sfs, vfs.OrphansDirName, nil)
				if err != nil {
					entry.PruneError = err
					continue
				}
				olddoc := entry.FileDoc
				newdoc := entry.FileDoc.Clone().(*vfs.FileDoc)
				newdoc.DirID = orphanDir.DirID
				err = sfs.c.ObjectMove(
					sfs.container, olddoc.DirID+"/"+olddoc.DocName,
					sfs.container, newdoc.DirID+"/"+newdoc.DocName,
				)
				if err != nil {
					entry.PruneError = err
					continue
				}
				_, err = sfs.c.ObjectCreate(sfs.container,
					olddoc.DirID+"/"+olddoc.DocName,
					false,
					"",
					dirContentType,
					nil,
				)
				if err != nil {
					entry.PruneError = err
					continue
				}
				err = sfs.Indexer.CreateFileDoc(newdoc)
				if err != nil {
					entry.PruneError = err
					continue
				}
			}
		}
	}
}

// UpdateFileDoc overrides the indexer's one since the swift fs indexes files
// using their DirID + Name value to preserve atomicity of the hierarchy.
//
// @override Indexer.UpdateFileDoc
func (sfs *swiftVFS) UpdateFileDoc(olddoc, newdoc *vfs.FileDoc) error {
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
		err = sfs.c.ObjectMove(
			sfs.container, olddoc.DirID+"/"+olddoc.DocName,
			sfs.container, newdoc.DirID+"/"+newdoc.DocName,
		)
		if err != nil {
			sfs.log.Errorf("[vfsswift] Could not move file %s/%s: %s",
				sfs.container, olddoc.DirID+"/"+olddoc.DocName, err.Error())
			return err
		}
	}
	return sfs.Indexer.UpdateFileDoc(olddoc, newdoc)
}

// UpdateDirDoc overrides the indexer's one since the swift fs indexes files
// using their DirID + Name value to preserve atomicity of the hierarchy.
//
// @override Indexer.UpdateDirDoc
func (sfs *swiftVFS) UpdateDirDoc(olddoc, newdoc *vfs.DirDoc) error {
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
		err = sfs.c.ObjectMove(
			sfs.container, olddoc.DirID+"/"+olddoc.DocName,
			sfs.container, newdoc.DirID+"/"+newdoc.DocName,
		)
		if err != nil {
			sfs.log.Errorf("[vfsswift] Could not move dir %s/%s: %s",
				sfs.container, olddoc.DirID+"/"+olddoc.DocName, err.Error())
			return err
		}
	}
	return sfs.Indexer.UpdateDirDoc(olddoc, newdoc)
}

func (sfs *swiftVFS) DirByID(fileID string) (*vfs.DirDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirByID(fileID)
}

func (sfs *swiftVFS) DirByPath(name string) (*vfs.DirDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirByPath(name)
}

func (sfs *swiftVFS) FileByID(fileID string) (*vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FileByID(fileID)
}

func (sfs *swiftVFS) FileByPath(name string) (*vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FileByPath(name)
}

func (sfs *swiftVFS) FilePath(doc *vfs.FileDoc) (string, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return "", lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FilePath(doc)
}

func (sfs *swiftVFS) DirOrFileByID(fileID string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirOrFileByID(fileID)
}

func (sfs *swiftVFS) DirOrFileByPath(name string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirOrFileByPath(name)
}

type swiftFileCreation struct {
	f       *swift.ObjectCreateFile
	w       int64
	size    int64
	fs      *swiftVFS
	name    string
	err     error
	meta    *vfs.MetaExtractor
	newdoc  *vfs.FileDoc
	olddoc  *vfs.FileDoc
	maxsize int64
}

func (f *swiftFileCreation) Read(p []byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileCreation) ReadAt(p []byte, off int64) (int, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileCreation) Seek(offset int64, whence int) (int64, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileCreation) Write(p []byte) (int, error) {
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

func (f *swiftFileCreation) Close() (err error) {
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
			etag := headers["Etag"]
			if l := len(etag); l >= 2 {
				if etag[0] == '"' {
					etag = etag[1:]
				}
				if etag[l-1] == '"' {
					etag = etag[:l-1]
				}
			}
			md5sum, err = hex.DecodeString(etag)
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

func (f *swiftFileCreation) CloseWithError(err error) error {
	if f.err == nil {
		f.err = err
	}
	return f.Close()
}

type swiftFileOpen struct {
	f  *swift.ObjectOpenFile
	br *bytes.Reader
}

func (f *swiftFileOpen) Read(p []byte) (int, error) {
	return f.f.Read(p)
}

func (f *swiftFileOpen) ReadAt(p []byte, off int64) (int, error) {
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

func (f *swiftFileOpen) Seek(offset int64, whence int) (int64, error) {
	return f.f.Seek(offset, whence)
}

func (f *swiftFileOpen) Write(p []byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileOpen) Close() error {
	return f.f.Close()
}

func (f *swiftFileOpen) CloseWithError(err error) error {
	return f.Close()
}

var (
	_ vfs.VFS  = &swiftVFS{}
	_ vfs.File = &swiftFileCreation{}
	_ vfs.File = &swiftFileOpen{}
)
