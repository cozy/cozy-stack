package vfsswift

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/ncw/swift"
)

var conn *swift.Connection

type swiftVFS struct {
	vfs.Indexer
	c      *swift.Connection
	domain string
	mu     vfs.Locker
}

// InitConnection should be used to initialize the connection to the
// OpenStack Swift server.
//
// This function is not thread-safe.
func InitConnection(fsURL *url.URL) (err error) {
	conn, err = config.NewSwiftConnection(fsURL)
	if err != nil {
		return err
	}
	log.Debugf("[vfsswift] Starting authentication with server %s", conn.AuthUrl)
	if err = conn.Authenticate(); err != nil {
		log.Errorf("[vfsswift] Authentication failed with the OpenStack Swift server on %s",
			conn.AuthUrl)
		return err
	}
	return nil
}

// New returns a vfs.VFS instance associated with the specified indexer and the
// swift storage url.
func New(index vfs.Indexer, mu vfs.Locker, domain string) (vfs.VFS, error) {
	if conn == nil {
		return nil, errors.New("vfsswift: global connection is not initialized")
	}
	if domain == "" {
		return nil, fmt.Errorf("vfsswift: specified domain is empty")
	}
	return &swiftVFS{
		Indexer: index,
		c:       conn,
		domain:  domain,
		mu:      mu,
	}, nil
}

func (sfs *swiftVFS) InitFs() error {
	sfs.mu.Lock()
	defer sfs.mu.Unlock()
	if err := sfs.Indexer.InitIndex(); err != nil {
		return err
	}
	return sfs.c.ContainerCreate(sfs.domain, nil)
}

func (sfs *swiftVFS) Delete() error {
	sfs.mu.Lock()
	defer sfs.mu.Unlock()
	err := sfs.c.ObjectsWalk(sfs.domain, nil, func(opts *swift.ObjectsOpts) (interface{}, error) {
		objNames, err := sfs.c.ObjectNames(sfs.domain, opts)
		if err != nil {
			return nil, err
		}
		_, err = sfs.c.BulkDelete(sfs.domain, objNames)
		return objNames, err
	})
	if err != nil {
		return err
	}
	return sfs.c.ContainerDelete(sfs.domain)
}

