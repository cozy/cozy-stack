package vfsswift

import (
	"bytes"
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
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/swift"
	"github.com/sirupsen/logrus"
)

type swiftVFSV3 struct {
	vfs.Indexer
	vfs.DiskThresholder
	c         *swift.Connection
	domain    string
	prefix    string
	container string
	mu        lock.ErrorRWLocker
	log       *logrus.Entry
}

const swiftV3ContainerPrefix = "cozy-v3-"

// NewV3 returns a vfs.VFS instance associated with the specified indexer and
// the swift storage url.
//
// This new V3 version uses only a single swift container per instance. We can
// easily put the thumbnails in the same container that the data. And, for the
// versioning, Swift Object Versioning is not what we want: it is as robust as
// we expected (we had encoding issue with the V1 layout for file with `? ` in
// the name), and it is poor in features (for example, we want to swap an old
// version with the current version without having to download/upload contents,
// and it is not supported).
func NewV3(db prefixer.Prefixer, index vfs.Indexer, disk vfs.DiskThresholder, mu lock.ErrorRWLocker) (vfs.VFS, error) {
	return &swiftVFSV3{
		Indexer:         index,
		DiskThresholder: disk,

		c:         config.GetSwiftConnection(),
		domain:    db.DomainName(),
		prefix:    db.DBPrefix(),
		container: swiftV3ContainerPrefix + db.DBPrefix(),
		mu:        mu,
		log:       logger.WithDomain(db.DomainName()).WithField("nspace", "vfsswift"),
	}, nil
}

// newInternalID returns a random string that can be used as an internal_vfs_id.
func newInternalID() string {
	return utils.RandomString(16)
}

// MakeObjectNameV3 builds the swift object name for a given file document. It
// creates a virtual subfolder by splitting the document ID, which should be 32
// bytes long, on the 27nth byte. This avoid having a flat hierarchy in swift
// with no bound. And it appends the internalID at the end to regroup all the
// versions of a file in the same virtual subfolder.
func MakeObjectNameV3(docID, internalID string) string {
	if len(docID) != 32 {
		return docID + "/" + internalID
	}
	return docID[:22] + "/" + docID[22:27] + "/" + docID[27:] + "/" + internalID
}

func (sfs *swiftVFSV3) DBPrefix() string {
	return sfs.prefix
}

func (sfs *swiftVFSV3) DomainName() string {
	return sfs.domain
}

func (sfs *swiftVFSV3) UseSharingIndexer(index vfs.Indexer) vfs.VFS {
	return &swiftVFSV3{
		Indexer:         index,
		DiskThresholder: sfs.DiskThresholder,
		c:               sfs.c,
		domain:          sfs.domain,
		container:       sfs.container,
		mu:              sfs.mu,
		log:             sfs.log,
	}
}

func (sfs *swiftVFSV3) ContainerNames() map[string]string {
	return map[string]string{"container": sfs.container}
}

func (sfs *swiftVFSV3) InitFs() error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	if err := sfs.Indexer.InitIndex(); err != nil {
		return err
	}
	if err := sfs.c.ContainerCreate(sfs.container, nil); err != nil {
		sfs.log.Errorf("Could not create container %q: %s",
			sfs.container, err.Error())
		return err
	}
	sfs.log.Infof("Created container %q", sfs.container)
	return nil
}

func (sfs *swiftVFSV3) Delete() error {
	containerMeta := swift.Metadata{"to-be-deleted": "1"}.ContainerHeaders()
	sfs.log.Infof("Marking container %q as to-be-deleted", sfs.container)
	err := sfs.c.ContainerUpdate(sfs.container, containerMeta)
	if err != nil {
		sfs.log.Errorf("Could not mark container %q as to-be-deleted: %s",
			sfs.container, err)
	}
	return sfs.deleteContainer(sfs.container)
}

func (sfs *swiftVFSV3) deleteContainer(container string) error {
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
	for len(objectNames) > 0 {
		objectToDelete := objectNames
		if len(objectToDelete) > 8000 {
			objectToDelete = objectToDelete[:8000]
		}
		_, err = sfs.c.BulkDelete(container, objectToDelete)
		if err != nil {
			return err
		}
		objectNames = objectNames[len(objectToDelete):]
	}
	return sfs.c.ContainerDelete(container)
}

