package apps

import (
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/spf13/afero"
)

var slugReg = regexp.MustCompile(`^[A-Za-z0-9\-]+$`)

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
	fetcher Fetcher
	fs      afero.Fs
	db      couchdb.Database

	man  Manifest
	src  *url.URL
	slug string

	err  error
	errc chan error
	manc chan Manifest
}

// InstallerOptions provides the slug name of the application along with the
// source URL.
type InstallerOptions struct {
	Type      AppType
	Operation Operation
	Slug      string
	SourceURL string
}

// Fetcher interface should be implemented by the underlying transport
// used to fetch the application data.
type Fetcher interface {
	// FetchManifest should returns an io.ReadCloser to read the
	// manifest data
	FetchManifest(src *url.URL) (io.ReadCloser, error)
	// Fetch should download the application and install it in the given
	// directory.
	Fetch(src *url.URL, appDir string) error
}

// NewInstaller creates a new Installer
func NewInstaller(db couchdb.Database, fs afero.Fs, opts *InstallerOptions) (*Installer, error) {
	if opts.Operation == 0 {
		panic("Missing installer operation")
	}

	slug := opts.Slug
	if slug == "" || !slugReg.MatchString(slug) {
		return nil, ErrInvalidSlugName
	}

	var manFilename string
	switch opts.Type {
	case Webapp:
		manFilename = WebappManifestName
	case Konnector:
		manFilename = KonnectorManifestName
	default:
		return nil, fmt.Errorf("unknown installer type %s", string(opts.Type))
	}

	man, err := GetBySlug(db, slug, opts.Type)
	if opts.Operation == Install {
		if err == nil {
			return nil, ErrAlreadyExists
		}
		if !couchdb.IsNotFoundError(err) {
			return nil, err
		}
		err = nil
		switch opts.Type {
		case Webapp:
			man = &WebappManifest{}
		case Konnector:
			man = &konnManifest{}
		}
	} else if couchdb.IsNotFoundError(err) {
		return nil, ErrNotFound
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
		src, err = url.Parse(man.Source())
	}
	if err != nil {
		return nil, err
	}

	var fetcher Fetcher
	switch src.Scheme {
	case "git":
		fetcher = newGitFetcher(fs, manFilename)
	default:
		return nil, ErrNotSupportedSource
	}

	return &Installer{
		fetcher: fetcher,
		db:      db,
		fs:      fs,

		man:  man,
		src:  src,
		slug: slug,

		errc: make(chan error, 1),
		manc: make(chan Manifest, 2),
	}, nil
}

// Install will install the application linked to the installer. It will
// report its progress or error (see Poll method).
func (i *Installer) Install() {
	defer i.endOfProc()
	i.man, i.err = i.install()
	return
}

// Update will update the application linked to the installer. It will
// report its progress or error (see Poll method).
func (i *Installer) Update() {
	defer i.endOfProc()
	if state := i.man.State(); state != Ready && state != Errored {
		i.man, i.err = nil, ErrBadState
	} else {
		i.man, i.err = i.update()
	}
	return
}

// Delete will remove the application linked to the installer.
func (i *Installer) Delete() (Manifest, error) {
	if state := i.man.State(); state != Ready && state != Errored {
		return nil, ErrBadState
	}
	if err := deleteManifest(i.db, i.man); err != nil {
		return nil, err
	}
	if err := i.fs.RemoveAll(i.baseDirName()); err != nil {
		return nil, err
	}
	return i.man, nil
}

func (i *Installer) endOfProc() {
	man, err := i.man, i.err
	if man == nil || err == ErrBadState {
		i.errc <- err
		return
	}
	if err != nil {
		man.SetState(Errored)
		man.SetError(err)
		updateManifest(i.db, man)
		i.errc <- err
		return
	}
	man.SetState(Ready)
	updateManifest(i.db, man)
	i.manc <- i.man
}

// install will perform the installation of an application. It returns the
// freshly fetched manifest from the source along with a possible error in case
// the installation went wrong.
//
// Note that the fetched manifest is returned even if an error occurred while
// upgrading.
func (i *Installer) install() (Manifest, error) {
	man := i.man
	if err := i.ReadManifest(Installing, man); err != nil {
		return nil, err
	}

	if err := createManifest(i.db, man); err != nil {
		return man, err
	}

	i.manc <- man

	err := i.fs.MkdirAll(i.baseDirName(), 0755)
	if err != nil {
		return man, err
	}

	err = i.fetcher.Fetch(i.src, i.baseDirName())
	return man, err
}

// update will perform the update of an already installed application. It
// returns the freshly fetched manifest from the source along with a possible
// error in case the update went wrong.
//
// Note that the fetched manifest is returned even if an error occurred while
// upgrading.
func (i *Installer) update() (Manifest, error) {
	man := i.man

	if err := i.ReadManifest(Upgrading, man); err != nil {
		return man, err
	}

	if err := updateManifest(i.db, man); err != nil {
		return man, err
	}

	i.manc <- man

	err := i.fetcher.Fetch(i.src, i.baseDirName())
	return man, err
}

func (i *Installer) baseDirName() string {
	return path.Join("/", i.slug)
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
		done := man.State() == Ready
		return man, done, nil
	case err := <-i.errc:
		return nil, false, err
	}
}

func updateManifest(db couchdb.Database, man Manifest) error {
	err := permissions.DestroyApp(db, man.Slug())
	if err != nil && !couchdb.IsNotFoundError(err) {
		return err
	}
	err = couchdb.UpdateDoc(db, man)
	if err != nil {
		return err
	}
	_, err = permissions.CreateAppSet(db, man.Slug(), man.Permissions())
	return err
}

func createManifest(db couchdb.Database, man Manifest) error {
	if err := couchdb.CreateNamedDocWithDB(db, man); err != nil {
		return err
	}
	_, err := permissions.CreateAppSet(db, man.Slug(), man.Permissions())
	return err
}

func deleteManifest(db couchdb.Database, man Manifest) error {
	err := permissions.DestroyApp(db, man.Slug())
	if err != nil && !couchdb.IsNotFoundError(err) {
		return err
	}
	return couchdb.DeleteDoc(db, man)
}
