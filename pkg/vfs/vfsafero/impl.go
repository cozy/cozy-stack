package vfsafero

// #nosec
import (
	"bytes"
	"crypto/md5"
	"fmt"
	"hash"
	"io"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/vfs"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/spf13/afero"
)

// aferoVFS is a struct implementing the vfs.VFS interface associated with
// an afero.Fs filesystem. The indexing of the elements of the filesystem is
// done in couchdb.
type aferoVFS struct {
	vfs.Indexer
	vfs.DiskThresholder

	fs  afero.Fs
	mu  lock.ErrorRWLocker
	pth string

	// whether or not the localfilesystem requires an initialisation of its root
	// directory
	osFS bool
}

// New returns a vfs.VFS instance associated with the specified indexer and
// storage url.
//
// The supported scheme of the storage url are file://, for an OS-FS store, and
// mem:// for an in-memory store. The backend used is the afero package.
func New(index vfs.Indexer, disk vfs.DiskThresholder, mu lock.ErrorRWLocker, fsURL *url.URL, domain string) (vfs.VFS, error) {
	if fsURL.Scheme != "mem" && fsURL.Path == "" {
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
		Indexer:         index,
		DiskThresholder: disk,

		fs:  fs,
		mu:  mu,
		pth: pth,
		// for now, only the file:// scheme needs a specific initialisation of its
		// root directory.
		osFS: fsURL.Scheme == "file",
	}, nil
}

// Init creates the root directory document and the trash directory for this
// file system.
func (afs *aferoVFS) InitFs() error {
	if lockerr := afs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer afs.mu.Unlock()
	if err := afs.Indexer.InitIndex(); err != nil {
		return err
	}
	// for a file:// fs, we need to create the root directory container
	if afs.osFS {
		if err := afero.NewOsFs().MkdirAll(afs.pth, 0755); err != nil {
			return err
		}
	}
	if err := afs.fs.Mkdir(vfs.TrashDirName, 0755); err != nil && !os.IsExist(err) {
		return err
	}
	return nil
}

// Delete removes all the elements associated with the filesystem.
func (afs *aferoVFS) Delete() error {
	if lockerr := afs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer afs.mu.Unlock()
	if afs.osFS {
		return afero.NewOsFs().RemoveAll(afs.pth)
	}
	return nil
}

func (afs *aferoVFS) CreateDir(doc *vfs.DirDoc) error {
	if lockerr := afs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer afs.mu.Unlock()
	err := afs.fs.Mkdir(doc.Fullpath, 0755)
	if err != nil {
		return err
	}
	if doc.ID() == "" {
		err = afs.Indexer.CreateDirDoc(doc)
	} else {
		err = afs.Indexer.CreateNamedDirDoc(doc)
	}
	if err != nil {
		afs.fs.Remove(doc.Fullpath) // #nosec
	}
	return err
}

