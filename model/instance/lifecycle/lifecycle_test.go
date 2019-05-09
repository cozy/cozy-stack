package lifecycle_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/stack"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/stretchr/testify/assert"

	_ "github.com/cozy/cozy-stack/worker/mails"
)

func TestGetInstanceNoDB(t *testing.T) {
	instance, err := lifecycle.GetInstance("no.instance.cozycloud.cc")
	if assert.Error(t, err, "An error is expected") {
		assert.Nil(t, instance)
		assert.Contains(t, err.Error(), "Instance not found", "the error is not explicit")
	}
}

func TestCreateInstance(t *testing.T) {
	instance, err := lifecycle.Create(&lifecycle.Options{
		Domain: "test.cozycloud.cc",
		Locale: "en",
	})
	if assert.NoError(t, err) {
		assert.NotEmpty(t, instance.ID())
		assert.Equal(t, instance.Domain, "test.cozycloud.cc")
	}
}

func TestCreateInstanceWithFewSettings(t *testing.T) {
	inst, err := lifecycle.Create(&lifecycle.Options{
		Domain:       "test2.cozycloud.cc",
		Timezone:     "Europe/Berlin",
		Email:        "alice@example.com",
		PublicName:   "Alice",
		Passphrase:   "password",
		SwiftCluster: 2,
		Settings:     "offer:freemium,context:my_context,auth_mode:two_factor_mail,uuid:XXX,locale:en,tos:20151111",
	})

	assert.NoError(t, err)
	assert.Equal(t, inst.Domain, "test2.cozycloud.cc")
	doc, err := inst.SettingsDocument()
	assert.NoError(t, err)
	assert.Equal(t, "Europe/Berlin", doc.M["tz"].(string))
	assert.Equal(t, "alice@example.com", doc.M["email"].(string))
	assert.Equal(t, "freemium", doc.M["offer"].(string))
	assert.Equal(t, "Alice", doc.M["public_name"].(string))

	assert.Equal(t, inst.UUID, "XXX")
	assert.Equal(t, inst.Locale, "en")
	assert.Equal(t, inst.TOSSigned, "1.0.0-20151111")
	assert.Equal(t, inst.ContextName, "my_context")
	assert.Equal(t, inst.AuthMode, instance.TwoFactorMail)
	assert.Equal(t, inst.SwiftCluster, 2)
}

func TestCreateInstanceWithMoreSettings(t *testing.T) {
	inst, err := lifecycle.Create(&lifecycle.Options{
		Domain:       "test3.cozycloud.cc",
		UUID:         "XXX",
		Locale:       "en",
		TOSSigned:    "20151111",
		TOSLatest:    "1.0.0-20151111",
		Timezone:     "Europe/Berlin",
		ContextName:  "my_context",
		Email:        "alice@example.com",
		PublicName:   "Alice",
		AuthMode:     "two_factor_mail",
		Passphrase:   "password",
		SwiftCluster: 2,
		Settings:     "offer:freemium",
	})

	assert.NoError(t, err)
	assert.Equal(t, inst.Domain, "test3.cozycloud.cc")
	doc, err := inst.SettingsDocument()
	assert.NoError(t, err)
	assert.Equal(t, "Europe/Berlin", doc.M["tz"].(string))
	assert.Equal(t, "alice@example.com", doc.M["email"].(string))
	assert.Equal(t, "freemium", doc.M["offer"].(string))
	assert.Equal(t, "Alice", doc.M["public_name"].(string))

	assert.Equal(t, inst.Locale, "en")
	assert.Equal(t, inst.TOSSigned, "1.0.0-20151111")
	assert.Equal(t, inst.ContextName, "my_context")
	assert.Equal(t, inst.AuthMode, instance.TwoFactorMail)
	assert.Equal(t, inst.SwiftCluster, 2)
}

