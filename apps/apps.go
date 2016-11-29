package apps

import (
	"encoding/json"
	"io"
	"net/url"
	"path"
	"regexp"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

const (
	// ManifestDocType is manifest type
	ManifestDocType = "io.cozy.manifests"
	// ManifestMaxSize is the manifest maximum size
	ManifestMaxSize = 2 << (2 * 10) // 2MB
)

// ManifestFilename is the name of the manifest at the root of the
// application directory
const ManifestFilename = "manifest.webapp"

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

var slugReg = regexp.MustCompile(`^[A-Za-z0-9\-]+$`)

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
	return ManifestDocType + "/" + m.Slug
}

// Rev return the manifest revision - see couchdb.Doc interface
func (m *Manifest) Rev() string { return m.ManRev }

// DocType returns the manifest doctype - see couchdb.Doc interfaces
func (m *Manifest) DocType() string { return ManifestDocType }

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

// Client interface should be implemented by the underlying transport
// used to fetch the application data.
type Client interface {
	// FetchManifest should returns an io.ReadCloser to read the
	// manifest data
	FetchManifest() (io.ReadCloser, error)
	// Fetch should download the application and install it in the given
	// directory.
	Fetch(vfsC vfs.Context, appdir string) error
}

// List returns the list of installed applications.
//
// TODO: pagination
func List(db couchdb.Database) ([]*Manifest, error) {
	var docs []*Manifest
	req := &couchdb.AllDocsRequest{Limit: 100}
	err := couchdb.GetAllDocs(db, ManifestDocType, req, &docs)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

// GetBySlug returns an app identified by its slug
func GetBySlug(db couchdb.Database, slug string) (*Manifest, error) {
	man := &Manifest{}
	err := couchdb.GetDoc(db, ManifestDocType, ManifestDocType+"/"+slug, man)
	if err != nil {
		return nil, err
	}
	return man, nil
}

// Installer is used to install or update applications.
type Installer struct {
	cli Client

	vfsC vfs.Context

	slug string
	src  string
	man  *Manifest

	err  error
	errc chan error
	manc chan *Manifest
}

// NewInstaller creates a new Installer
func NewInstaller(vfsC vfs.Context, slug, src string) (*Installer, error) {
	if slug == "" || !slugReg.MatchString(slug) {
		return nil, ErrInvalidSlugName
	}

	parsedSrc, err := url.Parse(src)
	if err != nil {
		return nil, err
	}

	var cli Client
	switch parsedSrc.Scheme {
	case "git":
		cli = newGitClient(vfsC, src)
	default:
		err = ErrNotSupportedSource
	}

	if err != nil {
		return nil, err
	}

	inst := &Installer{
		cli:  cli,
		vfsC: vfsC,

		slug: slug,
		src:  src,

		errc: make(chan error),
		manc: make(chan *Manifest),
	}

	return inst, err
}

// Install will install the application linked to the installer. It
// will report its progress or error using the WaitManifest method.
func (i *Installer) Install() (newman *Manifest, err error) {
	if i.err != nil {
		return nil, i.err
	}

	defer func() {
		if err != nil {
			err = i.handleErr(err)
		}
	}()

	_, err = i.getOrCreateManifest(i.src, i.slug)
	if err != nil {
		return
	}

	oldman := i.man
	if s := oldman.State; s != Available && s != Errored {
		return nil, ErrBadState
	}

	newman = &(*oldman)
	newman.State = Installing

	defer func() {
		if err != nil {
			newman.State = Errored
			i.updateManifest(newman)
		}
	}()

	err = i.updateManifest(newman)
	if err != nil {
		return
	}

	appdir := path.Join(vfs.AppsDirName, newman.Slug)
	_, err = vfs.MkdirAll(i.vfsC, appdir, nil)
	if err != nil {
		return
	}

	err = i.cli.Fetch(i.vfsC, appdir)
	if err != nil {
		return
	}

	newman.State = Ready
	err = i.updateManifest(newman)
	if err != nil {
		return
	}

	return
}

func (i *Installer) handleErr(err error) error {
	if i.err == nil {
		i.err = err
		i.errc <- err
	}
	return i.err
}

func (i *Installer) getOrCreateManifest(src, slug string) (man *Manifest, err error) {
	if i.err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			err = i.handleErr(err)
		} else {
			i.man = man
		}
	}()

	if i.man != nil {
		panic("Manifest is already defined")
	}

	man, err = GetBySlug(i.vfsC, slug)
	if err != nil && !couchdb.IsNotFoundError(err) {
		return nil, err
	}
	if err == nil {
		return man, nil
	}

	r, err := i.cli.FetchManifest()
	if err != nil {
		return nil, err
	}

	defer r.Close()
	man = &Manifest{}
	err = json.NewDecoder(io.LimitReader(r, ManifestMaxSize)).Decode(&man)
	if err != nil {
		return nil, ErrBadManifest
	}

	man.Slug = slug
	man.Source = src
	man.State = Available

	err = couchdb.CreateNamedDoc(i.vfsC, man)
	return
}

func (i *Installer) updateManifest(newman *Manifest) (err error) {
	if i.err != nil {
		return err
	}

	defer func() {
		if err != nil {
			err = i.handleErr(err)
		} else {
			i.man = newman
			i.manc <- newman
		}
	}()

	oldman := i.man
	if oldman == nil {
		panic("Manifest not defined")
	}

	newman.SetID(oldman.ID())
	newman.SetRev(oldman.Rev())

	return couchdb.UpdateDoc(i.vfsC, newman)
}

// WaitManifest should be used to monitor the progress of the
// Installer.
func (i *Installer) WaitManifest() (man *Manifest, done bool, err error) {
	select {
	case man = <-i.manc:
		done = man.State == Ready
		return
	case err = <-i.errc:
		return
	}
}
