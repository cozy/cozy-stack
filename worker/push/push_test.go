package push

import (
	"testing"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
)

func TestGetFirebaseClient(t *testing.T) {
	contextName := "foo"
	slug := "bar"

	// Ensure that the global fcmClient is nil for this test, and restore its
	// old value after the test
	oldFcmClient := fcmClient
	fcmClient = nil
	defer func() {
		fcmClient = oldFcmClient
	}()

	// Create an account type for the test
	typ := account.AccountType{
		DocID:         contextName + "/" + slug,
		Slug:          slug,
		AndroidAPIKey: "th3_f1r3b4s3_k3y",
	}
	err := couchdb.CreateNamedDoc(couchdb.GlobalSecretsDB, &typ)
	if !assert.NoError(t, err) {
		return
	}
	defer func() {
		_ = couchdb.DeleteDoc(couchdb.GlobalSecretsDB, &typ)
	}()

	client := getFirebaseClient(slug, contextName)
	assert.NotNil(t, client)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
}
