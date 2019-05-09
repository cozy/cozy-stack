package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"time"

	"github.com/Masterminds/semver"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/hooks"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/registry"
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
	fs       appfs.Copier
	db       prefixer.Prefixer
	endState State

	overridenParameters *json.RawMessage
	permissionsAcked    bool

	man  Manifest
	src  *url.URL
	slug string

	manc chan Manifest
	log  *logrus.Entry
}

// InstallerOptions provides the slug name of the application along with the
// source URL.
type InstallerOptions struct {
	Type             consts.AppType
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
	Fetch(src *url.URL, fs appfs.Copier, man Manifest) error
}

// NewInstaller creates a new Installer
func NewInstaller(db prefixer.Prefixer, fs appfs.Copier, opts *InstallerOptions) (*Installer, error) {
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

	var installType string
	switch opts.Operation {
	case Install:
		installType = "install"
	case Update:
		installType = "update"
	case Delete:
		installType = "delete"
	}

	log := logger.WithDomain(db.DomainName()).WithFields(logrus.Fields{
		"nspace":        "apps",
		"slug":          man.Slug(),
		"version_start": man.Version(),
		"type":          installType,
	})

	var manFilename string
	switch man.AppType() {
	case consts.WebappType:
		manFilename = WebappManifestName
	case consts.KonnectorType:
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
		case consts.WebappType:
			man = &WebappManifest{
				DocID:   consts.Apps + "/" + slug,
				DocSlug: slug,
			}
		case consts.KonnectorType:
			man = &KonnManifest{
				DocID:   consts.Konnectors + "/" + slug,
				DocSlug: slug,
			}
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
// Poll method) and should be run asynchronously inside a new goroutine:
// `go installer.Run()`.
func (i *Installer) Run() {
	if err := i.run(); err != nil {
		i.man.SetError(err)
		realtime.GetHub().Publish(i.db, realtime.EventUpdate, i.man.Clone(), nil)
	}
	i.notifyChannel()
}

// RunSync does the same work as Run but can be used synchronously.
func (i *Installer) RunSync() (Manifest, error) {
	i.manc = nil
	if err := i.run(); err != nil {
		return nil, err
	}
	return i.man.Clone().(Manifest), nil
}

func (i *Installer) run() (err error) {
	if i.man == nil {
		panic("Manifest is nil")
	}
	defer func() {
		if err != nil {
			i.log.Errorf("Could not commit installer process: %s", err)
		} else {
			i.log.Infof("Successful installer process: %s", i.man.Version())
		}
	}()
	i.log.Info("Start")
	switch i.op {
	case Install:
		return i.install()
	case Update:
		return i.update()
	case Delete:
		return i.delete()
	default:
		panic("Unknown operation")
	}
}

// install will perform the installation of an application. It returns the
// freshly fetched manifest from the source along with a possible error in case
// the installation went wrong.
//
// Note that the fetched manifest is returned even if an error occurred while
// upgrading.
func (i *Installer) install() error {
	args := []string{i.db.DomainName(), i.slug}
	return hooks.Execute("install-app", args, func() error {
		newManifest, err := i.ReadManifest(Installing)
		if err != nil {
			return err
		}
		i.man = newManifest
		i.sendRealtimeEvent()
		i.notifyChannel()
		if err := i.fetcher.Fetch(i.src, i.fs, i.man); err != nil {
			return err
		}
		i.man.SetState(i.endState)
		return i.man.Create(i.db)
	})
}

// checkSkipPermissions checks if the instance contexts is configured to skip
// permissions
func (i *Installer) checkSkipPermissions() (bool, error) {
	domain := i.Domain()
	if domain == prefixer.UnknownDomainName {
		return false, nil
	}

	inst, err := instance.GetFromCouch(domain)
	if err != nil {
		return false, err
	}
	ctxSettings, err := inst.SettingsContext()
	if err != nil {
		return false, err
	}

	sp, ok := ctxSettings["permissions_skip_verification"]
	if !ok {
		return false, nil
	}

	return sp.(bool), nil
}

// update will perform the update of an already installed application. It
// returns the freshly fetched manifest from the source along with a possible
// error in case the update went wrong.
//
// Note that the fetched manifest is returned even if an error occurred while
// upgrading.
func (i *Installer) update() error {
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
	availableVersion := ""
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
			// Check if we are going to skip the permissions
			skip, err := i.checkSkipPermissions()
			if err != nil {
				return err
			}
			if !skip {
				makeUpdate = false
				availableVersion = newManifest.Version()
			}
		}
	}

	oldTermsVersion := oldManifest.Terms().Version
	newTermsVersion := newManifest.Terms().Version

	termsAdded := oldTermsVersion == "" && newTermsVersion != ""
	termsUpdated := oldTermsVersion != newTermsVersion

	if (termsAdded || termsUpdated) && !i.permissionsAcked {
		makeUpdate = false
		availableVersion = newManifest.Version()
	}

	extraPerms := permission.Set{}
	var alteredPerms *permission.Permission
	// The "extraPerms" set represents the post-install alterations of the
	// permissions between the oldManifest and the current permissions.
	//
	// Even if makeUpdate is false, we are going to update the manifest document
	// to set an AvailableVersion. In this case, the current webapp/konnector
	// perms will be reapplied and custom ones will be lost if we don't rewrite
	// them.
	inst, err := instance.GetFromCouch(i.Domain())
	if err == nil {
		// Check if perms were added on the old manifest
		if i.man.AppType() == consts.WebappType {
			alteredPerms, err = permission.GetForWebapp(inst, i.man.Slug())
		} else if i.man.AppType() == consts.KonnectorType {
			alteredPerms, err = permission.GetForKonnector(inst, i.man.Slug())
		}
		if err != nil {
			return err
		}
	}

	if alteredPerms != nil {
		extraPerms, err = permission.Diff(oldManifest.Permissions(), alteredPerms.Permissions)
		if err != nil {
			return err
		}
	}

	if makeUpdate {
		i.man = newManifest
		i.sendRealtimeEvent()
		i.notifyChannel()
		if err := i.fetcher.Fetch(i.src, i.fs, i.man); err != nil {
			return err
		}
		i.man.SetAvailableVersion("")
		i.man.SetState(i.endState)
	} else {
		i.man.SetSource(i.src)
		if availableVersion != "" {
			i.man.SetAvailableVersion(availableVersion)
		}
		i.sendRealtimeEvent()
		i.notifyChannel()
	}

	return i.man.Update(i.db, extraPerms)
}

