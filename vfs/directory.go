package vfs

import (
	"time"

	"github.com/cozy/cozy-stack/couchdb"
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
	FolderID string `json:"folderID"`
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

// NewDirDoc is the DirDoc constructor. The given name is validated.
func NewDirDoc(name, folderID string, tags []string) (doc *DirDoc, err error) {
	if err = checkFileName(name); err != nil {
		return
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
