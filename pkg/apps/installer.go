package apps

import (
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"time"

	"github.com/cozy/cozy-stack/pkg/hooks"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/sirupsen/logrus"
)

var slugReg = regexp.MustCompile(`^[a-z0-9\-]+$`)

// Operation is the type of operation the installer is created for.
type Operation int

const (
	// Install operation for installing an application
	Install Operation = iota + 1
	// Update operation for updating an application
	Update
	// Delete operation for deleting an application
	Delete
)

// Installer is used to install or update applications.
type Installer struct {
	fetcher  Fetcher
	op       Operation
	fs       Copier
	db       prefixer.Prefixer
	endState State

	overridenParameters *json.RawMessage
	permissionsAcked    bool

	man  Manifest
	src  *url.URL
	slug string

	errc chan error
	manc chan Manifest
	log  *logrus.Entry
}

// InstallerOptions provides the slug name of the application along with the
// source URL.
type InstallerOptions struct {
	Type             AppType
	Operation        Operation
	Manifest         Manifest
	Slug             string
	SourceURL        string
	Deactivated      bool
	PermissionsAcked bool
	Registries       []*url.URL

	// Used to override the "Parameters" field of konnectors during installation.
	// This modification is useful to allow the parameterization of a konnector
	// at its installation as we do not have yet a registry up and running.
	OverridenParameters *json.RawMessage
}

// Fetcher interface should be implemented by the underlying transport
// used to fetch the application data.
type Fetcher interface {
	// FetchManifest should returns an io.ReadCloser to read the
	// manifest data
	FetchManifest(src *url.URL) (io.ReadCloser, error)
	// Fetch should download the application and install it in the given
	// directory.
	Fetch(src *url.URL, fs Copier, man Manifest) error
}

// NewInstaller creates a new Installer
func NewInstaller(db prefixer.Prefixer, fs Copier, opts *InstallerOptions) (*Installer, error) {
	man, err := initManifest(db, opts)
	if err != nil {
		return nil, err
	}

	var src *url.URL
	switch opts.Operation {
	case Install:
		if opts.SourceURL == "" {
			return nil, ErrMissingSource
		}
		src, err = url.Parse(opts.SourceURL)
	case Update, Delete:
		var srcString string
		if opts.SourceURL == "" {
			srcString = man.Source()
		} else {
			srcString = opts.SourceURL
		}
		src, err = url.Parse(srcString)
	default:
		panic("Unknown installer operation")
	}
	if err != nil {
		return nil, err
	}

	var endState State
	if opts.Deactivated || man.State() == Installed {
		endState = Installed
	} else {
		endState = Ready
	}

	log := logger.WithDomain(db.DomainName()).WithField("nspace", "apps")

	var manFilename string
	switch man.AppType() {
	case Webapp:
		manFilename = WebappManifestName
	case Konnector:
		manFilename = KonnectorManifestName
	}

	var fetcher Fetcher
	switch src.Scheme {
	case "git", "git+ssh", "ssh+git":
		fetcher = newGitFetcher(manFilename, log)
	case "http", "https":
		fetcher = newHTTPFetcher(manFilename, log)
	case "registry":
		fetcher = newRegistryFetcher(opts.Registries, log)
	case "file":
		fetcher = newFileFetcher(manFilename, log)
	default:
		return nil, ErrNotSupportedSource
	}

	return &Installer{
		fetcher:  fetcher,
		op:       opts.Operation,
		db:       db,
		fs:       fs,
		endState: endState,

		overridenParameters: opts.OverridenParameters,
		permissionsAcked:    opts.PermissionsAcked,

		man:  man,
		src:  src,
		slug: man.Slug(),

		errc: make(chan error, 1),
		manc: make(chan Manifest, 2),
		log:  log,
	}, nil
}

func initManifest(db prefixer.Prefixer, opts *InstallerOptions) (man Manifest, err error) {
	if man = opts.Manifest; man != nil {
		return man, nil
	}

	slug := opts.Slug
	if slug == "" || !slugReg.MatchString(slug) {
		return nil, ErrInvalidSlugName
	}

	if opts.Operation == Install {
		_, err = GetBySlug(db, slug, opts.Type)
		if err == nil {
			return nil, ErrAlreadyExists
		}
		if err != ErrNotFound {
			return nil, err
		}
		switch opts.Type {
		case Webapp:
			man = &WebappManifest{DocSlug: slug}
		case Konnector:
			man = &KonnManifest{DocSlug: slug}
		}
	} else {
		man, err = GetBySlug(db, slug, opts.Type)
		if err != nil {
			return nil, err
		}
	}

	if man == nil {
		panic("Bad or missing installer type")
	}

	return man, nil
}

// Slug return the slug of the application being installed.
func (i *Installer) Slug() string {
	return i.slug
}

// Domain return the domain of instance associated with the installer.
func (i *Installer) Domain() string {
	return i.db.DomainName()
}

// Run will install, update or delete the application linked to the installer,
// depending on specified operation. It will report its progress or error (see
// Poll method) and should be run asynchronously.
func (i *Installer) Run() {
	var err error

	if i.man == nil {
		panic("Manifest is nil")
	}

	switch i.op {
	case Install:
		err = i.install()
	case Update:
		err = i.update()
	case Delete:
		err = i.delete()
	default:
		panic("Unknown operation")
	}

	man := i.man.Clone().(Manifest)
	if err != nil {
		man.SetError(err)
		realtime.GetHub().Publish(i.db, realtime.EventUpdate, man.Clone(), nil)
	}
	i.manc <- man
}

