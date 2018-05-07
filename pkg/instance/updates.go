package instance

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/registry"
	"github.com/cozy/cozy-stack/pkg/utils"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/robfig/cron"
)

const numUpdaters = 50
const numUpdatersSingleInstance = 4

var log = logger.WithNamespace("updates")
var globalUpdating = make(chan struct{}, 1)

func init() {
	globalUpdating <- struct{}{}
}

type updateCron struct {
	stopped  chan struct{}
	finished chan struct{}
}

// UpdatesOptions is the option handler for updates:
//   - Slugs: allow to filter the application's slugs to update, if empty, all
//     applications are updated
//   - Force: forces the update, even if the user has not activated the auto-
//     update
//   - ForceRegistry: translates the git:// sourced application into
//     registry://
type UpdatesOptions struct {
	Slugs         []string
	Force         bool
	ForceRegistry bool
}

// StartUpdateCron starts the auto update process which launche a full auto
// updates of all the instances existing.
func StartUpdateCron() (utils.Shutdowner, error) {
	autoUpdates := config.GetConfig().AutoUpdates

	if !autoUpdates.Activated {
		return utils.NopShutdown, nil
	}

	u := &updateCron{
		stopped:  make(chan struct{}),
		finished: make(chan struct{}),
	}

	spec := strings.TrimPrefix(autoUpdates.Schedule, "@cron ")
	schedule, err := cron.Parse(spec)
	if err != nil {
		return nil, err
	}

	go func() {
		next := time.Now()
		defer func() { u.finished <- struct{}{} }()
		for {
			next = schedule.Next(next)
			select {
			case <-time.After(-time.Since(next)):
				if err := UpdateAll(&UpdatesOptions{}); err != nil {
					log.Error("Could not update all:", err)
				}
			case <-u.stopped:
				return
			}
		}
	}()

	return u, nil
}

func (u *updateCron) Shutdown(ctx context.Context) error {
	fmt.Print("  shutting down updaters...")
	u.stopped <- struct{}{}
	select {
	case <-ctx.Done():
		fmt.Println("timeouted.")
	case <-u.finished:
		fmt.Println("ok.")
	}
	return nil
}

// UpdateAll starts the auto-updates process for all instances. The slugs
// parameters can be used optionnaly to filter (whitelist) the applications'
// slug to update.
func UpdateAll(opts *UpdatesOptions) error {
	<-globalUpdating
	defer func() {
		globalUpdating <- struct{}{}
	}()

	insc := make(chan *apps.Installer)
	errc := make(chan error)

	var g sync.WaitGroup
	g.Add(numUpdaters)

	for i := 0; i < numUpdaters; i++ {
		go func() {
			for installer := range insc {
				_, err := installer.RunSync()
				if err != nil {
					err = fmt.Errorf("Could not update app %s: %s", installer.Slug(), err)
				}
				errc <- err
			}
			g.Done()
		}()
	}

	go func() {
		// TODO: filter instances that are AutoUpdate only
		ForeachInstances(func(inst *Instance) error {
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
func UpdateInstance(inst *Instance, opts *UpdatesOptions) error {
	insc := make(chan *apps.Installer)
	errc := make(chan error)

	var g sync.WaitGroup
	g.Add(numUpdatersSingleInstance)

	for i := 0; i < numUpdatersSingleInstance; i++ {
		go func() {
			for installer := range insc {
				_, err := installer.RunSync()
				if err != nil {
					err = fmt.Errorf("Could not update app %s: %s", installer.Slug(), err)
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

func installerPush(inst *Instance, insc chan *apps.Installer, errc chan error, opts *UpdatesOptions) {
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
				errc <- fmt.Errorf("Could not create installer for webapp %s: %s", app.Slug(), err)
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
				errc <- fmt.Errorf("Could not create installer for konnector %s: %s", app.Slug(), err)
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

func createInstaller(inst *Instance, registries []*url.URL, man apps.Manifest, opts *UpdatesOptions) (*apps.Installer, error) {
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
