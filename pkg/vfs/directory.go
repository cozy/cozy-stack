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
	"github.com/cozy/cozy-stack/web/jsonapi"
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

	parent *DirDoc
	files  []*FileDoc
	dirs   []*DirDoc
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
	parent, err := getParentDir(c, d.parent, d.DirID)
	if err != nil {
		return nil, err
	}
	d.parent = parent
	return parent, nil
}

// Links is used to generate a JSON-API link for the directory (part of
// jsonapi.Object interface)
func (d *DirDoc) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/files/" + d.DocID}
}

// Relationships is used to generate the content relationship in JSON-API format
// (part of the jsonapi.Object interface)
//
// TODO: pagination
func (d *DirDoc) Relationships() jsonapi.RelationshipMap {
	l := len(d.files) + len(d.dirs)
	i := 0

	data := make([]jsonapi.ResourceIdentifier, l)
	for _, child := range d.dirs {
		data[i] = jsonapi.ResourceIdentifier{ID: child.ID(), Type: child.DocType()}
		i++
	}

	for _, child := range d.files {
		data[i] = jsonapi.ResourceIdentifier{ID: child.ID(), Type: child.DocType()}
		i++
	}

	contents := jsonapi.Relationship{Data: data}

	var parent jsonapi.Relationship
	if d.ID() != consts.RootDirID {
		parent = jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Related: "/files/" + d.DirID,
			},
			Data: jsonapi.ResourceIdentifier{
				ID:   d.DirID,
				Type: consts.Files,
			},
		}
	}

	return jsonapi.RelationshipMap{
		"parent":   parent,
		"contents": contents,
	}
}

// Included is part of the jsonapi.Object interface
func (d *DirDoc) Included() []jsonapi.Object {
	var included []jsonapi.Object
	for _, child := range d.dirs {
		included = append(included, child)
	}
	for _, child := range d.files {
		included = append(included, child)
	}
	return included
}

// FetchFiles is used to fetch direct children of the directory.
//
// @TODO: add pagination control
func (d *DirDoc) FetchFiles(c Context) (err error) {
	d.files, d.dirs, err = fetchChildren(c, d)
	return err
}

// NewDirDoc is the DirDoc constructor. The given name is validated.
func NewDirDoc(name, dirID string, tags []string, parent *DirDoc) (*DirDoc, error) {
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

		parent: parent,
	}

	return doc, nil
}

// GetDirDoc is used to fetch directory document information
// form the database.
func GetDirDoc(c Context, fileID string, withChildren bool) (*DirDoc, error) {
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
	if withChildren {
		err = doc.FetchFiles(c)
	}
	return doc, err
}

// GetDirDocFromPath is used to fetch directory document information from
// the database from its path.
func GetDirDocFromPath(c Context, name string, withChildren bool) (*DirDoc, error) {
	if !path.IsAbs(name) {
		return nil, ErrNonAbsolutePath
	}

	var doc *DirDoc
	var err error

	var docs []*DirDoc
	sel := mango.Equal("path", path.Clean(name))
	req := &couchdb.FindRequest{Selector: sel, Limit: 1}
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

	if withChildren {
		err = doc.FetchFiles(c)
	}
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

	newdoc, err := NewDirDoc(*patch.Name, *patch.DirID, *patch.Tags, nil)
	if err != nil {
		return nil, err
	}

	newdoc.RestorePath = *patch.RestorePath

	var parent *DirDoc
	if newdoc.DirID != olddoc.DirID {
		parent, err = newdoc.Parent(c)
		if err != nil {
			return nil, err
		}
	} else {
		parent = olddoc.parent
	}

	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	newdoc.CreatedAt = cdate
	newdoc.UpdatedAt = *patch.UpdatedAt
	newdoc.parent = parent
	newdoc.files = olddoc.files
	newdoc.dirs = olddoc.dirs

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
	req := &couchdb.FindRequest{Selector: sel}
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
	err := doc.FetchFiles(c)
	if err != nil {
		return err
	}

	for _, dir := range doc.dirs {
		err = DestroyDirAndContent(c, dir)
		if err != nil {
			return err
		}
	}

	for _, file := range doc.files {
		err = DestroyFile(c, file)
		if err != nil {
			return err
		}
	}

	return err
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

func fetchChildren(c Context, parent *DirDoc) ([]*FileDoc, []*DirDoc, error) {
	var files []*FileDoc
	var dirs []*DirDoc
	var docs []*DirOrFileDoc
	sel := mango.Equal("dir_id", parent.ID())
	req := &couchdb.FindRequest{Selector: sel, Limit: 100}
	err := couchdb.FindDocs(c, consts.Files, req, &docs)
	if err != nil {
		return files, dirs, err
	}

	for _, doc := range docs {
		dir, file := doc.Refine()
		if dir != nil {
			dir.parent = parent
			dirs = append(dirs, dir)
		} else {
			file.parent = parent
			files = append(files, file)
		}
	}

	return files, dirs, nil
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
	_ couchdb.Doc    = &DirDoc{}
	_ jsonapi.Object = &DirDoc{}
)
