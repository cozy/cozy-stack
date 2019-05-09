package app_test

import (
	"fmt"
	"path"
	"testing"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/stretchr/testify/assert"
)

func TestWebappInstallBadSlug(t *testing.T) {
	manGen = manifestWebapp
	manName = app.WebappManifestName
	_, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		SourceURL: "git://foo.bar",
	})
	if assert.Error(t, err) {
		assert.Equal(t, app.ErrInvalidSlugName, err)
	}

	_, err = app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "coucou/",
		SourceURL: "git://foo.bar",
	})
	if assert.Error(t, err) {
		assert.Equal(t, app.ErrInvalidSlugName, err)
	}
}

func TestWebappInstallBadAppsSource(t *testing.T) {
	manGen = manifestWebapp
	manName = app.WebappManifestName
	_, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "app3",
		SourceURL: "foo://bar.baz",
	})
	if assert.Error(t, err) {
		assert.Equal(t, app.ErrNotSupportedSource, err)
	}

	_, err = app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "app4",
		SourceURL: "git://bar  .baz",
	})
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "invalid character")
	}

	_, err = app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "app5",
		SourceURL: "",
	})
	if assert.Error(t, err) {
		assert.Equal(t, app.ErrMissingSource, err)
	}
}

func TestWebappInstallSuccessful(t *testing.T) {
	manGen = manifestWebapp
	manName = app.WebappManifestName

	doUpgrade(1)

	inst, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "local-cozy-mini",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var state app.State
	var man app.Manifest
	for {
		var done bool
		var err2 error
		man, done, err2 = inst.Poll()
		if !assert.NoError(t, err2) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, app.Installing, man.State()) {
				return
			}
		} else if state == app.Installing {
			if !assert.EqualValues(t, app.Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}

	ok, err := afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"), []byte("1.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")

	inst2, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "local-cozy-mini",
		SourceURL: "git://localhost/",
	})
	assert.Nil(t, inst2)
	assert.Equal(t, app.ErrAlreadyExists, err)
}

func TestWebappInstallSuccessfulWithExtraPerms(t *testing.T) {
	manifest1 := func() string {
		return ` {
"description": "A mini app to test cozy-stack-v2",
"developer": {
	"name": "Cozy",
	"url": "cozy.io"
},
"license": "MIT",
"name": "mini-app",
"permissions": {
  "rule0": {
	"type": "io.cozy.files",
	"verbs": ["GET"],
	"values": ["foobar"]
  }
},
"slug": "mini-test-perms",
"type": "webapp",
"version": "1.0.0"
}`
	}

	manifest2 := func() string {
		return ` {
"description": "A mini app to test cozy-stack-v2",
"developer": {
	"name": "Cozy",
	"url": "cozy.io"
},
"license": "MIT",
"name": "mini-app",
"permissions": {
	"rule0": {
		"type": "io.cozy.files",
		"verbs": ["GET"],
		"values": ["foobar"]
	}
},
"slug": "mini-test-perms",
"type": "webapp",
"version": "2.0.0"
}`
	}
	manGen = manifest1
	manName = app.WebappManifestName
	finished := true

	instance, err := lifecycle.Create(&lifecycle.Options{
		Domain:             "test-keep-perms",
		OnboardingFinished: &finished,
	})
	assert.NoError(t, err)

	defer func() { _ = lifecycle.Destroy("test-keep-perms") }()

	inst, err := app.NewInstaller(instance, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "mini-test-perms",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}

	man, err := inst.RunSync()
	assert.NoError(t, err)
	assert.Contains(t, man.Version(), "1.0.0")

	// Altering permissions by adding a value and a verb
	newPerms, err := permission.UnmarshalScopeString("io.cozy.files:GET,POST:foobar,foobar2")
	assert.NoError(t, err)

	customRule := permission.Rule{
		Title:  "myCustomRule",
		Verbs:  permission.Verbs(permission.PUT),
		Type:   "io.cozy.custom",
		Values: []string{"myCustomValue"},
	}
	newPerms = append(newPerms, customRule)

	_, err = permission.UpdateWebappSet(instance, "mini-test-perms", newPerms)
	assert.NoError(t, err)

	p1, err := permission.GetForWebapp(instance, "mini-test-perms")
	assert.NoError(t, err)
	assert.False(t, p1.Permissions.HasSameRules(man.Permissions()))

	// Update the app
	manGen = manifest2
	inst2, err := app.NewInstaller(instance, fs, &app.InstallerOptions{
		Operation: app.Update,
		Type:      consts.WebappType,
		Slug:      "mini-test-perms",
		SourceURL: "git://localhost/",
	})
	assert.NoError(t, err)

	man, err = inst2.RunSync()
	assert.NoError(t, err)

	p2, err := permission.GetForWebapp(instance, "mini-test-perms")
	assert.NoError(t, err)
	assert.Contains(t, man.Version(), "2.0.0")
	// Assert the rules were kept
	assert.False(t, p2.Permissions.HasSameRules(man.Permissions()))
	assert.True(t, p1.Permissions.HasSameRules(p2.Permissions))
}

