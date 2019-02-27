// Package vfs is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a directory. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package vfs

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/filetype"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// ForbiddenFilenameChars is the list of forbidden characters in a filename.
const ForbiddenFilenameChars = "/\x00\n\r"

const (
	// TrashDirName is the path of the trash directory
	TrashDirName = "/.cozy_trash"
	// ThumbsDirName is the path of the directory for thumbnails
	ThumbsDirName = "/.thumbs"
	// WebappsDirName is the path of the directory in which apps are stored
	WebappsDirName = "/.cozy_apps"
	// KonnectorsDirName is the path of the directory in which konnectors source
	// are stored
	KonnectorsDirName = "/.cozy_konnectors"
	// OrphansDirName is the path of the directory used to store data-files added
	// in the index from a filesystem-check (fsck)
	OrphansDirName = "/.cozy_orphans"
)

const (
	conflictSuffix = " (__cozy__: "
	conflictFormat = "%s (__cozy__: %s)"
)

// maxWalkRecursive is the maximum amount of recursion allowed for the
// recursive walk process.
const maxWalkRecursive = 512

// ErrSkipDir is used in WalkFn as an error to skip the current
// directory. It is not returned by any function of the package.
var ErrSkipDir = errors.New("skip directories")

// ErrWalkOverflow is used in the walk process when the maximum amount of
// recursivity allowed is reached when browsing the index tree.
var ErrWalkOverflow = errors.New("vfs: walk overflow")

// Fs is an interface providing a set of high-level methods to interact with
// the file-system binaries and metadata.
type Fs interface {
	prefixer.Prefixer
	InitFs() error
	Delete() error

	// OpenFile return a file handler for reading associated with the given file
	// document. The file handler implements io.ReadCloser and io.Seeker.
	OpenFile(doc *FileDoc) (File, error)
	// CreateDir is used to create a new directory from its document.
	CreateDir(doc *DirDoc) error
	// CreateFile creates a new file or update the content of an existing file.
	// The first argument contains the document of the new or update version of
	// the file. The second argument is the optional old document of the old
	// version of the file.
	//
	// Warning: you MUST call the Close() method and check for its error.
	CreateFile(newdoc, olddoc *FileDoc) (File, error)
	// DestroyDirContent destroys all directories and files contained in a
	// directory.
	DestroyDirContent(doc *DirDoc) error
	// DestroyDirAndContent destroys all directories and files contained in a
	// directory and the directory itself.
	DestroyDirAndContent(doc *DirDoc) error
	// DestroyFile  destroys a file from the trash.
	DestroyFile(doc *FileDoc) error

	// Fsck return the list of inconsistencies in the VFS
	Fsck(func(log *FsckLog)) (err error)
}

// File is a reader, writer, seeker, closer iterface representing an opened
// file for reading or writing.
type File interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Writer
	io.Closer
}

// FilePather is an interface for computing the fullpath of a filedoc
type FilePather interface {
	FilePath(doc *FileDoc) (string, error)
}

