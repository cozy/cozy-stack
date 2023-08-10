package settings_test

import (
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	csettings "github.com/cozy/cozy-stack/model/settings"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/require"
)

func setClientsLimit(t *testing.T, inst *instance.Instance, limit float64) {
	inst.FeatureFlags = map[string]interface{}{"cozy.oauthclients.max": limit}
	require.NoError(t, instance.Update(inst))
}

func TestClientsUsage(t *testing.T) {
	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	testInstance := setup.GetTestInstance(&lifecycle.Options{
		Locale:      "en",
		Timezone:    "Europe/Berlin",
		Email:       "alice@example.com",
		ContextName: "test-context",
	})
	scope := consts.Settings + " " + consts.OAuthClients
	_, token := setup.GetTestClient(scope)

	svc := csettings.NewServiceMock(t)
	ts := setupRouter(t, testInstance, svc)

	flagship := oauth.Client{
		RedirectURIs: []string{"cozy://flagship"},
		ClientName:   "flagship-app",
		ClientKind:   "mobile",
		SoftwareID:   "github.com/cozy/cozy-stack/testing/flagship",
		Flagship:     true,
	}
	require.Nil(t, flagship.Create(testInstance, oauth.NotPending))

	t.Run("WithoutLimit", func(t *testing.T) {
		setClientsLimit(t, testInstance, -1)

		e := testutils.CreateTestClient(t, ts.URL)
		obj := e.GET("/settings/clients-usage").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.clients-usage")

		attrs := data.Value("attributes").Object()
		attrs.NotContainsKey("limit")
		attrs.ValueEqual("count", 1)
		attrs.ValueEqual("limitReached", false)
		attrs.ValueEqual("limitExceeded", false)
	})

	t.Run("WithLimitNotReached", func(t *testing.T) {
		setClientsLimit(t, testInstance, 2)

		e := testutils.CreateTestClient(t, ts.URL)
		obj := e.GET("/settings/clients-usage").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.clients-usage")

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("limit", 2)
		attrs.ValueEqual("count", 1)
		attrs.ValueEqual("limitReached", false)
		attrs.ValueEqual("limitExceeded", false)
	})

	t.Run("WithLimitReached", func(t *testing.T) {
		setClientsLimit(t, testInstance, 1)

		e := testutils.CreateTestClient(t, ts.URL)
		obj := e.GET("/settings/clients-usage").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.clients-usage")

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("limit", 1)
		attrs.ValueEqual("count", 1)
		attrs.ValueEqual("limitReached", true)
		attrs.ValueEqual("limitExceeded", false)
	})

	t.Run("WithLimitExceeded", func(t *testing.T) {
		setClientsLimit(t, testInstance, 0)

		e := testutils.CreateTestClient(t, ts.URL)
		obj := e.GET("/settings/clients-usage").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.clients-usage")

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("limit", 0)
		attrs.ValueEqual("count", 1)
		attrs.ValueEqual("limitReached", true)
		attrs.ValueEqual("limitExceeded", true)
	})
}
