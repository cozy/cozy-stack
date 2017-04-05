package apps

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

type konnManifest struct {
	DocRev string `json:"_rev,omitempty"` // konnManifest revision

	Name        string     `json:"name"`
	Type        string     `json:"type,omitempty"`
	DocSource   string     `json:"source"`
	DocSlug     string     `json:"slug"`
	DocState    State      `json:"state"`
	DocError    string     `json:"error,omitempty"`
	Icon        string     `json:"icon"`
	Description string     `json:"description"`
	Developer   *Developer `json:"developer"`

	DefaultLocale string `json:"default_locale"`
	Locales       map[string]struct {
		Description string `json:"description"`
	} `json:"locales"`

	Version        string          `json:"version"`
	License        string          `json:"license"`
	DocPermissions permissions.Set `json:"permissions"`
}

func (m *konnManifest) ID() string        { return m.DocType() + "/" + m.DocSlug }
func (m *konnManifest) Rev() string       { return m.DocRev }
func (m *konnManifest) DocType() string   { return consts.Konnectors }
func (m *konnManifest) SetID(id string)   {}
func (m *konnManifest) SetRev(rev string) { m.DocRev = rev }
func (m *konnManifest) Source() string    { return m.DocSource }
func (m *konnManifest) Slug() string      { return m.DocSlug }

func (m *konnManifest) State() State { return m.DocState }
func (m *konnManifest) Error() error {
	if m.DocError == "" {
		return nil
	}
	return errors.New(m.DocError)
}

func (m *konnManifest) SetState(state State) { m.DocState = state }
func (m *konnManifest) SetError(err error)   { m.DocError = err.Error() }
func (m *konnManifest) Permissions() permissions.Set {
	return m.DocPermissions
}

func (m *konnManifest) Valid(field, value string) bool {
	switch field {
	case "slug":
		return m.DocSlug == value
	case "state":
		return m.DocState == State(value)
	}
	return false
}

func (m *konnManifest) ReadManifest(r io.Reader, slug, sourceURL string) error {
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return ErrBadManifest
	}
	if m.Type != "node" {
		return ErrBadManifest
	}
	m.DocSlug = slug
	m.DocSource = sourceURL
	return nil
}

// GetKonnectorBySlug fetch the manifest of a konnector from the database given
// a slug.
func GetKonnectorBySlug(db couchdb.Database, slug string) (Manifest, error) {
	man := &konnManifest{}
	err := couchdb.GetDoc(db, consts.Konnectors, consts.Konnectors+"/"+slug, man)
	if err != nil {
		return nil, err
	}
	return man, nil
}

// ListKonnectors returns the list of installed konnectors applications.
//
// TODO: pagination
func ListKonnectors(db couchdb.Database) ([]Manifest, error) {
	var docs []*konnManifest
	req := &couchdb.AllDocsRequest{Limit: 100}
	err := couchdb.GetAllDocs(db, consts.Konnectors, req, &docs)
	if err != nil {
		return nil, err
	}
	mans := make([]Manifest, len(docs))
	for i, m := range docs {
		mans[i] = m
	}
	return mans, nil
}

var _ Manifest = &konnManifest{}
