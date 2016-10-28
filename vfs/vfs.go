// Package vfs is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a folder. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package vfs

import (
	"os"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/spf13/afero"
)

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
func GetDirOrFileDoc(c *Context, fileID string) (typ string, dirDoc *DirDoc, fileDoc *FileDoc, err error) {
	if fileID == RootFolderID {
		typ, dirDoc = DirType, getRootDirDoc()
		return
	}

	dirOrFile := &dirOrFile{}
	err = couchdb.GetDoc(c.db, FsDocType, fileID, dirOrFile)
	if err != nil {
		return
	}

	typ, dirDoc, fileDoc = dirOrFile.refine()
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

// getParentDir returns the parent directory document if nil.
func getParentDir(c *Context, parent *DirDoc, folderID string) (*DirDoc, error) {
	if parent != nil {
		return parent, nil
	}
	var err error
	if folderID == RootFolderID {
		parent = getRootDirDoc()
	} else {
		parent, err = GetDirDoc(c, folderID, false)
	}
	return parent, err
}

// @TODO: do a fetch from couchdb when instance creation is ok.
func getRootDirDoc() *DirDoc {
	return &DirDoc{
		ObjID:    RootFolderID,
		ObjRev:   "1-",
		Fullpath: "/",
	}
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
	var clone []string
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