// Indexer is an interface providing a common set of method for indexing layer
// of our VFS.
//
// An indexer is typically responsible for storing and indexing the files and
// directories metadata, as well as caching them if necessary.
type Indexer interface {
	InitIndex() error

	FilePather

	// DiskUsage computes the total size of the files contained in the VFS.
	DiskUsage() (int64, error)

	// CreateFileDoc creates and add in the index a new file document.
	CreateFileDoc(doc *FileDoc) error
	// CreateNamedFileDoc creates and add in the index a new file document with
	// its id already set.
	CreateNamedFileDoc(doc *FileDoc) error
	// UpdateFileDoc is used to update the document of a file. It takes the
	// new file document that you want to create and the old document,
	// representing the current revision of the file.
	UpdateFileDoc(olddoc, newdoc *FileDoc) error
	// DeleteFileDoc removes from the index the specified file document.
	DeleteFileDoc(doc *FileDoc) error

	// CreateDirDoc creates and add in the index a new directory document.
	CreateDirDoc(doc *DirDoc) error
	// CreateNamedDirDoc creates and add in the index a new directory document
	// with its id already set.
	CreateNamedDirDoc(doc *DirDoc) error
	// UpdateDirDoc is used to update the document of a directory. It takes the
	// new directory document that you want to create and the old document,
	// representing the current revision of the directory.
	UpdateDirDoc(olddoc, newdoc *DirDoc) error
	// DeleteDirDoc removes from the index the specified directory document.
	DeleteDirDoc(doc *DirDoc) error
	// DeleteDirDocAndContent removes from the index the specified directory as
	// well all its children. It returns the list of the children files ids that
	// were removed.
	DeleteDirDocAndContent(doc *DirDoc, onlyContent bool) (int64, []string, error)

	// DirByID returns the directory document information associated with the
	// specified identifier.
	DirByID(fileID string) (*DirDoc, error)
	// DirByPath returns the directory document information associated with the
	// specified path.
	DirByPath(name string) (*DirDoc, error)

	// FileByID returns the file document information associated with the
	// specified identifier.
	FileByID(fileID string) (*FileDoc, error)
	// FileByPath returns the file document information associated with the
	// specified path.
	FileByPath(name string) (*FileDoc, error)

	// DirOrFileByID returns the document from its identifier without knowing in
	// advance its type. One of the returned argument is not nil.
	DirOrFileByID(fileID string) (*DirDoc, *FileDoc, error)
	// DirOrFileByPath returns the document from its path without knowing in
	// advance its type. One of the returned argument is not nil.
	DirOrFileByPath(name string) (*DirDoc, *FileDoc, error)

	// DirIterator returns an iterator over the children of the specified
	// directory.
	DirIterator(doc *DirDoc, opts *IteratorOptions) DirIterator

	// DirBatch returns a batch of documents
	DirBatch(*DirDoc, couchdb.Cursor) ([]DirOrFileDoc, error)
	DirLength(*DirDoc) (int, error)
	DirChildExists(dirID, filename string) (bool, error)
	BatchDelete([]couchdb.Doc) error

	BuildTree(each ...func(*TreeFile)) (tree *Tree, err error)
	CheckIndexIntegrity(func(*FsckLog)) error
}

// DiskThresholder it an interface that can be implemeted to known how many space
// is available on the disk.
type DiskThresholder interface {
	// DiskQuota returns the total number of bytes allowed to be stored in the
	// VFS. If minus or equal to zero, it is considered without limit.
	DiskQuota() int64
}

// Thumbser defines an interface to define a thumbnail filesystem.
type Thumbser interface {
	ThumbExists(img *FileDoc, format string) (ok bool, err error)
	CreateThumb(img *FileDoc, format string) (ThumbFiler, error)
	RemoveThumbs(img *FileDoc, formats []string) error
	ServeThumbContent(w http.ResponseWriter, req *http.Request,
		img *FileDoc, format string) error
}

// ThumbFiler defines a interface to handle the creation of thumbnails. It is
// an io.Writer that can be aborted in case of error, or committed in case of
// success.
type ThumbFiler interface {
	io.Writer
	Abort() error
	Commit() error
}

// VFS is composed of the Indexer and Fs interface. It is the common interface
// used throughout the stack to access the VFS.
type VFS interface {
	Indexer
	DiskThresholder
	Fs

	// UseSharingIndexer returns a new Fs with an overload indexer that can be
	// used for the special purpose of the sharing.
	UseSharingIndexer(Indexer) VFS
}

// ErrIteratorDone is returned by the Next() method of the iterator when
// the iterator is actually done.
var ErrIteratorDone = errors.New("No more element in the iterator")

// IteratorOptions contains the options of the iterator.
type IteratorOptions struct {
	AfterID string
	ByFetch int
}

// DirIterator is the interface that an iterator over a specific directory
// should implement. The Next method will return a ErrIteratorDone when the
// iterator is over and does not have element anymore.
type DirIterator interface {
	Next() (*DirDoc, *FileDoc, error)
}

