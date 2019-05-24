package dispers

import (
	"errors"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Only the Conductor should save metadata
var prefix = prefixer.ConductorPrefixer

// Metadata are written on the confuctor's database. The querier can read those Metadata to know his training's state
type Metadata struct {
	Start       time.Time
	Time        string   `json:"date,omitempty"`
	Host        string   `json:"host,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Outcome     bool     `json:"outcome,omitempty"`
	Error       string   `json:"error,omitempty"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Output      string   `json:"output,omitempty"`
	Duration    string   `json:"duration,omitempty"`
}

// NewMetadata returns a new Metadata object
func NewMetadata(host string, name string, description string, tags []string) Metadata {
	now := time.Now()
	return Metadata{
		Time:        now.String(),
		Start:       now,
		Description: description,
		Name:        name,
		Tags:        tags,
	}
}

// Close finish writting Metadata
func (m *Metadata) Close(msg string, err error) error {
	now := time.Now()
	m.Duration = (now.Sub(m.Start)).String()
	m.Outcome = (err == nil)
	m.Output = msg
	if err != nil {
		m.Error = err.Error()
	}
	return nil
}

type MetadataDoc struct {
	MetaID   string     `json:"_id,omitempty"`
	MetaRev  string     `json:"_rev,omitempty"`
	TrainID  string     `json:"training,omitempty"`
	Metadata []Metadata `json:"metadata,omitempty"`
}

func (t *MetadataDoc) ID() string {
	return t.MetaID
}

func (t *MetadataDoc) Rev() string {
	return t.MetaRev
}

func (t *MetadataDoc) DocType() string {
	return "io.cozy.metadata"
}

func (t *MetadataDoc) Clone() couchdb.Doc {
	cloned := *t
	return &cloned
}

func (t *MetadataDoc) SetID(id string) {
	t.MetaID = id
}

func (t *MetadataDoc) SetRev(rev string) {
	t.MetaRev = rev
}

// RetrieveMetadata get Metadata from a MetadataDoc in CouchDB
func RetrieveMetadata(trainid string) ([]MetadataDoc, error) {

	couchdb.EnsureDBExist(prefix, "io.cozy.metadata")

	// Check if doc existing
	err := couchdb.DefineIndex(prefix, mango.IndexOnFields("io.cozy.metadata", "metadata-index", []string{"training"}))
	if err != nil {
		return []MetadataDoc{}, err
	}
	var out []MetadataDoc
	req := &couchdb.FindRequest{Selector: mango.Equal("training", trainid)}
	err = couchdb.FindDocs(prefix, "io.cozy.metadata", req, &out)
	if err != nil {
		return []MetadataDoc{}, err
	}
	return out, nil
}

// Push adds metadata to the MetadataDoc in Conductor's database
func (m *Metadata) Push(trainid string) error {

	out, err := RetrieveMetadata(trainid)
	if err != nil {
		return err
	}

	if len(out) == 1 {
		// Retrieve doc
		doc := out[0]
		// Update doc
		newArrayMetadata := make([]Metadata, len(doc.Metadata)+1)
		for index, item := range doc.Metadata {
			newArrayMetadata[index] = item
		}
		newArrayMetadata[len(doc.Metadata)] = *m
		doc.Metadata = newArrayMetadata
		// Upload doc
		err := couchdb.UpdateDoc(prefix, &doc)
		if err != nil {
			return err
		}
	} else if len(out) == 0 {
		// Create doc
		doc := &MetadataDoc{
			MetaID:   "",
			MetaRev:  "",
			TrainID:  trainid,
			Metadata: []Metadata{*m},
		}
		err := couchdb.CreateDoc(prefix, doc)
		if err != nil {
			return err
		}
	} else {
		return errors.New("Metadata : several docs for this training")
	}

	return nil
}
