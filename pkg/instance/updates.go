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
				if err := UpdateAll(false); err != nil {
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
func UpdateAll(force bool, slugs ...string) error {
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
				errc <- err
			}
			g.Done()
		}()
	}

	go func() {
		// TODO: filter instances that are AutoUpdate only
		ForeachInstances(func(inst *Instance) error {
			if force || !inst.NoAutoUpdate {
				installerPush(inst, insc, errc, slugs...)
			}
			return nil
		})
		close(insc)
	}()

	var errm error
	go func() {
		for err := range errc {
			if err != nil {
				errm = multierror.Append(errm, err)
			}
		}
	}()

	g.Wait()
	close(errc)

	return errm
}

// UpdateInstance starts the auto-update process on the given instance. The
// slugs parameters can be used to filter (whitelist) the applications' slug
func UpdateInstance(inst *Instance, slugs ...string) error {
	insc := make(chan *apps.Installer)
	errc := make(chan error)

	var g sync.WaitGroup
	g.Add(numUpdatersSingleInstance)

	for i := 0; i < numUpdatersSingleInstance; i++ {
		go func() {
			for installer := range insc {
				_, err := installer.RunSync()
				errc <- err
			}
			g.Done()
		}()
	}

	go func() {
		installerPush(inst, insc, errc, slugs...)
		close(insc)
	}()

	var errm error
	go func() {
		for err := range errc {
			if err != nil {
				errm = multierror.Append(errm, err)
			}
		}
	}()

	g.Wait()
	close(errc)

	return errm
}

func installerPush(inst *Instance, insc chan *apps.Installer, errc chan error, slugs ...string) {
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
			if filterSlug(app.Slug(), slugs) {
				continue
			}
			installer, err := createInstaller(inst, registries, app)
			if err != nil {
				errc <- err
			} else {
				insc <- installer
			}
		}
	}()

	go func() {
		defer g.Done()
		konnectors, err := apps.ListKonnectors(inst)
		if err != nil {
			errc <- err
			return
		}
		for _, app := range konnectors {
			if filterSlug(app.Slug(), slugs) {
				continue
			}
			installer, err := createInstaller(inst, registries, app)
			if err != nil {
				errc <- err
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

func createInstaller(inst *Instance, registries []*url.URL, man apps.Manifest) (*apps.Installer, error) {
	return apps.NewInstaller(inst, inst.AppsCopier(man.AppType()),
		&apps.InstallerOptions{
			Operation:  apps.Update,
			Manifest:   man,
			Registries: registries,
		},
	)
}
