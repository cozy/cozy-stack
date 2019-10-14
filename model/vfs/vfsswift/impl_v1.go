package vfsswift

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/ncw/swift"
	"github.com/sirupsen/logrus"
)

const (
	swiftV1ContainerPrefix     = "cozy-" // The main container
	swiftV1DataContainerPrefix = "data-" // For thumbnails
	versionSuffix              = "-version"
)

const maxFileSize = 5 << (3 * 10) // 5 GiB
const dirContentType = "directory"

type swiftVFS struct {
	vfs.Indexer
	vfs.DiskThresholder
	c             *swift.Connection
	domain        string
	prefix        string
	container     string
	version       string
	dataContainer string
	mu            lock.ErrorRWLocker
	log           *logrus.Entry
}

// New returns a vfs.VFS instance associated with the specified indexer and the
// swift storage url.
func New(db prefixer.Prefixer, index vfs.Indexer, disk vfs.DiskThresholder, mu lock.ErrorRWLocker) (vfs.VFS, error) {
	return &swiftVFS{
		Indexer:         index,
		DiskThresholder: disk,

		c:             config.GetSwiftConnection(),
		domain:        db.DomainName(),
		prefix:        db.DBPrefix(),
		container:     swiftV1ContainerPrefix + db.DBPrefix(),
		version:       swiftV1ContainerPrefix + db.DBPrefix() + versionSuffix,
		dataContainer: swiftV1DataContainerPrefix + db.DomainName(),
		mu:            mu,
		log:           logger.WithDomain(db.DomainName()).WithField("nspace", "vfsswift"),
	}, nil
}

func (sfs *swiftVFS) DBPrefix() string {
	return sfs.prefix
}

func (sfs *swiftVFS) DomainName() string {
	return sfs.domain
}

func (sfs *swiftVFS) UseSharingIndexer(index vfs.Indexer) vfs.VFS {
	return &swiftVFS{
		Indexer:         index,
		DiskThresholder: sfs.DiskThresholder,
		c:               sfs.c,
		domain:          sfs.domain,
		container:       sfs.container,
		version:         sfs.version,
		mu:              sfs.mu,
		log:             sfs.log,
	}
}

func (sfs *swiftVFS) ContainerNames() map[string]string {
	m := map[string]string{
		"container":      sfs.container,
		"version":        sfs.version,
		"data_container": sfs.dataContainer,
	}
	return m
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
			sfs.log.Errorf("Could not create container %s: %s",
				sfs.container, err.Error())
			return err
		}
		sfs.log.Errorf("Could not activate versioning for container %s: %s",
			sfs.container, err.Error())
		if err = sfs.c.ContainerDelete(sfs.version); err != nil {
			return err
		}
	}
	sfs.log.Infof("Created container %s", sfs.container)
	return nil
}

func (sfs *swiftVFS) Delete() error {
	containerMeta := swift.Metadata{"to-be-deleted": "1"}.ContainerHeaders()
	sfs.log.Infof("Marking containers %q, %q and %q as to-be-deleted",
		sfs.container, sfs.version, sfs.dataContainer)
	err := sfs.c.ContainerUpdate(sfs.container, containerMeta)
	if err != nil {
		sfs.log.Errorf("Could not mark container %q as to-be-deleted: %s",
			sfs.container, err)
	}
	err = sfs.c.ContainerUpdate(sfs.dataContainer, containerMeta)
	if err != nil {
		sfs.log.Errorf("Could not mark container %q as to-be-deleted: %s",
			sfs.dataContainer, err)
	}
	err = sfs.c.ContainerUpdate(sfs.version, containerMeta)
	if err != nil {
		sfs.log.Errorf("Could not mark container %q as to-be-deleted: %s",
			sfs.version, err)
	}
	if err = sfs.c.VersionDisable(sfs.container); err != nil {
		sfs.log.Errorf("Could not disable versioning on container %q: %s",
			sfs.container, err)
	}
	var errm error
	if err = DeleteContainer(sfs.c, sfs.version); err != nil {
		errm = multierror.Append(errm, err)
	}
	if err = DeleteContainer(sfs.c, sfs.container); err != nil {
		errm = multierror.Append(errm, err)
	}
	if err = DeleteContainer(sfs.c, sfs.dataContainer); err != nil {
		errm = multierror.Append(errm, err)
	}
	return errm
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
		true,
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

	var maxsize, newsize, oldsize, capsize int64
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
		if quotaBytes := int64(9.0 / 10.0 * float64(diskQuota)); diskUsage <= quotaBytes {
			capsize = quotaBytes - diskUsage
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
		true,
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
		capsize: capsize,
	}, nil
}