func (sfs *swiftVFSV3) CreateDir(doc *vfs.DirDoc) error {
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

func (sfs *swiftVFSV3) CreateFile(newdoc, olddoc *vfs.FileDoc) (vfs.File, error) {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.Unlock()

	diskQuota := sfs.DiskQuota()

	var maxsize, newsize, capsize int64
	maxsize = maxFileSize
	newsize = newdoc.ByteSize
	if diskQuota > 0 {
		diskUsage, err := sfs.DiskUsage()
		if err != nil {
			return nil, err
		}
		maxsize = diskQuota - diskUsage
		if maxsize > maxFileSize {
			maxsize = maxFileSize
		}
		if quotaBytes := int64(9.0 / 10.0 * float64(diskQuota)); diskUsage <= quotaBytes {
			capsize = quotaBytes - diskUsage
		}
	}
	if newsize > maxsize {
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

	if olddoc == nil {
		var exists bool
		exists, err = sfs.Indexer.DirChildExists(newdoc.DirID, newdoc.DocName)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, os.ErrExist
		}
		if newdoc.DocID, err = couchdb.UUID(sfs); err != nil {
			return nil, err
		}
	}

	newdoc.InternalID = newInternalID()
	objName := MakeObjectNameV3(newdoc.DocID, newdoc.InternalID)
	objMeta := swift.Metadata{
		"creation-name": newdoc.Name(),
		"created-at":    newdoc.CreatedAt.Format(time.RFC3339),
		"exec":          strconv.FormatBool(newdoc.Executable),
	}

	hash := hex.EncodeToString(newdoc.MD5Sum)
	f, err := sfs.c.ObjectCreate(
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
	extractor := vfs.NewMetaExtractor(newdoc)

	return &swiftFileCreationV3{
		fs:      sfs,
		f:       f,
		newdoc:  newdoc,
		olddoc:  olddoc,
		name:    objName,
		w:       0,
		size:    newsize,
		maxsize: maxsize,
		capsize: capsize,
		meta:    extractor,
	}, nil
}

func (sfs *swiftVFSV3) destroyDir(doc *vfs.DirDoc, onlyContent bool) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	fn := "DestroyDirAndContent"
	if onlyContent {
		fn = "DestroyDirContent"
	}
	diskUsage, _ := sfs.Indexer.DiskUsage()
	files, destroyed, err := sfs.Indexer.DeleteDirDocAndContent(doc, onlyContent)
	if err != nil {
		return err
	}
	objNames := make([]string, len(files))
	for i, file := range files {
		objNames[i] = MakeObjectNameV3(file.DocID, file.InternalID)
	}
	var allVersions []*vfs.Version
	for _, file := range files {
		if versions, err := vfs.VersionsFor(sfs, file.DocID); err == nil {
			for _, v := range versions {
				internalID := v.DocID
				if parts := strings.SplitN(v.DocID, "/", 2); len(parts) > 1 {
					internalID = parts[1]
				}
				objNames = append(objNames, MakeObjectNameV3(file.DocID, internalID))
				destroyed += v.ByteSize
			}
			allVersions = append(allVersions, versions...)
		}
	}
	if err = sfs.Indexer.BatchDeleteVersions(allVersions); err != nil {
		sfs.log.Infof("%s failed on BatchDeleteVersions: %s", fn, err)
	}
	_, errb := sfs.c.BulkDelete(sfs.container, objNames)
	if errb == swift.Forbidden {
		sfs.log.Infof("%s failed on BulkDelete: %s", fn, errb)
		errb = nil
		for _, objName := range objNames {
			if errd := sfs.c.ObjectDelete(sfs.container, objName); err == nil && errd != nil {
				sfs.log.Infof("%s failed on ObjectDelete: %s", fn, errd)
				err = errd
			}
		}
	}
	if err == nil && errb != nil {
		err = errb
	}
	vfs.DiskQuotaAfterDestroy(sfs, diskUsage, destroyed)
	return err
}

func (sfs *swiftVFSV3) DestroyDirContent(doc *vfs.DirDoc) error {
	return sfs.destroyDir(doc, true)
}

func (sfs *swiftVFSV3) DestroyDirAndContent(doc *vfs.DirDoc) error {
	return sfs.destroyDir(doc, false)
}

func (sfs *swiftVFSV3) DestroyFile(doc *vfs.FileDoc) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()
	diskUsage, _ := sfs.Indexer.DiskUsage()
	objNames := []string{
		MakeObjectNameV3(doc.DocID, doc.InternalID),
	}
	if err := sfs.Indexer.DeleteFileDoc(doc); err != nil {
		return err
	}
	destroyed := doc.ByteSize
	var err error
	if versions, err := vfs.VersionsFor(sfs, doc.DocID); err == nil {
		for _, v := range versions {
			internalID := v.DocID
			if parts := strings.SplitN(v.DocID, "/", 2); len(parts) > 1 {
				internalID = parts[1]
			}
			objNames = append(objNames, MakeObjectNameV3(doc.DocID, internalID))
			destroyed += v.ByteSize
		}
		err = sfs.Indexer.BatchDeleteVersions(versions)
		if err != nil {
			sfs.log.Infof("DestroyFile failed on BatchDeleteVersions: %s", err)
		}
	}
	_, errb := sfs.c.BulkDelete(sfs.container, objNames)
	if errb == swift.Forbidden {
		sfs.log.Infof("DestroyFile failed on BulkDelete: %s", err)
		errb = nil
		for _, objName := range objNames {
			if errd := sfs.c.ObjectDelete(sfs.container, objName); err == nil && errd != nil {
				sfs.log.Infof("DestroyFile failed on ObjectDelete: %s", errd)
				err = errd
			}
		}
	}
	if err == nil && errb != nil {
		err = errb
	}
	vfs.DiskQuotaAfterDestroy(sfs, diskUsage, destroyed)
	return err
}

