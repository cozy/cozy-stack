package apps

import (
	"encoding/json"
	"io"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/stack"
)

// Route is a struct to serve a folder inside an app
type Route struct {
	Folder string `json:"folder"`
	Index  string `json:"index"`
	Public bool   `json:"public"`
}

// NotFound returns true for a blank route (ie not found by FindRoute)
func (c *Route) NotFound() bool { return c.Folder == "" }

// Routes is a map for routing inside an application.
type Routes map[string]Route

// Service is a struct to define a service executed by the stack.
type Service struct {
	Type           string `json:"type"`
	File           string `json:"file"`
	TriggerOptions string `json:"trigger"`
	TriggerID      string `json:"trigger_id"`
}

// Services is a map to define services assciated with an application.
type Services map[string]*Service

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
	Editor      string     `json:"editor"`
	DocSource   string     `json:"source"`
	DocSlug     string     `json:"slug"`
	DocState    State      `json:"state"`
	DocError    string     `json:"error,omitempty"`
	Icon        string     `json:"icon"`
	Category    string     `json:"category"`
	Description string     `json:"description"`
	Developer   *Developer `json:"developer"`

	DefaultLocale string `json:"default_locale"`
	Locales       map[string]struct {
		Description string `json:"description"`
	} `json:"locales"`

	DocVersion     string          `json:"version"`
	License        string          `json:"license"`
	DocPermissions permissions.Set `json:"permissions"`
	Intents        []Intent        `json:"intents"`
	Routes         Routes          `json:"routes"`
	Services       Services        `json:"services"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`

	Instance SubDomainer `json:"-"` // Used for JSON-API links

	oldServices Services // Used to diff against when updating the app
}

// ID is part of the Manifest interface
func (m *WebappManifest) ID() string { return m.DocType() + "/" + m.DocSlug }

// Rev is part of the Manifest interface
func (m *WebappManifest) Rev() string { return m.DocRev }

// DocType is part of the Manifest interface
func (m *WebappManifest) DocType() string { return consts.Apps }

// Clone implements couchdb.Doc
func (m *WebappManifest) Clone() couchdb.Doc {
	cloned := *m
	if m.Developer != nil {
		dev := *m.Developer
		cloned.Developer = &dev
	}
	cloned.Intents = make([]Intent, len(m.Intents))
	copy(cloned.Intents, m.Intents)
	return &cloned
}

// SetID is part of the Manifest interface
func (m *WebappManifest) SetID(id string) {}

// SetRev is part of the Manifest interface
func (m *WebappManifest) SetRev(rev string) { m.DocRev = rev }

// Source is part of the Manifest interface
func (m *WebappManifest) Source() string { return m.DocSource }

// Version is part of the Manifest interface
func (m *WebappManifest) Version() string { return m.DocVersion }

// Slug is part of the Manifest interface
func (m *WebappManifest) Slug() string { return m.DocSlug }

// State is part of the Manifest interface
func (m *WebappManifest) State() State { return m.DocState }

// LastUpdate is part of the Manifest interface
func (m *WebappManifest) LastUpdate() time.Time { return m.UpdatedAt }

// SetState is part of the Manifest interface
func (m *WebappManifest) SetState(state State) { m.DocState = state }

// SetVersion is part of the Manifest interface
func (m *WebappManifest) SetVersion(version string) { m.DocVersion = version }

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

// ReadManifest is part of the Manifest interface
func (m *WebappManifest) ReadManifest(r io.Reader, slug, sourceURL string) error {
	var newManifest WebappManifest
	if err := json.NewDecoder(r).Decode(&newManifest); err != nil {
		return ErrBadManifest
	}

	newManifest.SetID(m.ID())
	newManifest.SetRev(m.Rev())
	newManifest.SetState(m.State())
	newManifest.CreatedAt = m.CreatedAt
	newManifest.Instance = m.Instance
	newManifest.DocSlug = slug
	newManifest.DocSource = sourceURL
	newManifest.oldServices = m.Services
	if newManifest.Routes == nil {
		newManifest.Routes = make(Routes)
		newManifest.Routes["/"] = Route{
			Folder: "/",
			Index:  "index.html",
			Public: false,
		}
	}

	*m = newManifest
	return nil
}

