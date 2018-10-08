package apps

import (
	"io"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

const (
	// ManifestMaxSize is the manifest maximum size
	ManifestMaxSize = 2 << (2 * 10) // 2MB
	// WebappManifestName is the name of the manifest at the root of the
	// client-side application directory
	WebappManifestName = "manifest.webapp"
	// KonnectorManifestName is the name of the manifest at the root of the
	// konnector application directory
	KonnectorManifestName = "manifest.konnector"
)

// State is the state of the application
type State string

const (
	// Installing state
	Installing = "installing"
	// Upgrading state
	Upgrading = "upgrading"
	// Installed state, can be used to state that an application has been
	// installed but needs a user interaction to be activated and "ready".
	Installed = "installed"
	// Ready state
	Ready = "ready"
	// Errored state
	Errored = "errored"
)

// AppType is an enum to represent the type of application: webapp clientside
// or konnector serverside.
type AppType int

const (
	// Webapp is the clientside application type
	Webapp AppType = iota + 1
	// Konnector is the serverside application type
	Konnector
)

// KonnectorArchiveName is the name of the archive created to store the
// konnectors sources.
const KonnectorArchiveName = "app.tar"

// SubDomainer is an interface with a single method to build an URL from a slug
type SubDomainer interface {
	SubDomain(s string) *url.URL
}

// Manifest interface is used by installer to encapsulate the manifest metadata
// that can represent either a webapp or konnector manifest
type Manifest interface {
	couchdb.Doc
	Match(field, expected string) bool
	ReadManifest(i io.Reader, slug, sourceURL string) (Manifest, error)

	Create(db prefixer.Prefixer) error
	Update(db prefixer.Prefixer) error
	Delete(db prefixer.Prefixer) error

	AppType() AppType
	Permissions() permissions.Set
	Source() string
	Version() string
	SetAvailableVersion(version string)
	Slug() string
	State() State
	LastUpdate() time.Time

	SetError(err error)
	Error() error

	SetSource(src *url.URL)
	SetState(state State)
	SetVersion(version string)
}

// GetBySlug returns an app manifest identified by its slug
func GetBySlug(db prefixer.Prefixer, slug string, appType AppType) (Manifest, error) {
	var man Manifest
	var err error
	switch appType {
	case Webapp:
		man, err = GetWebappBySlug(db, slug)
	case Konnector:
		man, err = GetKonnectorBySlug(db, slug)
	}
	if err != nil {
		return nil, err
	}
	return man, nil
}

func routeMatches(path, ctx []string) bool {
	for i, part := range ctx {
		if path[i] != part {
			return false
		}
	}
	return true
}