func (sfs *swiftVFSV3) OpenFile(doc *vfs.FileDoc) (vfs.File, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	objName := MakeObjectNameV3(doc.DocID, doc.InternalID)
	f, _, err := sfs.c.ObjectOpen(sfs.container, objName, false, nil)
	if err == swift.ObjectNotFound {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	return &swiftFileOpenV3{f, nil}, nil
}

func (sfs *swiftVFSV3) OpenFileVersion(doc *vfs.FileDoc, version *vfs.Version) (vfs.File, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	internalID := version.DocID
	if parts := strings.SplitN(version.DocID, "/", 2); len(parts) > 1 {
		internalID = parts[1]
	}
	objName := MakeObjectNameV3(doc.DocID, internalID)
	f, _, err := sfs.c.ObjectOpen(sfs.container, objName, false, nil)
	if err == swift.ObjectNotFound {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	return &swiftFileOpenV3{f, nil}, nil
}

func (sfs *swiftVFSV3) RevertFileVersion(doc *vfs.FileDoc, version *vfs.Version) error {
	if lockerr := sfs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer sfs.mu.Unlock()

	save := vfs.NewVersion(doc)
	if err := sfs.Indexer.CreateVersion(save); err != nil {
		return err
	}

	newdoc := doc.Clone().(*vfs.FileDoc)
	if parts := strings.SplitN(version.DocID, "/", 2); len(parts) > 1 {
		newdoc.InternalID = parts[1]
	}
	vfs.SetMetaFromVersion(newdoc, version)
	if err := sfs.Indexer.UpdateFileDoc(doc, newdoc); err != nil {
		_ = sfs.Indexer.DeleteVersion(save)
		return err
	}

	return sfs.Indexer.DeleteVersion(version)

}

// UpdateFileDoc calls the indexer UpdateFileDoc function and adds a few checks
// before actually calling this method:
//   - locks the filesystem for writing
//   - checks in case we have a move operation that the new path is available
//
// @override Indexer.UpdateFileDoc
func (sfs *swiftVFSV3) UpdateFileDoc(olddoc, newdoc *vfs.FileDoc) error {
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
func (sfs *swiftVFSV3) UpdateDirDoc(olddoc, newdoc *vfs.DirDoc) error {
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

func (sfs *swiftVFSV3) DirByID(fileID string) (*vfs.DirDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirByID(fileID)
}

func (sfs *swiftVFSV3) DirByPath(name string) (*vfs.DirDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirByPath(name)
}

func (sfs *swiftVFSV3) FileByID(fileID string) (*vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FileByID(fileID)
}

func (sfs *swiftVFSV3) FileByPath(name string) (*vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FileByPath(name)
}

func (sfs *swiftVFSV3) FilePath(doc *vfs.FileDoc) (string, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return "", lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FilePath(doc)
}

func (sfs *swiftVFSV3) DirOrFileByID(fileID string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirOrFileByID(fileID)
}

func (sfs *swiftVFSV3) DirOrFileByPath(name string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	if lockerr := sfs.mu.RLock(); lockerr != nil {
		return nil, nil, lockerr
	}
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirOrFileByPath(name)
}

// swiftFileCreationV3 represents a file open for writing. It is used to create
// a file or to modify the content of a file.
//
// swiftFileCreationV3 implements io.WriteCloser.
type swiftFileCreationV3 struct {
	fs      *swiftVFSV3
	f       *swift.ObjectCreateFile
	newdoc  *vfs.FileDoc
	olddoc  *vfs.FileDoc
	name    string
	w       int64
	size    int64
	maxsize int64
	capsize int64
	meta    *vfs.MetaExtractor
	err     error
}

func (f *swiftFileCreationV3) Read(p []byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileCreationV3) ReadAt(p []byte, off int64) (int, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileCreationV3) Seek(offset int64, whence int) (int64, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileCreationV3) Write(p []byte) (int, error) {
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

func (f *swiftFileCreationV3) Close() (err error) {
	defer func() {
		if err != nil {
			// remove the temporary file if an error occurred
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
		if err != nil {
			return err
		}
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
		if err != nil {
			return err
		}
		newdoc.MD5Sum = md5sum
	}

	if f.size < 0 {
		newdoc.ByteSize = written
	}

	if newdoc.ByteSize != written {
		return vfs.ErrContentLengthMismatch
	}

	lockerr := f.fs.mu.Lock()
	if lockerr != nil {
		return lockerr
	}
	defer f.fs.mu.Unlock()

	var v *vfs.Version
	if olddoc != nil {
		v = vfs.NewVersion(olddoc)
		err = f.fs.Indexer.UpdateFileDoc(olddoc, newdoc)
	} else if newdoc.ID() == "" {
		err = f.fs.Indexer.CreateFileDoc(newdoc)
	} else {
		err = f.fs.Indexer.CreateNamedFileDoc(newdoc)
	}
	if err != nil {
		return err
	}

	if v != nil {
		if errv := f.fs.Indexer.CreateVersion(v); errv != nil {
			internalID := v.DocID
			if parts := strings.SplitN(v.DocID, "/", 2); len(parts) > 1 {
				internalID = parts[1]
			}
			objName := MakeObjectNameV3(olddoc.DocID, internalID)
			_ = f.fs.c.ObjectDelete(f.fs.container, objName)
		}
	}

	if f.capsize > 0 && f.size >= f.capsize {
		vfs.PushDiskQuotaAlert(f.fs, true)
	}

	return nil
}

type swiftFileOpenV3 struct {
	f  *swift.ObjectOpenFile
	br *bytes.Reader
}

func (f *swiftFileOpenV3) Read(p []byte) (int, error) {
	return f.f.Read(p)
}

func (f *swiftFileOpenV3) ReadAt(p []byte, off int64) (int, error) {
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

func (f *swiftFileOpenV3) Seek(offset int64, whence int) (int64, error) {
	n, err := f.f.Seek(offset, whence)
	if err != nil {
		logger.WithNamespace("vfsswift-v3").Warnf("Can't seek: %s", err)
	}
	return n, err
}

func (f *swiftFileOpenV3) Write(p []byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileOpenV3) Close() error {
	return f.f.Close()
}

var (
	_ vfs.VFS  = &swiftVFSV3{}
	_ vfs.File = &swiftFileCreationV3{}
	_ vfs.File = &swiftFileOpenV3{}
)
