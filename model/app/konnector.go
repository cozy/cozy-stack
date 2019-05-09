package app

import (
	"encoding/json"
	"io"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// KonnManifest contains all the informations associated with an installed
// konnector.
type KonnManifest struct {
	DocID  string `json:"_id,omitempty"`
	DocRev string `json:"_rev,omitempty"`

	Name       string `json:"name"`
	NamePrefix string `json:"name_prefix,omitempty"`
	Editor     string `json:"editor"`
	Icon       string `json:"icon"`

	Type        string           `json:"type,omitempty"`
	License     string           `json:"license,omitempty"`
	Language    string           `json:"language,omitempty"`
	VendorLink  string           `json:"vendor_link"`
	Locales     *json.RawMessage `json:"locales,omitempty"`
	Langs       *json.RawMessage `json:"langs,omitempty"`
	Platforms   *json.RawMessage `json:"platforms,omitempty"`
	Categories  *json.RawMessage `json:"categories,omitempty"`
	Developer   *json.RawMessage `json:"developer,omitempty"`
	Screenshots *json.RawMessage `json:"screenshots,omitempty"`
	Tags        *json.RawMessage `json:"tags,omitempty"`
	Partnership *json.RawMessage `json:"partnership,omitempty"`

	Frequency    string           `json:"frequency,omitempty"`
	DataTypes    *json.RawMessage `json:"data_types,omitempty"`
	Doctypes     *json.RawMessage `json:"doctypes,omitempty"`
	Fields       *json.RawMessage `json:"fields,omitempty"`
	Folders      *json.RawMessage `json:"folders,omitempty"`
	Messages     *json.RawMessage `json:"messages,omitempty"`
	OAuth        *json.RawMessage `json:"oauth,omitempty"`
	TimeInterval *json.RawMessage `json:"time_interval,omitempty"`

	Aggregator *json.RawMessage `json:"aggregator,omitempty"`

	Parameters    *json.RawMessage `json:"parameters,omitempty"`
	Notifications Notifications    `json:"notifications"`

	// OnDeleteAccount can be used to specify a file path which will be executed
	// when an account associated with the konnector is deleted.
	OnDeleteAccount string `json:"on_delete_account,omitempty"`

	DocSlug             string         `json:"slug"`
	DocState            State          `json:"state"`
	DocSource           string         `json:"source"`
	DocVersion          string         `json:"version"`
	DocChecksum         string         `json:"checksum"`
	DocPermissions      permission.Set `json:"permissions"`
	DocAvailableVersion string         `json:"available_version,omitempty"`
	DocTerms            Terms          `json:"terms,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Err string `json:"error,omitempty"`
	err error
}

// ID is part of the Manifest interface
func (m *KonnManifest) ID() string { return m.DocID }

// Rev is part of the Manifest interface
func (m *KonnManifest) Rev() string { return m.DocRev }

// DocType is part of the Manifest interface
func (m *KonnManifest) DocType() string { return consts.Konnectors }

// Clone is part of the Manifest interface
func (m *KonnManifest) Clone() couchdb.Doc {
	cloned := *m

	cloned.DocPermissions = make(permission.Set, len(m.DocPermissions))
	copy(cloned.DocPermissions, m.DocPermissions)

	cloned.Locales = cloneRawMessage(m.Locales)
	cloned.Langs = cloneRawMessage(m.Langs)
	cloned.Platforms = cloneRawMessage(m.Platforms)
	cloned.Categories = cloneRawMessage(m.Categories)
	cloned.Developer = cloneRawMessage(m.Developer)
	cloned.Screenshots = cloneRawMessage(m.Screenshots)
	cloned.Tags = cloneRawMessage(m.Tags)
	cloned.Partnership = cloneRawMessage(m.Partnership)
	cloned.Parameters = cloneRawMessage(m.Parameters)

	cloned.DataTypes = cloneRawMessage(m.DataTypes)
	cloned.Doctypes = cloneRawMessage(m.Doctypes)
	cloned.Fields = cloneRawMessage(m.Fields)
	cloned.Folders = cloneRawMessage(m.Folders)
	cloned.Messages = cloneRawMessage(m.Messages)
	cloned.OAuth = cloneRawMessage(m.OAuth)
	cloned.TimeInterval = cloneRawMessage(m.TimeInterval)

	cloned.Aggregator = cloneRawMessage(m.Aggregator)

	cloned.Notifications = make(Notifications, len(m.Notifications))
	for k, v := range m.Notifications {
		props := (&v).Clone()
		cloned.Notifications[k] = *props
	}
	return &cloned
}

// SetID is part of the Manifest interface
func (m *KonnManifest) SetID(id string) { m.DocID = id }

// SetRev is part of the Manifest interface
func (m *KonnManifest) SetRev(rev string) { m.DocRev = rev }

// SetSource is part of the Manifest interface
func (m *KonnManifest) SetSource(src *url.URL) { m.DocSource = src.String() }

// Source is part of the Manifest interface
func (m *KonnManifest) Source() string { return m.DocSource }

// Version is part of the Manifest interface
func (m *KonnManifest) Version() string { return m.DocVersion }

// AvailableVersion is part of the Manifest interface
func (m *KonnManifest) AvailableVersion() string { return m.DocAvailableVersion }

// Checksum is part of the Manifest interface
func (m *KonnManifest) Checksum() string { return m.DocChecksum }

// Slug is part of the Manifest interface
func (m *KonnManifest) Slug() string { return m.DocSlug }

// State is part of the Manifest interface
func (m *KonnManifest) State() State { return m.DocState }

// LastUpdate is part of the Manifest interface
func (m *KonnManifest) LastUpdate() time.Time { return m.UpdatedAt }

// SetState is part of the Manifest interface
func (m *KonnManifest) SetState(state State) { m.DocState = state }

// SetVersion is part of the Manifest interface
func (m *KonnManifest) SetVersion(version string) { m.DocVersion = version }

// SetAvailableVersion is part of the Manifest interface
func (m *KonnManifest) SetAvailableVersion(version string) { m.DocAvailableVersion = version }

// SetChecksum is part of the Manifest interface
func (m *KonnManifest) SetChecksum(shasum string) { m.DocChecksum = shasum }

// AppType is part of the Manifest interface
func (m *KonnManifest) AppType() consts.AppType { return consts.KonnectorType }

// Terms is part of the Manifest interface
func (m *KonnManifest) Terms() Terms { return m.DocTerms }

// Permissions is part of the Manifest interface
func (m *KonnManifest) Permissions() permission.Set {
	return m.DocPermissions
}

// SetError is part of the Manifest interface
func (m *KonnManifest) SetError(err error) {
	m.SetState(Errored)
	m.Err = err.Error()
	m.err = err
}

// Error is part of the Manifest interface
func (m *KonnManifest) Error() error { return m.err }

// Match is part of the Manifest interface
func (m *KonnManifest) Match(field, value string) bool {
	switch field {
	case "slug":
		return m.DocSlug == value
	case "state":
		return m.DocState == State(value)
	}
	return false
}

// ReadManifest is part of the Manifest interface
func (m *KonnManifest) ReadManifest(r io.Reader, slug, sourceURL string) (Manifest, error) {
	var newManifest KonnManifest
	if err := json.NewDecoder(r).Decode(&newManifest); err != nil {
		return nil, ErrBadManifest
	}

	newManifest.SetID(consts.Konnectors + "/" + slug)
	newManifest.SetRev(m.Rev())
	newManifest.SetState(m.State())
	newManifest.CreatedAt = m.CreatedAt
	newManifest.DocSlug = slug
	newManifest.DocSource = sourceURL
	if newManifest.Parameters == nil {
		newManifest.Parameters = m.Parameters
	}

	return &newManifest, nil
}

// Create is part of the Manifest interface
func (m *KonnManifest) Create(db prefixer.Prefixer) error {
	m.DocID = consts.Konnectors + "/" + m.DocSlug
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	if err := couchdb.CreateNamedDocWithDB(db, m); err != nil {
		return err
	}
	_, err := permission.CreateKonnectorSet(db, m.Slug(), m.Permissions())
	return err
}

// Update is part of the Manifest interface
func (m *KonnManifest) Update(db prefixer.Prefixer, extraPerms permission.Set) error {
	m.UpdatedAt = time.Now()
	err := couchdb.UpdateDoc(db, m)
	if err != nil {
		return err
	}

	perms := m.Permissions()

	// Merging the potential extra permissions
	if len(extraPerms) > 0 {
		perms, err = permission.MergeExtraPermissions(perms, extraPerms)
		if err != nil {
			return err
		}
	}
	_, err = permission.UpdateKonnectorSet(db, m.Slug(), perms)
	return err
}

// Delete is part of the Manifest interface
func (m *KonnManifest) Delete(db prefixer.Prefixer) error {
	err := permission.DestroyKonnector(db, m.Slug())
	if err != nil && !couchdb.IsNotFoundError(err) {
		return err
	}
	return couchdb.DeleteDoc(db, m)
}

// GetKonnectorBySlug fetch the manifest of a konnector from the database given
// a slug.
func GetKonnectorBySlug(db prefixer.Prefixer, slug string) (*KonnManifest, error) {
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

// GetKonnectorBySlugAndUpdate fetch the KonnManifest and perform an update of
// the konnector if necessary and if the konnector was installed from the
// registry.
func GetKonnectorBySlugAndUpdate(db prefixer.Prefixer, slug string, copier appfs.Copier, registries []*url.URL) (*KonnManifest, error) {
	man, err := GetKonnectorBySlug(db, slug)
	if err != nil {
		return nil, err
	}
	return DoLazyUpdate(db, man, copier, registries).(*KonnManifest), nil
}

// ListKonnectors returns the list of installed konnectors applications.
//
// TODO: pagination
func ListKonnectors(db prefixer.Prefixer) ([]Manifest, error) {
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