func TestCreateInstanceBadDomain(t *testing.T) {
	_, err := lifecycle.Create(&lifecycle.Options{
		Domain: "..",
		Locale: "en",
	})
	assert.Error(t, err, "An error is expected")

	_, err = lifecycle.Create(&lifecycle.Options{
		Domain: ".",
		Locale: "en",
	})
	assert.Error(t, err, "An error is expected")

	_, err = lifecycle.Create(&lifecycle.Options{
		Domain: "foo/bar",
		Locale: "en",
	})
	assert.Error(t, err, "An error is expected")
}

func TestGetWrongInstance(t *testing.T) {
	instance, err := lifecycle.GetInstance("no.instance.cozycloud.cc")
	if assert.Error(t, err, "An error is expected") {
		assert.Nil(t, instance)
		assert.Contains(t, err.Error(), "Instance not found", "the error is not explicit")
	}
}

func TestGetCorrectInstance(t *testing.T) {
	instance, err := lifecycle.GetInstance("test.cozycloud.cc")
	if assert.NoError(t, err) {
		assert.NotNil(t, instance)
		assert.Equal(t, instance.Domain, "test.cozycloud.cc")
	}
}

func TestInstancehasOAuthSecret(t *testing.T) {
	i, err := lifecycle.GetInstance("test.cozycloud.cc")
	if assert.NoError(t, err) {
		assert.NotNil(t, i)
		assert.NotNil(t, i.OAuthSecret)
		assert.Equal(t, len(i.OAuthSecret), instance.OauthSecretLen)
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
	i, err := lifecycle.GetInstance("test.cozycloud.cc")
	if !assert.NoError(t, err, "cant fetch i") {
		return
	}
	assert.NotNil(t, i)
	assert.NotEmpty(t, i.RegisterToken)
	assert.Len(t, i.RegisterToken, instance.RegisterTokenLen)
	assert.NotEmpty(t, i.OAuthSecret)
	assert.Len(t, i.OAuthSecret, instance.OauthSecretLen)
	assert.NotEmpty(t, i.SessionSecret)
	assert.Len(t, i.SessionSecret, instance.SessionSecretLen)

	rtoken := i.RegisterToken
	pass := []byte("passphrase")
	empty := []byte("")
	badtoken := []byte("not-token")

	err = lifecycle.RegisterPassphrase(i, pass, empty)
	assert.Error(t, err, "RegisterPassphrase requires token")

	err = lifecycle.RegisterPassphrase(i, pass, badtoken)
	assert.Error(t, err, "RegisterPassphrase requires proper token")

	err = lifecycle.RegisterPassphrase(i, pass, rtoken)
	assert.NoError(t, err)

	assert.Empty(t, i.RegisterToken, "RegisterToken has not been removed")
	assert.NotEmpty(t, i.PassphraseHash, "PassphraseHash has not been saved")

	err = lifecycle.RegisterPassphrase(i, pass, rtoken)
	assert.Error(t, err, "RegisterPassphrase works only once")
}

func TestUpdatePassphrase(t *testing.T) {
	i, err := lifecycle.GetInstance("test.cozycloud.cc")
	if !assert.NoError(t, err, "cant fetch i") {
		return
	}
	assert.NotNil(t, i)
	assert.Empty(t, i.RegisterToken)
	assert.NotEmpty(t, i.OAuthSecret)
	assert.Len(t, i.OAuthSecret, instance.OauthSecretLen)
	assert.NotEmpty(t, i.SessionSecret)
	assert.Len(t, i.SessionSecret, instance.SessionSecretLen)

	oldHash := i.PassphraseHash
	oldSecret := i.SessionSecret

	currentPass := []byte("passphrase")
	newPass := []byte("new-passphrase")
	badPass := []byte("not-passphrase")
	empty := []byte("")

	err = lifecycle.UpdatePassphrase(i, newPass, empty, "", nil)
	assert.Error(t, err, "UpdatePassphrase requires the current passphrase")

	err = lifecycle.UpdatePassphrase(i, newPass, badPass, "", nil)
	assert.Error(t, err, "UpdatePassphrase requires the current passphrase")

	err = lifecycle.UpdatePassphrase(i, newPass, currentPass, "", nil)
	assert.NoError(t, err)

	assert.NotEmpty(t, i.PassphraseHash, "PassphraseHash has not been saved")
	assert.NotEqual(t, oldHash, i.PassphraseHash)
	assert.NotEqual(t, oldSecret, i.SessionSecret)
}

func TestRequestPassphraseReset(t *testing.T) {
	in, err := lifecycle.Create(&lifecycle.Options{
		Domain: "test.cozycloud.cc.pass_reset",
		Locale: "en",
	})
	if !assert.NoError(t, err) {
		return
	}
	err = lifecycle.RequestPassphraseReset(in)
	if !assert.NoError(t, err) {
		return
	}
	// token should not have been generated since we have not set a passphrase
	// yet
	if !assert.Nil(t, in.PassphraseResetToken) {
		return
	}
	err = lifecycle.RegisterPassphrase(in, []byte("MyPassphrase"), in.RegisterToken)
	if !assert.NoError(t, err) {
		return
	}
	err = lifecycle.RequestPassphraseReset(in)
	if !assert.NoError(t, err) {
		return
	}

	regToken := in.PassphraseResetToken
	regTime := in.PassphraseResetTime
	assert.NotNil(t, in.PassphraseResetToken)
	assert.True(t, !in.PassphraseResetTime.Before(time.Now().UTC()))

	err = lifecycle.RequestPassphraseReset(in)
	assert.Equal(t, instance.ErrResetAlreadyRequested, err)
	assert.EqualValues(t, regToken, in.PassphraseResetToken)
	assert.EqualValues(t, regTime, in.PassphraseResetTime)
}

func TestPassphraseRenew(t *testing.T) {
	in, err := lifecycle.Create(&lifecycle.Options{
		Domain: "test.cozycloud.cc.pass_renew",
		Locale: "en",
	})
	if !assert.NoError(t, err) {
		return
	}
	err = lifecycle.RegisterPassphrase(in, []byte("MyPassphrase"), in.RegisterToken)
	if !assert.NoError(t, err) {
		return
	}
	passHash := in.PassphraseHash
	err = lifecycle.PassphraseRenew(in, []byte("NewPass"), nil)
	if !assert.Error(t, err) {
		return
	}
	err = lifecycle.RequestPassphraseReset(in)
	if !assert.NoError(t, err) {
		return
	}
	err = lifecycle.PassphraseRenew(in, []byte("NewPass"), []byte("token"))
	if !assert.Error(t, err) {
		return
	}
	err = lifecycle.PassphraseRenew(in, []byte("NewPass"), in.PassphraseResetToken)
	if !assert.NoError(t, err) {
		return
	}
	assert.False(t, bytes.Equal(passHash, in.PassphraseHash))
}

func TestInstanceNoDuplicate(t *testing.T) {
	_, err := lifecycle.Create(&lifecycle.Options{
		Domain: "test.cozycloud.cc.duplicate",
		Locale: "en",
	})
	if !assert.NoError(t, err) {
		return
	}
	i, err := lifecycle.Create(&lifecycle.Options{
		Domain: "test.cozycloud.cc.duplicate",
		Locale: "en",
	})
	if assert.Error(t, err, "Should not be possible to create duplicate") {
		assert.Nil(t, i)
		assert.Contains(t, err.Error(), "Instance already exists", "the error is not explicit")
	}
}

func TestCheckPassphrase(t *testing.T) {
	inst, err := lifecycle.GetInstance("test.cozycloud.cc")
	if !assert.NoError(t, err, "cant fetch instance") {
		return
	}

	assert.Empty(t, inst.RegisterToken, "changes have been saved in db")
	assert.NotEmpty(t, inst.PassphraseHash, "changes have been saved in db")

	err = lifecycle.CheckPassphrase(inst, []byte("not-passphrase"))
	assert.Error(t, err)

	err = lifecycle.CheckPassphrase(inst, []byte("new-passphrase"))
	assert.NoError(t, err)
}

func TestCheckTOSNotSigned(t *testing.T) {
	now := time.Now()
	i, err := lifecycle.Create(&lifecycle.Options{
		Domain:    "tos.test.cozycloud.cc",
		Locale:    "en",
		TOSSigned: "1.0.0-" + now.Format("20060102"),
	})
	if !assert.NoError(t, err) {
		return
	}

	notSigned, deadline := i.CheckTOSNotSignedAndDeadline()
	assert.Empty(t, i.TOSLatest)
	assert.False(t, notSigned)
	assert.Equal(t, instance.TOSNone, deadline)

	err = lifecycle.Patch(i, &lifecycle.Options{
		TOSLatest: "1.0.1-" + now.Format("20060102"),
	})
	if !assert.NoError(t, err) {
		return
	}

	notSigned, deadline = i.CheckTOSNotSignedAndDeadline()
	assert.Empty(t, i.TOSLatest)
	assert.False(t, notSigned)
	assert.Equal(t, instance.TOSNone, deadline)

	err = lifecycle.Patch(i, &lifecycle.Options{
		TOSLatest: "2.0.1-" + now.Add(40*24*time.Hour).Format("20060102"),
	})
	if !assert.NoError(t, err) {
		return
	}

	notSigned, deadline = i.CheckTOSNotSignedAndDeadline()
	assert.NotEmpty(t, i.TOSLatest)
	assert.True(t, notSigned)
	assert.Equal(t, instance.TOSNone, deadline)

	err = lifecycle.Patch(i, &lifecycle.Options{
		TOSLatest: "2.0.1-" + now.Add(10*24*time.Hour).Format("20060102"),
	})
	if !assert.NoError(t, err) {
		return
	}
	notSigned, deadline = i.CheckTOSNotSignedAndDeadline()
	assert.NotEmpty(t, i.TOSLatest)
	assert.True(t, notSigned)
	assert.Equal(t, instance.TOSWarning, deadline)

	err = lifecycle.Patch(i, &lifecycle.Options{
		TOSLatest: "2.0.1-" + now.Format("20060102"),
	})
	if !assert.NoError(t, err) {
		return
	}
	notSigned, deadline = i.CheckTOSNotSignedAndDeadline()
	assert.NotEmpty(t, i.TOSLatest)
	assert.True(t, notSigned)
	assert.Equal(t, instance.TOSBlocked, deadline)

	err = lifecycle.Patch(i, &lifecycle.Options{
		TOSSigned: "2.0.1-" + now.Format("20060102"),
	})
	if !assert.NoError(t, err) {
		return
	}
	notSigned, deadline = i.CheckTOSNotSignedAndDeadline()
	assert.Empty(t, i.TOSLatest)
	assert.False(t, notSigned)
	assert.Equal(t, instance.TOSNone, deadline)
}

func TestInstanceDestroy(t *testing.T) {
	_ = lifecycle.Destroy("test.cozycloud.cc")

	_, err := lifecycle.Create(&lifecycle.Options{
		Domain: "test.cozycloud.cc",
		Locale: "en",
	})
	if !assert.NoError(t, err) {
		return
	}

	err = lifecycle.Destroy("test.cozycloud.cc")
	assert.NoError(t, err)

	err = lifecycle.Destroy("test.cozycloud.cc")
	if assert.Error(t, err) {
		assert.Equal(t, instance.ErrNotFound, err)
	}
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	db, err := checkup.HTTPChecker{URL: config.CouchURL().String()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}
	_, err = stack.Start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	cleanInstance()
	res := m.Run()
	cleanInstance()
	os.Exit(res)
}

func cleanInstance() {
	_ = lifecycle.Destroy("test.cozycloud.cc")
	_ = lifecycle.Destroy("test2.cozycloud.cc")
	_ = lifecycle.Destroy("test3.cozycloud.cc")
	_ = lifecycle.Destroy("test.cozycloud.cc.pass_reset")
	_ = lifecycle.Destroy("test.cozycloud.cc.pass_renew")
	_ = lifecycle.Destroy("test.cozycloud.cc.duplicate")
	_ = lifecycle.Destroy("tos.test.cozycloud.cc")
}

func getDB(t *testing.T, domain string) couchdb.Database {
	instance, err := lifecycle.GetInstance(domain)
	if !assert.NoError(t, err, "Should get instance %v", domain) {
		t.FailNow()
	}
	return instance
}
