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
)

const (
	// ManifestDocType is manifest type
	ManifestDocType = "io.cozy.manifests"
	// ManifestMaxSize is the manifest maximum size
	ManifestMaxSize = 2 << (2 * 10) // 2MB
)

// AppsDirectory is the name of the directory in which apps are stored
const AppsDirectory = "/_cozyapps"

type State string

const (
	Available    State = "available"
	Installing         = "installing"
	Upgrading          = "upgrading"
	Uninstalling       = "uninstalling"
	Errored            = "errored"
	Ready              = "ready"
)

var slugReg = regexp.MustCompile(`[A-Za-z0-9\\-]`)

var (
	ErrBadState = errors.New("Application is not in valid state to perform this operation")
)

type Client interface {
	FetchManifest() (io.ReadCloser, error)
	Fetch(appdir string) error
}

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
		cli = NewGitClient(vfsC, src)
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

	err = i.cli.Fetch(appdir)
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
		err = errors.New("Manifest is already defined")
		return
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
		return errors.New("Manifest not defined")
	}

	newman.SetID(oldman.ID())
	newman.SetRev(oldman.Rev())

	return couchdb.UpdateDoc(i.db, newman)
}

func (i *Installer) WaitManifest() (man *Manifest, err error) {
	select {
	case man = <-i.manc:
		return
	case err = <-i.errc:
		return
	}
}
