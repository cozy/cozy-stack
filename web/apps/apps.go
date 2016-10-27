package apps

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"

	"github.com/cozy/cozy-stack/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
)

// ManifestDocType is manifest type
const ManifestDocType = "io.cozy.manifests"

var (
	// ErrInvalidSlugName is used when the given slud name is not valid
	ErrInvalidSlugName = errors.New("Invalid slug name")
	// ErrNotSupportedSource is used when the source transport or
	// protocol is not supported
	ErrNotSupportedSource = errors.New("Invalid or not supported source scheme")
)

var slugReg = regexp.MustCompile(`[A-Za-z0-9\\-]`)

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
	State       string     `json:"state"`
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
	switch err {
	case ErrInvalidSlugName:
		return jsonapi.InvalidParameter("slug", err)
	case ErrNotSupportedSource:
		return jsonapi.InvalidParameter("Source", err)
	}
	return jsonapi.InternalServerError(err)
}

func InstallApplication(vfsC *vfs.Context, src *url.URL, slug string) (*Manifest, error) {
	var err error
	if !slugReg.MatchString(slug) {
		return nil, ErrInvalidSlugName
	}

	man := &Manifest{
		Slug:   slug,
		Source: src.String(),
	}

	var inst Installer
	switch src.Scheme {
	case "git":
		inst, err = NewGitInstaller(vfsC, man)
	default:
		err = ErrNotSupportedSource
	}

	if err != nil {
		return nil, err
	}

	err = inst.Install()

	return man, err
}

func InstallOrUpdateHandler(c *gin.Context) {
	vfsC, err := getVfsContext(c)
	if err != nil {
		return
	}

	src, err := url.Parse(c.Query("Source"))
	if err != nil {
		jsonapi.AbortWithError(c, jsonapi.InvalidParameter("Source", err))
		return
	}

	slug := c.Param("slug")
	man, err := InstallApplication(vfsC, src, slug)
	if err != nil {
		jsonapi.AbortWithError(c, wrapAppsError(err))
		return
	}

	jsonapi.Data(c, http.StatusAccepted, man, nil)
}

func Routes(router *gin.RouterGroup) {
	router.POST("/:slug", InstallOrUpdateHandler)
}

// TODO: get rid of this
func getVfsContext(c *gin.Context) (*vfs.Context, error) {
	instance := middlewares.GetInstance(c)
	dbprefix := instance.GetDatabasePrefix()
	fs, err := instance.GetStorageProvider()
	if err != nil {
		jsonapi.AbortWithError(c, jsonapi.InternalServerError(err))
		return nil, err
	}
	vfsC := vfs.NewContext(fs, dbprefix)
	return vfsC, nil
}