// RunSync does the same work as Run but can be used synchronously.
func (i *Installer) RunSync() (Manifest, error) {
	go i.Run()
	for {
		man, done, err := i.Poll()
		if err != nil {
			return nil, err
		}
		if done {
			return man, nil
		}
	}
}

// install will perform the installation of an application. It returns the
// freshly fetched manifest from the source along with a possible error in case
// the installation went wrong.
//
// Note that the fetched manifest is returned even if an error occurred while
// upgrading.
func (i *Installer) install() error {
	i.log.Infof("Start install: %s %s", i.slug, i.src.String())
	args := []string{i.db.DomainName(), i.slug}
	return hooks.Execute("install-app", args, func() error {
		newManifest, err := i.ReadManifest(Installing)
		if err != nil {
			return err
		}
		i.man = newManifest
		i.sendRealtimeEvent()
		i.manc <- i.man.Clone().(Manifest)
		if err := i.fetcher.Fetch(i.src, i.fs, i.man); err != nil {
			return err
		}
		i.man.SetState(i.endState)
		return i.man.Create(i.db)
	})
}

// update will perform the update of an already installed application. It
// returns the freshly fetched manifest from the source along with a possible
// error in case the update went wrong.
//
// Note that the fetched manifest is returned even if an error occurred while
// upgrading.
func (i *Installer) update() error {
	i.log.Infof("Start update: %s %s", i.slug, i.src.String())
	if err := i.checkState(i.man); err != nil {
		return err
	}

	oldManifest := i.man
	newManifest, err := i.ReadManifest(Upgrading)
	if err != nil {
		return err
	}

	// Fast path for registry:// and http:// sources: we do not need to go
	// further in the case where the fetched manifest has the same version has
	// the one in database.
	//
	// For git:// and file:// sources, it may be more complicated since we need
	// to actually fetch the data to extract the exact version of the manifest.
	makeUpdate := true
	switch i.src.Scheme {
	case "registry", "http", "https":
		makeUpdate = (newManifest.Version() != oldManifest.Version())
	}

	// Check the possible permissions changes before updating. If the
	// verifyPermissions flag is activated (for non manual updates for example),
	// we cancel out the update and mark the UpdateAvailable field of the
	// application instead of actually updating.
	if makeUpdate && !isPlatformApp(oldManifest) {
		oldPermissions := oldManifest.Permissions()
		newPermissions := newManifest.Permissions()
		samePermissions := newPermissions != nil && oldPermissions != nil &&
			newPermissions.HasSameRules(oldPermissions)
		if !samePermissions && !i.permissionsAcked {
			makeUpdate = false
		}
	}

	if makeUpdate {
		i.man = newManifest
		i.sendRealtimeEvent()
		i.manc <- i.man.Clone().(Manifest)
		if err := i.fetcher.Fetch(i.src, i.fs, i.man); err != nil {
			return err
		}
		i.man.SetState(i.endState)
	} else {
		i.man.SetAvailableVersion(newManifest.Version())
		i.sendRealtimeEvent()
		i.manc <- i.man.Clone().(Manifest)
	}

	return i.man.Update(i.db)
}

func (i *Installer) delete() error {
	i.log.Infof("Start delete: %s %s", i.slug, i.src.String())
	if err := i.checkState(i.man); err != nil {
		return err
	}
	args := []string{i.db.DomainName(), i.slug}
	return hooks.Execute("uninstall-app", args, func() error {
		return i.man.Delete(i.db)
	})
}

// checkState returns whether or not the manifest is in the right state to
// perform an update or deletion.
func (i *Installer) checkState(man Manifest) error {
	state := man.State()
	if state == Ready || state == Installed {
		return nil
	}
	if time.Since(man.LastUpdate()) > 15*time.Minute {
		return nil
	}
	return ErrBadState
}

// ReadManifest will fetch the manifest and read its JSON content into the
// passed manifest pointer.
//
// The State field of the manifest will be set to the specified state.
func (i *Installer) ReadManifest(state State) (Manifest, error) {
	r, err := i.fetcher.FetchManifest(i.src)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	newManifest, err := i.man.ReadManifest(io.LimitReader(r, ManifestMaxSize), i.slug, i.src.String())
	if err != nil {
		return nil, err
	}
	newManifest.SetState(state)

	shouldOverrideParameters := (i.overridenParameters != nil &&
		i.man.AppType() == Konnector &&
		i.src.Scheme != "registry")
	if shouldOverrideParameters {
		if m, ok := newManifest.(*KonnManifest); ok {
			m.Parameters = i.overridenParameters
		}
	}
	return newManifest, nil
}

func (i *Installer) sendRealtimeEvent() {
	realtime.GetHub().Publish(i.db, realtime.EventUpdate, i.man.Clone(), nil)
}

// Poll should be used to monitor the progress of the Installer.
func (i *Installer) Poll() (Manifest, bool, error) {
	man := <-i.manc
	done := false
	if s := man.State(); s == Ready || s == Installed || s == Errored {
		done = true
	}
	return man, done, man.Error()
}

func isPlatformApp(man Manifest) bool {
	if man.AppType() != Webapp {
		return false
	}
	return utils.IsInArray(man.Slug(), []string{
		"onboarding",
		"settings",
		"collect",
		"home",
		"photos",
		"drive",
		"store",
		"banks",
		"contacts",
	})
}
