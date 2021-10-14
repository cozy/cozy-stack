package vfsswift

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/ncw/swift/v2"
	"github.com/sirupsen/logrus"
)

type swiftVFSV2 struct {
	vfs.Indexer
	vfs.DiskThresholder
	c             *swift.Connection
	domain        string
	prefix        string
	container     string
	version       string
	dataContainer string
	mu            lock.ErrorRWLocker
	ctx           context.Context
	log           *logrus.Entry
}

const (
	swiftV2ContainerPrefixCozy = "cozy-v2-" // The main container
	swiftV2ContainerPrefixData = "data-v2-" // For thumbnails
)

// NewV2 returns a vfs.VFS instance associated with the specified indexer and
// the swift storage url.
//
// This version implements a simpler layout where swift does not contain any
// hierarchy: meaning no index informations. This help with index incoherency
// and as many performance improvements regarding moving / renaming folders.
func NewV2(db prefixer.Prefixer, index vfs.Indexer, disk vfs.DiskThresholder, mu lock.ErrorRWLocker) (vfs.VFS, error) {
	return &swiftVFSV2{
		Indexer:         index,
		DiskThresholder: disk,

		c:             config.GetSwiftConnection(),
		domain:        db.DomainName(),
		prefix:        db.DBPrefix(),
		container:     swiftV2ContainerPrefixCozy + db.DBPrefix(),
		version:       swiftV2ContainerPrefixCozy + db.DBPrefix() + versionSuffix,
		dataContainer: swiftV2ContainerPrefixData + db.DBPrefix(),
		mu:            mu,
		ctx:           context.Background(),
		log:           logger.WithDomain(db.DomainName()).WithField("nspace", "vfsswift"),
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

func (sfs *swiftVFSV2) DBPrefix() string {
	return sfs.prefix
}

func (sfs *swiftVFSV2) DomainName() string {
	return sfs.domain
}

func (sfs *swiftVFSV2) GetIndexer() vfs.Indexer {
	return sfs.Indexer
}

func (sfs *swiftVFSV2) UseSharingIndexer(index vfs.Indexer) vfs.VFS {
	return &swiftVFSV2{
		Indexer:         index,
		DiskThresholder: sfs.DiskThresholder,
		c:               sfs.c,
		domain:          sfs.domain,
		prefix:          sfs.prefix,
		container:       sfs.container,
		version:         sfs.version,
		dataContainer:   sfs.dataContainer,
		mu:              sfs.mu,
		ctx:             context.Background(),
		log:             sfs.log,
	}
}

func (sfs *swiftVFSV2) ContainerNames() map[string]string {
	m := map[string]string{
		"container":      sfs.container,
		"version":        sfs.version,
		"data_container": sfs.dataContainer,
	}
	return m
}

func (sfs *swiftVFSV2) InitFs() error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	if err := sfs.Indexer.InitIndex(); err != nil {
		return err
	}
	if err := sfs.c.VersionContainerCreate(sfs.ctx, sfs.container, sfs.version); err != nil {
		if err != swift.Forbidden {
			sfs.log.Errorf("Could not create container %s: %s",
				sfs.container, err.Error())
			return err
		}
		sfs.log.Errorf("Could not activate versioning for container %s: %s",
			sfs.container, err.Error())
		if err = sfs.c.ContainerDelete(sfs.ctx, sfs.version); err != nil {
			return err
		}
	}
	if err := sfs.c.ContainerCreate(sfs.ctx, sfs.dataContainer, nil); err != nil {
		sfs.log.Errorf("Could not create container %s: %s",
			sfs.dataContainer, err.Error())
		return err
	}
	sfs.log.Infof("Created container %s", sfs.container)
	return nil
}

func (sfs *swiftVFSV2) Delete() error {
	containerMeta := swift.Metadata{"to-be-deleted": "1"}.ContainerHeaders()
	sfs.log.Infof("Marking containers %q, %q and %q as to-be-deleted",
		sfs.container, sfs.version, sfs.dataContainer)
	err := sfs.c.ContainerUpdate(sfs.ctx, sfs.container, containerMeta)
	if err != nil {
		sfs.log.Errorf("Could not mark container %q as to-be-deleted: %s",
			sfs.container, err)
	}
	err = sfs.c.ContainerUpdate(sfs.ctx, sfs.dataContainer, containerMeta)
	if err != nil {
		sfs.log.Errorf("Could not mark container %q as to-be-deleted: %s",
			sfs.dataContainer, err)
	}
	err = sfs.c.ContainerUpdate(sfs.ctx, sfs.version, containerMeta)
	if err != nil {
		sfs.log.Errorf("Could not mark container %q as to-be-deleted: %s",
			sfs.version, err)
	}
	if err = sfs.c.VersionDisable(sfs.ctx, sfs.container); err != nil {
		sfs.log.Errorf("Could not disable versioning on container %q: %s",
			sfs.container, err)
	}
	var errm error
	if err = DeleteContainer(sfs.ctx, sfs.c, sfs.version); err != nil {
		errm = multierror.Append(errm, err)
	}
	if err = DeleteContainer(sfs.ctx, sfs.c, sfs.container); err != nil {
		errm = multierror.Append(errm, err)
	}
	if err = DeleteContainer(sfs.ctx, sfs.c, sfs.dataContainer); err != nil {
		errm = multierror.Append(errm, err)
	}
	return errm
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

func (sfs *swiftVFSV2) CreateFile(newdoc, olddoc *vfs.FileDoc, opts ...vfs.CreateOptions) (vfs.File, error) {
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
		if !vfs.OptionsAllowCreationInTrash(opts) {
			return nil, vfs.ErrParentInTrash
		}
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
		sfs.ctx,
		sfs.container,
		objName,
		true,
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
		capsize: capsize,
	}, nil
}

func (sfs *swiftVFSV2) DestroyDirContent(doc *vfs.DirDoc, push func(vfs.TrashJournal) error) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	diskUsage, _ := sfs.Indexer.DiskUsage()
	files, destroyed, err := sfs.Indexer.DeleteDirDocAndContent(doc, true)
	if err != nil {
		return err
	}
	vfs.DiskQuotaAfterDestroy(sfs, diskUsage, destroyed)
	if len(files) == 0 {
		return nil
	}
	objNames := make([]string, len(files))
	for i, file := range files {
		objNames[i] = MakeObjectName(file.DocID)
		_ = sfs.destroyFileVersions(objNames[i])
	}
	return push(vfs.TrashJournal{ObjectNames: objNames})
}

