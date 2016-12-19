package apps

import (
	"encoding/json"
	"io"
	"net/url"
	"path"
	"regexp"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

var slugReg = regexp.MustCompile(`^[A-Za-z0-9\-]+$`)

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

	if man.Contexts == nil {
		man.Contexts = make(Contexts)
		man.Contexts["/"] = Context{
			Folder: "/",
			Index:  "index.html",
			Public: false,
		}
	}

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
