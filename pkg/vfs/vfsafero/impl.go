package vfsafero

// #nosec
import (
	"bytes"
	"crypto/md5"
	"fmt"
	"hash"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/labstack/gommon/log"
	"github.com/spf13/afero"
)

// aferoVFS is a struct implementing the vfs.VFS interface associated with
// an afero.Fs filesystem. The indexing of the elements of the filesystem is
// done in couchdb.
type aferoVFS struct {
	db  couchdb.Database
	fs  afero.Fs
	pth string

	// whether or not the localfilesystem requires an initialisation of its root
	// directory
	osFS bool
}

// New returns a vfs.VFS instance associated with the specified couchdb
// database and storage url.
//
// The supported scheme of the storage url are file://, for an OS-FS store, and
// mem:// for an in-memory store. The backend used is the afero package.
func New(db couchdb.Database, fsURL *url.URL, domain string) (vfs.VFS, error) {
	if fsURL.Path == "" {
		return nil, fmt.Errorf("vfsafero: please check the supplied fs url: %s",
			fsURL.String())
	}
	if domain == "" {
		return nil, fmt.Errorf("vfsafero: specified domain is empty")
	}
	pth := path.Join(fsURL.Path, domain)
	var fs afero.Fs
	switch fsURL.Scheme {
	case "file":
		fs = afero.NewBasePathFs(afero.NewOsFs(), pth)
	case "mem":
		fs = afero.NewMemMapFs()
	default:
		return nil, fmt.Errorf("vfsafero: non supported scheme %s", fsURL.Scheme)
	}
	return &aferoVFS{
		db:  db,
		fs:  fs,
		pth: pth,
		// for now, only the file:// scheme needs a specific initialisation of its
		// root directory.
		osFS: fsURL.Scheme == "file",
	}, nil
}

// Init creates the root directory document and the trash directory for this
// file system.
func (afs *aferoVFS) Init() error {
	var err error
	// for a file:// fs, we need to create the root directory container
	if afs.osFS {
		rootFs := afero.NewOsFs()
		if err = rootFs.MkdirAll(afs.pth, 0755); err != nil {
			return err
		}
		defer func() {
			if err != nil {
				if rmerr := rootFs.RemoveAll(afs.pth); rmerr != nil {
					log.Warn("[instance] Could not remove the instance directory")
				}
			}
		}()
	}

	err = couchdb.CreateNamedDocWithDB(afs.db, &vfs.DirDoc{
		DocName:  "",
		Type:     consts.DirType,
		DocID:    consts.RootDirID,
		Fullpath: "/",
		DirID:    "",
	})
	if err != nil {
		return err
	}

	err = couchdb.CreateNamedDocWithDB(afs.db, &vfs.DirDoc{
		DocName:  path.Base(vfs.TrashDirName),
		Type:     consts.DirType,
		DocID:    consts.TrashDirID,
		Fullpath: vfs.TrashDirName,
		DirID:    consts.RootDirID,
	})
	if err != nil && !couchdb.IsConflictError(err) {
		return err
	}

	err = afs.fs.Mkdir(vfs.TrashDirName, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}
	return nil
}

// Delete removes all the elements associated with the filesystem.
func (afs *aferoVFS) Delete() error {
	if afs.osFS {
		return afero.NewOsFs().RemoveAll(afs.pth)
	}
	return nil
}

