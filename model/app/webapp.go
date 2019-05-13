package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/notification"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
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
	name string

	Type           string `json:"type"`
	File           string `json:"file"`
	Debounce       string `json:"debounce"`
	TriggerOptions string `json:"trigger"`
	TriggerID      string `json:"trigger_id"`
}

// Services is a map to define services assciated with an application.
type Services map[string]*Service

// Notifications is a map to define the notifications properties used by the
// application.
type Notifications map[string]notification.Properties

// Intent is a declaration of a service for other client-side apps
type Intent struct {
	Action string   `json:"action"`
	Types  []string `json:"type"`
	Href   string   `json:"href"`
}

// Terms of an application/konnector
type Terms struct {
	URL     string `json:"url"`
	Version string `json:"version"`
}

// WebappManifest contains all the informations associated with an installed web
// application.
type WebappManifest struct {
	DocID  string `json:"_id,omitempty"`
	DocRev string `json:"_rev,omitempty"`

	Name       string `json:"name"`
	NamePrefix string `json:"name_prefix,omitempty"`
	Editor     string `json:"editor"`
	Icon       string `json:"icon"`

	Type        string           `json:"type,omitempty"`
	License     string           `json:"license,omitempty"`
	Language    string           `json:"language,omitempty"`
	Category    string           `json:"category,omitempty"`
	VendorLink  string           `json:"vendor_link"`
	Locales     *json.RawMessage `json:"locales,omitempty"`
	Langs       *json.RawMessage `json:"langs,omitempty"`
	Platforms   *json.RawMessage `json:"platforms,omitempty"`
	Categories  *json.RawMessage `json:"categories,omitempty"`
	Developer   *json.RawMessage `json:"developer,omitempty"`
	Screenshots *json.RawMessage `json:"screenshots,omitempty"`
	Tags        *json.RawMessage `json:"tags,omitempty"`
	Partnership *json.RawMessage `json:"partnership,omitempty"`

	DocSlug             string         `json:"slug"`
	DocState            State          `json:"state"`
	DocSource           string         `json:"source"`
	DocChecksum         string         `json:"checksum"`
	DocVersion          string         `json:"version"`
	DocPermissions      permission.Set `json:"permissions"`
	DocAvailableVersion string         `json:"available_version,omitempty"`
	DocTerms            Terms          `json:"terms,omitempty"`

	Intents       []Intent      `json:"intents"`
	Routes        Routes        `json:"routes"`
	Services      Services      `json:"services"`
	Notifications Notifications `json:"notifications"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	FromAppsDir bool        `json:"-"` // Used in development
	Instance    SubDomainer `json:"-"` // Used for JSON-API links

	oldServices Services // Used to diff against when updating the app

	Err string `json:"error,omitempty"`
	err error

	// NOTE: Do not forget to propagate changes made to this structure to the
	// structure AppManifest in client/apps.go.
}

// ID is part of the Manifest interface
func (m *WebappManifest) ID() string { return m.DocID }

// Rev is part of the Manifest interface
func (m *WebappManifest) Rev() string { return m.DocRev }

// DocType is part of the Manifest interface
func (m *WebappManifest) DocType() string { return consts.Apps }

// Clone implements couchdb.Doc
func (m *WebappManifest) Clone() couchdb.Doc {
	cloned := *m

	cloned.Routes = make(Routes, len(m.Routes))
	for k, v := range m.Routes {
		cloned.Routes[k] = v
	}

	cloned.Services = make(Services, len(m.Services))
	for k, v := range m.Services {
		tmp := *v
		cloned.Services[k] = &tmp
	}

	cloned.Notifications = make(Notifications, len(m.Notifications))
	for k, v := range m.Notifications {
		props := (&v).Clone()
		cloned.Notifications[k] = *props
	}

	cloned.Locales = cloneRawMessage(m.Locales)
	cloned.Langs = cloneRawMessage(m.Langs)
	cloned.Platforms = cloneRawMessage(m.Platforms)
	cloned.Categories = cloneRawMessage(m.Categories)
	cloned.Developer = cloneRawMessage(m.Developer)
	cloned.Screenshots = cloneRawMessage(m.Screenshots)
	cloned.Tags = cloneRawMessage(m.Tags)
	cloned.Partnership = cloneRawMessage(m.Partnership)

	cloned.Intents = make([]Intent, len(m.Intents))
	copy(cloned.Intents, m.Intents)

	cloned.DocPermissions = make(permission.Set, len(m.DocPermissions))
	copy(cloned.DocPermissions, m.DocPermissions)

	return &cloned
}

// SetID is part of the Manifest interface
func (m *WebappManifest) SetID(id string) { m.DocID = id }

// SetRev is part of the Manifest interface
func (m *WebappManifest) SetRev(rev string) { m.DocRev = rev }

// SetSource is part of the Manifest interface
func (m *WebappManifest) SetSource(src *url.URL) { m.DocSource = src.String() }

// Source is part of the Manifest interface
func (m *WebappManifest) Source() string { return m.DocSource }

// Version is part of the Manifest interface
func (m *WebappManifest) Version() string { return m.DocVersion }

// AvailableVersion is part of the Manifest interface
func (m *WebappManifest) AvailableVersion() string { return m.DocAvailableVersion }

// Checksum is part of the Manifest interface
func (m *WebappManifest) Checksum() string { return m.DocChecksum }

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

// SetAvailableVersion is part of the Manifest interface
func (m *WebappManifest) SetAvailableVersion(version string) { m.DocAvailableVersion = version }

// SetChecksum is part of the Manifest interface
func (m *WebappManifest) SetChecksum(shasum string) { m.DocChecksum = shasum }

// AppType is part of the Manifest interface
func (m *WebappManifest) AppType() consts.AppType { return consts.WebappType }

// Terms is part of the Manifest interface
func (m *WebappManifest) Terms() Terms { return m.DocTerms }

// Permissions is part of the Manifest interface
func (m *WebappManifest) Permissions() permission.Set {
	return m.DocPermissions
}

// SetError is part of the Manifest interface
func (m *WebappManifest) SetError(err error) {
	m.SetState(Errored)
	m.Err = err.Error()
	m.err = err
}

// Error is part of the Manifest interface
func (m *WebappManifest) Error() error { return m.err }

// Match is part of the Manifest interface
func (m *WebappManifest) Match(field, value string) bool {
	switch field {
	case "slug":
		return m.DocSlug == value
	case "state":
		return m.DocState == State(value)
	}
	return false
}

// NameLocalized returns the name of the app in the given locale
func (m *WebappManifest) NameLocalized(locale string) string {
	if m.Locales != nil && locale != "" {
		var locales map[string]struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(*m.Locales, &locales); err == nil {
			if v, ok := locales[locale]; ok && v.Name != "" {
				return v.Name
			}
		}
	}
	return m.Name
}

// ReadManifest is part of the Manifest interface
func (m *WebappManifest) ReadManifest(r io.Reader, slug, sourceURL string) (Manifest, error) {
	var newManifest WebappManifest
	if err := json.NewDecoder(r).Decode(&newManifest); err != nil {
		return nil, ErrBadManifest
	}

	newManifest.SetID(consts.Apps + "/" + slug)
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

	return &newManifest, nil
}

// Create is part of the Manifest interface
func (m *WebappManifest) Create(db prefixer.Prefixer) error {
	if err := diffServices(db, m.Slug(), nil, m.Services); err != nil {
		return err
	}
	m.DocID = consts.Apps + "/" + m.DocSlug
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	if err := couchdb.CreateNamedDocWithDB(db, m); err != nil {
		return err
	}
	_, err := permission.CreateWebappSet(db, m.Slug(), m.Permissions())
	return err
}

// Update is part of the Manifest interface
func (m *WebappManifest) Update(db prefixer.Prefixer, extraPerms permission.Set) error {
	if err := diffServices(db, m.Slug(), m.oldServices, m.Services); err != nil {
		return err
	}
	m.UpdatedAt = time.Now()
	if err := couchdb.UpdateDoc(db, m); err != nil {
		return err
	}

	var err error
	perms := m.Permissions()

	// Merging the potential extra permissions
	if len(extraPerms) > 0 {
		perms, err = permission.MergeExtraPermissions(perms, extraPerms)
		if err != nil {
			return err
		}
	}

	_, err = permission.UpdateWebappSet(db, m.Slug(), perms)
	return err
}

// Delete is part of the Manifest interface
func (m *WebappManifest) Delete(db prefixer.Prefixer) error {
	err := diffServices(db, m.Slug(), m.Services, nil)
	if err != nil {
		return err
	}
	err = permission.DestroyWebapp(db, m.Slug())
	if err != nil && !couchdb.IsNotFoundError(err) {
		return err
	}
	return couchdb.DeleteDoc(db, m)
}

func diffServices(db prefixer.Prefixer, slug string, oldServices, newServices Services) error {
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
		newService.name = newName
	}

	for name, oldService := range oldServices {
		oldService.name = name
		newService, ok := newServices[name]
		if !ok {
			deleted = append(deleted, oldService)
			continue
		}
		delete(clone, name)
		if newService.File != oldService.File ||
			newService.Type != oldService.Type ||
			newService.TriggerOptions != oldService.TriggerOptions ||
			newService.Debounce != oldService.Debounce {
			deleted = append(deleted, oldService)
			created = append(created, newService)
		} else {
			*newService = *oldService
		}
		newService.name = name
	}
	for _, newService := range clone {
		created = append(created, newService)
	}

	sched := job.System()
	for _, service := range deleted {
		if err := sched.DeleteTrigger(db, service.TriggerID); err != nil && err != job.ErrNotFoundTrigger {
			return err
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
		msg := map[string]string{
			"slug": slug,
			"name": service.name,
		}
		trigger, err := job.NewTrigger(db, job.TriggerInfos{
			Type:       triggerType,
			WorkerType: "service",
			Debounce:   service.Debounce,
			Arguments:  triggerArgs,
		}, msg)
		if err != nil {
			return err
		}
		if err = sched.AddTrigger(trigger); err != nil {
			return err
		}
		service.TriggerID = trigger.ID()
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
		if !strings.EqualFold(action, intent.Action) {
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

// appsdir is a map of slug -> directory used in development for webapps that
// are not installed in the Cozy but serve directly from a directory.
var appsdir map[string]string

// SetupAppsDir allow to load some webapps from directories for development.
func SetupAppsDir(apps map[string]string) {
	if appsdir == nil {
		appsdir = make(map[string]string)
	}
	for app, dir := range apps {
		appsdir[app] = dir
	}
}

// FSForAppDir returns a FS for the webapp in development.
func FSForAppDir(slug string) afero.Fs {
	return afero.NewBasePathFs(afero.NewOsFs(), appsdir[slug])
}

// loadManifestFromDir returns a manifest for a webapp in development.
func loadManifestFromDir(slug string) (*WebappManifest, error) {
	dir, ok := appsdir[slug]
	if !ok {
		return nil, ErrNotFound
	}
	fs := FSForAppDir(slug)
	manFile, err := fs.Open(WebappManifestName)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Could not find the manifest in your app directory %s", dir)
		}
		return nil, err
	}
	app := &WebappManifest{}
	man, err := app.ReadManifest(manFile, slug, "file://localhost"+dir)
	if err != nil {
		return nil, fmt.Errorf("Could not parse the manifest: %s", err.Error())
	}
	app = man.(*WebappManifest)
	app.FromAppsDir = true
	app.DocState = Ready
	return app, nil
}

// GetWebappBySlug fetch the WebappManifest from the database given a slug.
func GetWebappBySlug(db prefixer.Prefixer, slug string) (*WebappManifest, error) {
	if slug == "" || !slugReg.MatchString(slug) {
		return nil, ErrInvalidSlugName
	}
	for app := range appsdir {
		if app == slug {
			return loadManifestFromDir(slug)
		}
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

// GetWebappBySlugAndUpdate fetch the WebappManifest and perform an update of
// the application if necessary and if the application was installed from the
// registry.
func GetWebappBySlugAndUpdate(db prefixer.Prefixer, slug string, copier appfs.Copier, registries []*url.URL) (*WebappManifest, error) {
	man, err := GetWebappBySlug(db, slug)
	if err != nil {
		return nil, err
	}
	return DoLazyUpdate(db, man, copier, registries).(*WebappManifest), nil
}

// ListWebapps returns the list of installed web applications.
//
// TODO: pagination
func ListWebapps(db prefixer.Prefixer) ([]*WebappManifest, error) {
	var docs []*WebappManifest
	req := &couchdb.AllDocsRequest{Limit: 100}
	err := couchdb.GetAllDocs(db, consts.Apps, req, &docs)
	if err != nil {
		return nil, err
	}
	for slug := range appsdir {
		if man, err := loadManifestFromDir(slug); err == nil {
			docs = append(docs, man)
		}
	}
	return docs, nil
}

func cloneRawMessage(m *json.RawMessage) *json.RawMessage {
	if m != nil {
		v := make(json.RawMessage, len(*m))
		copy(v, *m)
		return &v
	}
	return nil
}

var _ Manifest = &WebappManifest{}
