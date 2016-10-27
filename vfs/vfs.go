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
	"github.com/spf13/afero"
)

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
func GetDirOrFileDocFromPath(c *Context, pth string, withChildren bool) (typ string, dirDoc *DirDoc, fileDoc *FileDoc, err error) {
	dirDoc, err = GetDirDocFromPath(c, pth, withChildren)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	if err == nil {
		typ = DirType
		return
	}

	fileDoc, err = GetFileDocFromPath(c, pth)
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

func (c *Context) Stat(pth string) (os.FileInfo, error) {
	return c.fs.Stat(pth)
}

func (c *Context) Open(pth string) (afero.File, error) {
	return c.fs.Open(pth)
}

func (c *Context) ReadDir(pth string) ([]os.FileInfo, error) {
	return afero.ReadDir(c.fs, pth)
}

func (c *Context) Create(pth string) (*FileCreation, error) {
	pth = path.Clean(pth)

	filename, dirpath := path.Base(pth), path.Dir(pth)
	parent, err := GetDirDocFromPath(c, dirpath, false)
	if err != nil {
		return nil, err
	}

	exec := false
	extn := path.Ext(pth)
	mime, class := ExtractMimeAndClass(mimetype.TypeByExtension(extn))

	doc, err := NewFileDoc(filename, parent.ID(), -1, nil, mime, class, exec, []string{})
	if err != nil {
		return nil, err
	}

	return CreateFile(c, doc, nil)
}

func (c *Context) Mkdir(pth string) error {
	pth = path.Clean(pth)
	if pth == "/" {
		return nil
	}

	dirname, dirpath := path.Base(pth), path.Dir(pth)
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

func (c *Context) MkdirAll(pth string) error {
	var err error
	var dirs []string
	var base, file string
	var parent *DirDoc

	base = pth
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
		base = path.Dir(pth)
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

func (c *Context) Rename(oldpath, newpath string) error {
	typ, dir, file, err := GetDirOrFileDocFromPath(c, oldpath, false)
	if err != nil {
		return err
	}

	// simple rename
	if path.Dir(oldpath) == path.Dir(newpath) {
		newname := path.Base(newpath)
		patch := &DocPatch{Name: &newname}
		switch typ {
		case FileType:
			_, err = ModifyFileMetadata(c, file, patch)
		case DirType:
			_, err = ModifyDirMetadata(c, dir, patch)
		}
	} else {
		// @TODO
	}

	return err
}

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
