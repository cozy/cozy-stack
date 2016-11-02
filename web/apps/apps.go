// Package apps is the HTTP frontend of the application package. It
// exposes the HTTP api install, update or uninstall applications.
package apps

import (
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
)

// Access is a string representing the access permission level. It can
// either be read, write or readwrite.
type Access string

// Permissions is a map of key, a description and an access level.
type Permissions map[string]*struct {
	Description string `json:"description"`
	Access      Access `json:"access"`
}

// Developer is the name and url of a developer.
type Developer struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// Manifest contains all the informations about an application.
type Manifest struct {
	ManID  string `json:"_id,omitempty"`  // Manifest identifier
	ManRev string `json:"_rev,omitempty"` // Manifest revision

	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Source      string     `json:"source"`
	State       State      `json:"state"`
	Icon        string     `json:"icon"`
	Description string     `json:"description"`
	Developer   *Developer `json:"developer"`

	DefaultLocal string `json:"default_locale"`
	Locales      map[string]*struct {
		Description string `json:"description"`
	} `json:"locales"`

	Version     string       `json:"version"`
	License     string       `json:"license"`
	Permissions *Permissions `json:"permissions"`
}

// ID returns the manifest identifier - see couchdb.Doc interface
func (m *Manifest) ID() string {
	return m.ManID
}

// Rev return the manifest revision - see couchdb.Doc interface
func (m *Manifest) Rev() string {
	return m.ManRev
}

// DocType returns the manifest doctype - see couchdb.Doc interfaces
func (m *Manifest) DocType() string {
	return ManifestDocType
}

// SetID is used to change the file identifier - see couchdb.Doc
// interface
func (m *Manifest) SetID(id string) {
	m.ManID = id
}

// SetRev is used to change the file revision - see couchdb.Doc
// interface
func (m *Manifest) SetRev(rev string) {
	m.ManRev = rev
}

// SelfLink is used to generate a JSON-API link for the file - see
// jsonapi.Object interface
func (m *Manifest) SelfLink() string {
	return "/apps/" + m.ManID
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

func wrapAppsError(err error) *jsonapi.Error {
	if urlErr, isURLErr := err.(*url.Error); isURLErr {
		return jsonapi.InvalidParameter("Source", urlErr)
	}

	switch err {
	case ErrInvalidSlugName:
		return jsonapi.InvalidParameter("slug", err)
	case ErrNotSupportedSource:
		return jsonapi.InvalidParameter("Source", err)
	case ErrSourceNotReachable:
		return jsonapi.BadRequest(err)
	case ErrBadManifest:
		return jsonapi.BadRequest(err)
	}
	return jsonapi.InternalServerError(err)
}

// InstallHandler handles all POST /:slug request and tries to install
// the application with the given Source.
func InstallHandler(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	vfsC, err := instance.GetVFSContext()
	if err != nil {
		jsonapi.AbortWithError(c, jsonapi.InternalServerError(err))
		return
	}

	db := instance.GetDatabasePrefix()
	src := c.Query("Source")
	slug := c.Param("slug")
	inst, err := NewInstaller(vfsC, db, slug, src)
	if err != nil {
		jsonapi.AbortWithError(c, wrapAppsError(err))
		return
	}

	go inst.Install()

	man, err := inst.WaitManifest()
	if err != nil {
		jsonapi.AbortWithError(c, wrapAppsError(err))
		return
	}

	jsonapi.Data(c, http.StatusAccepted, man, nil)

	go func() {
		for {
			_, err := inst.WaitManifest()
			if err != nil {
				break
			}
			// TODO: do nothing for now
		}
	}()
}

// Routes sets the routing for the apps service
func Routes(router *gin.RouterGroup) {
	router.POST("/:slug", InstallHandler)
}
