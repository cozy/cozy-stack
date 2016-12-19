package apps

import (
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

const (
	// ManifestMaxSize is the manifest maximum size
	ManifestMaxSize = 2 << (2 * 10) // 2MB
	// ManifestFilename is the name of the manifest at the root of the
	// application directory
	ManifestFilename = "manifest.webapp"
)

// State is the state of the application
type State string

const (
	// Available state
	Available State = "available"
	// Installing state
	Installing = "installing"
	// Upgrading state
	Upgrading = "upgrading"
	// Uninstalling state
	Uninstalling = "uninstalling"
	// Errored state
	Errored = "errored"
	// Ready state
	Ready = "ready"
)

// Some well known slugs
const (
	OnboardingSlug = "onboarding"
	HomeSlug       = "home"
)

// Access is a string representing the access permission level. It can
// either be read, write or readwrite.
type Access string

// Permissions is a map of key, a description and an access level.
type Permissions map[string]struct {
	Description string `json:"description"`
	Access      Access `json:"access"`
}

// Context is a struct to serve a folder inside an app
type Context struct {
	Folder string `json:"folder"`
	Index  string `json:"index"`
	Public bool   `json:"public"`
}

// Contexts are a map for routing inside an application.
type Contexts map[string]Context

// Developer is the name and url of a developer.
type Developer struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// Manifest contains all the informations about an application.
type Manifest struct {
	ManRev string `json:"_rev,omitempty"` // Manifest revision

	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Source      string     `json:"source"`
	State       State      `json:"state"`
	Error       string     `json:"error,omitempty"`
	Icon        string     `json:"icon"`
	Description string     `json:"description"`
	Developer   *Developer `json:"developer"`

	DefaultLocal string `json:"default_locale"`
	Locales      map[string]struct {
		Description string `json:"description"`
	} `json:"locales"`

	Version     string       `json:"version"`
	License     string       `json:"license"`
	Permissions *Permissions `json:"permissions"`
	Contexts    Contexts     `json:"contexts"`
}

// ID returns the manifest identifier - see couchdb.Doc interface
func (m *Manifest) ID() string {
	return consts.Manifests + "/" + m.Slug
}

// Rev return the manifest revision - see couchdb.Doc interface
func (m *Manifest) Rev() string { return m.ManRev }

// DocType returns the manifest doctype - see couchdb.Doc interfaces
func (m *Manifest) DocType() string { return consts.Manifests }

// SetID is used to change the file identifier - see couchdb.Doc
// interface
func (m *Manifest) SetID(id string) {}

// SetRev is used to change the file revision - see couchdb.Doc
// interface
func (m *Manifest) SetRev(rev string) { m.ManRev = rev }

// SelfLink is used to generate a JSON-API link for the file - see
// jsonapi.Object interface
func (m *Manifest) SelfLink() string { return "/apps/" + m.Slug }

// Relationships is used to generate the parent relationship in JSON-API format
// - see jsonapi.Object interface
func (m *Manifest) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}

// Included is part of the jsonapi.Object interface
func (m *Manifest) Included() []jsonapi.Object {
	return []jsonapi.Object{}
}

// List returns the list of installed applications.
//
// TODO: pagination
func List(db couchdb.Database) ([]*Manifest, error) {
	var docs []*Manifest
	req := &couchdb.AllDocsRequest{Limit: 100}
	err := couchdb.GetAllDocs(db, consts.Manifests, req, &docs)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

// GetBySlug returns an app identified by its slug
func GetBySlug(db couchdb.Database, slug string) (*Manifest, error) {
	man := &Manifest{}
	err := couchdb.GetDoc(db, consts.Manifests, consts.Manifests+"/"+slug, man)
	if err != nil {
		return nil, err
	}
	return man, nil
}

// FindContext takes a path, returns the context which matches the best,
// and says if it is an exact match (in which case the index file should be served)
func (m *Manifest) FindContext(path string) (Context, bool) {
	parts := strings.Split(path, "/")
	lenParts := len(parts)

	var best Context
	specificity := 0
	for key, ctx := range m.Contexts {
		keys := strings.Split(key, "/")
		count := len(keys)
		if count > lenParts || count < specificity {
			continue
		}
		if contextMatches(parts, keys) {
			specificity = count
			best = ctx
		}
	}

	if parts[len(parts)-1] == "" {
		specificity++
	}

	return best, specificity == len(parts)
}

func contextMatches(path, ctx []string) bool {
	for i, part := range ctx {
		if path[i] != part {
			return false
		}
	}
	return true
}