func (sfs *swiftVFS) CreateDir(doc *vfs.DirDoc) error {
	sfs.mu.Lock()
	defer sfs.mu.Unlock()
	objName := doc.DirID + "/" + doc.DocName
	_, _, err := sfs.c.Object(sfs.domain, objName)
	if err != swift.ObjectNotFound {
		if err != nil {
			return err
		}
		return os.ErrExist
	}
	f, err := sfs.c.ObjectCreate(sfs.domain,
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
	return sfs.Indexer.CreateDirDoc(doc)
}

func (sfs *swiftVFS) CreateFile(newdoc, olddoc *vfs.FileDoc) (vfs.File, error) {
	sfs.mu.Lock()
	defer sfs.mu.Unlock()

	if olddoc != nil {
		newdoc.SetID(olddoc.ID())
		newdoc.SetRev(olddoc.Rev())
		newdoc.CreatedAt = olddoc.CreatedAt
	}

	objName := newdoc.DirID + "/" + newdoc.DocName
	if olddoc == nil {
		_, _, err := sfs.c.Object(sfs.domain, objName)
		if err != swift.ObjectNotFound {
			if err != nil {
				return nil, err
			}
			return nil, os.ErrExist
		}
	}

	var h swift.Headers
	if newdoc.ByteSize >= 0 {
		h = swift.Headers{"Content-Length": strconv.FormatInt(newdoc.ByteSize, 10)}
	}
	hash := hex.EncodeToString(newdoc.MD5Sum)
	f, err := sfs.c.ObjectCreate(
		sfs.domain,
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
		f:      f,
		mu:     sfs.mu,
		index:  sfs.Indexer,
		meta:   vfs.NewMetaExtractor(newdoc),
		newdoc: newdoc,
	}, nil
}

func (sfs *swiftVFS) DestroyDirContent(doc *vfs.DirDoc) error {
	sfs.mu.Lock()
	defer sfs.mu.Unlock()
	return sfs.destroyDirContent(doc)
}

func (sfs *swiftVFS) DestroyDirAndContent(doc *vfs.DirDoc) error {
	sfs.mu.Lock()
	defer sfs.mu.Unlock()
	return sfs.destroyDirAndContent(doc)
}

func (sfs *swiftVFS) DestroyFile(doc *vfs.FileDoc) error {
	sfs.mu.Lock()
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
	if err := sfs.c.ObjectDelete(sfs.domain, doc.DirID+"/"+doc.DocName); err != nil {
		return err
	}
	return sfs.Indexer.DeleteDirDoc(doc)
}

func (sfs *swiftVFS) destroyFile(doc *vfs.FileDoc) error {
	err := sfs.c.ObjectDelete(sfs.domain, doc.DirID+"/"+doc.DocName)
	if err != nil {
		return err
	}
	return sfs.Indexer.DeleteFileDoc(doc)
}

func (sfs *swiftVFS) OpenFile(doc *vfs.FileDoc) (vfs.File, error) {
	sfs.mu.RLock()
	defer sfs.mu.RUnlock()
	f, _, err := sfs.c.ObjectOpen(sfs.domain, doc.DirID+"/"+doc.DocName, false, nil)
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
	sfs.mu.Lock()
	defer sfs.mu.Unlock()
	if newdoc.DirID != olddoc.DirID || newdoc.DocName != olddoc.DocName {
		err := sfs.c.ObjectMove(
			sfs.domain, olddoc.DirID+"/"+olddoc.DocName,
			sfs.domain, newdoc.DirID+"/"+newdoc.DocName,
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
	sfs.mu.Lock()
	defer sfs.mu.Unlock()
	if newdoc.DirID != olddoc.DirID || newdoc.DocName != olddoc.DocName {
		err := sfs.c.ObjectMove(
			sfs.domain, olddoc.DirID+"/"+olddoc.DocName,
			sfs.domain, newdoc.DirID+"/"+newdoc.DocName,
		)
		if err != nil {
			return err
		}
	}
	return sfs.Indexer.UpdateDirDoc(olddoc, newdoc)
}

func (sfs *swiftVFS) DirByID(fileID string) (*vfs.DirDoc, error) {
	sfs.mu.RLock()
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirByID(fileID)
}

func (sfs *swiftVFS) DirByPath(name string) (*vfs.DirDoc, error) {
	sfs.mu.RLock()
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirByPath(name)
}

func (sfs *swiftVFS) FileByID(fileID string) (*vfs.FileDoc, error) {
	sfs.mu.RLock()
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FileByID(fileID)
}

func (sfs *swiftVFS) FileByPath(name string) (*vfs.FileDoc, error) {
	sfs.mu.RLock()
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FileByPath(name)
}

func (sfs *swiftVFS) FilePath(doc *vfs.FileDoc) (string, error) {
	sfs.mu.RLock()
	defer sfs.mu.RUnlock()
	return sfs.Indexer.FilePath(doc)
}

func (sfs *swiftVFS) DirOrFileByID(fileID string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	sfs.mu.RLock()
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirOrFileByID(fileID)
}

func (sfs *swiftVFS) DirOrFileByPath(name string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	sfs.mu.RLock()
	defer sfs.mu.RUnlock()
	return sfs.Indexer.DirOrFileByPath(name)
}

type swiftFileCreation struct {
	f      *swift.ObjectCreateFile
	w      int64
	mu     vfs.Locker
	err    error
	index  vfs.Indexer
	meta   *vfs.MetaExtractor
	newdoc *vfs.FileDoc
	olddoc *vfs.FileDoc
}

func (f *swiftFileCreation) Read(p []byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileCreation) Seek(offset int64, whence int) (int64, error) {
	return 0, os.ErrInvalid
}

func (f *swiftFileCreation) Write(p []byte) (int, error) {
	if f.meta != nil {
		if _, err := (*f.meta).Write(p); err != nil {
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
	return n, nil
}

func (f *swiftFileCreation) Close() (err error) {
	if err = f.f.Close(); err != nil {
		if f.meta != nil {
			(*f.meta).Abort(err)
		}
		return err
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

	if newdoc.ByteSize < 0 {
		newdoc.ByteSize = written
	}

	if newdoc.ByteSize != written {
		return vfs.ErrContentLengthMismatch
	}

	if olddoc == nil {
		return f.index.CreateFileDoc(newdoc)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	err = f.index.UpdateFileDoc(olddoc, newdoc)
	// If we reach a conflict error, the document has been modified while
	// uploading the content of the file.
	//
	// TODO: remove dep on couchdb, with a generalized conflict error for
	// UpdateFileDoc/UpdateDirDoc.
	if couchdb.IsConflictError(err) {
		resdoc, err := f.index.FileByID(olddoc.ID())
		if err != nil {
			return err
		}
		resdoc.Metadata = newdoc.Metadata
		resdoc.ByteSize = newdoc.ByteSize
		return f.index.UpdateFileDoc(resdoc, resdoc)
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
