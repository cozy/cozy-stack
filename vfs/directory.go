package vfs

import (
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/couchdb"
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
	// Parent folder identifier
	FolderID string `json:"folder_id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Directory path on VFS
	Fullpath string   `json:"path"`
	Tags     []string `json:"tags"`

	files []*FileDoc
	dirs  []*DirDoc
}

// ID returns the directory qualified identifier
func (d *DirDoc) ID() string { return d.DocID }

// Rev returns the directory revision
func (d *DirDoc) Rev() string { return d.DocRev }

// DocType returns the directory document type
func (d *DirDoc) DocType() string { return FsDocType }

// SetID changes the directory qualified identifier
func (d *DirDoc) SetID(id string) { d.DocID = id }

// SetRev changes the directory revision
func (d *DirDoc) SetRev(rev string) { d.DocRev = rev }

func (d *DirDoc) calcPath(c Context) error {
	if d.Fullpath != "" {
		return nil
	}
	parent, err := GetDirDoc(c, d.FolderID, false)
	if err != nil {
		return err
	}
	parentPath, err := parent.Path(c)
	if err != nil {
		return err
	}
	d.Fullpath = path.Join(parentPath, d.Name)
	return nil
}

// Path is used to generate the file path
func (d *DirDoc) Path(c Context) (string, error) {
	if err := d.calcPath(c); err != nil {
		return "", err
	}
	return d.Fullpath, nil
}

// SelfLink is used to generate a JSON-API link for the directory (part of
// jsonapi.Object interface)
func (d *DirDoc) SelfLink() string {
	return "/files/" + d.DocID
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
	if d.ID() != RootFolderID {
		parent = jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Related: "/files/" + d.FolderID,
			},
			Data: jsonapi.ResourceIdentifier{
				ID:   d.FolderID,
				Type: FsDocType,
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
	d.files, d.dirs, err = c.FSCache().DirFiles(c, d)
	return err
}

// NewDirDoc is the DirDoc constructor. The given name is validated.
func NewDirDoc(name, folderID string, tags []string) (doc *DirDoc, err error) {
	if err = checkFileName(name); err != nil {
		return
	}

	if folderID == "" {
		folderID = RootFolderID
	}

	tags = uniqueTags(tags)

	createDate := time.Now()
	doc = &DirDoc{
		Type:     DirType,
		Name:     name,
		FolderID: folderID,

		CreatedAt: createDate,
		UpdatedAt: createDate,
		Tags:      tags,
	}

	return
}

// GetDirDoc is used to fetch directory document information
// form the database.
func GetDirDoc(c Context, fileID string, withChildren bool) (*DirDoc, error) {
	doc, err := c.FSCache().DirByID(c, fileID)
	if err != nil {
		if fileID == RootFolderID {
			panic("Root folder is not in database")
		}
		return nil, err
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
	doc, err := c.FSCache().DirByPath(c, name)
	if err != nil {
		return nil, err
	}
	if withChildren {
		err = doc.FetchFiles(c)
	}
	return doc, err
}

// CreateDir is the method for creating a new directory
func CreateDir(c Context, doc *DirDoc) (err error) {
	pth, err := doc.Path(c)
	if err != nil {
		return err
	}

	err = c.FS().Mkdir(pth, 0755)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			c.FS().Remove(pth)
		}
	}()

	return c.FSCache().CreateDir(c, doc)
}

// CreateRootDirDoc creates the root folder document for this context
func CreateRootDirDoc(c Context) error {
	return couchdb.CreateNamedDocWithDB(c, &DirDoc{
		Name:     "",
		Type:     DirType,
		DocID:    RootFolderID,
		Fullpath: "/",
		FolderID: "",
	})
}

// CreateTrashDir creates the trash folder for this context
func CreateTrashDir(c Context) error {
	err := c.FS().Mkdir(TrashDirName, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}
	err = couchdb.CreateNamedDocWithDB(c, &DirDoc{
		Name:     path.Base(TrashDirName),
		Type:     DirType,
		DocID:    TrashFolderID,
		Fullpath: TrashDirName,
		FolderID: RootFolderID,
	})
	if err != nil && !couchdb.IsConflictError(err) {
		return err
	}
	return nil
}

// ModifyDirMetadata modify the metadata associated to a directory. It
// can be used to rename or move the directory in the VFS.
func ModifyDirMetadata(c Context, olddoc *DirDoc, patch *DocPatch) (newdoc *DirDoc, err error) {
	id := olddoc.ID()
	if id == RootFolderID || id == TrashFolderID {
		return nil, os.ErrInvalid
	}

	cdate := olddoc.CreatedAt
	patch, err = normalizeDocPatch(&DocPatch{
		Name:      &olddoc.Name,
		FolderID:  &olddoc.FolderID,
		Tags:      &olddoc.Tags,
		UpdatedAt: &olddoc.UpdatedAt,
	}, patch, cdate)

	if err != nil {
		return
	}

	newdoc, err = NewDirDoc(*patch.Name, *patch.FolderID, *patch.Tags)
	if err != nil {
		return
	}

	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	newdoc.CreatedAt = cdate
	newdoc.UpdatedAt = *patch.UpdatedAt
	newdoc.files = olddoc.files
	newdoc.dirs = olddoc.dirs

	oldpath, err := olddoc.Path(c)
	if err != nil {
		return
	}
	newpath, err := newdoc.Path(c)
	if err != nil {
		return
	}

	if oldpath != newpath {
		err = safeRenameDirectory(c, oldpath, newpath)
		if err != nil {
			return
		}
	}

	err = c.FSCache().UpdateDir(c, olddoc, newdoc)
	return
}

// TrashDir is used to delete a directory given its document
func TrashDir(c Context, olddoc *DirDoc) (newdoc *DirDoc, err error) {
	oldpath, err := olddoc.Path(c)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(oldpath, TrashDirName) {
		return nil, ErrFileInTrash
	}
	trashFolderID := TrashFolderID
	tryOrUseSuffix(olddoc.Name, "%scozy__%s", func(name string) error {
		newdoc, err = ModifyDirMetadata(c, olddoc, &DocPatch{
			FolderID: &trashFolderID,
			Name:     &name,
		})
		return err
	})
	return
}

func safeRenameDirectory(c Context, oldpath, newpath string) error {
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
