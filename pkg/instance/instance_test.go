package instance

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestSubdomain(t *testing.T) {
	instance := Instance{
		Domain: "foo.example.com",
	}
	cfg := config.GetConfig()
	was := cfg.Subdomains
	defer func() { cfg.Subdomains = was }()

	cfg.Subdomains = config.NestedSubdomains
	u := instance.SubDomain("calendar")
	assert.Equal(t, "https://calendar.foo.example.com/", u.String())

	cfg.Subdomains = config.FlatSubdomains
	u = instance.SubDomain("calendar")
	assert.Equal(t, "https://foo-calendar.example.com/", u.String())
}

func TestGetInstanceNoDB(t *testing.T) {
	instance, err := Get("no.instance.cozycloud.cc")
	if assert.Error(t, err, "An error is expected") {
		assert.Nil(t, instance)
		assert.Contains(t, err.Error(), "Instance not found", "the error is not explicit")
	}
}

func TestCreateInstance(t *testing.T) {
	instance, err := Create(&Options{
		Domain: "test.cozycloud.cc",
		Locale: "en",
	})
	if assert.NoError(t, err) {
		assert.NotEmpty(t, instance.ID())
		assert.Equal(t, instance.Domain, "test.cozycloud.cc")
	}
}

func TestCreateInstanceWithSettings(t *testing.T) {
	instance, err := Create(&Options{
		Domain:   "test2.cozycloud.cc",
		Locale:   "en",
		Timezone: "Europe/Berlin",
		Email:    "alice@example.com",
	})
	assert.NoError(t, err)
	assert.Equal(t, instance.Domain, "test2.cozycloud.cc")
	var doc couchdb.JSONDoc
	err = couchdb.GetDoc(instance, consts.Settings, consts.InstanceSettingsID, &doc)
	assert.NoError(t, err)
	assert.Equal(t, "Europe/Berlin", doc.M["tz"].(string))
	assert.Equal(t, "alice@example.com", doc.M["email"].(string))
}

func TestCreateInstanceBadDomain(t *testing.T) {
	_, err := Create(&Options{
		Domain: "..",
		Locale: "en",
	})
	assert.Error(t, err, "An error is expected")

	_, err = Create(&Options{
		Domain: ".",
		Locale: "en",
	})
	assert.Error(t, err, "An error is expected")

	_, err = Create(&Options{
		Domain: "foo/bar",
		Locale: "en",
	})
	assert.Error(t, err, "An error is expected")
}

func TestGetWrongInstance(t *testing.T) {
	instance, err := Get("no.instance.cozycloud.cc")
	if assert.Error(t, err, "An error is expected") {
		assert.Nil(t, instance)
		assert.Contains(t, err.Error(), "Instance not found", "the error is not explicit")
	}
}

func TestGetCorrectInstance(t *testing.T) {
	instance, err := Get("test.cozycloud.cc")
	if assert.NoError(t, err) {
		assert.NotNil(t, instance)
		assert.Equal(t, instance.Domain, "test.cozycloud.cc")
	}
}

func TestInstancehasOAuthSecret(t *testing.T) {
	instance, err := Get("test.cozycloud.cc")
	if assert.NoError(t, err) {
		assert.NotNil(t, instance)
		assert.NotNil(t, instance.OAuthSecret)
		assert.Equal(t, len(instance.OAuthSecret), oauthSecretLen)
	}
}

func TestInstanceHasRootDir(t *testing.T) {
	var root vfs.DirDoc
	prefix := getDB(t, "test.cozycloud.cc")
	err := couchdb.GetDoc(prefix, consts.Files, consts.RootDirID, &root)
	if assert.NoError(t, err) {
		assert.Equal(t, root.Fullpath, "/")
	}
}

