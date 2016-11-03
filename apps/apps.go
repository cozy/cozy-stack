package apps

import (
	"encoding/json"
	"errors"
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

// AppsDirectory is the name of the directory in which apps are stored
const AppsDirectory = "/_cozyapps"

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

var slugReg = regexp.MustCompile(`[A-Za-z0-9\\-]`)

var (
	// ErrInvalidSlugName is used when the given slud name is not valid
	ErrInvalidSlugName = errors.New("Invalid slug name")
	// ErrNotSupportedSource is used when the source transport or
	// protocol is not supported
	ErrNotSupportedSource = errors.New("Invalid or not supported source scheme")
	// ErrSourceNotReachable is used when the given source for
	// application is not reachable
	ErrSourceNotReachable = errors.New("Application source is not reachable")
	// ErrBadManifest when the manifest is not valid or malformed
	ErrBadManifest = errors.New("Application manifest is invalid or malformed")
	// ErrBadState is used when trying to use the application while in a
	// state that is not appropriate for the given operation.
	ErrBadState = errors.New("Application is not in valid state to perform this operation")
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
func (m *Manifest) ID() string { return m.ManID }

// Rev return the manifest revision - see couchdb.Doc interface
func (m *Manifest) Rev() string { return m.ManRev }

// DocType returns the manifest doctype - see couchdb.Doc interfaces
func (m *Manifest) DocType() string { return ManifestDocType }

// SetID is used to change the file identifier - see couchdb.Doc
// interface
func (m *Manifest) SetID(id string) { m.ManID = id }

// SetRev is used to change the file revision - see couchdb.Doc
// interface
func (m *Manifest) SetRev(rev string) { m.ManRev = rev }

// SelfLink is used to generate a JSON-API link for the file - see
// jsonapi.Object interface
func (m *Manifest) SelfLink() string { return "/apps/" + m.ManID }

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
	Fetch(vfsC *vfs.Context, appdir string) error
}

// Installer is used to install or update applications.
type Installer struct {
	cli Client

	// TODO: fix this mess with contexts
	db   string
	vfsC *vfs.Context

	slug string
	src  string
	man  *Manifest

	err  error
	errc chan error
	manc chan *Manifest
}

// NewInstaller creates a new Installer
// @TODO: fix this mess with contexts
func NewInstaller(vfsC *vfs.Context, db, slug, src string) (*Installer, error) {
	if !slugReg.MatchString(slug) {
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
		db:   db,
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

	err = i.updateManifest(newman)
	if err != nil {
		return
	}

	appdir := path.Join(AppsDirectory, newman.Slug)
	err = i.vfsC.MkdirAll(appdir)
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
			i.manc <- man
		}
	}()

	if i.man != nil {
		panic("Manifest is already defined")
	}

	man = &Manifest{}
	err = couchdb.GetDoc(i.db, ManifestDocType, slug, man)
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
	err = json.NewDecoder(io.LimitReader(r, ManifestMaxSize)).Decode(&man)
	if err != nil {
		return nil, ErrBadManifest
	}

	man.Slug = slug
	man.Source = src
	man.State = Available

	err = couchdb.CreateDoc(i.db, man)
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

	return couchdb.UpdateDoc(i.db, newman)
}

// WaitManifest should be used to monitor the progress of the
// Installer.
func (i *Installer) WaitManifest() (man *Manifest, err error) {
	select {
	case man = <-i.manc:
		return
	case err = <-i.errc:
		return
	}
}
