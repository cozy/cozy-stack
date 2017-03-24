package vfsswift

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/ncw/swift"
)

var conn *swift.Connection

type swiftVFS struct {
	db     couchdb.Database
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

// New returns a vfs.VFS instance associated with the specified couchdb
// database and the swift storage url.
func New(db couchdb.Database, fsURL *url.URL, domain string) (vfs.VFS, error) {
	if conn == nil {
		return nil, errors.New("vfsswift: global connection is not initialized")
	}
	return &swiftVFS{
		db:     db,
		c:      conn,
		domain: domain,
	}, nil
}

func confOrEnv(val string) string {
	if val == "" || val[0] != '$' {
		return val
	}
	return os.Getenv(strings.TrimSpace(val[1:]))
}

func (sfs *swiftVFS) Init() error {
	err := couchdb.CreateNamedDocWithDB(sfs.db, &vfs.DirDoc{
		DocName:  "",
		Type:     consts.DirType,
		DocID:    consts.RootDirID,
		Fullpath: "/",
		DirID:    "",
	})
	if err != nil {
		return err
	}

	err = couchdb.CreateNamedDocWithDB(sfs.db, &vfs.DirDoc{
		DocName:  path.Base(vfs.TrashDirName),
		Type:     consts.DirType,
		DocID:    consts.TrashDirID,
		Fullpath: vfs.TrashDirName,
		DirID:    consts.RootDirID,
	})
	if err != nil && !couchdb.IsConflictError(err) {
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

func (sfs *swiftVFS) DiskUsage() (int64, error) {
	var doc couchdb.ViewResponse
	err := couchdb.ExecView(sfs.db, consts.DiskUsageView, &couchdb.ViewRequest{
		Reduce: true,
	}, &doc)
	if err != nil {
		return 0, err
	}
	if len(doc.Rows) == 0 {
		return 0, nil
	}
	// Reduce of _count should give us a number value
	f64, ok := doc.Rows[0].Value.(float64)
	if !ok {
		return 0, vfs.ErrWrongCouchdbState
	}
	return int64(f64), nil
}

func (sfs *swiftVFS) DirByID(fileID string) (*vfs.DirDoc, error) {
	doc := &vfs.DirDoc{}
	err := couchdb.GetDoc(sfs.db, consts.Files, fileID, doc)
	if couchdb.IsNotFoundError(err) {
		err = os.ErrNotExist
	}
	if err != nil {
		if fileID == consts.RootDirID {
			panic("Root directory is not in database")
		}
		if fileID == consts.TrashDirID {
			panic("Trash directory is not in database")
		}
		return nil, err
	}
	if doc.Type != consts.DirType {
		return nil, os.ErrNotExist
	}
	return doc, err
}

func (sfs *swiftVFS) DirByPath(name string) (*vfs.DirDoc, error) {
	if !path.IsAbs(name) {
		return nil, vfs.ErrNonAbsolutePath
	}
	var docs []*vfs.DirDoc
	sel := mango.Equal("path", path.Clean(name))
	req := &couchdb.FindRequest{
		UseIndex: "dir-by-path",
		Selector: sel,
		Limit:    1,
	}
	err := couchdb.FindDocs(sfs.db, consts.Files, req, &docs)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		if name == "/" {
			panic("Root directory is not in database")
		}
		return nil, os.ErrNotExist
	}
	return docs[0], nil
}

func (sfs *swiftVFS) FileByID(fileID string) (*vfs.FileDoc, error) {
	doc := &vfs.FileDoc{}
	err := couchdb.GetDoc(sfs.db, consts.Files, fileID, doc)
	if couchdb.IsNotFoundError(err) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	if doc.Type != consts.FileType {
		return nil, os.ErrNotExist
	}
	return doc, nil
}

func (sfs *swiftVFS) FileByPath(name string) (*vfs.FileDoc, error) {
	if !path.IsAbs(name) {
		return nil, vfs.ErrNonAbsolutePath
	}
	parent, err := sfs.DirByPath(path.Dir(name))
	if err != nil {
		return nil, err
	}
	selector := mango.Map{
		"dir_id": parent.DocID,
		"name":   path.Base(name),
		"type":   consts.FileType,
	}
	var docs []*vfs.FileDoc
	req := &couchdb.FindRequest{
		UseIndex: "dir-file-child",
		Selector: selector,
		Limit:    1,
	}
	err = couchdb.FindDocs(sfs.db, consts.Files, req, &docs)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, os.ErrNotExist
	}
	return docs[0], nil
}

func (sfs *swiftVFS) DirOrFileByID(fileID string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	dirOrFile := &vfs.DirOrFileDoc{}
	err := couchdb.GetDoc(sfs.db, consts.Files, fileID, dirOrFile)
	if err != nil {
		return nil, nil, err
	}
	dirDoc, fileDoc := dirOrFile.Refine()
	return dirDoc, fileDoc, nil
}

func (sfs *swiftVFS) DirOrFileByPath(name string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	dirDoc, err := sfs.DirByPath(name)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err == nil {
		return dirDoc, nil, nil
	}
	fileDoc, err := sfs.FileByPath(name)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err == nil {
		return nil, fileDoc, nil
	}
	return nil, nil, err
}

func (sfs *swiftVFS) DirIterator(doc *vfs.DirDoc, opts *vfs.IteratorOptions) vfs.DirIterator {
	return vfs.NewIterator(sfs.db, doc, opts)
}

func (sfs *swiftVFS) CreateDir(doc *vfs.DirDoc) error {
	return couchdb.CreateDoc(sfs.db, doc)
}

func (sfs *swiftVFS) CreateFile(newdoc, olddoc *vfs.FileDoc) (vfs.File, error) {
	if olddoc != nil {
		newdoc.SetID(olddoc.ID())
		newdoc.SetRev(olddoc.Rev())
		newdoc.CreatedAt = olddoc.CreatedAt
	} else if err := couchdb.CreateDoc(sfs.db, newdoc); err != nil {
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
		db:     sfs.db,
		meta:   vfs.NewMetaExtractor(newdoc),
		newdoc: newdoc,
	}, nil
}

func (sfs *swiftVFS) UpdateDir(olddoc, newdoc *vfs.DirDoc) error {
	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	oldpath, err := olddoc.Path(sfs)
	if err != nil {
		return err
	}
	newpath, err := newdoc.Path(sfs)
	if err != nil {
		return err
	}
	if oldpath != newpath {
		err = bulkUpdateDocsPath(sfs.db, oldpath, newpath)
		if err != nil {
			return err
		}
	}
	return couchdb.UpdateDoc(sfs.db, newdoc)
}

func (sfs *swiftVFS) UpdateFile(olddoc, newdoc *vfs.FileDoc) error {
	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	return couchdb.UpdateDoc(sfs.db, newdoc)
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
	return couchdb.DeleteDoc(sfs.db, doc)
}

func (sfs *swiftVFS) DestroyFile(doc *vfs.FileDoc) error {
	err := sfs.c.ObjectDelete(sfs.domain, doc.ID())
	if err != nil {
		return err
	}
	return couchdb.DeleteDoc(sfs.db, doc)
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
	db     couchdb.Database
	meta   *vfs.MetaExtractor
	newdoc *vfs.FileDoc
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
		return n, err
	}
	f.w += int64(n)
	return n, nil
}