func TestInstanceHasIndexes(t *testing.T) {
	var results []*vfs.DirDoc
	prefix := getDB(t, "test.cozycloud.cc")
	req := &couchdb.FindRequest{Selector: mango.Equal("path", "/")}
	err := couchdb.FindDocs(prefix, consts.Files, req, &results)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestRegisterPassphrase(t *testing.T) {
	instance, err := Get("test.cozycloud.cc")
	if !assert.NoError(t, err, "cant fetch instance") {
		return
	}
	assert.NotNil(t, instance)
	assert.NotEmpty(t, instance.RegisterToken)
	assert.Len(t, instance.RegisterToken, registerTokenLen)
	assert.NotEmpty(t, instance.OAuthSecret)
	assert.Len(t, instance.OAuthSecret, oauthSecretLen)
	assert.NotEmpty(t, instance.SessionSecret)
	assert.Len(t, instance.SessionSecret, sessionSecretLen)

	rtoken := instance.RegisterToken
	pass := []byte("passphrase")
	empty := []byte("")
	badtoken := []byte("not-token")

	err = instance.RegisterPassphrase(pass, empty)
	assert.Error(t, err, "RegisterPassphrase requires token")

	err = instance.RegisterPassphrase(pass, badtoken)
	assert.Error(t, err, "RegisterPassphrase requires proper token")

	err = instance.RegisterPassphrase(pass, rtoken)
	assert.NoError(t, err)

	assert.Empty(t, instance.RegisterToken, "RegisterToken has not been removed")
	assert.NotEmpty(t, instance.PassphraseHash, "PassphraseHash has not been saved")

	err = instance.RegisterPassphrase(pass, rtoken)
	assert.Error(t, err, "RegisterPassphrase works only once")
}

func TestUpdatePassphrase(t *testing.T) {
	instance, err := Get("test.cozycloud.cc")
	if !assert.NoError(t, err, "cant fetch instance") {
		return
	}
	assert.NotNil(t, instance)
	assert.Empty(t, instance.RegisterToken)
	assert.NotEmpty(t, instance.OAuthSecret)
	assert.Len(t, instance.OAuthSecret, oauthSecretLen)
	assert.NotEmpty(t, instance.SessionSecret)
	assert.Len(t, instance.SessionSecret, sessionSecretLen)

	oldHash := instance.PassphraseHash
	oldSecret := instance.SessionSecret

	currentPass := []byte("passphrase")
	newPass := []byte("new-passphrase")
	badPass := []byte("not-passphrase")
	empty := []byte("")

	err = instance.UpdatePassphrase(newPass, empty)
	assert.Error(t, err, "UpdatePassphrase requires the current passphrase")

	err = instance.UpdatePassphrase(newPass, badPass)
	assert.Error(t, err, "UpdatePassphrase requires the current passphrase")

	err = instance.UpdatePassphrase(newPass, currentPass)
	assert.NoError(t, err)

	assert.NotEmpty(t, instance.PassphraseHash, "PassphraseHash has not been saved")
	assert.NotEqual(t, oldHash, instance.PassphraseHash)
	assert.NotEqual(t, oldSecret, instance.SessionSecret)
}

func TestCheckPassphrase(t *testing.T) {
	instance, err := Get("test.cozycloud.cc")
	if !assert.NoError(t, err, "cant fetch instance") {
		return
	}

	assert.Empty(t, instance.RegisterToken, "changes have been saved in db")
	assert.NotEmpty(t, instance.PassphraseHash, "changes have been saved in db")

	err = instance.CheckPassphrase([]byte("not-passphrase"))
	assert.Error(t, err)

	err = instance.CheckPassphrase([]byte("new-passphrase"))
	assert.NoError(t, err)

}

func TestInstanceNoDuplicate(t *testing.T) {
	_, err := Create(&Options{
		Domain: "test.cozycloud.cc.duplicate",
		Locale: "en",
	})
	if !assert.NoError(t, err) {
		return
	}
	i, err := Create(&Options{
		Domain: "test.cozycloud.cc.duplicate",
		Locale: "en",
	})
	if assert.Error(t, err, "Should not be possible to create duplicate") {
		assert.Nil(t, i)
		assert.Contains(t, err.Error(), "Instance already exists", "the error is not explicit")
	}
}

func TestInstanceDestroy(t *testing.T) {
	Destroy("test.cozycloud.cc")

	_, err := Create(&Options{
		Domain: "test.cozycloud.cc",
		Locale: "en",
	})
	if !assert.NoError(t, err) {
		return
	}

	inst, err := Destroy("test.cozycloud.cc")
	if assert.NoError(t, err) {
		assert.NotNil(t, inst)
	}

	inst, err = Destroy("test.cozycloud.cc")
	if assert.Error(t, err) {
		assert.Equal(t, ErrNotFound, err)
		assert.Nil(t, inst)
	}
}

func TestGetFs(t *testing.T) {
	instance := Instance{
		Domain:     "test-provider.cozycloud.cc",
		StorageURL: "mem://test",
	}
	content := []byte{'b', 'a', 'r'}
	err := instance.makeStorageFs()
	assert.NoError(t, err)
	storage := instance.FS()
	assert.NotNil(t, storage, "the instance should have a memory storage provider")
	err = afero.WriteFile(storage, "foo", content, 0644)
	assert.NoError(t, err)
	storage = instance.FS()
	assert.NotNil(t, storage, "the instance should have a memory storage provider")
	buf, err := afero.ReadFile(storage, "foo")
	assert.NoError(t, err)
	assert.Equal(t, content, buf, "the storage should have persist the content of the foo file")
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	db, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}
	Destroy("test.cozycloud.cc")
	Destroy("test2.cozycloud.cc")
	Destroy("test.cozycloud.cc.duplicate")

	os.RemoveAll("/usr/local/var/cozy2/")

	res := m.Run()

	Destroy("test.cozycloud.cc")
	Destroy("test2.cozycloud.cc")
	Destroy("test.cozycloud.cc.duplicate")

	os.Exit(res)
}

func getDB(t *testing.T, domain string) couchdb.Database {
	instance, err := Get(domain)
	if !assert.NoError(t, err, "Should get instance %v", domain) {
		t.FailNow()
	}
	return instance
}
