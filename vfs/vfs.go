// Package vfs is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a folder. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package vfs

import (
	"errors"
	mimetype "mime"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/couchdb/mango"
	"github.com/spf13/afero"
)

// Indexes is the list of required indexes by the VFS inside CouchDB.
var Indexes = []mango.Index{
	// Used to lookup a file given its parent
	mango.IndexOnFields("folder_id", "name", "type"),
	// Used to lookup a directory given its path
	mango.IndexOnFields("path"),
	// Used to lookup children of a directory
	mango.IndexOnFields("folder_id"),
}

// DefaultContentType is used for files uploaded with no content-type
const DefaultContentType = "application/octet-stream"

// ForbiddenFilenameChars is the list of forbidden characters in a filename.
const ForbiddenFilenameChars = "/\x00"

const (
	// FsDocType is document type
	FsDocType = "io.cozy.files"
	// RootFolderID is the identifier of the root directory
	RootFolderID = "io.cozy.files.rootdir"
	// TrashFolderID is the identifier of the trash directory
	TrashFolderID = "io.cozy.files.trashdir"
	// TrashDirName is the path of the trash directory
	TrashDirName = "/.cozy_trash"
	// AppsDirName is the path of the directory in which apps are stored
	AppsDirName = "/.cozy_apps"
)

const (
	// DirType is the type attribute for directories
	DirType = "directory"
	// FileType is the type attribute for files
	FileType = "file"
)

// ErrSkipDir is used in WalkFn as an error to skip the current
// directory. It is not returned by any function of the package.
var ErrSkipDir = errors.New("skip directories")

// Context is used to convey the afero.Fs object along with the
// CouchDb database prefix.
type Context interface {
	couchdb.Database
	FS() afero.Fs
}

