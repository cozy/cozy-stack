package apps

import (
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

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

// Intent is a declaration of a service for other client-side apps
type Intent struct {
	Action string   `json:"action"`
	Types  []string `json:"type"`
	Href   string   `json:"href"`
}

// WebappManifest contains all the informations associated with an installed web
// application.
type WebappManifest struct {
	DocRev string `json:"_rev,omitempty"` // WebappManifest revision

	Type string `json:"type,omitempty"`

	Name        string     `json:"name"`
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
	Intents        []Intent        `json:"intents"`
	Routes         Routes          `json:"routes"`

	Instance SubDomainer `json:"-"` // Used for JSON-API links
}

// ID is part of the Manifest interface
func (m *WebappManifest) ID() string { return m.DocType() + "/" + m.DocSlug }

// Rev is part of the Manifest interface
func (m *WebappManifest) Rev() string { return m.DocRev }

// DocType is part of the Manifest interface
func (m *WebappManifest) DocType() string { return consts.Apps }

// SetID is part of the Manifest interface
func (m *WebappManifest) SetID(id string) {}

// SetRev is part of the Manifest interface
func (m *WebappManifest) SetRev(rev string) { m.DocRev = rev }

// Source is part of the Manifest interface
func (m *WebappManifest) Source() string { return m.DocSource }

// Slug is part of the Manifest interface
func (m *WebappManifest) Slug() string { return m.DocSlug }

// State is part of the Manifest interface
func (m *WebappManifest) State() State { return m.DocState }

// Error is part of the Manifest interface
func (m *WebappManifest) Error() error {
	if m.DocError == "" {
		return nil
	}
	return errors.New(m.DocError)
}

// SetState is part of the Manifest interface
func (m *WebappManifest) SetState(state State) { m.DocState = state }

// SetError is part of the Manifest interface
func (m *WebappManifest) SetError(err error) { m.DocError = err.Error() }

// Permissions is part of the Manifest interface
func (m *WebappManifest) Permissions() permissions.Set {
	return m.DocPermissions
}

// Valid is part of the Manifest interface
func (m *WebappManifest) Valid(field, value string) bool {
	switch field {
	case "slug":
		return m.DocSlug == value
	case "state":
		return m.DocState == State(value)
	}
	return false
}

// ReadManifest  is part of the Manifest interface
func (m *WebappManifest) ReadManifest(r io.Reader, slug, sourceURL string) error {
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return ErrBadManifest
	}

	m.DocSlug = slug
	m.DocSource = sourceURL

	if m.Routes == nil {
		m.Routes = make(Routes)
		m.Routes["/"] = Route{
			Folder: "/",
			Index:  "index.html",
			Public: false,
		}
	}
	return nil
}

// FindRoute takes a path, returns the route which matches the best,
// and the part that remains unmatched
func (m *WebappManifest) FindRoute(vpath string) (Route, string) {
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

// FindIntent returns an intent for the given action and type if the manifest has one
func (m *WebappManifest) FindIntent(action, typ string) *Intent {
	for _, intent := range m.Intents {
		if strings.ToUpper(action) != strings.ToUpper(intent.Action) {
			continue
		}
		for _, t := range intent.Types {
			if t == typ {
				return &intent
			}
			// Allow a joker for mime-types like image/*
			if strings.HasSuffix(t, "/*") {
				if strings.SplitN(t, "/", 2)[0] == strings.SplitN(typ, "/", 2)[0] {
					return &intent
				}
			}
		}
	}
	return nil
}

// GetWebappBySlug fetch the WebappManifest from the database given a slug.
func GetWebappBySlug(db couchdb.Database, slug string) (*WebappManifest, error) {
	man := &WebappManifest{}
	err := couchdb.GetDoc(db, consts.Apps, consts.Apps+"/"+slug, man)
	if err != nil {
		return nil, err
	}
	return man, nil
}

// ListWebapps returns the list of installed web applications.
//
// TODO: pagination
func ListWebapps(db couchdb.Database) ([]*WebappManifest, error) {
	var docs []*WebappManifest
	req := &couchdb.AllDocsRequest{Limit: 100}
	err := couchdb.GetAllDocs(db, consts.Apps, req, &docs)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

var _ Manifest = &WebappManifest{}