// Create is part of the Manifest interface
func (m *WebappManifest) Create(db couchdb.Database) error {
	var err error
	m.Services, err = diffServices(db, m.Slug(), nil, m.Services)
	if err != nil {
		return err
	}
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	if err := couchdb.CreateNamedDocWithDB(db, m); err != nil {
		return err
	}
	_, err = permissions.CreateWebappSet(db, m.Slug(), m.Permissions())
	return err
}

// Update is part of the Manifest interface
func (m *WebappManifest) Update(db couchdb.Database) error {
	var err error
	m.Services, err = diffServices(db, m.Slug(), m.oldServices, m.Services)
	if err != nil {
		return err
	}
	m.UpdatedAt = time.Now()
	err = couchdb.UpdateDoc(db, m)
	if err != nil {
		return err
	}
	_, err = permissions.UpdateWebappSet(db, m.Slug(), m.Permissions())
	return err
}

// Delete is part of the Manifest interface
func (m *WebappManifest) Delete(db couchdb.Database) error {
	_, err := diffServices(db, m.Slug(), m.Services, nil)
	if err != nil {
		return err
	}
	err = permissions.DestroyWebapp(db, m.Slug())
	if err != nil && !couchdb.IsNotFoundError(err) {
		return err
	}
	return couchdb.DeleteDoc(db, m)
}

func diffServices(db couchdb.Database, slug string, oldServices, newServices Services) (Services, error) {
	domain := db.Prefix()

	if oldServices == nil {
		oldServices = make(Services)
	}
	if newServices == nil {
		newServices = make(Services)
	}

	var deleted []*Service
	var created []*Service

	var clone = make(Services)
	for newName, newService := range newServices {
		clone[newName] = newService
	}

	for name, oldService := range oldServices {
		newService, ok := newServices[name]
		if !ok {
			deleted = append(deleted, oldService)
			continue
		}
		delete(clone, name)
		if newService.File != oldService.File ||
			newService.Type != oldService.Type ||
			newService.TriggerOptions != oldService.TriggerOptions {
			deleted = append(deleted, oldService)
			created = append(created, newService)
		}
	}
	for _, newService := range clone {
		created = append(created, newService)
	}

	sched := stack.GetScheduler()
	for _, service := range deleted {
		if err := sched.Delete(domain, service.TriggerID); err != nil {
			return nil, err
		}
	}

	for _, service := range created {
		var triggerType string
		var triggerArgs string
		triggerOpts := strings.SplitN(service.TriggerOptions, " ", 2)
		if len(triggerOpts) > 0 {
			triggerType = strings.TrimSpace(triggerOpts[0])
		}
		if len(triggerOpts) > 1 {
			triggerArgs = strings.TrimSpace(triggerOpts[1])
		}
		msg, err := jobs.NewMessage(jobs.JSONEncoding, map[string]string{
			"slug":         slug,
			"type":         service.Type,
			"service_file": service.File,
		})
		if err != nil {
			return nil, err
		}
		trigger, err := scheduler.NewTrigger(&scheduler.TriggerInfos{
			Type:       triggerType,
			WorkerType: "service",
			Domain:     domain,
			Arguments:  triggerArgs,
			Message:    msg,
		})
		if err != nil {
			return nil, err
		}
		if err = sched.Add(trigger); err != nil {
			return nil, err
		}
		service.TriggerID = trigger.ID()
	}

	return newServices, nil
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
	if slug == "" || !slugReg.MatchString(slug) {
		return nil, ErrInvalidSlugName
	}
	man := &WebappManifest{}
	err := couchdb.GetDoc(db, consts.Apps, consts.Apps+"/"+slug, man)
	if couchdb.IsNotFoundError(err) {
		return nil, ErrNotFound
	}
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
