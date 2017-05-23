package vfsswift

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/swift"
)

const versionSuffix = "-version"

type swiftVFS struct {
	vfs.Indexer
	vfs.DiskThresholder
	c         *swift.Connection
	container string
	version   string
	mu        lock.ErrorRWLocker
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
		container: "cozy-" + domain,
		version:   "cozy-" + domain + versionSuffix,
		mu:        mu,
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
			log.Errorf("[vfsswift] Could not create container %s: %s",
				sfs.container, err.Error())
			return err
		}
		log.Errorf("[vfsswift] Could not activate versioning for container %s: %s",
			sfs.container, err.Error())
		if err = sfs.c.ContainerDelete(sfs.version); err != nil {
			return err
		}
	}
	log.Infof("[vfsswift] Created container %s", sfs.container)
	return nil
}

func (sfs *swiftVFS) Delete() error {
	err := sfs.deleteContainer(sfs.version)
	if err != nil {
		log.Errorf("[vfsswift] Could not delete version container %s: %s",
			sfs.version, err.Error())
		return err
	}
	err = sfs.deleteContainer(sfs.container)
	if err != nil {
		log.Errorf("[vfsswift] Could not delete container %s: %s",
			sfs.container, err.Error())
		return err
	}
	log.Infof("[vfsswift] Deleted container %s", sfs.container)
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
	objName := doc.DirID + "/" + doc.DocName
	_, _, err := sfs.c.Object(sfs.container, objName)
	if err != swift.ObjectNotFound {
		if err != nil {
			return err
		}
		return os.ErrExist
	}
	f, err := sfs.c.ObjectCreate(sfs.container,
		objName,
		false,
		"",
		"directory",
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

	var maxsize, newsize int64
	newsize = newdoc.ByteSize
	if diskQuota > 0 {
		diskUsage, err := sfs.DiskUsage()
		if err != nil {
			return nil, err
		}

		var oldsize int64
		if olddoc != nil {
			oldsize = olddoc.Size()
		}
		maxsize = diskQuota - diskUsage
		if maxsize <= 0 || (newsize >= 0 && (newsize-oldsize) > maxsize) {
			return nil, vfs.ErrFileTooBig
		}
	} else {
		maxsize = -1 // no limit
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

	objName := newdoc.DirID + "/" + newdoc.DocName
	if olddoc == nil {
		_, _, err = sfs.c.Object(sfs.container, objName)
		if err != swift.ObjectNotFound {
			if err != nil {
				return nil, err
			}
			return nil, os.ErrExist
		}
	}

	var h swift.Headers
	if newsize >= 0 {
		h = swift.Headers{"Content-Length": strconv.FormatInt(newsize, 10)}
	}
	hash := hex.EncodeToString(newdoc.MD5Sum)
	f, err := sfs.c.ObjectCreate(
		sfs.container,
		objName,
		hash != "",
		hash,
		newdoc.Mime,
		h,
	)
	if err != nil {
		return nil, err
	}
	return &swiftFileCreation{
		f:       f,
		fs:      sfs,
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
	for {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
			break
		}
		if d != nil {
			err = sfs.destroyDirAndContent(d)
		} else {
			err = sfs.destroyFile(f)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (sfs *swiftVFS) destroyDirAndContent(doc *vfs.DirDoc) error {
	err := sfs.destroyDirContent(doc)
	if err != nil {
		return err
	}
	if err := sfs.c.ObjectDelete(sfs.container, doc.DirID+"/"+doc.DocName); err != nil {
		return err
	}
	return sfs.Indexer.DeleteDirDoc(doc)
}

func (sfs *swiftVFS) destroyFile(doc *vfs.FileDoc) error {
	objName := doc.DirID + "/" + doc.DocName
	err := sfs.c.ObjectDelete(sfs.container, objName)
	if err != nil {
		return err
	}
	versionObjNames, err := sfs.c.VersionObjectList(sfs.version, objName)
	// could happened if the versionning could not be enabled, in which case we
	// do not propagate the error.
	if err == swift.ContainerNotFound {
		return sfs.Indexer.DeleteFileDoc(doc)
	}
	if err != nil {
		return err
	}
	if len(versionObjNames) > 0 {
		_, err = sfs.c.BulkDelete(sfs.version, versionObjNames)
		if err != nil {
			log.Errorf("[vfsswift] Could not delete version of %s: %s",
				objName, err.Error())
		}
	}
	return sfs.Indexer.DeleteFileDoc(doc)
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
	return &swiftFileOpen{f}, nil
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
		err := sfs.c.ObjectMove(
			sfs.container, olddoc.DirID+"/"+olddoc.DocName,
			sfs.container, newdoc.DirID+"/"+newdoc.DocName,
		)
		if err != nil {
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
		err := sfs.c.ObjectMove(
			sfs.container, olddoc.DirID+"/"+olddoc.DocName,
			sfs.container, newdoc.DirID+"/"+newdoc.DocName,
		)
		if err != nil {
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

	size := f.newdoc.ByteSize
	if size >= 0 && f.w > size {
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
		}
	}()

	if err = f.f.Close(); err != nil {
		if f.meta != nil {
			(*f.meta).Abort(err)
		}
		if f.err == nil {
			f.err = err
		}
	}

	if f.err != nil {
		return f.err
	}

	newdoc, olddoc, written := f.newdoc, f.olddoc, f.w
	if f.meta != nil {
		if errc := (*f.meta).Close(); errc == nil {
			newdoc.Metadata = (*f.meta).Result()
		}
	}

	// The actual check of the optionally given md5 hash is handled by the swift
	// library.
	if newdoc.MD5Sum == nil {
		headers, err := f.f.Headers()
		if err == nil {
			md5sum, err := hex.DecodeString(headers["Etag"])
			if err == nil {
				newdoc.MD5Sum = md5sum
			}
		}
	}

	if newdoc.ByteSize < 0 {
		newdoc.ByteSize = written
	}

	if newdoc.ByteSize != written {
		return vfs.ErrContentLengthMismatch
	}

	if olddoc == nil {
		if newdoc.ID() == "" {
			return f.fs.Indexer.CreateFileDoc(newdoc)
		}
		return f.fs.Indexer.CreateNamedFileDoc(newdoc)
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
	return nil
}

type swiftFileOpen struct {
	f *swift.ObjectOpenFile
}

func (f *swiftFileOpen) Read(p []byte) (int, error) {
	return f.f.Read(p)
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

var (
	_ vfs.VFS  = &swiftVFS{}
	_ vfs.File = &swiftFileCreation{}
	_ vfs.File = &swiftFileOpen{}
)