func (afs *aferoVFS) CreateFile(newdoc, olddoc *vfs.FileDoc) (vfs.File, error) {
	if lockerr := afs.mu.Lock(); lockerr != nil {
		return nil, lockerr
	}
	defer afs.mu.Unlock()

	diskQuota := afs.DiskQuota()

	var maxsize, newsize int64
	newsize = newdoc.ByteSize
	if diskQuota > 0 {
		diskUsage, err := afs.DiskUsage()
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

	newpath, err := afs.Indexer.FilePath(newdoc)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(newpath, vfs.TrashDirName+"/") {
		return nil, vfs.ErrParentInTrash
	}

	var bakpath string
	if olddoc != nil {
		bakpath = fmt.Sprintf("/.%s_%s", olddoc.ID(), olddoc.Rev())
		if err = safeRenameFile(afs.fs, newpath, bakpath); err != nil {
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
		w:    0,
		f:    f,
		size: newsize,

		afs:     afs,
		newdoc:  newdoc,
		olddoc:  olddoc,
		bakpath: bakpath,
		newpath: newpath,
		maxsize: maxsize,

		hash: hash,
		meta: extractor,
	}, nil
}

func (afs *aferoVFS) DestroyDirContent(doc *vfs.DirDoc) error {
	if lockerr := afs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer afs.mu.Unlock()
	return afs.destroyDirContent(doc)
}

func (afs *aferoVFS) DestroyDirAndContent(doc *vfs.DirDoc) error {
	if lockerr := afs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer afs.mu.Unlock()
	return afs.destroyDirAndContent(doc)
}

func (afs *aferoVFS) DestroyFile(doc *vfs.FileDoc) error {
	if lockerr := afs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer afs.mu.Unlock()
	return afs.destroyFile(doc)
}

func (afs *aferoVFS) destroyDirContent(doc *vfs.DirDoc) error {
	iter := afs.Indexer.DirIterator(doc, nil)
	var errm error
	for {
		d, f, erri := iter.Next()
		if erri == vfs.ErrIteratorDone {
			return errm
		}
		if erri != nil {
			return erri
		}
		var errd error
		if d != nil {
			errd = afs.destroyDirAndContent(d)
		} else {
			errd = afs.destroyFile(f)
		}
		if errd != nil {
			errm = multierror.Append(errm, errd)
		}
	}
}

func (afs *aferoVFS) destroyDirAndContent(doc *vfs.DirDoc) error {
	err := afs.destroyDirContent(doc)
	if err != nil {
		return err
	}
	err = afs.fs.RemoveAll(doc.Fullpath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return afs.Indexer.DeleteDirDoc(doc)
}

func (afs *aferoVFS) destroyFile(doc *vfs.FileDoc) error {
	path, err := afs.Indexer.FilePath(doc)
	if err != nil {
		return err
	}
	err = afs.fs.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return afs.Indexer.DeleteFileDoc(doc)
}

func (afs *aferoVFS) OpenFile(doc *vfs.FileDoc) (vfs.File, error) {
	if lockerr := afs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer afs.mu.RUnlock()
	name, err := afs.Indexer.FilePath(doc)
	if err != nil {
		return nil, err
	}
	f, err := afs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return &aferoFileOpen{f}, nil
}

func (afs *aferoVFS) Fsck() ([]vfs.FsckError, error) {
	if lockerr := afs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer afs.mu.RUnlock()
	root, err := afs.Indexer.DirByPath("/")
	if err != nil {
		return nil, err
	}
	var errors []vfs.FsckError
	return afs.fsckWalk(root, errors)
}

func (afs *aferoVFS) fsckWalk(dir *vfs.DirDoc, errors []vfs.FsckError) ([]vfs.FsckError, error) {
	entries := make(map[string]struct{})
	iter := afs.Indexer.DirIterator(dir, nil)
	for {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
			break
		}
		if err != nil {
			return nil, err
		}
		var fullpath string
		if f != nil {
			entries[f.DocName] = struct{}{}
			fullpath = path.Join(dir.Fullpath, f.DocName)
			stat, err := afs.fs.Stat(fullpath)
			if _, ok := err.(*os.PathError); ok {
				errors = append(errors, vfs.FsckError{
					Filename: fullpath,
					Message:  "the file is present in CouchDB but not on the local FS",
				})
			} else if err != nil {
				return nil, err
			} else if stat.IsDir() {
				errors = append(errors, vfs.FsckError{
					Filename: fullpath,
					Message:  "it's a file in CouchDB but a directory on the local FS",
				})
			}
		} else {
			entries[d.DocName] = struct{}{}
			stat, err := afs.fs.Stat(d.Fullpath)
			if _, ok := err.(*os.PathError); ok {
				errors = append(errors, vfs.FsckError{
					Filename: d.Fullpath,
					Message:  "the directory is present in CouchDB but not on the local FS",
				})
			} else if err != nil {
				return nil, err
			} else if !stat.IsDir() {
				errors = append(errors, vfs.FsckError{
					Filename: fullpath,
					Message:  "it's a directory in CouchDB but a file on the local FS",
				})
			} else {
				if errors, err = afs.fsckWalk(d, errors); err != nil {
					return nil, err
				}
			}
		}
	}

	fileinfos, err := afero.ReadDir(afs.fs, dir.Fullpath)
	if err != nil {
		return nil, err
	}
	for _, fileinfo := range fileinfos {
		if _, ok := entries[fileinfo.Name()]; !ok {
			filename := path.Join(dir.Fullpath, fileinfo.Name())
			if filename == "/.cozy_apps" || filename == "/.cozy_konnectors" || filename == "/.thumbs" {
				continue
			}
			what := "file"
			if fileinfo.IsDir() {
				what = "dir"
			}
			msg := fmt.Sprintf("the %s is present in CouchDB but not on the local FS", what)
			errors = append(errors, vfs.FsckError{
				Filename: filename,
				Message:  msg,
			})
		}
	}

	return errors, nil
}

// UpdateFileDoc overrides the indexer's one since the afero.Fs is by essence
// also indexed by path. When moving a file, the index has to be moved and the
// filesystem should also be updated.
//
// @override Indexer.UpdateFileDoc
func (afs *aferoVFS) UpdateFileDoc(olddoc, newdoc *vfs.FileDoc) error {
	if lockerr := afs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer afs.mu.Unlock()
	if newdoc.DirID != olddoc.DirID || newdoc.DocName != olddoc.DocName {
		oldpath, err := afs.Indexer.FilePath(olddoc)
		if err != nil {
			return err
		}
		newpath, err := afs.Indexer.FilePath(newdoc)
		if err != nil {
			return err
		}
		err = safeRenameFile(afs.fs, oldpath, newpath)
		if err != nil {
			return err
		}
	}
	if newdoc.Executable != olddoc.Executable {
		newpath, err := afs.Indexer.FilePath(newdoc)
		if err != nil {
			return err
		}
		err = afs.fs.Chmod(newpath, newdoc.Mode())
		if err != nil {
			return err
		}
	}
	return afs.Indexer.UpdateFileDoc(olddoc, newdoc)
}

// UpdateDirDoc overrides the indexer's one since the afero.Fs is by essence
// also indexed by path. When moving a file, the index has to be moved and the
// filesystem should also be updated.
//
// @override Indexer.UpdateDirDoc
func (afs *aferoVFS) UpdateDirDoc(olddoc, newdoc *vfs.DirDoc) error {
	if lockerr := afs.mu.Lock(); lockerr != nil {
		return lockerr
	}
	defer afs.mu.Unlock()
	if newdoc.Fullpath != olddoc.Fullpath {
		if err := safeRenameDir(afs, olddoc.Fullpath, newdoc.Fullpath); err != nil {
			return err
		}
	}
	return afs.Indexer.UpdateDirDoc(olddoc, newdoc)
}

func (afs *aferoVFS) DirByID(fileID string) (*vfs.DirDoc, error) {
	if lockerr := afs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer afs.mu.RUnlock()
	return afs.Indexer.DirByID(fileID)
}

func (afs *aferoVFS) DirByPath(name string) (*vfs.DirDoc, error) {
	if lockerr := afs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer afs.mu.RUnlock()
	return afs.Indexer.DirByPath(name)
}

func (afs *aferoVFS) FileByID(fileID string) (*vfs.FileDoc, error) {
	if lockerr := afs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer afs.mu.RUnlock()
	return afs.Indexer.FileByID(fileID)
}

func (afs *aferoVFS) FileByPath(name string) (*vfs.FileDoc, error) {
	if lockerr := afs.mu.RLock(); lockerr != nil {
		return nil, lockerr
	}
	defer afs.mu.RUnlock()
	return afs.Indexer.FileByPath(name)
}

func (afs *aferoVFS) FilePath(doc *vfs.FileDoc) (string, error) {
	if lockerr := afs.mu.RLock(); lockerr != nil {
		return "", lockerr
	}
	defer afs.mu.RUnlock()
	return afs.Indexer.FilePath(doc)
}

func (afs *aferoVFS) DirOrFileByID(fileID string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	if lockerr := afs.mu.RLock(); lockerr != nil {
		return nil, nil, lockerr
	}
	defer afs.mu.RUnlock()
	return afs.Indexer.DirOrFileByID(fileID)
}

func (afs *aferoVFS) DirOrFileByPath(name string) (*vfs.DirDoc, *vfs.FileDoc, error) {
	if lockerr := afs.mu.RLock(); lockerr != nil {
		return nil, nil, lockerr
	}
	defer afs.mu.RUnlock()
	return afs.Indexer.DirOrFileByPath(name)
}

// aferoFileOpen represents a file handle opened for reading.
type aferoFileOpen struct {
	f afero.File
}

func (f *aferoFileOpen) Read(p []byte) (int, error) {
	return f.f.Read(p)
}

func (f *aferoFileOpen) ReadAt(p []byte, off int64) (int, error) {
	return f.f.ReadAt(p, off)
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
	size    int64              // total file size, -1 if unknown
	afs     *aferoVFS          // parent vfs
	newdoc  *vfs.FileDoc       // new document
	olddoc  *vfs.FileDoc       // old document
	newpath string             // file new path
	bakpath string             // backup file path in case of modifying an existing file
	maxsize int64              // maximum size allowed for the file
	hash    hash.Hash          // hash we build up along the file
	meta    *vfs.MetaExtractor // extracts metadata from the content
	err     error              // write error
}

func (f *aferoFileCreation) Read(p []byte) (int, error) {
	return 0, os.ErrInvalid
}

func (f *aferoFileCreation) ReadAt(p []byte, off int64) (int, error) {
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
	if f.maxsize >= 0 && f.w > f.maxsize {
		f.err = vfs.ErrFileTooBig
		return n, f.err
	}

	if f.size >= 0 && f.w > f.size {
		f.err = vfs.ErrContentLengthMismatch
		return n, f.err
	}

	if f.meta != nil {
		if _, err = (*f.meta).Write(p); err != nil && err != io.ErrClosedPipe {
			(*f.meta).Abort(err)
			f.meta = nil
		}
	}

	_, err = f.hash.Write(p)
	return n, err
}

func (f *aferoFileCreation) Close() (err error) {
	defer func() {
		if err == nil && f.olddoc != nil {
			// remove the backup if no error occured
			f.afs.fs.Remove(f.bakpath) // #nosec
		} else if err != nil && f.olddoc != nil {
			// put back backup file revision in case on error occurred
			f.afs.fs.Rename(f.bakpath, f.newpath) // #nosec
		} else if err != nil {
			// remove the new file if an error occured
			f.afs.fs.Remove(f.newpath) // #nosec
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

	md5sum := f.hash.Sum(nil)
	if newdoc.MD5Sum == nil {
		newdoc.MD5Sum = md5sum
	}

	if !bytes.Equal(newdoc.MD5Sum, md5sum) {
		return vfs.ErrInvalidHash
	}

	if newdoc.ByteSize < 0 {
		newdoc.ByteSize = written
	}

	if newdoc.ByteSize != written {
		return vfs.ErrContentLengthMismatch
	}

	lockerr := f.afs.mu.Lock()
	if lockerr != nil {
		return lockerr
	}
	defer f.afs.mu.Unlock()
	if olddoc == nil {
		if newdoc.ID() == "" {
			return f.afs.Indexer.CreateFileDoc(newdoc)
		}
		return f.afs.Indexer.CreateNamedFileDoc(newdoc)
	}
	return f.afs.Indexer.UpdateFileDoc(olddoc, newdoc)
}

func safeCreateFile(name string, mode os.FileMode, fs afero.Fs) (afero.File, error) {
	// write only (O_WRONLY), try to create the file and check that it
	// does not already exist (O_CREATE|O_EXCL).
	flag := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	return fs.OpenFile(name, flag, mode)
}

func safeRenameFile(fs afero.Fs, oldpath, newpath string) error {
	newpath = path.Clean(newpath)
	oldpath = path.Clean(oldpath)

	if !path.IsAbs(newpath) || !path.IsAbs(oldpath) {
		return vfs.ErrNonAbsolutePath
	}

	_, err := fs.Stat(newpath)
	if err == nil {
		return os.ErrExist
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return fs.Rename(oldpath, newpath)
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

var (
	_ vfs.VFS  = &aferoVFS{}
	_ vfs.File = &aferoFileOpen{}
	_ vfs.File = &aferoFileCreation{}
)
