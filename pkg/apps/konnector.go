package apps

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

// KonnManifest contains all the informations associated with an installed
// konnector.
type KonnManifest struct {
	DocRev string `json:"_rev,omitempty"` // KonnManifest revision

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

	DocVersion     string          `json:"version"`
	License        string          `json:"license"`
	DocPermissions permissions.Set `json:"permissions"`
}

// ID is part of the Manifest interface
func (m *KonnManifest) ID() string { return m.DocType() + "/" + m.DocSlug }

// Rev is part of the Manifest interface
func (m *KonnManifest) Rev() string { return m.DocRev }

// DocType is part of the Manifest interface
func (m *KonnManifest) DocType() string { return consts.Konnectors }

// Clone is part of the Manifest interface
func (m *KonnManifest) Clone() couchdb.Doc {
	cloned := *m
	if m.Developer != nil {
		dev := *m.Developer
		cloned.Developer = &dev
	}
	return &cloned
}

// SetID is part of the Manifest interface
func (m *KonnManifest) SetID(id string) {}

// SetRev is part of the Manifest interface
func (m *KonnManifest) SetRev(rev string) { m.DocRev = rev }

// Source is part of the Manifest interface
func (m *KonnManifest) Source() string { return m.DocSource }

// Version is part of the Manifest interface
func (m *KonnManifest) Version() string { return m.DocVersion }

// Slug is part of the Manifest interface
func (m *KonnManifest) Slug() string { return m.DocSlug }

// State is part of the Manifest interface
func (m *KonnManifest) State() State { return m.DocState }

// Error is part of the Manifest interface
func (m *KonnManifest) Error() error {
	if m.DocError == "" {
		return nil
	}
	return errors.New(m.DocError)
}

// SetState is part of the Manifest interface
func (m *KonnManifest) SetState(state State) { m.DocState = state }

// SetError is part of the Manifest interface
func (m *KonnManifest) SetError(err error) { m.DocError = err.Error() }

// SetVersion is part of the Manifest interface
func (m *KonnManifest) SetVersion(version string) { m.DocVersion = version }

// Permissions is part of the Manifest interface
func (m *KonnManifest) Permissions() permissions.Set {
	return m.DocPermissions
}

// Valid is part of the Manifest interface
func (m *KonnManifest) Valid(field, value string) bool {
	switch field {
	case "slug":
		return m.DocSlug == value
	case "state":
		return m.DocState == State(value)
	}
	return false
}

// ReadManifest is part of the Manifest interface
func (m *KonnManifest) ReadManifest(r io.Reader, slug, sourceURL string) error {
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

// Create is part of the Manifest interface
func (m *KonnManifest) Create(db couchdb.Database) error {
	if err := couchdb.CreateNamedDocWithDB(db, m); err != nil {
		return err
	}
	_, err := permissions.CreateKonnectorSet(db, m.Slug(), m.Permissions())
	return err
}

// Update is part of the Manifest interface
func (m *KonnManifest) Update(db couchdb.Database) error {
	err := permissions.DestroyKonnector(db, m.Slug())
	if err != nil && !couchdb.IsNotFoundError(err) {
		return err
	}
	err = couchdb.UpdateDoc(db, m)
	if err != nil {
		return err
	}
	_, err = permissions.CreateKonnectorSet(db, m.Slug(), m.Permissions())
	return err
}

// Delete is part of the Manifest interface
func (m *KonnManifest) Delete(db couchdb.Database) error {
	err := permissions.DestroyKonnector(db, m.Slug())
	if err != nil && !couchdb.IsNotFoundError(err) {
		return err
	}
	return couchdb.DeleteDoc(db, m)
}

// GetKonnectorBySlug fetch the manifest of a konnector from the database given
// a slug.
func GetKonnectorBySlug(db couchdb.Database, slug string) (*KonnManifest, error) {
	if slug == "" || !slugReg.MatchString(slug) {
		return nil, ErrInvalidSlugName
	}
	man := &KonnManifest{}
	err := couchdb.GetDoc(db, consts.Konnectors, consts.Konnectors+"/"+slug, man)
	if couchdb.IsNotFoundError(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return man, nil
}

// ListKonnectors returns the list of installed konnectors applications.
//
// TODO: pagination
func ListKonnectors(db couchdb.Database) ([]Manifest, error) {
	var docs []*KonnManifest
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

var _ Manifest = &KonnManifest{}
