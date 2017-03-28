package apps

import (
	"net/url"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

const (
	// ManifestMaxSize is the manifest maximum size
	ManifestMaxSize = 2 << (2 * 10) // 2MB
	// WebappManifest is the name of the manifest at the root of the
	// client-side application directory
	WebappManifest = "manifest.webapp"
	// KonnectorManifest is the name of the manifest at the root of the
	// konnector application directory
	KonnectorManifest = "manifest.konnectors"
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

// Access is a string representing the access permission level. It can
// either be read, write or readwrite.
type Access string

// Route is a struct to serve a folder inside an app
type Route struct {
	Folder string `json:"folder"`
	Index  string `json:"index"`
	Public bool   `json:"public"`
}

// NotFound returns true for a blank route (ie not found by FindRoute)
func (c *Route) NotFound() bool { return c.Folder == "" }

// Routes are a map for routing inside an application.
type Routes map[string]Route

// Developer is the name and url of a developer.
type Developer struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// SubDomainer is an interface with a single method to build an URL from a slug
type SubDomainer interface {
	SubDomain(s string) *url.URL
}

// Manifest contains all the informations about an application.
type Manifest struct {
	ManRev string `json:"_rev,omitempty"` // Manifest revision

	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Type        string     `json:"type,omitempty"`
	Source      string     `json:"source"`
	State       State      `json:"state"`
	Error       string     `json:"error,omitempty"`
	Icon        string     `json:"icon"`
	Description string     `json:"description"`
	Developer   *Developer `json:"developer"`

	DefaultLocale string `json:"default_locale"`
	Locales       map[string]struct {
		Description string `json:"description"`
	} `json:"locales"`

	Version     string           `json:"version"`
	License     string           `json:"license"`
	Permissions *permissions.Set `json:"permissions"`
	Routes      Routes           `json:"routes"`

	Instance SubDomainer `json:"-"` // Used for JSON-API links
}

// ID returns the manifest identifier - see couchdb.Doc interface
func (m *Manifest) ID() string {
	return consts.Apps + "/" + m.Slug
}

// Rev return the manifest revision - see couchdb.Doc interface
func (m *Manifest) Rev() string { return m.ManRev }

// DocType returns the manifest doctype - see couchdb.Doc interfaces
func (m *Manifest) DocType() string { return consts.Apps }

// SetID is used to change the file identifier - see couchdb.Doc
// interface
func (m *Manifest) SetID(id string) {}

// SetRev is used to change the file revision - see couchdb.Doc
// interface
func (m *Manifest) SetRev(rev string) { m.ManRev = rev }

// Links is used to generate a JSON-API link for the file - see
// jsonapi.Object interface
func (m *Manifest) Links() *jsonapi.LinksList {
	links := jsonapi.LinksList{
		Self: "/apps/" + m.Slug,
	}
	if m.Icon != "" {
		links.Icon = "/apps/" + m.Slug + "/icon"
	}
	if m.State == Ready && m.Instance != nil {
		links.Related = m.Instance.SubDomain(m.Slug).String()
	}
	return &links
}

// Relationships is used to generate the parent relationship in JSON-API format
// - see jsonapi.Object interface
func (m *Manifest) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}

// Included is part of the jsonapi.Object interface
func (m *Manifest) Included() []jsonapi.Object {
	return []jsonapi.Object{}
}

// Valid implements permissions.Validable on Manifest
func (m *Manifest) Valid(field, value string) bool {
	switch field {
	case "slug":
		return m.Slug == value
	case "state":
		return m.State == State(value)
	}
	return false
}

// List returns the list of installed applications.
//
// TODO: pagination
func List(db couchdb.Database) ([]*Manifest, error) {
	var docs []*Manifest
	req := &couchdb.AllDocsRequest{Limit: 100}
	err := couchdb.GetAllDocs(db, consts.Apps, req, &docs)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

// GetBySlug returns an app identified by its slug
func GetBySlug(db couchdb.Database, slug string) (*Manifest, error) {
	man := &Manifest{}
	err := couchdb.GetDoc(db, consts.Apps, consts.Apps+"/"+slug, man)
	if err != nil {
		return nil, err
	}
	return man, nil
}

// CreateDefaultRoute creates a default route if the manifest has no routes
func (m *Manifest) CreateDefaultRoute() {
	if m.Routes == nil {
		m.Routes = make(Routes)
		m.Routes["/"] = Route{
			Folder: "/",
			Index:  "index.html",
			Public: false,
		}
	}
}

// FindRoute takes a path, returns the route which matches the best,
// and the part that remains unmatched
func (m *Manifest) FindRoute(vpath string) (Route, string) {
	parts := strings.Split(vpath, "/")
	lenParts := len(parts)

	var best Route
	rest := ""
	specificity := 0
	for key, ctx := range m.Routes {
		var keys []string
		if key == "/" {
			keys = []string{""}
		} else {
			keys = strings.Split(key, "/")
		}
		count := len(keys)
		if count > lenParts || count < specificity {
			continue
		}
		if routeMatches(parts, keys) {
			specificity = count
			best = ctx
			rest = path.Join(parts[count:]...)
		}
	}

	return best, rest
}

func routeMatches(path, ctx []string) bool {
	for i, part := range ctx {
		if path[i] != part {
			return false
		}
	}
	return true
}

var (
	_ jsonapi.Object        = &Manifest{}
	_ permissions.Validable = &Manifest{}
)
