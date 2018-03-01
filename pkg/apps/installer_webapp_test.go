package apps_test

import (
	"fmt"
	"path"
	"testing"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/stretchr/testify/assert"
)

func TestWebappInstallBadSlug(t *testing.T) {
	manGen = manifestWebapp
	manName = apps.WebappManifestName
	_, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		SourceURL: "git://foo.bar",
	})
	if assert.Error(t, err) {
		assert.Equal(t, apps.ErrInvalidSlugName, err)
	}

	_, err = apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		Slug:      "coucou/",
		SourceURL: "git://foo.bar",
	})
	if assert.Error(t, err) {
		assert.Equal(t, apps.ErrInvalidSlugName, err)
	}
}

func TestWebappInstallBadAppsSource(t *testing.T) {
	manGen = manifestWebapp
	manName = apps.WebappManifestName
	_, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		Slug:      "app3",
		SourceURL: "foo://bar.baz",
	})
	if assert.Error(t, err) {
		assert.Equal(t, apps.ErrNotSupportedSource, err)
	}

	_, err = apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		Slug:      "app4",
		SourceURL: "git://bar  .baz",
	})
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "invalid character")
	}

	_, err = apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		Slug:      "app5",
		SourceURL: "",
	})
	if assert.Error(t, err) {
		assert.Equal(t, apps.ErrMissingSource, err)
	}
}

func TestWebappInstallSuccessful(t *testing.T) {
	manGen = manifestWebapp
	manName = apps.WebappManifestName

	doUpgrade(1)

	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		Slug:      "local-cozy-mini",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var state apps.State
	var man apps.Manifest
	for {
		var done bool
		var err2 error
		man, done, err2 = inst.Poll()
		if !assert.NoError(t, err2) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, apps.Installing, man.State()) {
				return
			}
		} else if state == apps.Installing {
			if !assert.EqualValues(t, apps.Ready, man.State()) {
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

	ok, err := afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"), []byte("1.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")

	inst2, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		Slug:      "local-cozy-mini",
		SourceURL: "git://localhost/",
	})
	assert.Nil(t, inst2)
	assert.Equal(t, apps.ErrAlreadyExists, err)
}

func TestWebappUpgradeNotExist(t *testing.T) {
	manGen = manifestWebapp
	manName = apps.WebappManifestName
	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Update,
		Type:      apps.Webapp,
		Slug:      "cozy-app-not-exist",
	})
	assert.Nil(t, inst)
	assert.Equal(t, apps.ErrNotFound, err)

	inst, err = apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Delete,
		Type:      apps.Webapp,
		Slug:      "cozy-app-not-exist",
	})
	assert.Nil(t, inst)
	assert.Equal(t, apps.ErrNotFound, err)
}