// DocPatch is a struct containing modifiable fields from file and
// directory documents.
type DocPatch struct {
	Name        *string    `json:"name,omitempty"`
	DirID       *string    `json:"dir_id,omitempty"`
	RestorePath *string    `json:"restore_path,omitempty"`
	Tags        *[]string  `json:"tags,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	Executable  *bool      `json:"executable,omitempty"`
	MD5Sum      *[]byte    `json:"md5sum,omitempty"`
	Class       *string    `json:"class,omitempty"`
}

// DirOrFileDoc is a union struct of FileDoc and DirDoc. It is useful to
// unmarshal documents from couch.
type DirOrFileDoc struct {
	*DirDoc

	// fields from FileDoc not contained in DirDoc
	ByteSize   int64    `json:"size,string"`
	MD5Sum     []byte   `json:"md5sum,omitempty"`
	Mime       string   `json:"mime,omitempty"`
	Class      string   `json:"class,omitempty"`
	Executable bool     `json:"executable,omitempty"`
	Trashed    bool     `json:"trashed,omitempty"`
	Metadata   Metadata `json:"metadata,omitempty"`
}

// Clone is part of the couchdb.Doc interface
func (fd *DirOrFileDoc) Clone() couchdb.Doc {
	panic("DirOrFileDoc must not be cloned")
}

// Refine returns either a DirDoc or FileDoc pointer depending on the type of
// the DirOrFileDoc
func (fd *DirOrFileDoc) Refine() (*DirDoc, *FileDoc) {
	switch fd.Type {
	case consts.DirType:
		return fd.DirDoc, nil
	case consts.FileType:
		return nil, &FileDoc{
			Type:         fd.Type,
			DocID:        fd.DocID,
			DocRev:       fd.DocRev,
			DocName:      fd.DocName,
			DirID:        fd.DirID,
			RestorePath:  fd.RestorePath,
			CreatedAt:    fd.CreatedAt,
			UpdatedAt:    fd.UpdatedAt,
			ByteSize:     fd.ByteSize,
			MD5Sum:       fd.MD5Sum,
			Mime:         fd.Mime,
			Class:        fd.Class,
			Executable:   fd.Executable,
			Trashed:      fd.Trashed,
			Tags:         fd.Tags,
			Metadata:     fd.Metadata,
			ReferencedBy: fd.ReferencedBy,
		}
	}
	return nil, nil
}

// Stat returns the FileInfo of the specified file or directory.
func Stat(fs VFS, name string) (os.FileInfo, error) {
	d, f, err := fs.DirOrFileByPath(name)
	if err != nil {
		return nil, err
	}
	if d != nil {
		return d, nil
	}
	return f, nil
}

// OpenFile returns a file handler of the specified name. It is a
// generalized the generilized call used to open a file. It opens the
// file with the given flag (O_RDONLY, O_WRONLY, O_CREATE, O_EXCL) and
// permission.
func OpenFile(fs VFS, name string, flag int, perm os.FileMode) (File, error) {
	if flag&os.O_RDWR != 0 || flag&os.O_APPEND != 0 {
		return nil, os.ErrInvalid
	}
	if flag&os.O_CREATE != 0 && flag&os.O_EXCL == 0 {
		return nil, os.ErrInvalid
	}

	name = path.Clean(name)

	if flag == os.O_RDONLY {
		doc, err := fs.FileByPath(name)
		if err != nil {
			return nil, err
		}
		return fs.OpenFile(doc)
	}

	var dirID string
	olddoc, err := fs.FileByPath(name)
	if os.IsNotExist(err) && flag&os.O_CREATE != 0 {
		var parent *DirDoc
		parent, err = fs.DirByPath(path.Dir(name))
		if err != nil {
			return nil, err
		}
		dirID = parent.ID()
	}
	if err != nil {
		return nil, err
	}

	if olddoc != nil {
		dirID = olddoc.DirID
	}

	if dirID == "" {
		return nil, os.ErrInvalid
	}

	filename := path.Base(name)
	exec := false
	trashed := false
	mime, class := ExtractMimeAndClassFromFilename(filename)
	newdoc, err := NewFileDoc(filename, dirID, -1, nil, mime, class, time.Now(), exec, trashed, []string{})
	if err != nil {
		return nil, err
	}
	return fs.CreateFile(newdoc, olddoc)
}

// Create creates a new file with specified and returns a File handler
// that can be used for writing.
func Create(fs VFS, name string) (File, error) {
	return OpenFile(fs, name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
}

// Mkdir creates a new directory with the specified name
func Mkdir(fs VFS, name string, tags []string) (*DirDoc, error) {
	name = path.Clean(name)
	if name == "/" {
		return nil, ErrParentDoesNotExist
	}

	dirname, dirpath := path.Base(name), path.Dir(name)
	parent, err := fs.DirByPath(dirpath)
	if err != nil {
		return nil, err
	}

	dir, err := NewDirDocWithParent(dirname, parent, tags)
	if err != nil {
		return nil, err
	}

	if err = fs.CreateDir(dir); err != nil {
		return nil, err
	}

	return dir, nil
}

// MkdirAll creates a directory named path, along with any necessary
// parents, and returns nil, or else returns an error.
func MkdirAll(fs VFS, name string) (*DirDoc, error) {
	var err error
	var dirs []string
	var base, file string
	var parent *DirDoc

	base = name
	for {
		parent, err = fs.DirByPath(base)
		if os.IsNotExist(err) {
			base, file = path.Dir(base), path.Base(base)
			dirs = append(dirs, file)
			continue
		}
		if err != nil {
			return nil, err
		}
		break
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		parent, err = NewDirDocWithParent(dirs[i], parent, nil)
		if err == nil {
			err = fs.CreateDir(parent)
			// XXX MkdirAll has no lock, so we have to consider the risk of a race condition
			if os.IsExist(err) {
				parent, err = fs.DirByPath(path.Join(parent.Fullpath, dirs[i]))
			}
		}
		if err != nil {
			return nil, err
		}
	}

	return parent, nil
}

// Rename will rename a file or directory from a specified path to
// another.
func Rename(fs VFS, oldpath, newpath string) error {
	dir, file, err := fs.DirOrFileByPath(oldpath)
	if err != nil {
		return err
	}

	newname := path.Base(newpath)

	var newdirID *string
	if path.Dir(oldpath) != path.Dir(newpath) {
		var parent *DirDoc
		parent, err = fs.DirByPath(path.Dir(newpath))
		if err != nil {
			return err
		}
		newdirID = &parent.DirID
	} else {
		newdirID = nil
	}

	patch := &DocPatch{
		Name:  &newname,
		DirID: newdirID,
	}

	if dir != nil {
		_, err = ModifyDirMetadata(fs, dir, patch)
	} else {
		_, err = ModifyFileMetadata(fs, file, patch)
	}

	return err
}

// Remove removes the specified named file or directory.
func Remove(fs VFS, name string) error {
	dir, file, err := fs.DirOrFileByPath(name)
	if err != nil {
		return err
	}
	if file != nil {
		return fs.DestroyFile(file)
	}
	empty, err := dir.IsEmpty(fs)
	if err != nil {
		return err
	}
	if !empty {
		return ErrDirNotEmpty
	}
	return fs.DestroyDirAndContent(dir)
}

// RemoveAll removes the specified name file or directory and its content.
func RemoveAll(fs VFS, name string) error {
	dir, file, err := fs.DirOrFileByPath(name)
	if err != nil {
		return err
	}
	if dir != nil {
		return fs.DestroyDirAndContent(dir)
	}
	return fs.DestroyFile(file)
}

// Exists returns wether or not the specified path exist in the file system.
func Exists(fs VFS, name string) (bool, error) {
	_, _, err := fs.DirOrFileByPath(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// DirExists returns wether or not the specified path exist in the file system
// and is associated with a directory.
func DirExists(fs VFS, name string) (bool, error) {
	_, err := fs.DirByPath(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// WalkFn type works like filepath.WalkFn type function. It receives
// as argument the complete name of the file or directory, the type of
// the document, the actual directory or file document and a possible
// error.
type WalkFn func(name string, dir *DirDoc, file *FileDoc, err error) error

// Walk walks the file tree document rooted at root. It should work
// like filepath.Walk.
func Walk(fs Indexer, root string, walkFn WalkFn) error {
	dir, file, err := fs.DirOrFileByPath(root)
	if err != nil {
		return walkFn(root, dir, file, err)
	}
	return walk(fs, root, dir, file, walkFn, 0)
}

// WalkByID walks the file tree document rooted at root. It should work
// like filepath.Walk.
func WalkByID(fs Indexer, fileID string, walkFn WalkFn) error {
	dir, file, err := fs.DirOrFileByID(fileID)
	if err != nil {
		return walkFn("", dir, file, err)
	}
	if dir != nil {
		return walk(fs, dir.Fullpath, dir, file, walkFn, 0)
	}
	root, err := file.Path(fs)
	if err != nil {
		return walkFn("", dir, file, err)
	}
	return walk(fs, root, dir, file, walkFn, 0)
}

func walk(fs Indexer, name string, dir *DirDoc, file *FileDoc, walkFn WalkFn, count int) error {
	if count >= maxWalkRecursive {
		return ErrWalkOverflow
	}
	err := walkFn(name, dir, file, nil)
	if err != nil {
		if dir != nil && err == ErrSkipDir {
			return nil
		}
		return err
	}
	if file != nil {
		return nil
	}
	iter := fs.DirIterator(dir, nil)
	for {
		d, f, err := iter.Next()
		if err == ErrIteratorDone {
			break
		}
		if err != nil {
			return walkFn(name, nil, nil, err)
		}
		var fullpath string
		if f != nil {
			fullpath = path.Join(name, f.DocName)
		} else {
			fullpath = path.Join(name, d.DocName)
		}
		if err = walk(fs, fullpath, d, f, walkFn, count+1); err != nil {
			return err
		}
	}
	return nil
}

// ExtractMimeAndClass returns a mime and class value from the
// specified content-type. For now it only takes the first segment of
// the type as the class and the whole type as mime.
func ExtractMimeAndClass(contentType string) (mime, class string) {
	if contentType == "" {
		contentType = filetype.DefaultType
	}

	charsetIndex := strings.Index(contentType, ";")
	if charsetIndex >= 0 {
		mime = contentType[:charsetIndex]
	} else {
		mime = contentType
	}

	mime = strings.TrimSpace(mime)
	switch mime {
	case filetype.DefaultType:
		class = "files"
	case "application/x-apple-diskimage", "application/x-msdownload":
		class = "binary"
	case "text/html", "text/css", "text/xml", "application/js", "text/x-c",
		"text/x-go", "text/x-python", "application/x-ruby":
		class = "code"
	case "application/pdf":
		class = "pdf"
	case "application/vnd.ms-powerpoint", "application/x-iwork-keynote-sffkey",
		"application/vnd.oasis.opendocument.graphics",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation":
		class = "slide"
	case "application/vnd.ms-excel", "application/x-iwork-numbers-sffnumbers",
		"application/vnd.oasis.opendocument.spreadsheet",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		class = "spreadsheet"
	case "application/msword", "application/x-iwork-pages-sffpages",
		"application/vnd.oasis.opendocument.text",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		class = "text"
	case "application/x-7z-compressed", "application/x-rar-compressed",
		"application/zip", "application/gzip", "application/x-tar":
		class = "zip"
	default:
		slashIndex := strings.Index(mime, "/")
		if slashIndex >= 0 {
			class = mime[:slashIndex]
		} else {
			class = mime
		}
	}

	return mime, class
}

// ExtractMimeAndClassFromFilename is a shortcut of
// ExtractMimeAndClass used to generate the mime and class from a
// filename.
func ExtractMimeAndClassFromFilename(name string) (mime, class string) {
	ext := path.Ext(name)
	return ExtractMimeAndClass(filetype.ByExtension(ext))
}

var cbDiskQuotaAlert func(domain string, exceeded bool)

// RegisterDiskQuotaAlertCallback allows to register a callback function called
// when the instance reaches, a fall behind, 90% of its quota capacity.
func RegisterDiskQuotaAlertCallback(cb func(domain string, exceeded bool)) {
	cbDiskQuotaAlert = cb
}

// PushDiskQuotaAlert can be used to notify when the VFS reaches, or fall
// behind, its quota alert of 90% of its total capacity.
func PushDiskQuotaAlert(fs VFS, exceeded bool) {
	if cbDiskQuotaAlert != nil {
		cbDiskQuotaAlert(fs.DomainName(), exceeded)
	}
}

// DiskQuotaAfterDestroy is a helper function that can be used after files or
// directories have be erased from the disk in order to register that the disk
// quota alert has fall behind (or not).
func DiskQuotaAfterDestroy(fs VFS, diskUsageBeforeWrite, destroyed int64) {
	if diskUsageBeforeWrite <= 0 {
		return
	}
	diskQuota := fs.DiskQuota()
	quotaBytes := int64(9.0 / 10.0 * float64(diskQuota))
	if diskUsageBeforeWrite >= quotaBytes &&
		diskUsageBeforeWrite-destroyed < quotaBytes {
		PushDiskQuotaAlert(fs, false)
	}
}

// getRestoreDir returns the restoration directory document from a file a
// directory path. The specified file path should be part of the trash
// directory.
func getRestoreDir(fs VFS, name, restorePath string) (*DirDoc, error) {
	if !strings.HasPrefix(name, TrashDirName) {
		return nil, ErrFileNotInTrash
	}

	// If the restore path is not set, it means that the file is part of a
	// directory hierarchy which has been trashed. The parent directory at the
	// root of the trash directory is the document which contains the information
	// of the restore path.
	//
	// For instance, when trying the restore the baz file inside
	// TrashDirName/foo/bar/baz/quz, it should extract the "foo" (root) and
	// "bar/baz" (rest) parts of the path.
	if restorePath == "" {
		name = strings.TrimPrefix(name, TrashDirName+"/")
		split := strings.Index(name, "/")
		if split >= 0 {
			root := name[:split]
			rest := path.Dir(name[split+1:])
			doc, err := fs.DirByPath(TrashDirName + "/" + root)
			if err != nil {
				return nil, err
			}
			if doc.RestorePath != "" {
				restorePath = path.Join(doc.RestorePath, doc.DocName, rest)
			}
		}
	}

	// This should not happened but is here in case we could not resolve the
	// restore path
	if restorePath == "" {
		restorePath = "/"
	}

	// If the restore directory does not exist anymore, we re-create the
	// directory hierarchy to restore the file in.
	restoreDir, err := fs.DirByPath(restorePath)
	if os.IsNotExist(err) {
		return MkdirAll(fs, restorePath)
	}
	return restoreDir, err
}

func normalizeDocPatch(data, patch *DocPatch, cdate time.Time) (*DocPatch, error) {
	if patch.DirID == nil {
		patch.DirID = data.DirID
	}

	if patch.RestorePath == nil {
		patch.RestorePath = data.RestorePath
	}

	if patch.Name == nil {
		patch.Name = data.Name
	}

	if patch.Tags == nil {
		patch.Tags = data.Tags
	}

	if patch.UpdatedAt == nil || patch.UpdatedAt.Unix() < 0 {
		patch.UpdatedAt = data.UpdatedAt
	}

	if patch.UpdatedAt.Before(cdate) {
		return nil, ErrIllegalTime
	}

	if patch.Executable == nil {
		patch.Executable = data.Executable
	}

	return patch, nil
}

func checkFileName(str string) error {
	if str == "" || strings.ContainsAny(str, ForbiddenFilenameChars) {
		return ErrIllegalFilename
	}
	return nil
}

func uniqueTags(tags []string) []string {
	m := make(map[string]struct{})
	clone := make([]string, 0)
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := m[tag]; !ok {
			clone = append(clone, tag)
			m[tag] = struct{}{}
		}
	}
	return clone
}
