package vfsswift

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/ncw/swift"
)

var conn *swift.Connection

type swiftVFS struct {
	vfs.Indexer
	c      *swift.Connection
	domain string
}

// InitConnection should be used to initialize the connection to the
// OpenStack Swift server.
//
// This function is not thread-safe.
func InitConnection(fsURL *url.URL) error {
	auth := &url.URL{
		Scheme: "http",
		Host:   fsURL.Host,
		Path:   "/identity/v3",
	}

	q := fsURL.Query()
	var username, password string
	if q.Get("UserName") != "" {
		username = confOrEnv(q.Get("UserName"))
		password = confOrEnv(q.Get("Password"))
	} else {
		password = confOrEnv(q.Get("Token"))
	}

	conn = &swift.Connection{
		UserName:       username,
		ApiKey:         password,
		AuthUrl:        auth.String(),
		Domain:         confOrEnv(q.Get("UserDomainName")),
		Tenant:         confOrEnv(q.Get("ProjectName")),
		TenantId:       confOrEnv(q.Get("ProjectID")),
		TenantDomain:   confOrEnv(q.Get("ProjectDomain")),
		TenantDomainId: confOrEnv(q.Get("ProjectDomainID")),
	}
	if err := conn.Authenticate(); err != nil {
		log.Errorf("[vfsswift] Authentication failed with the OpenStack Swift server on %s",
			auth.String())
		return err
	}
	return nil
}

// New returns a vfs.VFS instance associated with the specified indexer and the
// swift storage url.
func New(index vfs.Indexer, fsURL *url.URL, domain string) (vfs.VFS, error) {
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
	}, nil
}

func confOrEnv(val string) string {
	if val == "" || val[0] != '$' {
		return val
	}
	return os.Getenv(strings.TrimSpace(val[1:]))
}

func (sfs *swiftVFS) InitFs() error {
	if err := sfs.Indexer.InitIndex(); err != nil {
		return err
	}
	return sfs.c.ContainerCreate(sfs.domain, nil)
}

func (sfs *swiftVFS) Delete() error {
	return sfs.c.ObjectsWalk(sfs.domain, nil, func(opts *swift.ObjectsOpts) (interface{}, error) {
		objNames, err := sfs.c.ObjectNames(sfs.domain, opts)
		if err != nil {
			return nil, err
		}
		_, err = sfs.c.BulkDelete(sfs.domain, objNames)
		return objNames, err
	})
}

func (sfs *swiftVFS) CreateDir(doc *vfs.DirDoc) error {
	return sfs.Indexer.CreateDirDoc(doc)
}

func (sfs *swiftVFS) CreateFile(newdoc, olddoc *vfs.FileDoc) (vfs.File, error) {
	if olddoc != nil {
		newdoc.SetID(olddoc.ID())
		newdoc.SetRev(olddoc.Rev())
		newdoc.CreatedAt = olddoc.CreatedAt
	} else if err := sfs.Indexer.CreateFileDoc(newdoc); err != nil {
		return nil, err
	}

	hash := hex.EncodeToString(newdoc.MD5Sum)
	fw, err := sfs.c.ObjectCreate(
		sfs.domain,
		newdoc.ID(),
		hash != "",
		hash,
		newdoc.Mime,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return &swiftFileCreation{
		f:      fw,
		index:  sfs.Indexer,
		meta:   vfs.NewMetaExtractor(newdoc),
		newdoc: newdoc,
		olddoc: olddoc,
	}, nil
}

func (sfs *swiftVFS) DestroyDirContent(doc *vfs.DirDoc) error {
	iter := sfs.DirIterator(doc, nil)
	for {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
			break
		}
		if d != nil {
			err = sfs.DestroyDirAndContent(d)
		} else {
			err = sfs.DestroyFile(f)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (sfs *swiftVFS) DestroyDirAndContent(doc *vfs.DirDoc) error {
	err := sfs.DestroyDirContent(doc)
	if err != nil {
		return err
	}
	return sfs.Indexer.DeleteDirDoc(doc)
}

func (sfs *swiftVFS) DestroyFile(doc *vfs.FileDoc) error {
	err := sfs.c.ObjectDelete(sfs.domain, doc.ID())
	if err != nil {
		return err
	}
	return sfs.Indexer.DeleteFileDoc(doc)
}

func (sfs *swiftVFS) OpenFile(doc *vfs.FileDoc) (vfs.File, error) {
	f, _, err := sfs.c.ObjectOpen(sfs.domain, doc.ID(), false, nil)
	if err == swift.ObjectNotFound {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	return &swiftFileOpen{f}, nil
}

type swiftFileCreation struct {
	f      *swift.ObjectCreateFile
	w      int64
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
		(*f.meta).Write(p)
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
	defer func() {
		// if an error has occured while writing to the file (meaning that the file
		// is not fully committed on the server), and the file creation was not
		// part of an overwriting of an existing file (olddoc == nil), we delete
		// the created document.
		if err != nil && f.olddoc == nil {
			f.index.DeleteFileDoc(f.newdoc)
		}
	}()

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
		(*f.meta).Close()
		newdoc.Metadata = (*f.meta).Result()
	}

	if newdoc.ByteSize < 0 {
		newdoc.ByteSize = written
	}

	if newdoc.ByteSize != written {
		return vfs.ErrContentLengthMismatch
	}

	return f.index.UpdateFileDoc(olddoc, newdoc)
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
