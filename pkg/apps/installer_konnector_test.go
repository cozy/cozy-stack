package apps_test

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"path"
	"testing"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/instance/lifecycle"
	"github.com/stretchr/testify/assert"
)

func compressedFileContainsBytes(fs afero.Fs, filename string, content []byte) (ok bool, err error) {
	f, err := fs.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return
	}
	defer gr.Close()
	b, err := ioutil.ReadAll(gr)
	if err != nil {
		return
	}
	ok = bytes.Contains(b, content)
	return
}

func TestKonnectorInstallSuccessful(t *testing.T) {
	manGen = manifestKonnector
	manName = apps.KonnectorManifestName

	doUpgrade(1)

	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      consts.KonnectorType,
		Slug:      "local-konnector",
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

	ok, err := afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), apps.KonnectorManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), apps.KonnectorManifestName+".gz"), []byte("1.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")

	inst2, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      consts.KonnectorType,
		Slug:      "local-konnector",
		SourceURL: "git://localhost/",
	})
	assert.Nil(t, inst2)
	assert.Equal(t, apps.ErrAlreadyExists, err)
}

func TestKonnectorUpgradeNotExist(t *testing.T) {
	manGen = manifestKonnector
	manName = apps.KonnectorManifestName
	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Update,
		Type:      consts.KonnectorType,
		Slug:      "cozy-konnector-not-exist",
	})
	assert.Nil(t, inst)
	assert.Equal(t, apps.ErrNotFound, err)

	inst, err = apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Delete,
		Type:      consts.KonnectorType,
		Slug:      "cozy-konnector-not-exist",
	})
	assert.Nil(t, inst)
	assert.Equal(t, apps.ErrNotFound, err)
}

func TestKonnectorInstallWithUpgrade(t *testing.T) {
	manGen = manifestKonnector
	manName = apps.KonnectorManifestName

	doUpgrade(1)

	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      consts.KonnectorType,
		Slug:      "cozy-konnector-b",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Run()

	var man apps.Manifest
	for {
		var done bool
		man, done, err = inst.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if done {
			break
		}
	}

	ok, err := afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), apps.KonnectorManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), apps.KonnectorManifestName+".gz"), []byte("1.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")

	doUpgrade(2)

	inst, err = apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Update,
		Type:      consts.KonnectorType,
		Slug:      "cozy-konnector-b",
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

	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), apps.KonnectorManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), apps.KonnectorManifestName+".gz"), []byte("2.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
}

func manifestKonnector1() string {
	return `{
  "description": "A mini konnector to test cozy-stack-v2",
  "type": "node",
  "developer": {
    "name": "Bruno",
    "url": "cozy.io"
  },
  "license": "MIT",
  "name": "mini-app",
  "permissions": {
	"bills": {
		"type": "io.cozy.bills"
	}
  },
  "slug": "mini",
  "type": "konnector",
  "version": "1.0.0"
}`
}

func manifestKonnector2() string {
	return `{
  "description": "A mini konnector to test cozy-stack-v2",
  "type": "node",
  "developer": {
    "name": "Bruno",
    "url": "cozy.io"
  },
  "license": "MIT",
  "name": "mini-app",
  "permissions": {
	"bills": {
		"type": "io.cozy.bills"
	},
	"files": {
	  "type": "io.cozy.files"
	}
  },
  "slug": "mini",
  "type": "konnector",
  "version": "2.0.0"
}`
}
func TestKonnectorUpdateSkipPerms(t *testing.T) {
	// Generating test instance
	finished := true
	conf := config.GetConfig()
	conf.Contexts = map[string]interface{}{
		"foocontext": map[string]interface{}{},
	}

	instance, err := lifecycle.Create(&lifecycle.Options{
		Domain:             "test-skip-perms",
		ContextName:        "foocontext",
		OnboardingFinished: &finished,
	})

	defer lifecycle.Destroy("test-skip-perms")

	assert.NoError(t, err)

	manGen = manifestKonnector1
	manName = apps.KonnectorManifestName

	inst, err := apps.NewInstaller(instance, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      consts.KonnectorType,
		Slug:      "cozy-konnector-test-skip",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}

	var man apps.Manifest

	man, err = inst.RunSync()
	konnManifest := man.(*apps.KonnManifest)
	assert.NoError(t, err)
	assert.Empty(t, konnManifest.AvailableVersion())
	assert.Contains(t, konnManifest.Version(), "1.0.0")

	// Will now update. New perms will be added, preventing an upgrade
	manGen = manifestKonnector2

	inst, err = apps.NewInstaller(instance, fs, &apps.InstallerOptions{
		Operation: apps.Update,
		Type:      consts.KonnectorType,
		Slug:      "cozy-konnector-test-skip",
	})
	if !assert.NoError(t, err) {
		return
	}

	man, err = inst.RunSync()
	konnManifest = man.(*apps.KonnManifest)
	assert.NoError(t, err)
	assert.Contains(t, konnManifest.AvailableVersion(), "2.0.0")
	assert.Contains(t, konnManifest.Version(), "1.0.0") // Assert we stayed on our version

	// Change configuration to tell we skip the verifications
	conf.Contexts = map[string]interface{}{
		"foocontext": map[string]interface{}{
			"permissions_skip_verification": true,
		},
	}

	man2, err := inst.RunSync()
	konnManifest = man2.(*apps.KonnManifest)
	assert.NoError(t, err)
	// Assert we upgraded version, and the perms have changed
	assert.False(t, man.Permissions().HasSameRules(man2.Permissions()))
	assert.Empty(t, konnManifest.AvailableVersion())
	assert.Contains(t, konnManifest.Version(), "2.0.0")
}

func TestKonnectorInstallAndUpgradeWithBranch(t *testing.T) {
	manGen = manifestKonnector
	manName = apps.KonnectorManifestName
	doUpgrade(3)

	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      consts.KonnectorType,
		Slug:      "local-konnector-branch",
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

	ok, err := afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), apps.KonnectorManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), apps.KonnectorManifestName+".gz"), []byte("3.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")

	doUpgrade(4)

	inst, err = apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Update,
		Type:      consts.KonnectorType,
		Slug:      "local-konnector-branch",
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

	ok, err = afero.Exists(baseFS, path.Join("/", man.Slug(), man.Version(), apps.KonnectorManifestName+".gz"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = compressedFileContainsBytes(baseFS, path.Join("/", man.Slug(), man.Version(), apps.KonnectorManifestName+".gz"), []byte("4.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
}

func TestKonnectorUninstall(t *testing.T) {
	manGen = manifestKonnector
	manName = apps.KonnectorManifestName
	inst1, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      consts.KonnectorType,
		Slug:      "konnector-delete",
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
		Type:      consts.KonnectorType,
		Slug:      "konnector-delete",
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
		Type:      consts.KonnectorType,
		Slug:      "konnector-delete",
	})
	assert.Nil(t, inst3)
	assert.Equal(t, apps.ErrNotFound, err)
}

func TestKonnectorInstallBadType(t *testing.T) {
	manGen = manifestWebapp
	manName = apps.WebappManifestName

	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      consts.KonnectorType,
		Slug:      "cozy-bad-type",
		SourceURL: "git://localhost/",
	})
	assert.NoError(t, err)
	_, err = inst.RunSync()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Manifest types are not the same")
}
