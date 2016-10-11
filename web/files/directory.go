package files

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/spf13/afero"
)

type dirAttributes struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Tags      []string  `json:"tags"`
}

type dirDoc struct {
	QID      string         `json:"_id"`
	DRev     string         `json:"_rev,omitempty"`
	Attrs    *dirAttributes `json:"attributes"`
	FolderID string         `json:"folderID"`
	Path     string         `json:"path"`
}

func (d *dirDoc) ID() string {
	return d.QID
}

func (d *dirDoc) Rev() string {
	return d.DRev
}

func (d *dirDoc) DocType() string {
	return string(FolderDocType)
}

func (d *dirDoc) SetID(id string) {
	d.QID = id
}

func (d *dirDoc) SetRev(rev string) {
	d.DRev = rev
}

// implement temporary interface JSONApier
func (d *dirDoc) ToJSONApi() ([]byte, error) {
	qid := d.QID
	dat := map[string]interface{}{
		"type":       d.DocType(),
		"id":         qid[strings.Index(qid, "/")+1:],
		"rev":        d.Rev(),
		"attributes": d.Attrs,
	}
	m := map[string]interface{}{
		"data": dat,
	}
	return json.Marshal(m)
}

// CreateDirectory is the method for creating a new directory
func CreateDirectory(m *DocMetadata, fs afero.Fs, dbPrefix string) (jsonapier jsonapi.JSONApier, err error) {
	if m.Type != FolderDocType {
		err = errDocTypeInvalid
		return
	}

	pth, _, err := createNewFilePath(m, fs, dbPrefix)
	if err != nil {
		return
	}

	createDate := time.Now()
	attrs := &dirAttributes{
		Name:      m.Name,
		CreatedAt: createDate,
		UpdatedAt: createDate,
		Tags:      m.Tags,
	}

	doc := &dirDoc{
		Attrs:    attrs,
		FolderID: m.FolderID,
		Path:     pth,
	}

	// @TODO: make this "atomic"
	if err = couchdb.CreateDoc(dbPrefix, doc.DocType(), doc); err != nil {
		return
	}

	if err = fs.Mkdir(pth, 0777); err != nil {
		return
	}

	jsonapier = jsonapi.JSONApier(doc)
	return
}
