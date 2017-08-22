package apps

import (
	"io"
	"net/url"
	"regexp"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/hooks"
	"github.com/cozy/cozy-stack/pkg/logger"
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
	db       couchdb.Database
	endState State

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
	Type        AppType
	Operation   Operation
	Slug        string
	SourceURL   string
	Deactivated bool
	Registries  []*url.URL
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
func NewInstaller(db couchdb.Database, fs Copier, opts *InstallerOptions) (*Installer, error) {
	if opts.Operation == 0 {
		panic("Missing installer operation")
	}
	if opts.Type != Webapp && opts.Type != Konnector {
		panic("Bad or missing installer type")
	}

	slug := opts.Slug
	if slug == "" || !slugReg.MatchString(slug) {
		return nil, ErrInvalidSlugName
	}

	// For konnectors applications, we actually create a tar archive in which the
	// sources are stored before copying the archive into the application
	// storage.
	if opts.Type == Konnector {
		fs = newTarCopier(fs, KonnectorArchiveName)
	}

	man, err := GetBySlug(db, slug, opts.Type)
	if opts.Operation == Install {
		if err == nil {
			return nil, ErrAlreadyExists
		}
		if err != ErrNotFound {
			return nil, err
		}
		err = nil
		switch opts.Type {
		case Webapp:
			man = &WebappManifest{}
		case Konnector:
			man = &KonnManifest{}
		}
	} else if err != nil {
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

	log := logger.WithDomain(db.Prefix())

	var manFilename string
	switch opts.Type {
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
	default:
		return nil, ErrNotSupportedSource
	}

	return &Installer{
		fetcher:  fetcher,
		op:       opts.Operation,
		db:       db,
		fs:       fs,
		endState: endState,

		man:  man,
		src:  src,
		slug: slug,

		errc: make(chan error, 1),
		manc: make(chan Manifest, 2),
		log:  log,
	}, nil
}

// Run will install, update or delete the application linked to the installer,
// depending on specified operation. It will report its progress or error (see
// Poll method) and should be run asynchronously.
func (i *Installer) Run() {
	var err error

	man := i.man
	if man == nil {
		panic("Manifest is nil")
	}

	switch i.op {
	case Install:
		err = i.install(man)
	case Update:
		err = i.update(man)
	case Delete:
		err = i.delete(man)
	default:
		panic("Unknown operation")
	}

	if err == ErrBadState {
		i.errc <- err
		return
	}

	if err != nil {
		man.SetState(Errored)
		man.SetError(err)
		man.Update(i.db)
		i.errc <- err
		return
	}

	if i.op != Delete {
		man.SetState(i.endState)
		man.Update(i.db)
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
func (i *Installer) install(man Manifest) error {
	i.log.Infof("[apps] Start install: %s %s", i.slug, i.src.String())
	args := []string{i.db.Prefix(), i.slug}
	return hooks.Execute("install-app", args, func() error {
		if err := i.ReadManifest(Installing, man); err != nil {
			return err
		}
		if err := man.Create(i.db); err != nil {
			return err
		}
		i.manc <- man
		return i.fetcher.Fetch(i.src, i.fs, man)
	})
}

// update will perform the update of an already installed application. It
// returns the freshly fetched manifest from the source along with a possible
// error in case the update went wrong.
//
// Note that the fetched manifest is returned even if an error occurred while
// upgrading.
func (i *Installer) update(man Manifest) error {
	i.log.Infof("[apps] Start update: %s %s", i.slug, i.src.String())
	if err := i.checkState(man); err != nil {
		return err
	}
	if err := i.ReadManifest(Upgrading, man); err != nil {
		return err
	}
	if err := man.Update(i.db); err != nil {
		return err
	}
	i.manc <- man
	return i.fetcher.Fetch(i.src, i.fs, man)
}

func (i *Installer) delete(man Manifest) error {
	i.log.Infof("[apps] Start delete: %s %s", i.slug, i.src.String())
	if err := i.checkState(man); err != nil {
		return err
	}
	args := []string{i.db.Prefix(), i.slug}
	return hooks.Execute("remove-app", args, func() error {
		return man.Delete(i.db)
	})
}

// checkState returns whether or not the manifest is in the right state to
// perform an update or deletion.
func (i *Installer) checkState(man Manifest) error {
	state := man.State()
	if state == Ready ||
		state == Installed ||
		state == Errored {
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
func (i *Installer) ReadManifest(state State, man Manifest) error {
	r, err := i.fetcher.FetchManifest(i.src)
	if err != nil {
		return err
	}
	defer r.Close()
	man.SetState(state)
	return man.ReadManifest(io.LimitReader(r, ManifestMaxSize), i.slug, i.src.String())
}

// Poll should be used to monitor the progress of the Installer.
func (i *Installer) Poll() (Manifest, bool, error) {
	select {
	case man := <-i.manc:
		state := man.State()
		// state can be errored in final stage of the process, for instance when an
		// Errored application is being uninstalled.
		done := (state == Ready || state == Installed || state == Errored)
		return man, done, nil
	case err := <-i.errc:
		return nil, false, err
	}
}
