package settings_test

import (
	"errors"
	"testing"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/session"
	csettings "github.com/cozy/cozy-stack/model/settings"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/gavv/httpexpect/v2"
)

func TestExternalTies(t *testing.T) {
	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	testInstance := setup.GetTestInstance(&lifecycle.Options{
		Locale:      "en",
		Timezone:    "Europe/Berlin",
		Email:       "alice@example.com",
		ContextName: "test-context",
	})
	sessCookie := session.CookieName(testInstance)

	svc := csettings.NewServiceMock(t)
	ts := setupRouter(t, testInstance, svc)

	t.Run("WithBlockingSubscription", func(t *testing.T) {
		svc.On("GetExternalTies", testInstance).Return(&csettings.ExternalTies{
			HasBlockingSubscription: true,
		}, nil).Once()

		e := testutils.CreateTestClient(t, ts.URL)
		obj := e.GET("/settings/external-ties").
			WithCookie(sessCookie, "connected").
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.HasValue("type", "io.cozy.settings")
		data.HasValue("id", "io.cozy.settings.external-ties")

		attrs := data.Value("attributes").Object()
		attrs.HasValue("has_blocking_subscription", true)
	})

	t.Run("WithClouderyError", func(t *testing.T) {
		svc.On("GetExternalTies", testInstance).Return(nil, errors.New("unauthorized")).Once()

		e := testutils.CreateTestClient(t, ts.URL)
		e.GET("/settings/external-ties").
			WithCookie(sessCookie, "connected").
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(500).
			Body().Contains("unauthorized")
	})
}
