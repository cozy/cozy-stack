package vfs

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
)

// DirDoc is a struct containing all the informations about a
// directory. It implements the couchdb.Doc and jsonapi.Object
// interfaces.
type DirDoc struct {
	// Type of document. Useful to (de)serialize and filter the data
	// from couch.
	Type string `json:"type"`
	// Qualified file identifier
	DocID string `json:"_id,omitempty"`
	// Directory revision
	DocRev string `json:"_rev,omitempty"`
	// Directory name
	Name string `json:"name"`
	// Parent directory identifier
	DirID       string `json:"dir_id"`
	RestorePath string `json:"restore_path,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Tags      []string  `json:"tags"`

	// Directory path on VFS
	Fullpath string `json:"path"`
}

// ID returns the directory qualified identifier
func (d *DirDoc) ID() string { return d.DocID }

// Rev returns the directory revision
func (d *DirDoc) Rev() string { return d.DocRev }

// DocType returns the directory document type
func (d *DirDoc) DocType() string { return consts.Files }

// SetID changes the directory qualified identifier
func (d *DirDoc) SetID(id string) { d.DocID = id }

// SetRev changes the directory revision
func (d *DirDoc) SetRev(rev string) { d.DocRev = rev }

// Path is used to generate the file path
func (d *DirDoc) Path(c Context) (string, error) {
	if d.Fullpath == "" {
		parent, err := d.Parent(c)
		if err != nil {
			return "", err
		}
		parentPath, err := parent.Path(c)
		if err != nil {
			return "", err
		}
		d.Fullpath = path.Join(parentPath, d.Name)
	}
	return d.Fullpath, nil
}

// Parent returns the parent directory document
func (d *DirDoc) Parent(c Context) (*DirDoc, error) {
	return GetDirDoc(c, d.DirID)
}

// ChildrenIterator returns an iterator to iterate over the children of
// the directory.
func (d *DirDoc) ChildrenIterator(c Context, opts *IteratorOptions) *Iterator {
	return NewIterator(c, mango.Equal("dir_id", d.DocID), opts)
}

// IsEmpty returns whether or not the directory has at least one child.
func (d *DirDoc) IsEmpty(c Context) (bool, error) {
	iter := d.ChildrenIterator(c, &IteratorOptions{ByFetch: 1})
	_, _, err := iter.Next()
	if err == ErrIteratorDone {
		return true, nil
	}
	return false, err
}

// NewDirDoc is the DirDoc constructor. The given name is validated.
func NewDirDoc(name, dirID string, tags []string) (*DirDoc, error) {
	if err := checkFileName(name); err != nil {
		return nil, err
	}

	if dirID == "" {
		dirID = consts.RootDirID
	}

	tags = uniqueTags(tags)

	createDate := time.Now()
	doc := &DirDoc{
		Type:  consts.DirType,
		Name:  name,
		DirID: dirID,

		CreatedAt: createDate,
		UpdatedAt: createDate,
		Tags:      tags,
	}

	return doc, nil
}

// GetDirDoc is used to fetch directory document information
// form the database.
func GetDirDoc(c Context, fileID string) (*DirDoc, error) {
	doc := &DirDoc{}
	err := couchdb.GetDoc(c, consts.Files, fileID, doc)
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

// GetDirDocFromPath is used to fetch directory document information from
// the database from its path.
func GetDirDocFromPath(c Context, name string) (*DirDoc, error) {
	if !path.IsAbs(name) {
		return nil, ErrNonAbsolutePath
	}

	var doc *DirDoc
	var err error

	var docs []*DirDoc
	sel := mango.Equal("path", path.Clean(name))
	req := &couchdb.FindRequest{
		UseIndex: "dir-by-path",
		Selector: sel,
		Limit:    1,
	}
	err = couchdb.FindDocs(c, consts.Files, req, &docs)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		if name == "/" {
			panic("Root directory is not in database")
		}
		return nil, os.ErrNotExist
	}
	doc = docs[0]
	return doc, err
}

// CreateDir is the method for creating a new directory
func CreateDir(c Context, doc *DirDoc) error {
	pth, err := doc.Path(c)
	if err != nil {
		return err
	}

	err = c.FS().Mkdir(pth, 0755)
	if err != nil {
		return err
	}

	err = couchdb.CreateDoc(c, doc)
	if err != nil {
		c.FS().Remove(pth)
	}
	return err
}

// CreateRootDirDoc creates the root directory document for this context
func CreateRootDirDoc(c Context) error {
	return couchdb.CreateNamedDocWithDB(c, &DirDoc{
		Name:     "",
		Type:     consts.DirType,
		DocID:    consts.RootDirID,
		Fullpath: "/",
		DirID:    "",
	})
}

// CreateTrashDir creates the trash directory for this context
func CreateTrashDir(c Context) error {
	err := c.FS().Mkdir(TrashDirName, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}
	err = couchdb.CreateNamedDocWithDB(c, &DirDoc{
		Name:     path.Base(TrashDirName),
		Type:     consts.DirType,
		DocID:    consts.TrashDirID,
		Fullpath: TrashDirName,
		DirID:    consts.RootDirID,
	})
	if err != nil && !couchdb.IsConflictError(err) {
		return err
	}
	return nil
}

// ModifyDirMetadata modify the metadata associated to a directory. It
// can be used to rename or move the directory in the VFS.
func ModifyDirMetadata(c Context, olddoc *DirDoc, patch *DocPatch) (*DirDoc, error) {
	id := olddoc.ID()
	if id == consts.RootDirID || id == consts.TrashDirID {
		return nil, os.ErrInvalid
	}

	var err error
	cdate := olddoc.CreatedAt
	patch, err = normalizeDocPatch(&DocPatch{
		Name:        &olddoc.Name,
		DirID:       &olddoc.DirID,
		RestorePath: &olddoc.RestorePath,
		Tags:        &olddoc.Tags,
		UpdatedAt:   &olddoc.UpdatedAt,
	}, patch, cdate)

	if err != nil {
		return nil, err
	}

	newdoc, err := NewDirDoc(*patch.Name, *patch.DirID, *patch.Tags)
	if err != nil {
		return nil, err
	}

	newdoc.RestorePath = *patch.RestorePath

	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	newdoc.CreatedAt = cdate
	newdoc.UpdatedAt = *patch.UpdatedAt

	oldpath, err := olddoc.Path(c)
	if err != nil {
		return nil, err
	}
	newpath, err := newdoc.Path(c)
	if err != nil {
		return nil, err
	}

	if oldpath != newpath {
		err = safeRenameDir(c, oldpath, newpath)
		if err != nil {
			return nil, err
		}
		err = bulkUpdateDocsPath(c, oldpath, newpath)
		if err != nil {
			return nil, err
		}
	}

	err = couchdb.UpdateDoc(c, newdoc)
	return newdoc, err
}

// @TODO remove this method and use couchdb bulk updates instead
func bulkUpdateDocsPath(c Context, oldpath, newpath string) error {
	var children []*DirDoc
	sel := mango.StartWith("path", oldpath+"/")
	req := &couchdb.FindRequest{
		UseIndex: "dir-by-path",
		Selector: sel,
	}
	err := couchdb.FindDocs(c, consts.Files, req, &children)
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
				errc <- couchdb.UpdateDoc(c, child)
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

// TrashDir is used to delete a directory given its document
func TrashDir(c Context, olddoc *DirDoc) (*DirDoc, error) {
	oldpath, err := olddoc.Path(c)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(oldpath, TrashDirName) {
		return nil, ErrFileInTrash
	}

	trashDirID := consts.TrashDirID
	restorePath := path.Dir(oldpath)

	var newdoc *DirDoc
	tryOrUseSuffix(olddoc.Name, conflictFormat, func(name string) error {
		newdoc, err = ModifyDirMetadata(c, olddoc, &DocPatch{
			DirID:       &trashDirID,
			RestorePath: &restorePath,
			Name:        &name,
		})
		return err
	})
	return newdoc, err
}

// RestoreDir is used to restore a trashed directory given its document
func RestoreDir(c Context, olddoc *DirDoc) (*DirDoc, error) {
	oldpath, err := olddoc.Path(c)
	if err != nil {
		return nil, err
	}
	restoreDir, err := getRestoreDir(c, oldpath, olddoc.RestorePath)
	if err != nil {
		return nil, err
	}
	var newdoc *DirDoc
	var emptyStr string
	name := stripSuffix(olddoc.Name, conflictSuffix)
	tryOrUseSuffix(name, "%s (%s)", func(name string) error {
		newdoc, err = ModifyDirMetadata(c, olddoc, &DocPatch{
			DirID:       &restoreDir.DocID,
			RestorePath: &emptyStr,
			Name:        &name,
		})
		return err
	})
	return newdoc, err
}

// DestroyDirContent destroy all directories and files contained in a directory.
func DestroyDirContent(c Context, doc *DirDoc) error {
	iter := doc.ChildrenIterator(c, nil)
	for {
		d, f, err := iter.Next()
		if err == ErrIteratorDone {
			break
		}
		if d != nil {
			err = DestroyDirAndContent(c, d)
		} else {
			err = DestroyFile(c, f)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// DestroyDirAndContent destroy a directory and its content
func DestroyDirAndContent(c Context, doc *DirDoc) error {
	err := DestroyDirContent(c, doc)
	if err != nil {
		return err
	}
	dirpath, err := doc.Path(c)
	if err != nil {
		return err
	}
	err = c.FS().RemoveAll(dirpath)
	if err != nil {
		return err
	}
	err = couchdb.DeleteDoc(c, doc)
	return err
}

func safeRenameDir(c Context, oldpath, newpath string) error {
	newpath = path.Clean(newpath)
	oldpath = path.Clean(oldpath)

	if !path.IsAbs(newpath) || !path.IsAbs(oldpath) {
		return ErrNonAbsolutePath
	}

	if strings.HasPrefix(newpath, oldpath+"/") {
		return ErrForbiddenDocMove
	}

	_, err := c.FS().Stat(newpath)
	if err == nil {
		return os.ErrExist
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return c.FS().Rename(oldpath, newpath)
}

var (
	_ couchdb.Doc = &DirDoc{}
)