func (sfs *swiftVFSV2) DestroyDirAndContent(doc *vfs.DirDoc, push func(vfs.TrashJournal) error) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	diskUsage, _ := sfs.Indexer.DiskUsage()
	files, destroyed, err := sfs.Indexer.DeleteDirDocAndContent(doc, false)
	if err != nil {
		return err
	}
	vfs.DiskQuotaAfterDestroy(sfs, diskUsage, destroyed)
	if len(files) == 0 {
		return nil
	}
	objNames := make([]string, len(files))
	for i, file := range files {
		objNames[i] = MakeObjectName(file.DocID)
		_ = sfs.destroyFileVersions(objNames[i])
	}
	return push(vfs.TrashJournal{ObjectNames: objNames})
}

func (sfs *swiftVFSV2) DestroyFile(doc *vfs.FileDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	diskUsage, _ := sfs.Indexer.DiskUsage()
	objName := MakeObjectName(doc.DocID)
	err := sfs.Indexer.DeleteFileDoc(doc)
	if err == nil {
		err = sfs.destroyFileVersions(objName)
		if err != nil {
			sfs.log.Errorf("Could not delete version of %s: %s",
				objName, err.Error())
		}
		err = sfs.c.ObjectDelete(sfs.ctx, sfs.container, objName)
	}
	if err == nil {
		vfs.DiskQuotaAfterDestroy(sfs, diskUsage, doc.ByteSize)
	}
	return err
}

func (sfs *swiftVFSV2) destroyFileVersions(objName string) error {
	versionObjNames, err := sfs.c.VersionObjectList(sfs.ctx, sfs.version, objName)
	// could happened if the versionning could not be enabled, in which case we
	// do not propagate the error.
	if err == swift.ContainerNotFound || err == swift.ObjectNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if len(versionObjNames) > 0 {
		_, err = sfs.c.BulkDelete(sfs.ctx, sfs.version, versionObjNames)
		return err
	}
	return nil
}

