package instance

import (
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/cozy/cozy-stack/pkg/apps"
	multierror "github.com/hashicorp/go-multierror"
)

const numUpdaters = 50

var updating uint32

func UpdateAll() error {
	if !atomic.CompareAndSwapUint32(&updating, 0, 1) {
		return nil
	}

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
		ForeachInstances(func(inst *Instance) error {
			update(inst, insc, errc)
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

	atomic.SwapUint32(&updating, 0)
	return errm
}

func UpdateInstance(inst *Instance) error {
	insc := make(chan *apps.Installer)
	errc := make(chan error)

	var g sync.WaitGroup
	g.Add(4)

	for i := 0; i < 4; i++ {
		go func() {
			for installer := range insc {
				_, err := installer.RunSync()
				errc <- err
			}
			g.Done()
		}()
	}

	go func() {
		update(inst, insc, errc)
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

func update(inst *Instance, insc chan *apps.Installer, errc chan error) {
	if !inst.AutoUpdate {
		return
	}

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

func createInstaller(inst *Instance, registries []*url.URL, man apps.Manifest) (*apps.Installer, error) {
	return apps.NewInstaller(inst, inst.AppsCopier(man.AppType()),
		&apps.InstallerOptions{
			Operation:  apps.Update,
			Manifest:   man,
			Registries: registries,
		},
	)
}
