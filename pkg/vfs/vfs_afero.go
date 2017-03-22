package vfs

// #nosec
import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/labstack/gommon/log"
	"github.com/spf13/afero"
)

type AferoVFS struct {
	db  couchdb.Database
	fs  afero.Fs
	url *url.URL

	// whether or not the localfilesystem requires an initialisation of its root
	// directory
	rootInit bool
}

func NewAferoVFS(db couchdb.Database, storageURL string) (*AferoVFS, error) {
	u, err := url.Parse(storageURL)
	if err != nil {
		return nil, err
	}
	fs, err := createFS(u)
	if err != nil {
		return nil, err
	}
	return &AferoVFS{
		db:  db,
		fs:  fs,
		url: u,
		// for now, only the file:// scheme needs a specific initialisation of its
		// root directory.
		rootInit: u.Scheme == "file",
	}, nil
}

func createFS(u *url.URL) (afero.Fs, error) {
	switch u.Scheme {
	case "file":
		return afero.NewBasePathFs(afero.NewOsFs(), u.Path), nil
	case "mem":
		return afero.NewMemMapFs(), nil
	}
	return nil, errors.New("vfs_afero: non supported type")
}

// Init creates the root directory document and the trash directory for this
// file system.
func (afs *AferoVFS) Init() error {
	var err error
	// for a file:// fs, we need to create the root directory container
	if afs.rootInit {
		var rootFs afero.Fs
		rootFsURL := config.BuildAbsFsURL("/")
		rootFs, err = createFS(rootFsURL)
		if err != nil {
			return err
		}
		if err = rootFs.MkdirAll(afs.url.Path, 0755); err != nil {
			return err
		}

		defer func() {
			if err != nil {
				if rmerr := rootFs.RemoveAll(afs.url.Path); rmerr != nil {
					log.Warn("[instance] Could not remove the instance directory")
				}
			}
		}()
	}

	err = couchdb.CreateNamedDocWithDB(afs.db, &DirDoc{
		DocName:  "",
		Type:     consts.DirType,
		DocID:    consts.RootDirID,
		Fullpath: "/",
		DirID:    "",
	})
	if err != nil {
		return err
	}

	err = couchdb.CreateNamedDocWithDB(afs.db, &DirDoc{
		DocName:  path.Base(TrashDirName),
		Type:     consts.DirType,
		DocID:    consts.TrashDirID,
		Fullpath: TrashDirName,
		DirID:    consts.RootDirID,
	})
	if err != nil && !couchdb.IsConflictError(err) {
		return err
	}

	err = afs.fs.Mkdir(TrashDirName, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}
	return nil
}

func (afs *AferoVFS) Destroy() error {
	if !afs.rootInit {
		return nil
	}
	rootFsURL := config.BuildAbsFsURL("/")
	rootFs, err := createFS(rootFsURL)
	if err != nil {
		return err
	}
	return rootFs.RemoveAll(afs.url.Path)
}

