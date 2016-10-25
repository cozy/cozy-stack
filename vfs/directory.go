package vfs

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/couchdb/mango"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/spf13/afero"
)

// DirDoc is a struct containing all the informations about a
// directory. It implements the couchdb.Doc and jsonapi.Object
// interfaces.
type DirDoc struct {
	// Qualified file identifier
	DID string `json:"_id,omitempty"`
	// Directory revision
	DRev string `json:"_rev,omitempty"`
	// Directory name
	Name string `json:"name"`
	// Parent folder identifier
	FolderID string `json:"folder_id"`
	// Directory path on VFS
	Path string `json:"path"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Tags      []string  `json:"tags"`
}

// ID returns the directory qualified identifier (part of couchdb.Doc interface)
func (d *DirDoc) ID() string {
	return d.DID
}

// Rev returns the directory revision (part of couchdb.Doc interface)
func (d *DirDoc) Rev() string {
	return d.DRev
}

// DocType returns the directory document type (part of couchdb.Doc
// interface)
func (d *DirDoc) DocType() string {
	return string(FolderDocType)
}

// SetID is used to change the directory qualified identifier (part of
// couchdb.Doc interface)
func (d *DirDoc) SetID(id string) {
	d.DID = id
}

// SetRev is used to change the directory revision (part of
// couchdb.Doc interface)
func (d *DirDoc) SetRev(rev string) {
	d.DRev = rev
}

// SelfLink is used to generate a JSON-API link for the directory (part of
// jsonapi.Object interface)
func (d *DirDoc) SelfLink() string {
	return "/files/" + d.DID
}

// Relationships is used to generate the content relationship in JSON-API format
// (part of the jsonapi.Object interface)
func (d *DirDoc) Relationships() jsonapi.RelationshipMap {
	// TODO
	return jsonapi.RelationshipMap{}
}

// Included is part of the jsonapi.Object interface
func (d *DirDoc) Included() []jsonapi.Object {
	// TODO
	return []jsonapi.Object{}
}

func fetchChildrenDeep(c *Context, parent *DirDoc, doctype DocType, docs interface{}) error {
	req := &couchdb.FindRequest{
		Selector: mango.StartWith("path", parent.Path+"/"),
	}
	return couchdb.FindDocs(c.db, string(doctype), req, docs)
}

// NewDirDoc is the DirDoc constructor. The given name is validated.
func NewDirDoc(name, folderID string, tags []string) (doc *DirDoc, err error) {
	if err = checkFileName(name); err != nil {
		return
	}

	if folderID == "" {
		folderID = RootFolderID
	}

	createDate := time.Now()
	doc = &DirDoc{
		Name:     name,
		FolderID: folderID,

		CreatedAt: createDate,
		UpdatedAt: createDate,
		Tags:      tags,
	}

	return
}

// GetDirectoryDoc is used to fetch directory document information
// form the database.
func GetDirectoryDoc(c *Context, fileID string) (doc *DirDoc, err error) {
	doc = &DirDoc{}
	err = couchdb.GetDoc(c.db, string(FolderDocType), fileID, doc)
	if couchdb.IsNotFoundError(err) {
		err = ErrParentDoesNotExist
	}
	return
}

// GetDirectoryDocFromPath is used to fetch directory document information from
// the database from its path.
func GetDirectoryDocFromPath(c *Context, pth string) (*DirDoc, error) {
	var docs []*DirDoc
	req := &couchdb.FindRequest{
		Selector: mango.Equal("path", path.Clean(pth)),
		Limit:    1,
	}
	err := couchdb.FindDocs(c.db, string(FolderDocType), req, &docs)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, os.ErrNotExist
	}
	return docs[0], nil
}

// CreateDirectory is the method for creating a new directory
func CreateDirectory(c *Context, doc *DirDoc) (err error) {
	pth, _, err := getFilePath(c, doc.Name, doc.FolderID)
	if err != nil {
		return err
	}

	err = c.fs.Mkdir(pth, 0755)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			c.fs.Remove(pth)
		}
	}()

	doc.Path = pth

	return couchdb.CreateDoc(c.db, doc)
}

// ModifyDirectoryMetadata modify the metadata associated to a
// directory. It can be used to rename or move the directory in the
// VFS.
func ModifyDirectoryMetadata(c *Context, olddoc *DirDoc, data *DocMetaAttributes) (newdoc *DirDoc, err error) {
	pth := olddoc.Path
	name := olddoc.Name
	tags := olddoc.Tags
	folderID := olddoc.FolderID
	mdate := olddoc.UpdatedAt

	if data.FolderID != nil && *data.FolderID != folderID {
		folderID = *data.FolderID
		pth, _, err = getFilePath(c, name, folderID)
		if err != nil {
			return
		}
	}

	if data.Name != "" {
		name = data.Name
		pth = path.Join(path.Dir(pth), name)
	}

	if data.Tags != nil {
		tags = appendTags(tags, data.Tags)
	}

	if data.UpdatedAt != nil {
		mdate = *data.UpdatedAt
	}

	if mdate.Before(olddoc.CreatedAt) {
		err = ErrIllegalTime
		return
	}

	newdoc, err = NewDirDoc(name, folderID, tags)
	if err != nil {
		return
	}

	newdoc.SetID(olddoc.ID())
	newdoc.SetRev(olddoc.Rev())
	newdoc.CreatedAt = olddoc.CreatedAt
	newdoc.UpdatedAt = mdate
	newdoc.Path = pth

	if pth != olddoc.Path {
		err = renameDirectory(olddoc.Path, pth, c.fs)
		if err != nil {
			return
		}
	}

	err = bulkUpdateDocsPath(c, olddoc, pth)
	if err != nil {
		return
	}

	err = couchdb.UpdateDoc(c.db, newdoc)
	return
}

// @TODO remove this method and use couchdb bulk updates instead
func bulkUpdateDocsPath(c *Context, olddoc *DirDoc, newpath string) error {
	var children []*DirDoc

	err := fetchChildrenDeep(c, olddoc, FolderDocType, &children)
	if err != nil || len(children) == 0 {
		return err
	}

	oldpath := path.Clean(olddoc.Path)
	errc := make(chan error)

	for _, child := range children {
		go func(child *DirDoc) {
			if !strings.HasPrefix(child.Path, oldpath+"/") {
				errc <- fmt.Errorf("Child has wrong base directory")
			} else {
				child.Path = path.Join(newpath, child.Path[len(oldpath)+1:])
				errc <- couchdb.UpdateDoc(c.db, child)
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

func renameDirectory(oldpath, newpath string, fs afero.Fs) error {
	newpath = path.Clean(newpath)
	oldpath = path.Clean(oldpath)

	if !path.IsAbs(newpath) || !path.IsAbs(oldpath) {
		return fmt.Errorf("renameDirectory: paths should be absolute")
	}

	if strings.HasPrefix(newpath, oldpath+"/") {
		return ErrForbiddenDocMove
	}

	return fs.Rename(oldpath, newpath)
}