func (f *swiftFileCreation) Close() error {
	if err := f.f.Close(); err != nil {
		if f.meta != nil {
			(*f.meta).Abort(err)
		}
		return err
	}

	newdoc, written := f.newdoc, f.w
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

	return couchdb.UpdateDoc(f.db, newdoc)
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

// @TODO remove this method and use couchdb bulk updates instead
func bulkUpdateDocsPath(db couchdb.Database, oldpath, newpath string) error {
	var children []*vfs.DirDoc
	sel := mango.StartWith("path", oldpath+"/")
	req := &couchdb.FindRequest{
		UseIndex: "dir-by-path",
		Selector: sel,
	}
	err := couchdb.FindDocs(db, consts.Files, req, &children)
	if err != nil || len(children) == 0 {
		return err
	}

	errc := make(chan error)

	for _, child := range children {
		go func(child *vfs.DirDoc) {
			if !strings.HasPrefix(child.Fullpath, oldpath+"/") {
				errc <- fmt.Errorf("Child has wrong base directory")
			} else {
				child.Fullpath = path.Join(newpath, child.Fullpath[len(oldpath)+1:])
				errc <- couchdb.UpdateDoc(db, child)
			}
		}(child)
	}

	for range children {
		if e := <-errc; e != nil {
			err = e
		}
	}

	return err
}

var (
	_ vfs.VFS  = &swiftVFS{}
	_ vfs.File = &swiftFileCreation{}
	_ vfs.File = &swiftFileOpen{}
)
