// Package vfs is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a folder. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package vfs

import (
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

// RootFolderID is the identifier of the root folder
const RootFolderID = "io.cozy.files.rootdir"

// FsDocType is document type
const FsDocType = "io.cozy.files"

const (
	// DirType is the type attribute for directories
	DirType = "directory"
	// FileType is the type attribute for files
	FileType = "file"
)

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

func (fd *dirOrFile) refine() (typ string, dir *DirDoc, file *FileDoc) {
	typ = fd.Type
	switch typ {
	case DirType:
		dir = &fd.DirDoc
	case FileType:
		file = &FileDoc{
			Type:       fd.Type,
			ObjID:      fd.ObjID,
			ObjRev:     fd.ObjRev,
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
func GetDirOrFileDoc(c *Context, fileID string, withChildren bool) (typ string, dirDoc *DirDoc, fileDoc *FileDoc, err error) {
	dirOrFile := &dirOrFile{}
	err = couchdb.GetDoc(c.db, FsDocType, fileID, dirOrFile)
	if err != nil {
		return
	}

	typ, dirDoc, fileDoc = dirOrFile.refine()
	if typ == DirType && withChildren {
		dirDoc.FetchFiles(c)
	}
	return
}

// GetDirOrFileDocFromPath is used to fetch a document from its path
// without knowning in advance its type.
func GetDirOrFileDocFromPath(c *Context, name string, withChildren bool) (typ string, dirDoc *DirDoc, fileDoc *FileDoc, err error) {
	dirDoc, err = GetDirDocFromPath(c, name, withChildren)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	if err == nil {
		typ = DirType
		return
	}

	fileDoc, err = GetFileDocFromPath(c, name)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	if err == nil {
		typ = FileType
		return
	}

	return
}

// Context is used to convey the afero.Fs object along with the
// CouchDb database prefix.
type Context struct {
	fs afero.Fs
	db string
}

// NewContext is the constructor function for Context
func NewContext(fs afero.Fs, dbprefix string) *Context {
	return &Context{fs, dbprefix}
}

// Stat returns the FileInfo of the specified file or directory.
func (c *Context) Stat(name string) (os.FileInfo, error) {
	return c.fs.Stat(name)
}

// Open returns a file handler of the specified name that can be used
// for reading.
func (c *Context) Open(name string) (*File, error) {
	return c.OpenFile(name, os.O_RDONLY, 0)
}

// OpenFile returns a file handler of the specified name. It is a
// generalized the generilized call used to open a file. It opens the
// file with the given flag (O_RDONLY, O_WRONLY, O_CREATE, O_EXCL) and
// permission.
func (c *Context) OpenFile(name string, flag int, perm os.FileMode) (*File, error) {
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
func (c *Context) Create(name string) (*File, error) {
	return c.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
}

// ReadDir returns a list of FileInfo of all the direct children of
// the specified directory.
func (c *Context) ReadDir(name string) ([]os.FileInfo, error) {
	return afero.ReadDir(c.fs, name)
}

// Mkdir creates a new directory with the specified name
func (c *Context) Mkdir(name string) error {
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

	return CreateDirectory(c, dir)
}

// MkdirAll creates a directory named path, along with any necessary
// parents, and returns nil, or else returns an error.
func (c *Context) MkdirAll(name string) error {
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
			err = CreateDirectory(c, parent)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// Rename will rename a file or directory from a specified path to
// another.
func (c *Context) Rename(oldpath, newpath string) error {
	typ, dir, file, err := GetDirOrFileDocFromPath(c, oldpath, false)
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

	switch typ {
	case FileType:
		_, err = ModifyFileMetadata(c, file, patch)
	case DirType:
		_, err = ModifyDirMetadata(c, dir, patch)
	}

	return err
}

// Remove removes the specified named file or directory.
func (c *Context) Remove(name string) error {
	// TODO: fix this remove method implemented for now only to support
	// go-git. This method should also remove the document from
	// database.
	return c.fs.Remove(name)
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
func getParentDir(c *Context, parent *DirDoc, folderID string) (*DirDoc, error) {
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
