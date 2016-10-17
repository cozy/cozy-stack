package vfs

import (
	"encoding/json"
	"time"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/spf13/afero"
)

// DirDoc is a struct containing all the informations about a
// directory. It implements the couchdb.Doc and jsonapi.JSONApier
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

// ToJSONApi implements temporary interface JSONApier to serialize
// the directory document
func (d *DirDoc) ToJSONApi() ([]byte, error) {
	attrs := map[string]interface{}{
		"name":       d.Name,
		"created_at": d.CreatedAt,
		"updated_at": d.UpdatedAt,
		"tags":       d.Tags,
	}
	data := map[string]interface{}{
		"type":       d.DocType(),
		"id":         d.ID(),
		"rev":        d.Rev(),
		"attributes": attrs,
	}
	m := map[string]interface{}{
		"data": data,
	}
	return json.Marshal(m)
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

// CreateDirectory is the method for creating a new directory
func CreateDirectory(doc *DirDoc, fs afero.Fs, dbPrefix string) error {
	var err error

	pth, _, err := createNewFilePath(doc.Name, doc.FolderID, fs, dbPrefix)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			fs.Remove(pth)
		}
	}()

	doc.Path = pth

	if err = couchdb.CreateDoc(dbPrefix, doc); err != nil {
		return err
	}

	if err = fs.Mkdir(pth, 0755); err != nil {
		return err
	}

	return nil
}
