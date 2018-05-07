package updates

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/registry"
	multierror "github.com/hashicorp/go-multierror"
)

const numUpdaters = 4
const numUpdatersSingleInstance = 4

func init() {
	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType:   "updates",
		Concurrency:  1,
		MaxExecCount: 1,
		Timeout:      1 * time.Hour,
		WorkerFunc:   Worker,
	})
}

// Options is the option handler for updates:
//   - Slugs: allow to filter the application's slugs to update, if empty, all
//     applications are updated
//   - Force: forces the update, even if the user has not activated the auto-
//     update
//   - ForceRegistry: translates the git:// sourced application into
//     registry://
type Options struct {
	Slugs         []string
	Domain        string
	AllDomains    bool
	Force         bool
	ForceRegistry bool
}

// Worker is the worker method to launch the updates.
func Worker(ctx *jobs.WorkerContext) error {
	var opts Options
	if err := ctx.UnmarshalMessage(&opts); err != nil {
		return err
	}
	if opts.AllDomains {
		return UpdateAll(&opts)
	}
	if opts.Domain != "" {
		inst, err := instance.Get(opts.Domain)
		if err != nil {
			return err
		}
		return UpdateInstance(inst, &opts)
	}
	return nil
}

// UpdateAll starts the auto-updates process for all instances. The slugs
// parameters can be used optionnaly to filter (whitelist) the applications'
// slug to update.
func UpdateAll(opts *Options) error {
	insc := make(chan *apps.Installer)
	errc := make(chan error)

	var g sync.WaitGroup
	g.Add(numUpdaters)

	for i := 0; i < numUpdaters; i++ {
		go func() {
			for installer := range insc {
				_, err := installer.RunSync()
				if err != nil {
					err = fmt.Errorf("Could not update app %s on %q: %s",
						installer.Slug(), installer.Domain(), err)
				}
				errc <- err
			}
			g.Done()
		}()
	}

	go func() {
		// TODO: filter instances that are AutoUpdate only
		instance.ForeachInstances(func(inst *instance.Instance) error {
			if opts.Force || !inst.NoAutoUpdate {
				installerPush(inst, insc, errc, opts)
			}
			return nil
		})
		close(insc)
		g.Wait()
		close(errc)
	}()

	var errm error
	for err := range errc {
		if err != nil {
			errm = multierror.Append(errm, err)
		}
	}

	return errm
}

// UpdateInstance starts the auto-update process on the given instance. The
// slugs parameters can be used to filter (whitelist) the applications' slug
func UpdateInstance(inst *instance.Instance, opts *Options) error {
	insc := make(chan *apps.Installer)
	errc := make(chan error)

	var g sync.WaitGroup
	g.Add(numUpdatersSingleInstance)

	for i := 0; i < numUpdatersSingleInstance; i++ {
		go func() {
			for installer := range insc {
				_, err := installer.RunSync()
				if err != nil {
					err = fmt.Errorf("Could not update app %s on %q: %s",
						installer.Slug(), installer.Domain(), err)
				}
				errc <- err
			}
			g.Done()
		}()
	}

	go func() {
		installerPush(inst, insc, errc, opts)
		close(insc)
		g.Wait()
		close(errc)
	}()

	var errm error
	for err := range errc {
		if err != nil {
			errm = multierror.Append(errm, err)
		}
	}

	return errm
}

func installerPush(inst *instance.Instance, insc chan *apps.Installer, errc chan error, opts *Options) {
	registries, err := inst.Registries()
	if err != nil {
		errc <- err
		return
	}

	var g sync.WaitGroup
	g.Add(2)

	go func() {
		defer g.Done()
		webapps, err := apps.ListWebapps(inst)
		if err != nil {
			errc <- err
			return
		}
		for _, app := range webapps {
			if filterSlug(app.Slug(), opts.Slugs) {
				continue
			}
			installer, err := createInstaller(inst, registries, app, opts)
			if err != nil {
				errc <- fmt.Errorf("Could not create installer for webapp %s on %q: %s",
					app.Slug(), inst.Domain, err)
			} else {
				insc <- installer
			}
		}
	}()

	go func() {
		defer g.Done()
		konnectors, err := apps.ListKonnectors(inst)
		if err != nil {
			errc <- fmt.Errorf("Could not list konnectors: %s", err)
			return
		}
		for _, app := range konnectors {
			if filterSlug(app.Slug(), opts.Slugs) {
				continue
			}
			installer, err := createInstaller(inst, registries, app, opts)
			if err != nil {
				errc <- fmt.Errorf("Could not create installer for konnector %s on %q: %s",
					app.Slug(), inst.Domain, err)
			} else {
				insc <- installer
			}
		}
	}()

	g.Wait()
}

func filterSlug(slug string, slugs []string) bool {
	if len(slugs) == 0 {
		return false
	}
	for _, s := range slugs {
		if s == slug {
			return false
		}
	}
	return true
}

func createInstaller(inst *instance.Instance, registries []*url.URL, man apps.Manifest, opts *Options) (*apps.Installer, error) {
	var sourceURL string
	if opts.ForceRegistry {
		originalSourceURL, err := url.Parse(man.Source())
		if err == nil && (originalSourceURL.Scheme == "git" ||
			originalSourceURL.Scheme == "git+ssh" ||
			originalSourceURL.Scheme == "ssh+git") {
			var channel string
			if man.AppType() == apps.Webapp && strings.HasPrefix(originalSourceURL.Fragment, "build") {
				channel = "dev"
			} else {
				channel = "stable"
			}
			_, err := registry.GetLatestVersion(man.Slug(), channel, registries)
			if err == nil {
				sourceURL = fmt.Sprintf("registry://%s/%s", man.Slug(), channel)
			}
		}
	}
	return apps.NewInstaller(inst, inst.AppsCopier(man.AppType()),
		&apps.InstallerOptions{
			Operation:  apps.Update,
			Manifest:   man,
			Registries: registries,
			SourceURL:  sourceURL,
		},
	)
}