func (afs *aferoVFS) DiskUsage() (int64, error) {
	var doc couchdb.ViewResponse
	err := couchdb.ExecView(afs.db, consts.DiskUsageView, &couchdb.ViewRequest{
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

func (afs *aferoVFS) DirByID(fileID string) (*vfs.DirDoc, error) {
	doc := &vfs.DirDoc{}
	err := couchdb.GetDoc(afs.db, consts.Files, fileID, doc)
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

func (afs *aferoVFS) DirByPath(name string) (*vfs.DirDoc, error) {
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
	err := couchdb.FindDocs(afs.db, consts.Files, req, &docs)
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

func (afs *aferoVFS) FileByID(fileID string) (*vfs.FileDoc, error) {
	doc := &vfs.FileDoc{}
	err := couchdb.GetDoc(afs.db, consts.Files, fileID, doc)
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

func (afs *aferoVFS) FileByPath(name string) (*vfs.FileDoc, error) {
	if !path.IsAbs(name) {
		return nil, vfs.ErrNonAbsolutePath
	}
	parent, err := afs.DirByPath(path.Dir(name))
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
	err = couchdb.FindDocs(afs.db, consts.Files, req, &docs)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, os.ErrNotExist
	}
	return docs[0], nil
}

func (afs *aferoVFS) DirOrFileByID(fileID string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	dirOrFile := &vfs.DirOrFileDoc{}
	err := couchdb.GetDoc(afs.db, consts.Files, fileID, dirOrFile)
	if err != nil {
		return nil, nil, err
	}
	dirDoc, fileDoc := dirOrFile.Refine()
	return dirDoc, fileDoc, nil
}

func (afs *aferoVFS) DirOrFileByPath(name string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	dirDoc, err := afs.DirByPath(name)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err == nil {
		return dirDoc, nil, nil
	}

	fileDoc, err := afs.FileByPath(name)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err == nil {
		return nil, fileDoc, nil
	}

	return nil, nil, err
}

func (afs *aferoVFS) DirIterator(doc *vfs.DirDoc, opts *vfs.IteratorOptions) vfs.DirIterator {
	return vfs.NewIterator(afs.db, doc, opts)
}

func (afs *aferoVFS) CreateDir(doc *vfs.DirDoc) error {
	pth, err := doc.Path(afs)
	if err != nil {
		return err
	}
	err = afs.fs.Mkdir(pth, 0755)
	if err != nil {
		return err
	}
	err = couchdb.CreateDoc(afs.db, doc)
	if err != nil {
		afs.fs.Remove(pth)
	}
	return err
}

func (afs *aferoVFS) CreateFile(newdoc, olddoc *vfs.FileDoc) (vfs.File, error) {
	newpath, err := newdoc.Path(afs)
	if err != nil {
		return nil, err
	}

	var bakpath string
	if olddoc != nil {
		bakpath = fmt.Sprintf("/.%s_%s", olddoc.ID(), olddoc.Rev())
		if err = safeRenameFile(afs, newpath, bakpath); err != nil {
			// in case of a concurrent access to this method, it can happened
			// that the file has already been renamed. In this case the
			// safeRenameFile will return an os.ErrNotExist error. But this
			// error is misleading since it does not reflect the conflict.
			if os.IsNotExist(err) {
				err = vfs.ErrConflict
			}
			return nil, err
		}
	}

	if olddoc != nil {
		newdoc.SetID(olddoc.ID())
		newdoc.SetRev(olddoc.Rev())
		newdoc.CreatedAt = olddoc.CreatedAt
	}

	f, err := safeCreateFile(newpath, newdoc.Mode(), afs.fs)
	if err != nil {
		return nil, err
	}

	hash := md5.New() // #nosec
	extractor := vfs.NewMetaExtractor(newdoc)

	return &aferoFileCreation{
		w: 0,
		f: f,

		afs:     afs,
		newdoc:  newdoc,
		olddoc:  olddoc,
		bakpath: bakpath,
		newpath: newpath,

		hash: hash,
		meta: extractor,
	}, nil
}

func (afs *aferoVFS) UpdateDir(olddoc, newdoc *vfs.DirDoc) error {
	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	oldpath, err := olddoc.Path(afs)
	if err != nil {
		return err
	}
	newpath, err := newdoc.Path(afs)
	if err != nil {
		return err
	}
	if oldpath != newpath {
		err = safeRenameDir(afs, oldpath, newpath)
		if err != nil {
			return err
		}
		err = bulkUpdateDocsPath(afs.db, oldpath, newpath)
		if err != nil {
			return err
		}
	}
	return couchdb.UpdateDoc(afs.db, newdoc)
}

func (afs *aferoVFS) UpdateFile(olddoc, newdoc *vfs.FileDoc) error {
	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())

	oldpath, err := olddoc.Path(afs)
	if err != nil {
		return err
	}
	newpath, err := newdoc.Path(afs)
	if err != nil {
		return err
	}
	if newpath != oldpath {
		err = safeRenameFile(afs, oldpath, newpath)
		if err != nil {
			return err
		}
	}

	if newdoc.Executable != olddoc.Executable {
		err = afs.fs.Chmod(newpath, newdoc.Mode())
		if err != nil {
			return err
		}
	}
	return couchdb.UpdateDoc(afs.db, newdoc)
}

func (afs *aferoVFS) DestroyDirContent(doc *vfs.DirDoc) error {
	iter := afs.DirIterator(doc, nil)
	for {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
			break
		}
		if d != nil {
			err = afs.DestroyDirAndContent(d)
		} else {
			err = afs.DestroyFile(f)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (afs *aferoVFS) DestroyDirAndContent(doc *vfs.DirDoc) error {
	err := afs.DestroyDirContent(doc)
	if err != nil {
		return err
	}
	dirpath, err := doc.Path(afs)
	if err != nil {
		return err
	}
	err = afs.fs.RemoveAll(dirpath)
	if err != nil {
		return err
	}
	return couchdb.DeleteDoc(afs.db, doc)
}

func (afs *aferoVFS) DestroyFile(doc *vfs.FileDoc) error {
	path, err := doc.Path(afs)
	if err != nil {
		return err
	}
	err = afs.fs.Remove(path)
	if err != nil {
		return err
	}
	return couchdb.DeleteDoc(afs.db, doc)
}

func (afs *aferoVFS) OpenFile(doc *vfs.FileDoc) (vfs.File, error) {
	name, err := doc.Path(afs)
	if err != nil {
		return nil, err
	}
	f, err := afs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return &aferoFileOpen{f}, nil
}

// aferoFileOpen represents a file handle opened for reading.
type aferoFileOpen struct {
	f afero.File
}

func (f *aferoFileOpen) Read(p []byte) (int, error) {
	return f.f.Read(p)
}

func (f *aferoFileOpen) Seek(offset int64, whence int) (int64, error) {
	return f.f.Seek(offset, whence)
}

func (f *aferoFileOpen) Write(p []byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *aferoFileOpen) Close() error {
	return f.f.Close()
}

// aferoFileCreation represents a file open for writing. It is used to
// create of file or to modify the content of a file.
//
// aferoFileCreation implements io.WriteCloser.
type aferoFileCreation struct {
	f       afero.File         // file handle
	w       int64              // total size written
	afs     *aferoVFS          // parent vfs
	newdoc  *vfs.FileDoc       // new document
	olddoc  *vfs.FileDoc       // old document if any
	newpath string             // file new path
	bakpath string             // backup file path in case of modifying an existing file
	hash    hash.Hash          // hash we build up along the file
	meta    *vfs.MetaExtractor // extracts metadata from the content
	err     error              // write error
}

func (f *aferoFileCreation) Read(p []byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *aferoFileCreation) Seek(offset int64, whence int) (int64, error) {
	return 0, os.ErrInvalid
}

func (f *aferoFileCreation) Write(p []byte) (int, error) {
	n, err := f.f.Write(p)
	if err != nil {
		f.err = err
		return n, err
	}

	f.w += int64(n)

	if f.meta != nil {
		(*f.meta).Write(p)
	}

	_, err = f.hash.Write(p)
	return n, err
}

func (f *aferoFileCreation) Close() error {
	var err error

	defer func() {
		werr := f.err
		if f.olddoc != nil {
			// put back backup file revision in case on error occurred while
			// modifying file content or remove the backup file otherwise
			if err != nil || werr != nil {
				f.afs.fs.Rename(f.bakpath, f.newpath)
			} else {
				f.afs.fs.Remove(f.bakpath)
			}
		} else if err != nil || werr != nil {
			// remove file if an error occurred while file creation
			f.afs.fs.Remove(f.newpath)
		}
	}()

	err = f.f.Close()
	if err != nil {
		if f.meta != nil {
			(*f.meta).Abort(err)
		}
		return err
	}

	newdoc, olddoc, written := f.newdoc, f.olddoc, f.w

	if f.meta != nil {
		(*f.meta).Close()
		newdoc.Metadata = (*f.meta).Result()
	}

	md5sum := f.hash.Sum(nil)
	if newdoc.MD5Sum == nil {
		newdoc.MD5Sum = md5sum
	}

	if !bytes.Equal(newdoc.MD5Sum, md5sum) {
		err = vfs.ErrInvalidHash
		return err
	}

	if newdoc.ByteSize < 0 {
		newdoc.ByteSize = written
	}

	if newdoc.ByteSize != written {
		err = vfs.ErrContentLengthMismatch
		return err
	}

	if olddoc != nil {
		err = couchdb.UpdateDoc(f.afs.db, newdoc)
	} else {
		err = couchdb.CreateDoc(f.afs.db, newdoc)
	}
	return err
}

func safeCreateFile(name string, mode os.FileMode, fs afero.Fs) (afero.File, error) {
	// write only (O_WRONLY), try to create the file and check that it
	// does not already exist (O_CREATE|O_EXCL).
	flag := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	return fs.OpenFile(name, flag, mode)
}

func safeRenameFile(afs *aferoVFS, oldpath, newpath string) error {
	newpath = path.Clean(newpath)
	oldpath = path.Clean(oldpath)

	if !path.IsAbs(newpath) || !path.IsAbs(oldpath) {
		return vfs.ErrNonAbsolutePath
	}

	_, err := afs.fs.Stat(newpath)
	if err == nil {
		return os.ErrExist
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return afs.fs.Rename(oldpath, newpath)
}

func safeRenameDir(afs *aferoVFS, oldpath, newpath string) error {
	newpath = path.Clean(newpath)
	oldpath = path.Clean(oldpath)

	if !path.IsAbs(newpath) || !path.IsAbs(oldpath) {
		return vfs.ErrNonAbsolutePath
	}

	if strings.HasPrefix(newpath, oldpath+"/") {
		return vfs.ErrForbiddenDocMove
	}

	_, err := afs.fs.Stat(newpath)
	if err == nil {
		return os.ErrExist
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return afs.fs.Rename(oldpath, newpath)
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
	_ vfs.VFS  = &aferoVFS{}
	_ vfs.File = &aferoFileOpen{}
	_ vfs.File = &aferoFileCreation{}
)
