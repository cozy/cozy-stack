package app_test

import (
	"fmt"
	"testing"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/stretchr/testify/assert"
)

func TestListWebappsWithPagination(t *testing.T) {
	of := true
	testInstance, err := lifecycle.Create(&lifecycle.Options{
		Domain:             "test-list-webapp-pagination",
		ContextName:        "foocontext",
		OnboardingFinished: &of,
	})
	assert.NoError(t, err)

	defer func() {
		_ = lifecycle.Destroy(testInstance.Domain)
	}()

	// Install the apps
	for _, a := range []string{"drive", "home", "settings"} {
		installer, err := app.NewInstaller(testInstance, app.Copier(consts.WebappType, testInstance), &app.InstallerOptions{
			Operation:  app.Install,
			Type:       consts.WebappType,
			SourceURL:  fmt.Sprintf("registry://%s", a),
			Slug:       a,
			Registries: testInstance.Registries(),
		})
		assert.NoError(t, err)
		_, err = installer.RunSync()
		assert.NoError(t, err)
	}

	// Test the pagination
	apps, next, err := app.ListWebappsWithPagination(testInstance, 1, "")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(apps))
	assert.NotEqual(t, "", next)

	// Retreiving the first two apps
	apps, next, err = app.ListWebappsWithPagination(testInstance, 2, "")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(apps))
	assert.NotEqual(t, "", next)

	// Same limit as before, we should get the last app
	apps, next, err = app.ListWebappsWithPagination(testInstance, 2, next)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(apps))
	assert.Equal(t, "", next)

	// With limit = 0, the default limit will be applied, we should get all the
	// apps
	apps, next, err = app.ListWebappsWithPagination(testInstance, 0, next)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(apps))
	assert.Equal(t, "", next)
}
