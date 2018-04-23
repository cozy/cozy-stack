package apps_test

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"path"
	"testing"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/pkg/apps"
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
		Type:      apps.Konnector,
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
		Type:      apps.Konnector,
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
		Type:      apps.Konnector,
		Slug:      "cozy-konnector-not-exist",
	})
	assert.Nil(t, inst)
	assert.Equal(t, apps.ErrNotFound, err)

	inst, err = apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Delete,
		Type:      apps.Konnector,
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
		Type:      apps.Konnector,
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
		Type:      apps.Konnector,
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

func TestKonnectorInstallAndUpgradeWithBranch(t *testing.T) {
	manGen = manifestKonnector
	manName = apps.KonnectorManifestName
	doUpgrade(3)

	inst, err := apps.NewInstaller(db, fs, &apps.InstallerOptions{
		Operation: apps.Install,
		Type:      apps.Konnector,
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
		Type:      apps.Konnector,
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
		Type:      apps.Konnector,
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
		Type:      apps.Konnector,
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
		Type:      apps.Konnector,
		Slug:      "konnector-delete",
	})
	assert.Nil(t, inst3)
	assert.Equal(t, apps.ErrNotFound, err)
}
