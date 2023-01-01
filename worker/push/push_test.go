package push

import (
	"testing"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	err := couchdb.CreateNamedDoc(prefixer.SecretsPrefixer, &typ)
	require.NoError(t, err)

	defer func() {
		_ = couchdb.DeleteDoc(prefixer.SecretsPrefixer, &typ)
	}()

	client := getFirebaseClient(slug, contextName)
	assert.NotNil(t, client)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
}