func TestWebappUpgradeNotExist(t *testing.T) {
	manGen = manifestWebapp
	manName = app.WebappManifestName
	inst, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Update,
		Type:      consts.WebappType,
		Slug:      "cozy-app-not-exist",
	})
	assert.Nil(t, inst)
	assert.Equal(t, app.ErrNotFound, err)

	inst, err = app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Delete,
		Type:      consts.WebappType,
		Slug:      "cozy-app-not-exist",
	})
	assert.Nil(t, inst)
	assert.Equal(t, app.ErrNotFound, err)
}

func TestWebappInstallWithUpgrade(t *testing.T) {
	manGen = manifestWebapp
	manName = app.WebappManifestName

	defer func() {
		localServices = ""
	}()

	localServices = `{
		"service1": {

			"type": "node",
			"file": "/services/service1.js",
			"trigger": "@cron 0 0 0 * * *"
		}
	}`

	doUpgrade(1)

	inst, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "cozy-app-b",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}

	man, err := inst.RunSync()
	assert.NoError(t, err)

	ok, err := afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"), []byte("1.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	version1 := man.Version()

	manWebapp := man.(*app.WebappManifest)
	if assert.NotNil(t, manWebapp.Services["service1"]) {
		service1 := manWebapp.Services["service1"]
		assert.Equal(t, "/services/service1.js", service1.File)
		assert.Equal(t, "@cron 0 0 0 * * *", service1.TriggerOptions)
		assert.Equal(t, "node", service1.Type)
		assert.NotEmpty(t, service1.TriggerID)
	}

	doUpgrade(2)
	localServices = ""

	inst, err = app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Update,
		Type:      consts.WebappType,
		Slug:      "cozy-app-b",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var state app.State
	for {
		var done bool
		man, done, err = inst.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, app.Upgrading, man.State()) {
				return
			}
		} else if state == app.Upgrading {
			if !assert.EqualValues(t, app.Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}
	version2 := man.Version()

	fmt.Println("versions:", version1, version2)

	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"), []byte("2.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	manWebapp = man.(*app.WebappManifest)
	assert.Nil(t, manWebapp.Services["service1"])
}

func TestWebappInstallAndUpgradeWithBranch(t *testing.T) {
	manGen = manifestWebapp
	manName = app.WebappManifestName
	doUpgrade(3)

	inst, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "local-cozy-mini-branch",
		SourceURL: "git://localhost/#branch",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var state app.State
	var man app.Manifest
	for {
		var done bool
		var err2 error
		man, done, err2 = inst.Poll()
		if !assert.NoError(t, err2) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, app.Installing, man.State()) {
				return
			}
		} else if state == app.Installing {
			if !assert.EqualValues(t, app.Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}

	ok, err := afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"), []byte("3.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), "branch.gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The good branch was checked out")

	doUpgrade(4)

	inst, err = app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Update,
		Type:      consts.WebappType,
		Slug:      "local-cozy-mini-branch",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	state = ""
	for {
		var done bool
		var err2 error
		man, done, err2 = inst.Poll()
		if !assert.NoError(t, err2) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, app.Upgrading, man.State()) {
				return
			}
		} else if state == app.Upgrading {
			if !assert.EqualValues(t, app.Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}

	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"), []byte("4.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), "branch.gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The good branch was checked out")

	doUpgrade(5)

	inst, err = app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Update,
		Type:      consts.WebappType,
		Slug:      "local-cozy-mini-branch",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}

	man, err = inst.RunSync()
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "git://localhost/", man.Source())

	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), app.WebappManifestName+".gz"), []byte("5.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), "branch.gz"))
	assert.NoError(t, err)
	assert.False(t, ok, "The good branch was checked out")
}