func TestWebappInstallWithUpgrade(t *testing.T) {
	manGen = manifestWebapp
	manName = apps.WebappManifestName

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

	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		Slug:      "cozy-app-b",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}

	man, err := inst.RunSync()
	assert.NoError(t, err)

	ok, err := afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"), []byte("1.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	version1 := man.Version()

	manWebapp := man.(*apps.WebappManifest)
	if assert.NotNil(t, manWebapp.Services["service1"]) {
		service1 := manWebapp.Services["service1"]
		assert.Equal(t, "/services/service1.js", service1.File)
		assert.Equal(t, "@cron 0 0 0 * * *", service1.TriggerOptions)
		assert.Equal(t, "node", service1.Type)
		assert.NotEmpty(t, service1.TriggerID)
	}

	doUpgrade(2)
	localServices = ""

	inst, err = apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Update,
		Type:      apps.Webapp,
		Slug:      "cozy-app-b",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var state apps.State
	for {
		var done bool
		man, done, err = inst.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, apps.Upgrading, man.State()) {
				return
			}
		} else if state == apps.Upgrading {
			if !assert.EqualValues(t, apps.Ready, man.State()) {
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

	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"), []byte("2.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	manWebapp = man.(*apps.WebappManifest)
	assert.Nil(t, manWebapp.Services["service1"])
}

func TestWebappInstallAndUpgradeWithBranch(t *testing.T) {
	manGen = manifestWebapp
	manName = apps.WebappManifestName
	doUpgrade(3)

	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		Slug:      "local-cozy-mini-branch",
		SourceURL: "git://localhost/#branch",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var state apps.State
	var man apps.Manifest
	for {
		var done bool
		var err2 error
		man, done, err2 = inst.Poll()
		if !assert.NoError(t, err2) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, apps.Installing, man.State()) {
				return
			}
		} else if state == apps.Installing {
			if !assert.EqualValues(t, apps.Ready, man.State()) {
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

	ok, err := afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"), []byte("3.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), "branch.gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The good branch was checked out")

	doUpgrade(4)

	inst, err = apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Update,
		Type:      apps.Webapp,
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
			if !assert.EqualValues(t, apps.Upgrading, man.State()) {
				return
			}
		} else if state == apps.Upgrading {
			if !assert.EqualValues(t, apps.Ready, man.State()) {
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

	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"), []byte("4.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), "branch.gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The good branch was checked out")

	doUpgrade(5)

	inst, err = apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Update,
		Type:      apps.Webapp,
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

	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), apps.WebappManifestName+".gz"), []byte("5.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), "branch.gz"))
	assert.NoError(t, err)
	assert.False(t, ok, "The good branch was checked out")
}

func TestWebappInstallFromGithub(t *testing.T) {
	manGen = manifestWebapp
	manName = apps.WebappManifestName
	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		Slug:      "github-cozy-mini",
		SourceURL: "git://github.com/nono/cozy-mini.git",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var state apps.State
	for {
		man, done, err := inst.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, apps.Installing, man.State()) {
				return
			}
		} else if state == apps.Installing {
			if !assert.EqualValues(t, apps.Ready, man.State()) {
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

func TestWebappInstallFromGitlab(t *testing.T) {
	manGen = manifestWebapp
	manName = apps.WebappManifestName
	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		Slug:      "gitlab-cozy-mini",
		SourceURL: "git://framagit.org/nono/cozy-mini.git",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var state apps.State
	for {
		man, done, err := inst.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, apps.Installing, man.State()) {
				return
			}
		} else if state == apps.Installing {
			if !assert.EqualValues(t, apps.Ready, man.State()) {
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
	manName = apps.WebappManifestName
	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
		Slug:      "http-cozy-drive",
		SourceURL: "https://github.com/cozy/cozy-drive/archive/71e5cde66f754f986afc7111962ed2dd361bd5e4.tar.gz",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var state apps.State
	for {
		man, done, err := inst.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, apps.Installing, man.State()) {
				return
			}
		} else if state == apps.Installing {
			if !assert.EqualValues(t, apps.Ready, man.State()) {
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
	manName = apps.WebappManifestName
	inst1, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
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
	inst2, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Delete,
		Type:      apps.Webapp,
		Slug:      "github-cozy-delete",
	})
	if !assert.NoError(t, err) {
		return
	}
	_, err = inst2.RunSync()
	if !assert.NoError(t, err) {
		return
	}
	inst3, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Delete,
		Type:      apps.Webapp,
		Slug:      "github-cozy-delete",
	})
	assert.Nil(t, inst3)
	assert.Equal(t, apps.ErrNotFound, err)
}

func TestWebappUninstallErrored(t *testing.T) {
	manGen = manifestWebapp
	manName = apps.WebappManifestName

	inst1, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Webapp,
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

	inst2, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Update,
		Type:      apps.Webapp,
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

	inst3, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Delete,
		Type:      apps.Webapp,
		Slug:      "github-cozy-delete-errored",
	})
	if !assert.NoError(t, err) {
		return
	}
	_, err = inst3.RunSync()
	if !assert.NoError(t, err) {
		return
	}

	inst4, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Delete,
		Type:      apps.Webapp,
		Slug:      "github-cozy-delete-errored",
	})
	assert.Nil(t, inst4)
	assert.Equal(t, apps.ErrNotFound, err)
}
