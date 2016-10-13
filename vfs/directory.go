package vfs

import (
	"encoding/json"
	"time"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/spf13/afero"
)

// DirAttributes is a struct with the attributes of a directory
type DirAttributes struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Tags      []string  `json:"tags"`
}

// DirDoc is a struct containing all the informations about a
// directory. It implements the couchdb.Doc and jsonapi.JSONApier
// interfaces.
type DirDoc struct {
	// Qualified file identifier
	DID string `json:"_id,omitempty"`
	// Directory revision
	DRev string `json:"_rev,omitempty"`
	// Directory attributes
	Attrs *DirAttributes `json:"attributes"`
	// Parent folder identifier
	FolderID string `json:"folderID"`
	// Directory path on VFS
	Path string `json:"path"`
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
	data := map[string]interface{}{
		"type":       d.DocType(),
		"id":         d.ID(),
		"rev":        d.Rev(),
		"attributes": d.Attrs,
	}
	m := map[string]interface{}{
		"data": data,
	}
	return json.Marshal(m)
}

// CreateDirectory is the method for creating a new directory
func CreateDirectory(m *DocAttributes, fs afero.Fs, dbPrefix string) (doc *DirDoc, err error) {
	if m.docType != FolderDocType {
		err = ErrDocTypeInvalid
		return
	}

	pth, _, err := createNewFilePath(m, fs, dbPrefix)
	if err != nil {
		return
	}

	createDate := time.Now()
	attrs := &DirAttributes{
		Name:      m.name,
		CreatedAt: createDate,
		UpdatedAt: createDate,
		Tags:      m.tags,
	}

	doc = &DirDoc{
		Attrs:    attrs,
		FolderID: m.folderID,
		Path:     pth,
	}

	defer func() {
		if err != nil {
			fs.Remove(pth)
		}
	}()

	if err = couchdb.CreateDoc(dbPrefix, doc); err != nil {
		return
	}

	if err = fs.Mkdir(pth, 0755); err != nil {
		return
	}

	return
}
