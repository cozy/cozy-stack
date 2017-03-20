// Package vfs is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a directory. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package vfs

import (
	"errors"
	mimetype "mime"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/spf13/afero"
)

// DefaultContentType is used for files uploaded with no content-type
const DefaultContentType = "application/octet-stream"

// ForbiddenFilenameChars is the list of forbidden characters in a filename.
const ForbiddenFilenameChars = "/\x00"

const (
	// TrashDirName is the path of the trash directory
	TrashDirName = "/.cozy_trash"
	// AppsDirName is the path of the directory in which apps are stored
	AppsDirName = "/.cozy_apps"
)

const (
	conflictSuffix = " (__cozy__: "
	conflictFormat = "%s (__cozy__: %s)"
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
	Name        *string    `json:"name,omitempty"`
	DirID       *string    `json:"dir_id,omitempty"`
	RestorePath *string    `json:"restore_path,omitempty"`
	Tags        *[]string  `json:"tags,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	Executable  *bool      `json:"executable,omitempty"`
}

// DirOrFileDoc is a union struct of FileDoc and DirDoc. It is useful to
// unmarshal documents from couch.
type DirOrFileDoc struct {
	DirDoc

	// fields from FileDoc not contained in DirDoc
	Size       int64    `json:"size,string"`
	MD5Sum     []byte   `json:"md5sum"`
	Mime       string   `json:"mime"`
	Class      string   `json:"class"`
	Executable bool     `json:"executable"`
	Metadata   Metadata `json:"metadata,omitempty"`
}

// Refine returns either a DirDoc or FileDoc pointer depending on the type of
// the DirOrFileDoc
func (fd *DirOrFileDoc) Refine() (*DirDoc, *FileDoc) {
	switch fd.Type {
	case consts.DirType:
		return &fd.DirDoc, nil
	case consts.FileType:
		return nil, &FileDoc{
			Type:        fd.Type,
			DocID:       fd.DocID,
			DocRev:      fd.DocRev,
			Name:        fd.Name,
			DirID:       fd.DirID,
			RestorePath: fd.RestorePath,
			CreatedAt:   fd.CreatedAt,
			UpdatedAt:   fd.UpdatedAt,
			Size:        fd.Size,
			MD5Sum:      fd.MD5Sum,
			Mime:        fd.Mime,
			Class:       fd.Class,
			Executable:  fd.Executable,
			Tags:        fd.Tags,
			Metadata:    fd.Metadata,
		}
	}
	return nil, nil
}

// GetDirOrFileDoc is used to fetch a document from its identifier
// without knowing in advance its type.
func GetDirOrFileDoc(c Context, fileID string, withChildren bool) (*DirDoc, *FileDoc, error) {
	dirOrFile := &DirOrFileDoc{}
	err := couchdb.GetDoc(c, consts.Files, fileID, dirOrFile)
	if err != nil {
		return nil, nil, err
	}

	dirDoc, fileDoc := dirOrFile.Refine()
	if dirDoc != nil && withChildren {
		dirDoc.FetchFiles(c)
	}
	return dirDoc, fileDoc, nil
}

// GetDirOrFileDocFromPath is used to fetch a document from its path
// without knowning in advance its type.
func GetDirOrFileDocFromPath(c Context, name string, withChildren bool) (*DirDoc, *FileDoc, error) {
	dirDoc, err := GetDirDocFromPath(c, name, withChildren)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err == nil {
		return dirDoc, nil, nil
	}

	fileDoc, err := GetFileDocFromPath(c, name)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err == nil {
		return nil, fileDoc, nil
	}

	return nil, nil, err
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

	var dirID string
	olddoc, err := GetFileDocFromPath(c, name)
	if os.IsNotExist(err) && flag&os.O_CREATE != 0 {
		var parent *DirDoc
		parent, err = GetDirDocFromPath(c, path.Dir(name), false)
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
	mime, class := ExtractMimeAndClassFromFilename(filename)
	newdoc, err := NewFileDoc(filename, dirID, -1, nil, mime, class, time.Now(), exec, []string{})
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

// ReadDir returns a list of FileInfo of all the direct children of
// the specified directory.
func ReadDir(c Context, name string) ([]os.FileInfo, error) {
	if !path.IsAbs(name) {
		return nil, ErrNonAbsolutePath
	}

	return afero.ReadDir(c.FS(), name)
}

// Mkdir creates a new directory with the specified name
func Mkdir(c Context, name string, tags []string) (*DirDoc, error) {
	name = path.Clean(name)
	if name == "/" {
		return nil, ErrParentDoesNotExist
	}

	dirname, dirpath := path.Base(name), path.Dir(name)
	parent, err := GetDirDocFromPath(c, dirpath, false)
	if err != nil {
		return nil, err
	}

	dir, err := NewDirDoc(dirname, parent.ID(), tags, nil)
	if err != nil {
		return nil, err
	}

	if err = CreateDir(c, dir); err != nil {
		return nil, err
	}

	return dir, nil
}

// MkdirAll creates a directory named path, along with any necessary
// parents, and returns nil, or else returns an error.
func MkdirAll(c Context, name string, tags []string) (*DirDoc, error) {
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
			return nil, err
		}
		break
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		parent, err = NewDirDoc(dirs[i], parent.ID(), nil, parent)
		if err == nil {
			err = CreateDir(c, parent)
		}
		if err != nil {
			return nil, err
		}
	}

	return parent, nil
}

// Rename will rename a file or directory from a specified path to
// another.
func Rename(c Context, oldpath, newpath string) error {
	dir, file, err := GetDirOrFileDocFromPath(c, oldpath, false)
	if err != nil {
		return err
	}

	newname := path.Base(newpath)

	var newdirID *string
	if path.Dir(oldpath) != path.Dir(newpath) {
		var parent *DirDoc
		parent, err = GetDirDocFromPath(c, path.Dir(newpath), false)
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
	dir, file, err := GetDirOrFileDocFromPath(c, name, true)
	if err != nil {
		return err
	}
	if dir != nil {
		if len(dir.files) > 0 {
			return ErrDirNotEmpty
		}
		return DestroyDirAndContent(c, dir)
	}
	return DestroyFile(c, file)
}

// RemoveAll removes the specified name file or directory and its content.
func RemoveAll(c Context, name string) error {
	if !path.IsAbs(name) {
		return ErrNonAbsolutePath
	}
	dir, file, err := GetDirOrFileDocFromPath(c, name, true)
	if err != nil {
		return err
	}
	if dir != nil {
		return DestroyDirAndContent(c, dir)
	}
	return DestroyFile(c, file)
}

// DiskUsage computes the total size of the files
func DiskUsage(c Context) (int64, error) {
	var doc couchdb.ViewResponse
	err := couchdb.ExecView(c, consts.DiskUsageView, &couchdb.ViewRequest{
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
		return 0, ErrWrongCouchdbState
	}

	return int64(f64), nil
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

	return mime, class
}

// ExtractMimeAndClassFromFilename is a shortcut of
// ExtractMimeAndClass used to generate the mime and class from a
// filename.
func ExtractMimeAndClassFromFilename(name string) (mime, class string) {
	ext := path.Ext(name)
	return ExtractMimeAndClass(mimetype.TypeByExtension(ext))
}

// getParentDir returns the parent directory document if nil.
func getParentDir(c Context, parent *DirDoc, dirID string) (*DirDoc, error) {
	if parent != nil {
		return parent, nil
	}
	var err error
	parent, err = GetDirDoc(c, dirID, false)
	return parent, err
}

// getRestoreDir returns the restoration directory document from a file a
// directory path. The specified file path should be part of the trash
// directory.
func getRestoreDir(c Context, name, restorePath string) (*DirDoc, error) {
	if !strings.HasPrefix(name, TrashDirName) {
		return nil, ErrFileNotInTrash
	}

	// If the restore path is set, it means that the file is part of a directory
	// hierarchy which has been trashed. The parent directory at the root of the
	// trash directory is the document which contains the information of
	// the restore path.
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
			doc, err := GetDirDocFromPath(c, TrashDirName+"/"+root, false)
			if err != nil {
				return nil, err
			}
			if doc.RestorePath != "" {
				restorePath = path.Join(doc.RestorePath, doc.Name, rest)
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
	restoreDir, err := GetDirDocFromPath(c, restorePath, false)
	if os.IsNotExist(err) {
		restoreDir, err = MkdirAll(c, restorePath, nil)
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
