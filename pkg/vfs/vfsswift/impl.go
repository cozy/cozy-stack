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
	var err error
	q := fsURL.Query()

	var authURL *url.URL
	auth := confOrEnv(q.Get("AuthURL"))
	if auth == "" {
		authURL = &url.URL{
			Scheme: "http",
			Host:   fsURL.Host,
			Path:   "/identity/v3",
		}
	} else {
		authURL, err = url.Parse(auth)
		if err != nil {
			return fmt.Errorf("vfsswift: could not parse AuthURL %s", err)
		}
	}

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
		AuthUrl:        authURL.String(),
		Domain:         confOrEnv(q.Get("UserDomainName")),
		Tenant:         confOrEnv(q.Get("ProjectName")),
		TenantId:       confOrEnv(q.Get("ProjectID")),
		TenantDomain:   confOrEnv(q.Get("ProjectDomain")),
		TenantDomainId: confOrEnv(q.Get("ProjectDomainID")),
	}
	if err := conn.Authenticate(); err != nil {
		log.Errorf("[vfsswift] Authentication failed with the OpenStack Swift server on %s",
			authURL.String())
		return err
	}
	return nil
}

// New returns a vfs.VFS instance associated with the specified indexer and the
// swift storage url.
func New(index vfs.Indexer, domain string) (vfs.VFS, error) {
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
	if olddoc != nil {
		newdoc.SetID(olddoc.ID())
		newdoc.SetRev(olddoc.Rev())
		newdoc.CreatedAt = olddoc.CreatedAt
	}
	objName := newdoc.DirID + "/" + newdoc.DocName
	_, _, err := sfs.c.Object(sfs.domain, objName)
	if err != swift.ObjectNotFound {
		if err != nil {
			return nil, err
		}
		return nil, os.ErrExist
	}
	hash := hex.EncodeToString(newdoc.MD5Sum)
	f, err := sfs.c.ObjectCreate(
		sfs.domain,
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
		f:      f,
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
	if err := sfs.c.ObjectDelete(sfs.domain, doc.DirID+"/"+doc.DocName); err != nil {
		return err
	}
	return sfs.Indexer.DeleteDirDoc(doc)
}

func (sfs *swiftVFS) DestroyFile(doc *vfs.FileDoc) error {
	err := sfs.c.ObjectDelete(sfs.domain, doc.DirID+"/"+doc.DocName)
	if err != nil {
		return err
	}
	return sfs.Indexer.DeleteFileDoc(doc)
}

func (sfs *swiftVFS) OpenFile(doc *vfs.FileDoc) (vfs.File, error) {
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