func (i *Installer) notifyChannel() {
	if i.manc != nil {
		i.manc <- i.man.Clone().(Manifest)
	}
}

func (i *Installer) delete() error {
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

	var buf bytes.Buffer
	tee := io.TeeReader(r, &buf)

	newManifest, err := i.man.ReadManifest(io.LimitReader(tee, ManifestMaxSize), i.slug, i.src.String())
	if err != nil {
		return nil, err
	}
	newManifest.SetState(state)

	shouldOverrideParameters := (i.overridenParameters != nil &&
		i.man.AppType() == consts.KonnectorType &&
		i.src.Scheme != "registry")
	if shouldOverrideParameters {
		if m, ok := newManifest.(*KonnManifest); ok {
			m.Parameters = i.overridenParameters
		}
	}

	// Checking the new manifest apptype to prevent human mistakes (like asking
	// a konnector installation instead of a webapp)
	newAppType := struct {
		AppType string `json:"type"`
	}{}

	var newManifestAppType consts.AppType
	if err = json.Unmarshal(buf.Bytes(), &newAppType); err == nil {
		if newAppType.AppType == "konnector" {
			newManifestAppType = consts.KonnectorType
		}
		if newAppType.AppType == "webapp" {
			newManifestAppType = consts.WebappType
		}
	}

	appTypesEmpty := i.man.AppType() == 0 || newManifestAppType == 0
	appTypesMismatch := i.man.AppType() != newManifestAppType

	if !appTypesEmpty && appTypesMismatch {
		return nil, fmt.Errorf("Manifest types are not the sames. Expected %d, got %d. Are you sure of %s type ? (konnector/webapp)", i.man.AppType(), newManifestAppType, i.man.Slug())
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

// DoLazyUpdate tries to update an application before using it
func DoLazyUpdate(db prefixer.Prefixer, man Manifest, copier appfs.Copier, registries []*url.URL) Manifest {
	src, err := url.Parse(man.Source())
	if err != nil || src.Scheme != "registry" {
		return man
	}
	var v *registry.Version
	channel, _ := getRegistryChannel(src)
	v, errv := registry.GetLatestVersion(man.Slug(), channel, registries)
	if errv != nil || v.Version == man.Version() {
		return man
	}
	if man.AvailableVersion() != "" && v.Version == man.AvailableVersion() {
		return man
	}
	if channel == "stable" && !isMoreRecent(man.Version(), v.Version) {
		return man
	}
	inst, err := NewInstaller(db, copier, &InstallerOptions{
		Operation:  Update,
		Manifest:   man,
		Registries: registries,
		SourceURL:  src.String(),
	})
	if err != nil {
		return man
	}
	newman, err := inst.RunSync()
	if err != nil {
		return man
	}
	return newman
}

// isMoreRecent returns true if b is greater than a
func isMoreRecent(a, b string) bool {
	vA, err := semver.NewVersion(a)
	if err != nil {
		return true
	}
	vB, err := semver.NewVersion(b)
	if err != nil {
		return false
	}
	return vB.GreaterThan(vA)
}

func isPlatformApp(man Manifest) bool {
	if man.AppType() != consts.WebappType {
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