func TestWebappInstallFromGithub(t *testing.T) {
	manGen = manifestWebapp
	manName = app.WebappManifestName
	inst, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "github-cozy-mini",
		SourceURL: "git://github.com/nono/cozy-mini.git",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var state app.State
	for {
		man, done, err := inst.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, app.Installing, man.State()) {
				return
			}
		} else if state == app.Installing {
			if !assert.EqualValues(t, app.Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}
}

func TestWebappInstallFromHTTP(t *testing.T) {
	manGen = manifestWebapp
	manName = app.WebappManifestName
	inst, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "http-cozy-drive",
		SourceURL: "https://github.com/cozy/cozy-drive/archive/71e5cde66f754f986afc7111962ed2dd361bd5e4.tar.gz",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var state app.State
	for {
		man, done, err := inst.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, app.Installing, man.State()) {
				return
			}
		} else if state == app.Installing {
			if !assert.EqualValues(t, app.Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}
}

func TestWebappUninstall(t *testing.T) {
	manGen = manifestWebapp
	manName = app.WebappManifestName
	inst1, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "github-cozy-delete",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}
	go inst1.Run()
	for {
		var done bool
		_, done, err = inst1.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if done {
			break
		}
	}
	inst2, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Delete,
		Type:      consts.WebappType,
		Slug:      "github-cozy-delete",
	})
	if !assert.NoError(t, err) {
		return
	}
	_, err = inst2.RunSync()
	if !assert.NoError(t, err) {
		return
	}
	inst3, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Delete,
		Type:      consts.WebappType,
		Slug:      "github-cozy-delete",
	})
	assert.Nil(t, inst3)
	assert.Equal(t, app.ErrNotFound, err)
}

func TestWebappUninstallErrored(t *testing.T) {
	manGen = manifestWebapp
	manName = app.WebappManifestName

	inst1, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "github-cozy-delete-errored",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}
	go inst1.Run()
	for {
		var done bool
		_, done, err = inst1.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if done {
			break
		}
	}

	inst2, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Update,
		Type:      consts.WebappType,
		Slug:      "github-cozy-delete-errored",
		SourceURL: "git://foobar.does.not.exist/",
	})
	if !assert.NoError(t, err) {
		return
	}
	go inst2.Run()
	for {
		var done bool
		_, done, err = inst2.Poll()
		if done || err != nil {
			break
		}
	}
	if !assert.Error(t, err) {
		return
	}

	inst3, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Delete,
		Type:      consts.WebappType,
		Slug:      "github-cozy-delete-errored",
	})
	if !assert.NoError(t, err) {
		return
	}
	_, err = inst3.RunSync()
	if !assert.NoError(t, err) {
		return
	}

	inst4, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Delete,
		Type:      consts.WebappType,
		Slug:      "github-cozy-delete-errored",
	})
	assert.Nil(t, inst4)
	assert.Equal(t, app.ErrNotFound, err)
}

func TestWebappInstallBadType(t *testing.T) {
	manGen = manifestKonnector
	manName = app.KonnectorManifestName

	inst, err := app.NewInstaller(db, fs, &app.InstallerOptions{
		Operation: app.Install,
		Type:      consts.WebappType,
		Slug:      "cozy-bad-type",
		SourceURL: "git://localhost/",
	})
	assert.NoError(t, err)
	_, err = inst.RunSync()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Manifest types are not the same")
}