func (sfs *swiftVFSV2) EnsureErased(journal vfs.TrashJournal) error {
	// No lock needed
	_, err := sfs.c.BulkDelete(sfs.ctx, sfs.container, journal.ObjectNames)
	if err == swift.Forbidden {
		sfs.log.Warnf("EnsureErased failed on BulkDelete: %s", err)
		err = nil
		for _, objName := range journal.ObjectNames {
			errd := sfs.c.ObjectDelete(sfs.ctx, sfs.container, objName)
			if err == nil && errd != nil && errd != swift.ObjectNotFound {
				sfs.log.Infof("EnsureErased failed on ObjectDelete: %s", errd)
				err = errd
			}
		}
	}
	return err
}

func (sfs *swiftVFSV2) OpenFile(doc *vfs.FileDoc) (vfs.File, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	objName := MakeObjectName(doc.DocID)
	f, _, err := sfs.c.ObjectOpen(sfs.ctx, sfs.container, objName, false, nil)
	if err == swift.ObjectNotFound {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	return &swiftFileOpenV2{f, nil}, nil
}

func (sfs *swiftVFSV2) DissociateFile(src, dst *vfs.FileDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()

	// Copy the file
	srcName := MakeObjectName(src.DocID)
	dstName := MakeObjectName(dst.DocID)
	headers := swift.Metadata{
		"creation-name":  src.Name(),
		"created-at":     src.CreatedAt.Format(time.RFC3339),
		"dissociated-of": src.ID(),
	}.ObjectHeaders()
	if _, err := sfs.c.ObjectCopy(sfs.ctx, sfs.container, srcName, sfs.container, dstName, headers); err != nil {
		return err
	}
	if err := sfs.Indexer.CreateFileDoc(dst); err != nil {
		_ = sfs.c.ObjectDelete(sfs.ctx, sfs.container, dstName)
		return err
	}

	// Remove the source
	return sfs.c.ObjectDelete(sfs.ctx, sfs.container, srcName)
}

func (sfs *swiftVFSV2) DissociateDir(src, dst *vfs.DirDoc) error {
	// This function is not implemented in Swift layout v2
	return os.ErrNotExist
}

func (sfs *swiftVFSV2) OpenFileVersion(doc *vfs.FileDoc, version *vfs.Version) (vfs.File, error) {
	// The versioning is not implemented in Swift layout v2
	return nil, os.ErrNotExist
}

func (sfs *swiftVFSV2) ImportFileVersion(version *vfs.Version, content io.ReadCloser) error {
	if err := content.Close(); err != nil {
		return err
	}
	// The versioning is not implemented in Swift layout v1
	return os.ErrNotExist
}

func (sfs *swiftVFSV2) RevertFileVersion(doc *vfs.FileDoc, version *vfs.Version) error {
	// The versioning is not implemented in Swift layout v2
	return os.ErrNotExist
}

func (sfs *swiftVFSV2) CleanOldVersion(fileID string, version *vfs.Version) error {
	// The versioning is not implemented in Swift layout v2
	return os.ErrNotExist
}

func (sfs *swiftVFSV2) ClearOldVersions() error {
	// The versioning is not implemented in Swift layout v2
	return os.ErrNotExist
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
	capsize int64
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
		if err == nil {
			if f.capsize > 0 && f.size >= f.capsize {
				vfs.PushDiskQuotaAlert(f.fs, true)
			}
		} else {
			// Deleting the object should be secure since we use X-Versions-Location
			// on the container and the old object should be restored.
			_ = f.fs.c.ObjectDelete(f.fs.ctx, f.fs.container, f.name)

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

type swiftFileOpenV2 struct {
	f  *swift.ObjectOpenFile
	br *bytes.Reader
}

func (f *swiftFileOpenV2) Read(p []byte) (int, error) {
	return f.f.Read(p)
}

func (f *swiftFileOpenV2) ReadAt(p []byte, off int64) (int, error) {
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
	n, err := f.f.Seek(context.Background(), offset, whence)
	if err != nil {
		logger.WithNamespace("vfsswift-v2").Warnf("Can't seek: %s", err)
	}
	return n, err
}

func (f *swiftFileOpenV2) Write(p []byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileOpenV2) Close() error {
	return f.f.Close()
}

var (
	_ vfs.VFS  = &swiftVFSV2{}
	_ vfs.File = &swiftFileCreationV2{}
	_ vfs.File = &swiftFileOpenV2{}
)