// DocPatch is a struct containing modifiable fields from file and
// directory documents.
type DocPatch struct {
	Name       *string    `json:"name,omitempty"`
	FolderID   *string    `json:"folder_id,omitempty"`
	Tags       *[]string  `json:"tags,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
	Executable *bool      `json:"executable,omitempty"`
}

// dirOrFile is a union struct of FileDoc and DirDoc. It is useful to
// unmarshal documents from couch.
type dirOrFile struct {
	DirDoc

	// fields from FileDoc not contained in DirDoc
	Size       int64  `json:"size,string"`
	MD5Sum     []byte `json:"md5sum"`
	Mime       string `json:"mime"`
	Class      string `json:"class"`
	Executable bool   `json:"executable"`
}

func (fd *dirOrFile) refine() (dir *DirDoc, file *FileDoc) {
	switch fd.Type {
	case DirType:
		dir = &fd.DirDoc
	case FileType:
		file = &FileDoc{
			Type:       fd.Type,
			DocID:      fd.DocID,
			DocRev:     fd.DocRev,
			Name:       fd.Name,
			FolderID:   fd.FolderID,
			CreatedAt:  fd.CreatedAt,
			UpdatedAt:  fd.UpdatedAt,
			Size:       fd.Size,
			MD5Sum:     fd.MD5Sum,
			Mime:       fd.Mime,
			Class:      fd.Class,
			Executable: fd.Executable,
			Tags:       fd.Tags,
		}
	}
	return
}

// GetDirOrFileDoc is used to fetch a document from its identifier
// without knowing in advance its type.
func GetDirOrFileDoc(c Context, fileID string, withChildren bool) (dirDoc *DirDoc, fileDoc *FileDoc, err error) {
	dirOrFile := &dirOrFile{}
	err = couchdb.GetDoc(c, FsDocType, fileID, dirOrFile)
	if err != nil {
		return
	}

	dirDoc, fileDoc = dirOrFile.refine()
	if dirDoc != nil && withChildren {
		dirDoc.FetchFiles(c)
	}
	return
}

// GetDirOrFileDocFromPath is used to fetch a document from its path
// without knowning in advance its type.
func GetDirOrFileDocFromPath(c Context, name string, withChildren bool) (dirDoc *DirDoc, fileDoc *FileDoc, err error) {
	dirDoc, err = GetDirDocFromPath(c, name, withChildren)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	if err == nil {
		return
	}

	fileDoc, err = GetFileDocFromPath(c, name)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	if err == nil {
		return
	}

	return
}

// Stat returns the FileInfo of the specified file or directory.
func Stat(c Context, name string) (os.FileInfo, error) {
	return c.FS().Stat(name)
}

// OpenFile returns a file handler of the specified name. It is a
// generalized the generilized call used to open a file. It opens the
// file with the given flag (O_RDONLY, O_WRONLY, O_CREATE, O_EXCL) and
// permission.
func OpenFile(c Context, name string, flag int, perm os.FileMode) (*File, error) {
	if flag&os.O_RDWR != 0 || flag&os.O_APPEND != 0 {
		return nil, os.ErrInvalid
	}
	if flag&os.O_CREATE != 0 && flag&os.O_EXCL == 0 {
		return nil, os.ErrInvalid
	}

	name = path.Clean(name)

	if flag == os.O_RDONLY {
		doc, err := GetFileDocFromPath(c, name)
		if err != nil {
			return nil, err
		}
		return Open(c, doc)
	}

	var err error
	var folderID string
	var olddoc *FileDoc
	var parent *DirDoc

	if flag&os.O_CREATE != 0 {
		if parent, err = GetDirDocFromPath(c, path.Dir(name), false); err != nil {
			return nil, err
		}
		folderID = parent.ID()
	} else if flag&os.O_WRONLY != 0 {
		if olddoc, err = GetFileDocFromPath(c, name); err != nil {
			return nil, err
		}
		folderID = olddoc.FolderID
	}

	if folderID == "" {
		return nil, os.ErrInvalid
	}

	filename := path.Base(name)
	exec := false
	mime, class := ExtractMimeAndClassFromFilename(filename)
	newdoc, err := NewFileDoc(filename, folderID, -1, nil, mime, class, exec, []string{})
	if err != nil {
		return nil, err
	}
	return CreateFile(c, newdoc, olddoc)
}

// Create creates a new file with specified and returns a File handler
// that can be used for writing.
func Create(c Context, name string) (*File, error) {
	return OpenFile(c, name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
}

// Touch creates a file or directory or updates its modification time
// if it already exists.
func Touch(c Context, name string) error {
	dir, file, err := GetDirOrFileDocFromPath(c, name, false)
	if os.IsNotExist(err) {
		var f *File
		f, err = Create(c, name)
		if err != nil {
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}

	now := time.Now()
	patch := &DocPatch{UpdatedAt: &now}
	if dir != nil {
		_, err = ModifyDirMetadata(c, dir, patch)
	} else {
		_, err = ModifyFileMetadata(c, file, patch)
	}

	return err
}

// ReadDir returns a list of FileInfo of all the direct children of
// the specified directory.
func ReadDir(c Context, name string) ([]os.FileInfo, error) {
	if !path.IsAbs(name) {
		return nil, ErrNonAbsolutePath
	}

	return afero.ReadDir(c.FS(), name)
}

// Mkdir creates a new directory with the specified name
func Mkdir(c Context, name string) error {
	name = path.Clean(name)
	if name == "/" {
		return nil
	}

	dirname, dirpath := path.Base(name), path.Dir(name)
	parent, err := GetDirDocFromPath(c, dirpath, false)
	if err != nil {
		return err
	}

	dir, err := NewDirDoc(dirname, parent.ID(), nil, nil)
	if err != nil {
		return err
	}

	return CreateDir(c, dir)
}

// MkdirAll creates a directory named path, along with any necessary
// parents, and returns nil, or else returns an error.
func MkdirAll(c Context, name string) error {
	var err error
	var dirs []string
	var base, file string
	var parent *DirDoc

	base = name
	for {
		parent, err = GetDirDocFromPath(c, base, false)
		if os.IsNotExist(err) {
			base, file = path.Dir(base), path.Base(base)
			dirs = append(dirs, file)
			continue
		}
		if err != nil {
			return err
		}
		break
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		parent, err = NewDirDoc(dirs[i], parent.ID(), nil, parent)
		if err == nil {
			err = CreateDir(c, parent)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// Rename will rename a file or directory from a specified path to
// another.
func Rename(c Context, oldpath, newpath string) error {
	dir, file, err := GetDirOrFileDocFromPath(c, oldpath, false)
	if err != nil {
		return err
	}

	newname := path.Base(newpath)

	var newfolderID *string
	if path.Dir(oldpath) != path.Dir(newpath) {
		var parent *DirDoc
		parent, err = GetDirDocFromPath(c, path.Dir(newpath), false)
		if err != nil {
			return err
		}
		newfolderID = &parent.FolderID
	} else {
		newfolderID = nil
	}

	patch := &DocPatch{
		Name:     &newname,
		FolderID: newfolderID,
	}

	if dir != nil {
		_, err = ModifyDirMetadata(c, dir, patch)
	} else {
		_, err = ModifyFileMetadata(c, file, patch)
	}

	return err
}

// Remove removes the specified named file or directory.
func Remove(c Context, name string) error {
	if !path.IsAbs(name) {
		return ErrNonAbsolutePath
	}

	// TODO: fix this remove method implemented for now only to support
	// go-git. This method should also remove the document from
	// database.
	return c.FS().Remove(name)
}

// Trash moves the specified file or directory into the trash. The
// deleteDirContent boolean parameter if set to true will allow to
// remove a directory even if its not empty.
func Trash(c Context, name string, deleteDirContent bool) error {
	dir, file, err := GetDirOrFileDocFromPath(c, name, true)
	if err != nil {
		return err
	}

	if dir != nil {
		if !deleteDirContent && len(dir.files)+len(dir.dirs) == 0 {
			return ErrDirectoryNotEmpty
		}
		_, err = TrashDir(c, dir)
	} else {
		_, err = TrashFile(c, file)
	}

	return err
}

// WalkFn type works like filepath.WalkFn type function. It receives
// as argument the complete name of the file or directory, the type of
// the document, the actual directory or file document and a possible
// error.
type WalkFn func(name string, dir *DirDoc, file *FileDoc, err error) error

// Walk walks the file tree document rooted at root. It should work
// like filepath.Walk.
func Walk(c Context, root string, walkFn WalkFn) error {
	dir, file, err := GetDirOrFileDocFromPath(c, root, false)
	if err != nil {
		return walkFn(root, dir, file, err)
	}
	return walk(c, root, dir, file, walkFn)
}

func walk(c Context, name string, dir *DirDoc, file *FileDoc, walkFn WalkFn) error {
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

	err = dir.FetchFiles(c)
	if err != nil {
		return walkFn(name, dir, nil, err)
	}

	for _, d := range dir.dirs {
		fullpath := path.Join(name, d.Name)
		err = walk(c, fullpath, d, nil, walkFn)
		if err != nil && err != ErrSkipDir {
			return err
		}
	}

	for _, f := range dir.files {
		fullpath := path.Join(name, f.Name)
		err = walk(c, fullpath, nil, f, walkFn)
		if err != nil {
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
		contentType = DefaultContentType
	}

	charsetIndex := strings.Index(contentType, ";")
	if charsetIndex >= 0 {
		mime = contentType[:charsetIndex]
	} else {
		mime = contentType
	}

	// @TODO improve for specific mime types
	slashIndex := strings.Index(contentType, "/")
	if slashIndex >= 0 {
		class = contentType[:slashIndex]
	} else {
		class = contentType
	}

	return
}

// ExtractMimeAndClassFromFilename is a shortcut of
// ExtractMimeAndClass used to generate the mime and class from a
// filename.
func ExtractMimeAndClassFromFilename(name string) (mime, class string) {
	ext := path.Ext(name)
	return ExtractMimeAndClass(mimetype.TypeByExtension(ext))
}

// getParentDir returns the parent directory document if nil.
func getParentDir(c Context, parent *DirDoc, folderID string) (*DirDoc, error) {
	if parent != nil {
		return parent, nil
	}
	var err error
	parent, err = GetDirDoc(c, folderID, false)
	return parent, err
}

func normalizeDocPatch(data, patch *DocPatch, cdate time.Time) (*DocPatch, error) {
	if patch.FolderID == nil {
		patch.FolderID = data.FolderID
	}

	if patch.Name == nil {
		patch.Name = data.Name
	}

	if patch.Tags == nil {
		patch.Tags = data.Tags
	}

	if patch.UpdatedAt == nil {
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
