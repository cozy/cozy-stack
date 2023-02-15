package app

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/model/vfs/vfsafero"
	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/spf13/afero"
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

// KonnectorArchiveName is the name of the archive created to store the
// konnectors sources.
const KonnectorArchiveName = "app.tar"

// LocalResource is used to store information about a locally installed
// konnector or webapp.
// It is meant to be used by developers via the CLI.
type LocalResource struct {
	Type consts.AppType
	Dir  string
}

// SubDomainer is an interface with a single method to build an URL from a slug
type SubDomainer interface {
	SubDomain(s string) *url.URL
}

// Manifest interface is used by installer to encapsulate the manifest metadata
// that can represent either a webapp or konnector manifest
type Manifest interface {
	couchdb.Doc
	Fetch(field string) []string
	ReadManifest(i io.Reader, slug, sourceURL string) (Manifest, error)

	Create(db prefixer.Prefixer) error
	Update(db prefixer.Prefixer, extraPerms permission.Set) error
	Delete(db prefixer.Prefixer) error

	AppType() consts.AppType
	Permissions() permission.Set
	Source() string
	Version() string
	AvailableVersion() string
	Checksum() string
	Slug() string
	State() State
	LastUpdate() time.Time
	Terms() Terms

	Name() string
	Icon() string
	Notifications() Notifications

	SetError(err error)
	Error() error

	SetSlug(slug string)
	SetSource(src *url.URL)
	SetState(state State)
	SetVersion(version string)
	SetAvailableVersion(version string)
	SetChecksum(shasum string)
}

// localResources is a map of slug -> LocalResource used during the development
// of webapps that are not installed in the Cozy but served from local
// directories.
var localResources map[string]LocalResource

// SetupLocalResources allows to load some webapps and konnectors from local
// directories for development.
func SetupLocalResources(resources map[string]LocalResource) {
	if localResources == nil {
		localResources = make(map[string]LocalResource)
	}
	for slug, res := range resources {
		localResources[slug] = res
	}
}

// FSForLocalResource returns a FS for the webapp or konnector in development
func FSForLocalResource(slug string) appfs.FileServer {
	base := baseFSForLocalResource(slug)
	return appfs.NewAferoFileServer(base, func(_, _, _, file string) string {
		return path.Join("/", file)
	})
}

func baseFSForLocalResource(slug string) afero.Fs {
	return afero.NewBasePathFs(afero.NewOsFs(), localResources[slug].Dir)
}

// loadManifestFromDir returns a manifest for a webapp or konnector in
// development.
func loadManifestFromDir(slug string) (*WebappManifest, *KonnManifest, error) {
	res, ok := localResources[slug]
	if !ok {
		return nil, nil, ErrNotFound
	}

	fs := baseFSForLocalResource(slug)
	dir := res.Dir

	if res.Type == consts.WebappType {
		manFile, err := fs.Open(WebappManifestName)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil, fmt.Errorf("Could not find the manifest in your app directory %s", dir)
			}
			return nil, nil, err
		}
		app := &WebappManifest{
			doc: &couchdb.JSONDoc{},
		}
		man, err := app.ReadManifest(manFile, slug, "file://localhost"+dir)
		if err != nil {
			return nil, nil, fmt.Errorf("Could not parse the manifest: %s", err.Error())
		}
		app = man.(*WebappManifest)
		app.FromLocalDir = true
		app.val.State = Ready
		return app, nil, nil
	} else {
		manFile, err := fs.Open(KonnectorManifestName)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil, fmt.Errorf("Could not find the manifest in your konnector directory %s", dir)
			}
			return nil, nil, err
		}
		konn := &KonnManifest{
			doc: &couchdb.JSONDoc{},
		}
		man, err := konn.ReadManifest(manFile, slug, "file://localhost"+dir)
		if err != nil {
			return nil, nil, fmt.Errorf("Could not parse the manifest: %s", err.Error())
		}
		konn = man.(*KonnManifest)
		konn.FromLocalDir = true
		konn.val.State = Ready
		return nil, konn, nil
	}
}

// GetBySlug returns an app manifest identified by its slug
func GetBySlug(db prefixer.Prefixer, slug string, appType consts.AppType) (Manifest, error) {
	var man Manifest
	var err error
	switch appType {
	case consts.WebappType:
		man, err = GetWebappBySlug(db, slug)
	case consts.KonnectorType:
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

// UpgradeInstalledState is used to force the legacy "installed" state to
// "ready" for a webapp or a konnector manifest
func UpgradeInstalledState(inst *instance.Instance, man Manifest) error {
	if man.State() == Installed {
		man.SetState(Ready)
		return man.Update(inst, nil)
	}
	return nil
}

// Copier returns the application copier associated with the specified
// application type
func Copier(appsType consts.AppType, inst *instance.Instance) appfs.Copier {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile:
		var baseDirName string
		switch appsType {
		case consts.WebappType:
			baseDirName = vfs.WebappsDirName
		case consts.KonnectorType:
			baseDirName = vfs.KonnectorsDirName
		}
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, inst.DirName(), baseDirName))
		return appfs.NewAferoCopier(baseFS)
	case config.SchemeMem:
		baseFS := vfsafero.GetMemFS("apps")
		return appfs.NewAferoCopier(baseFS)
	case config.SchemeSwift, config.SchemeSwiftSecure:
		return appfs.NewSwiftCopier(config.GetSwiftConnection(), appsType)
	default:
		panic(fmt.Sprintf("instance: unknown storage provider %s", fsURL.Scheme))
	}
}

// AppsFileServer returns the web-application file server associated to this
// instance.
func AppsFileServer(i *instance.Instance) appfs.FileServer {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile:
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), vfs.WebappsDirName))
		return appfs.NewAferoFileServer(baseFS, nil)
	case config.SchemeMem:
		baseFS := vfsafero.GetMemFS("apps")
		return appfs.NewAferoFileServer(baseFS, nil)
	case config.SchemeSwift, config.SchemeSwiftSecure:
		return appfs.NewSwiftFileServer(config.GetSwiftConnection(), consts.WebappType)
	default:
		panic(fmt.Sprintf("instance: unknown storage provider %s", fsURL.Scheme))
	}
}

// KonnectorsFileServer returns the web-application file server associated to this
// instance.
func KonnectorsFileServer(i *instance.Instance) appfs.FileServer {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile:
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), vfs.KonnectorsDirName))
		return appfs.NewAferoFileServer(baseFS, nil)
	case config.SchemeMem:
		baseFS := vfsafero.GetMemFS("apps")
		return appfs.NewAferoFileServer(baseFS, nil)
	case config.SchemeSwift, config.SchemeSwiftSecure:
		return appfs.NewSwiftFileServer(config.GetSwiftConnection(), consts.KonnectorType)
	default:
		panic(fmt.Sprintf("instance: unknown storage provider %s", fsURL.Scheme))
	}
}