// CreateDir is the method for creating a new directory
func (afs *AferoVFS) CreateDir(doc *DirDoc) error {
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

// UpdateDir
func (afs *AferoVFS) UpdateDir(olddoc, newdoc *DirDoc) error {
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

// DestroyDirContent destroy all directories and files contained in a directory.
func (afs *AferoVFS) DestroyDirContent(doc *DirDoc) error {
	iter := afs.DirIterator(doc, nil)
	for {
		d, f, err := iter.Next()
		if err == ErrIteratorDone {
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

// DestroyDirAndContent destroy all directories and files contained in a
// directory and the directory itself.
func (afs *AferoVFS) DestroyDirAndContent(doc *DirDoc) error {
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
	err = couchdb.DeleteDoc(afs.db, doc)
	return err
}

// DirByID is used to fetch directory document information form the database.
func (afs *AferoVFS) DirByID(fileID string) (*DirDoc, error) {
	doc := &DirDoc{}
	err := couchdb.GetDoc(afs.db, consts.Files, fileID, doc)
	if couchdb.IsNotFoundError(err) {
		err = ErrParentDoesNotExist
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

// DirByPath is used to fetch directory document information from the database
// from its path.
func (afs *AferoVFS) DirByPath(name string) (*DirDoc, error) {
	if !path.IsAbs(name) {
		return nil, ErrNonAbsolutePath
	}
	var docs []*DirDoc
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

func (afs *AferoVFS) DirIterator(doc *DirDoc, opts *IteratorOptions) DirIterator {
	return NewLocalIterator(afs.db, doc, opts)
}

// -- Files

// CreateFile is used to create file or modify an existing file
// content. It returns a localFileCreation handle. Along with the vfs
// context, it receives the new file document that you want to create.
// It can also receive the old document, representing the current
// revision of the file. In this case it will try to modify the file,
// otherwise it will create it.
//
// Warning: you MUST call the Close() method and check for its error.
// The Close() method will actually create or update the document in
// couchdb. It will also check the md5 hash if required.
func (afs *AferoVFS) CreateFile(newdoc, olddoc *FileDoc) (File, error) {
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
				err = ErrConflict
			}
			return nil, err
		}
	}

	if olddoc != nil {
		newdoc.SetID(olddoc.ID())
		newdoc.SetRev(olddoc.Rev())
		newdoc.CreatedAt = olddoc.CreatedAt
	}

	f, err := safeCreateFile(newpath, newdoc.Executable, afs.fs)
	if err != nil {
		return nil, err
	}

	hash := md5.New() // #nosec
	extractor := NewMetaExtractor(newdoc)

	fc := &localFileCreation{
		w: 0,

		newdoc:  newdoc,
		olddoc:  olddoc,
		bakpath: bakpath,
		newpath: newpath,

		hash: hash,
		meta: extractor,
	}

	return &localFile{afs: afs, f: f, fc: fc}, nil
}

func (afs *AferoVFS) OpenFile(doc *FileDoc) (File, error) {
	name, err := doc.Path(afs)
	if err != nil {
		return nil, err
	}
	f, err := afs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return &localFile{afs: afs, f: f, fc: nil}, nil
}

// DestroyFile definitively destroy a file from the trash.
func (afs *AferoVFS) DestroyFile(doc *FileDoc) error {
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

func (afs *AferoVFS) UpdateFile(olddoc, newdoc *FileDoc) error {
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
		err = afs.fs.Chmod(newpath, getFileMode(newdoc.Executable))
		if err != nil {
			return err
		}
	}
	return couchdb.UpdateDoc(afs.db, newdoc)
}

// GetFileDoc is used to fetch file document information form the
// database.
func (afs *AferoVFS) FileByID(fileID string) (*FileDoc, error) {
	doc := &FileDoc{}
	err := couchdb.GetDoc(afs.db, consts.Files, fileID, doc)
	if err != nil {
		return nil, err
	}
	if doc.Type != consts.FileType {
		return nil, os.ErrNotExist
	}
	return doc, nil
}

// GetFileDocFromPath is used to fetch file document information from
// the database from its path.
func (afs *AferoVFS) FileByPath(name string) (*FileDoc, error) {
	if !path.IsAbs(name) {
		return nil, ErrNonAbsolutePath
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
	var docs []*FileDoc
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

// GetDirOrFileDoc is used to fetch a document from its identifier
// without knowing in advance its type.
func (afs *AferoVFS) DirOrFileByID(fileID string) (*DirDoc, *FileDoc, error) {
	dirOrFile := &DirOrFileDoc{}
	err := couchdb.GetDoc(afs.db, consts.Files, fileID, dirOrFile)
	if err != nil {
		return nil, nil, err
	}
	dirDoc, fileDoc := dirOrFile.Refine()
	return dirDoc, fileDoc, nil
}

// GetDirOrFileDocFromPath is used to fetch a document from its path
// without knowning in advance its type.
func (afs *AferoVFS) DirOrFileByPath(name string) (*DirDoc, *FileDoc, error) {
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

// localFile represents a file handle. It can be used either for writing OR
// reading, but not both at the same time.
type localFile struct {
	afs *AferoVFS          // afero file system
	f   afero.File         // file handle
	fc  *localFileCreation // file creation handle
}

// localFileCreation represents a file open for writing. It is used to
// create of file or to modify the content of a file.
//
// localFileCreation implements io.WriteCloser.
type localFileCreation struct {
	w       int64          // total size written
	newdoc  *FileDoc       // new document
	olddoc  *FileDoc       // old document if any
	newpath string         // file new path
	bakpath string         // backup file path in case of modifying an existing file
	hash    hash.Hash      // hash we build up along the file
	meta    *MetaExtractor // extracts metadata from the content
	err     error          // write error
}

// Read bytes from the file into given buffer - part of io.Reader
// This method can be called on read mode only
func (f *localFile) Read(p []byte) (int, error) {
	if f.fc != nil {
		return 0, os.ErrInvalid
	}
	return f.f.Read(p)
}

// Seek into the file - part of io.Reader
// This method can be called on read mode only
func (f *localFile) Seek(offset int64, whence int) (int64, error) {
	if f.fc != nil {
		return 0, os.ErrInvalid
	}
	return f.f.Seek(offset, whence)
}

// Write bytes to the file - part of io.WriteCloser
// This method can be called in write mode only
func (f *localFile) Write(p []byte) (int, error) {
	if f.fc == nil {
		return 0, os.ErrInvalid
	}

	n, err := f.f.Write(p)
	if err != nil {
		f.fc.err = err
		return n, err
	}

	f.fc.w += int64(n)

	if f.fc.meta != nil {
		(*f.fc.meta).Write(p)
	}

	_, err = f.fc.hash.Write(p)
	return n, err
}

// Close the handle and commit the document in database if all checks
// are OK. It is important to check errors returned by this method.
func (f *localFile) Close() error {
	if f.fc == nil {
		return f.f.Close()
	}

	var err error
	fc := f.fc

	defer func() {
		werr := fc.err
		if fc.olddoc != nil {
			// put back backup file revision in case on error occurred while
			// modifying file content or remove the backup file otherwise
			if err != nil || werr != nil {
				f.afs.fs.Rename(fc.bakpath, fc.newpath)
			} else {
				f.afs.fs.Remove(fc.bakpath)
			}
		} else if err != nil || werr != nil {
			// remove file if an error occurred while file creation
			f.afs.fs.Remove(fc.newpath)
		}
	}()

	err = f.f.Close()
	if err != nil {
		if f.fc.meta != nil {
			(*f.fc.meta).Abort(err)
		}
		return err
	}

	newdoc, olddoc, written := fc.newdoc, fc.olddoc, fc.w

	if f.fc.meta != nil {
		(*f.fc.meta).Close()
		newdoc.Metadata = (*f.fc.meta).Result()
	}

	md5sum := fc.hash.Sum(nil)
	if newdoc.MD5Sum == nil {
		newdoc.MD5Sum = md5sum
	}

	if !bytes.Equal(newdoc.MD5Sum, md5sum) {
		err = ErrInvalidHash
		return err
	}

	if newdoc.ByteSize < 0 {
		newdoc.ByteSize = written
	}

	if newdoc.ByteSize != written {
		err = ErrContentLengthMismatch
		return err
	}

	if olddoc != nil {
		err = couchdb.UpdateDoc(f.afs.db, newdoc)
	} else {
		err = couchdb.CreateDoc(f.afs.db, newdoc)
	}
	return err
}

func safeCreateFile(name string, executable bool, fs afero.Fs) (afero.File, error) {
	// write only (O_WRONLY), try to create the file and check that it
	// does not already exist (O_CREATE|O_EXCL).
	flag := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	mode := getFileMode(executable)
	return fs.OpenFile(name, flag, mode)
}

func safeRenameFile(afs *AferoVFS, oldpath, newpath string) error {
	newpath = path.Clean(newpath)
	oldpath = path.Clean(oldpath)

	if !path.IsAbs(newpath) || !path.IsAbs(oldpath) {
		return ErrNonAbsolutePath
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

func safeRenameDir(afs *AferoVFS, oldpath, newpath string) error {
	newpath = path.Clean(newpath)
	oldpath = path.Clean(oldpath)

	if !path.IsAbs(newpath) || !path.IsAbs(oldpath) {
		return ErrNonAbsolutePath
	}

	if strings.HasPrefix(newpath, oldpath+"/") {
		return ErrForbiddenDocMove
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
	var children []*DirDoc
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
		go func(child *DirDoc) {
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
	_ VFS = &AferoVFS{}
)