func (sfs *swiftVFS) DestroyDirContent(doc *vfs.DirDoc, push func(vfs.TrashJournal) error) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	diskUsage, _ := sfs.Indexer.DiskUsage()
	destroyed, err := sfs.destroyDirContent(doc)
	if err == nil {
		vfs.DiskQuotaAfterDestroy(sfs, diskUsage, destroyed)
	}
	return err
}

func (sfs *swiftVFS) DestroyDirAndContent(doc *vfs.DirDoc, push func(vfs.TrashJournal) error) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	diskUsage, _ := sfs.Indexer.DiskUsage()
	destroyed, err := sfs.destroyDirAndContent(doc)
	if err == nil {
		vfs.DiskQuotaAfterDestroy(sfs, diskUsage, destroyed)
	}
	return err
}

func (sfs *swiftVFS) DestroyFile(doc *vfs.FileDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	diskUsage, _ := sfs.Indexer.DiskUsage()
	err := sfs.destroyFile(doc)
	if err == nil {
		vfs.DiskQuotaAfterDestroy(sfs, diskUsage, doc.ByteSize)
	}
	return err
}

func (sfs *swiftVFS) destroyDirContent(doc *vfs.DirDoc) (int64, error) {
	iter := sfs.DirIterator(doc, nil)
	var n int64
	var errm error
	for {
		d, f, erri := iter.Next()
		if erri == vfs.ErrIteratorDone {
			return n, errm
		}
		if erri != nil {
			return n, erri
		}
		var errd error
		var destroyed int64
		if d != nil {
			destroyed, errd = sfs.destroyDirAndContent(d)
		} else {
			destroyed, errd = f.ByteSize, sfs.destroyFile(f)
		}
		if errd != nil {
			errm = multierror.Append(errm, errd)
		} else {
			n += destroyed
		}
	}
}

func (sfs *swiftVFS) destroyDirAndContent(doc *vfs.DirDoc) (int64, error) {
	n, err := sfs.destroyDirContent(doc)
	if err != nil {
		return 0, err
	}
	err = sfs.c.ObjectDelete(sfs.container, doc.DirID+"/"+doc.DocName)
	if err != nil && err != swift.ObjectNotFound {
		return 0, err
	}
	err = sfs.Indexer.DeleteDirDoc(doc)
	return n, err
}

func (sfs *swiftVFS) destroyFile(doc *vfs.FileDoc) error {
	objName := doc.DirID + "/" + doc.DocName
	err := sfs.destroyFileVersions(objName)
	if err != nil {
		sfs.log.Errorf("Could not delete version of %s: %s",
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

func (sfs *swiftVFS) EnsureErased(journal vfs.TrashJournal) error {
	return errors.New("EnsureErased is not available for Swift layout v1")
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

func (sfs *swiftVFS) OpenFileVersion(doc *vfs.FileDoc, version *vfs.Version) (vfs.File, error) {
	// The versioning is not implemented in Swift layout v1
	return nil, os.ErrNotExist
}

func (sfs *swiftVFS) RevertFileVersion(doc *vfs.FileDoc, version *vfs.Version) error {
	// The versioning is not implemented in Swift layout v1
	return os.ErrNotExist
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
			sfs.log.Errorf("Could not move file %s/%s: %s",
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
			sfs.log.Errorf("Could not move dir %s/%s: %s",
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
	capsize int64
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
		if err == nil {
			if f.capsize > 0 && f.size >= f.capsize {
				vfs.PushDiskQuotaAlert(f.fs, true)
			}
		} else {
			// Deleting the object should be secure since we use X-Versions-Location
			// on the container and the old object should be restored.
			_ = f.fs.c.ObjectDelete(f.fs.container, f.name)

			// If an error has occurred that is not due to the index update, we should
			// delete the file from the index.
			_, isCouchErr := couchdb.IsCouchError(err)
			if !isCouchErr && f.olddoc == nil {
				_ = f.fs.Indexer.DeleteFileDoc(f.newdoc)
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
	if olddoc == nil {
		olddoc = newdoc.Clone().(*vfs.FileDoc)
	}

	if f.meta != nil {
		if errc := (*f.meta).Close(); errc == nil {
			vfs.MergeMetadata(newdoc, (*f.meta).Result())
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
	if f.olddoc == nil || !f.olddoc.Trashed {
		newdoc.Trashed = false
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

type swiftFileOpen struct {
	f  *swift.ObjectOpenFile
	br *bytes.Reader
}

func (f *swiftFileOpen) Read(p []byte) (int, error) {
	return f.f.Read(p)
}

func (f *swiftFileOpen) ReadAt(p []byte, off int64) (int, error) {
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
	n, err := f.f.Seek(offset, whence)
	if err != nil {
		l := logger.WithNamespace("vfsswift-v1")
		l.Warnf("Can't seek: %s", err)
	}
	return n, err
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
