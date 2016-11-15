package apps

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/sourcegraph/checkup"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

type TestContext struct {
	prefix string
	fs     afero.Fs
}

func (c TestContext) Prefix() string { return c.prefix }
func (c TestContext) FS() afero.Fs   { return c.fs }

var c = &TestContext{
	prefix: "apps-test/",
	fs:     afero.NewMemMapFs(),
}

func TestInstallBadSlug(t *testing.T) {
	_, err := NewInstaller(c, "", "git://foo.bar")
	if assert.Error(t, err) {
		assert.Equal(t, ErrInvalidSlugName, err)
	}

	_, err = NewInstaller(c, "coucou/", "git://foo.bar")
	if assert.Error(t, err) {
		assert.Equal(t, ErrInvalidSlugName, err)
	}
}

func TestInstallBadAppsSource(t *testing.T) {
	_, err := NewInstaller(c, "app2", "")
	if assert.Error(t, err) {
		assert.Equal(t, ErrNotSupportedSource, err)
	}

	_, err = NewInstaller(c, "app3", "foo://bar.baz")
	if assert.Error(t, err) {
		assert.Equal(t, ErrNotSupportedSource, err)
	}

	_, err = NewInstaller(c, "app4", "git://bar  .baz")
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "invalid character")
	}
}

func TestInstallSuccessful(t *testing.T) {
	inst, err := NewInstaller(c, "cozy-mini", "git://github.com/nono/cozy-mini.git")
	if !assert.NoError(t, err) {
		return
	}

	go inst.Install()

	var state State
	for {
		man, done, err := inst.WaitManifest()
		if !assert.NoError(t, err) {
			return
		}
		if state == "" {
			assert.EqualValues(t, Installing, man.State)
		} else if state == Installing {
			assert.EqualValues(t, Ready, man.State)
			assert.True(t, done)
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State
	}
}

func TestInstallAldreadyExist(t *testing.T) {
	inst, err := NewInstaller(c, "conflictslug", "git://github.com/nono/cozy-mini.git")
	if !assert.NoError(t, err) {
		return
	}

	go inst.Install()

	for {
		var done bool
		_, done, err = inst.WaitManifest()
		if !assert.NoError(t, err) {
			return
		}
		if done {
			break
		}
	}

	inst, err = NewInstaller(c, "conflictslug", "git://github.com/nono/cozy-mini.git")
	if !assert.NoError(t, err) {
		return
	}

	go inst.Install()

	_, _, err = inst.WaitManifest()
	assert.Error(t, err)
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	db, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	err = couchdb.ResetDB(c, ManifestDocType)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.ResetDB(c, vfs.FsDocType)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	for _, index := range vfs.Indexes {
		err = couchdb.DefineIndex(c, vfs.FsDocType, index)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	if err = vfs.CreateRootDirDoc(c); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	res := m.Run()

	os.Exit(res)
}
