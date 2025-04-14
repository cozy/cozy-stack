package settings_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/session"
	csettings "github.com/cozy/cozy-stack/model/settings"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	websettings "github.com/cozy/cozy-stack/web/settings"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/cozy/cozy-stack/worker/mails"
)

func setupRouter(t *testing.T, inst *instance.Instance, svc csettings.Service) *httptest.Server {
	t.Helper()

	handler := echo.New()
	handler.HTTPErrorHandler = errors.ErrorHandler
	group := handler.Group("/settings", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(context echo.Context) error {
			context.Set("instance", inst)

			cookie, err := context.Request().Cookie(session.CookieName(inst))
			if err != http.ErrNoCookie {
				require.NoError(t, err, "Could not get session cookie")
				if cookie.Value == "connected" {
					sess, _ := session.New(inst, session.LongRun, "")
					context.Set("session", sess)
				}
			}

			return next(context)
		}
	})

	websettings.NewHTTPHandler(svc).Register(group)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	return ts
}

func TestSettings(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var instanceRev string
	var oauthClientID string

	config.UseTestFile(t)
	conf := config.GetConfig()
	conf.Assets = "../../assets"
	conf.Contexts[config.DefaultInstanceContext] = map[string]interface{}{
		"logos": map[string]interface{}{
			"home": map[string]interface{}{
				"light": []interface{}{
					map[string]interface{}{"src": "/logos/main_cozy.png", "alt": "Cozy Cloud"},
				},
			},
		},
	}
	was := conf.Subdomains
	conf.Subdomains = config.NestedSubdomains
	defer func() { conf.Subdomains = was }()

	_ = web.LoadSupportedLocales()
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	testInstance := setup.GetTestInstance(&lifecycle.Options{
		Locale:      "en",
		Timezone:    "Europe/Berlin",
		Email:       "alice@example.com",
		ContextName: "test-context",
	})
	scope := consts.Settings + " " + consts.OAuthClients
	_, token := setup.GetTestClient(scope)
	sessCookie := session.CookieName(testInstance)

	svc := csettings.NewServiceMock(t)
	ts := setupRouter(t, testInstance, svc)
	ts.Config.Handler.(*echo.Echo).Renderer = render
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	tsURL := ts.URL

	t.Run("GetContext", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		testutils.WithManager(t, testInstance, testutils.ManagerConfig{URL: "http://manager.example.org"})

		obj := e.GET("/settings/context").
			WithCookie(sessCookie, "connected").
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.HasValue("type", "io.cozy.settings")
		data.HasValue("id", "io.cozy.settings.context")

		attrs := data.Value("attributes").Object()
		attrs.HasValue("manager_url", "http://manager.example.org")
		attrs.HasValue("logos", map[string]interface{}{
			"home": map[string]interface{}{
				"light": []interface{}{
					map[string]interface{}{"src": "/logos/main_cozy.png", "alt": "Cozy Cloud"},
				},
			},
		})
	})

	t.Run("PatchWithGoodRev", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		doc1, err := testInstance.SettingsDocument()
		require.NoError(t, err)

		// We are going to patch an instance with newer values, and give the good rev
		e.PUT("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithHeader("Content-Type", "application/vnd.api+json").
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(fmt.Sprintf(`{
      "data": {
        "type": "io.cozy.settings",
        "id": "io.cozy.settings.instance",
        "meta": {
          "rev": "%s"
        },
        "attributes": {
          "tz": "Europe/London",
          "email": "alice@example.org",
          "locale": "fr"
        }
      }
    }`, doc1.Rev()))).
			Expect().Status(200)
	})

	t.Run("PatchWithBadRev", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		// We are going to patch an instance with newer values, but with a totally
		// random rev
		rev := "6-2d9b7ef014d10549c2b4e206672d3e44"

		e.PUT("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithHeader("Content-Type", "application/vnd.api+json").
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(fmt.Sprintf(`{
        "data": {
          "type": "io.cozy.settings",
          "id": "io.cozy.settings.instance",
          "meta": {
            "rev": "%s"
          },
          "attributes": {
            "tz": "Europe/Berlin",
            "email": "alice@example.com",
            "locale": "en"
          }
        }
      }`, rev))).
			Expect().Status(409)
	})

	t.Run("PatchWithBadRevNoChanges", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		// We are defining a random rev, but make no changes in the instance values
		rev := "6-2d9b7ef014d10549c2b4e206672d3e44"

		e.PUT("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithHeader("Content-Type", "application/vnd.api+json").
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(fmt.Sprintf(`{
        "data": {
          "type": "io.cozy.settings",
          "id": "io.cozy.settings.instance",
          "meta": {
            "rev": "%s"
          },
          "attributes": {
            "tz": "Europe/London",
            "email": "alice@example.org",
            "locale": "fr"
          }
        }
      }`, rev))).
			Expect().Status(200)
	})

	t.Run("PatchWithBadRevAndChanges", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		// We are defining a random rev, but make changes in the instance values
		rev := "6-2d9b7ef014d10549c2b4e206672d3e44"

		e.PUT("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithHeader("Content-Type", "application/vnd.api+json").
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(fmt.Sprintf(`{
        "data": {
          "type": "io.cozy.settings",
          "id": "io.cozy.settings.instance",
          "meta": {
            "rev": "%s"
          },
          "attributes": {
            "tz": "Europe/London",
            "email": "alice@example.com",
            "locale": "en"
          }
        }
      }`, rev))).
			Expect().Status(409)
	})

	t.Run("DiskUsage", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		obj := e.GET("/settings/disk-usage").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		e.GET("/settings/disk-usage").
			WithCookie(sessCookie, "connected").
			Expect().Status(401)

		data := obj.Value("data").Object()
		data.HasValue("type", "io.cozy.settings")
		data.HasValue("id", "io.cozy.settings.disk-usage")

		attrs := data.Value("attributes").Object()
		attrs.HasValue("used", "0")
		attrs.HasValue("files", "0")
		attrs.HasValue("versions", "0")
	})

	t.Run("RegisterPassphraseWrongToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		e.POST("/settings/passphrase").
			WithCookie(sessCookie, "connected").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "passphrase":     "MyFirstPassphrase",
        "iterations":     50000,
        "register_token": "BADBEEF",
      }`)).
			Expect().Status(400)

		e.POST("/settings/passphrase").
			WithCookie(sessCookie, "connected").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "passphrase":     "MyFirstPassphrase",
        "iterations":     50000,
        "register_token": "XYZ",
      }`)).
			Expect().Status(400)
	})

	t.Run("RegisterPassphraseCorrectToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		res := e.POST("/settings/passphrase").
			WithCookie(sessCookie, "connected").
			WithJSON(map[string]interface{}{
				"passphrase":     "MyFirstPassphrase",
				"iterations":     50000,
				"register_token": hex.EncodeToString(testInstance.RegisterToken),
				"key":            "xxx",
			}).
			Expect().Status(200)

		res.Cookies().Length().IsEqual(1)
		res.Cookie("cozysessid").Value().NotEmpty()
	})

	t.Run("UpdatePassphraseWithWrongPassphrase", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		e.PUT("/settings/passphrase").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "new_passphrase":     "MyPassphrase",
        "current_passphrase": "BADBEEF",
        "iterations":         50000
      }`)).
			Expect().Status(400)
	})

	t.Run("UpdatePassphraseSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		res := e.PUT("/settings/passphrase").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "new_passphrase":     "MyUpdatedPassphrase",
        "current_passphrase": "MyFirstPassphrase",
        "iterations":         50000
      }`)).
			Expect().Status(204)

		res.Cookies().Length().IsEqual(1)
		res.Cookie("cozysessid").Value().NotEmpty()
	})

	t.Run("UpdatePassphraseWithForce", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		e.PUT("/settings/passphrase").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "new_passphrase": "MyPassphrase",
        "iterations":     50000,
        "force":          true
      }`)).
			Expect().Status(400)

		passwordDefined := false
		testInstance.PasswordDefined = &passwordDefined

		e.PUT("/settings/passphrase").
			WithCookie(sessCookie, "connected").
			WithQuery("Force", true).
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "new_passphrase": "MyPassphrase",
        "iterations":     50000,
        "force":          true
      }`)).
			Expect().Status(204)
	})

	t.Run("CheckPassphrase", func(t *testing.T) {
		t.Run("invalid", func(t *testing.T) {
			e := testutils.CreateTestClient(t, tsURL)

			e.POST("/settings/passphrase/check").
				WithCookie(sessCookie, "connected").
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-Type", "application/json").
				WithBytes([]byte(`{
        "passphrase": "Invalid Passphrase"
      }`)).
				Expect().Status(403)
		})

		t.Run("valid", func(t *testing.T) {
			e := testutils.CreateTestClient(t, tsURL)

			e.POST("/settings/passphrase/check").
				WithCookie(sessCookie, "connected").
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-Type", "application/json").
				WithBytes([]byte(`{
        "passphrase": "MyPassphrase"
      }`)).
				Expect().Status(204)
		})
	})

	t.Run("GetHint", func(t *testing.T) {
		t.Run("WithNoHint", func(t *testing.T) {
			e := testutils.CreateTestClient(t, tsURL)

			e.GET("/settings/hint").
				WithCookie(sessCookie, "connected").
				WithHeader("Authorization", "Bearer "+token).
				Expect().Status(404)
		})

		t.Run("WithHint", func(t *testing.T) {
			e := testutils.CreateTestClient(t, tsURL)

			setting, err := settings.Get(testInstance)
			assert.NoError(t, err)
			setting.PassphraseHint = "my hint"
			err = couchdb.UpdateDoc(testInstance, setting)
			assert.NoError(t, err)

			e.GET("/settings/hint").
				WithCookie(sessCookie, "connected").
				WithHeader("Authorization", "Bearer "+token).
				Expect().Status(204)
		})
	})

	t.Run("UpdateHint", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		e.PUT("/settings/hint").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "hint": "my updated hint"
      }`)).
			Expect().Status(204)

		setting, err := settings.Get(testInstance)
		assert.NoError(t, err)
		assert.Equal(t, "my updated hint", setting.PassphraseHint)
	})

	t.Run("GetPassphraseParameters", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		obj := e.GET("/settings/passphrase").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.HasValue("type", "io.cozy.settings")
		data.HasValue("id", "io.cozy.settings.passphrase")

		attrs := data.Value("attributes").Object()
		attrs.HasValue("salt", "me@"+testInstance.Domain)
		attrs.HasValue("kdf", 0.0)
		attrs.HasValue("iterations", 50000)
	})

	t.Run("GetCapabilities", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)
		svc.On("GetLegalNoticeUrl", testInstance).Return("", nil).Once()

		e.GET("/settings/instance").
			WithCookie(sessCookie, "connected").
			Expect().Status(401)

		obj := e.GET("/settings/capabilities").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.HasValue("type", "io.cozy.settings")
		data.HasValue("id", "io.cozy.settings.capabilities")

		attrs := data.Value("attributes").Object()
		attrs.HasValue("file_versioning", true)
		attrs.HasValue("can_auth_with_password", true)
		attrs.HasValue("can_auth_with_magic_links", false)
		attrs.HasValue("can_auth_with_oidc", false)
	})

	t.Run("GetInstance", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		e.GET("/settings/instance").
			WithCookie(sessCookie, "connected").
			Expect().Status(401)

		testInstance.RegisterToken = []byte("test")

		e.GET("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithQuery("registerToken", "74657374").
			Expect().Status(200)

		testInstance.RegisterToken = []byte{}
		svc.On("GetLegalNoticeUrl", testInstance).Return("https://testmanager.cozycloud.cc/tos/12345.pdf", nil).Once()

		obj := e.GET("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.HasValue("type", "io.cozy.settings")
		data.HasValue("id", "io.cozy.settings.instance")

		meta := data.Value("meta").Object()
		instanceRev = meta.Value("rev").String().NotEmpty().Raw()

		attrs := data.Value("attributes").Object()
		attrs.HasValue("email", "alice@example.org")
		attrs.HasValue("tz", "Europe/London")
		attrs.HasValue("locale", "en")
		attrs.HasValue("password_defined", true)
		attrs.HasValue("legal_notice_url", "https://testmanager.cozycloud.cc/tos/12345.pdf")
	})

	t.Run("UpdateInstance", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		obj := e.PUT("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithHeader("Content-Type", "application/vnd.api+json").
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte(fmt.Sprintf(`{
        "data": {
          "type": "io.cozy.settings",
          "id": "io.cozy.settings.instance",
          "meta": {
            "rev": "%s"
          },
          "attributes": {
            "tz": "Europe/Paris",
            "email": "alice@example.net",
            "locale": "fr"
          }
        }
      }`, instanceRev))).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.HasValue("type", "io.cozy.settings")
		data.HasValue("id", "io.cozy.settings.instance")

		meta := data.Value("meta").Object()
		instanceRev = meta.Value("rev").String().NotEmpty().NotEqual(instanceRev).Raw()

		attrs := data.Value("attributes").Object()
		attrs.HasValue("email", "alice@example.net")
		attrs.HasValue("tz", "Europe/Paris")
		attrs.HasValue("locale", "fr")
	})

	t.Run("GetUpdatedInstance", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)
		svc.On("GetLegalNoticeUrl", testInstance).Return("", nil).Once()

		obj := e.GET("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.HasValue("type", "io.cozy.settings")
		data.HasValue("id", "io.cozy.settings.instance")

		meta := data.Value("meta").Object()
		meta.HasValue("rev", instanceRev)

		attrs := data.Value("attributes").Object()
		attrs.HasValue("email", "alice@example.net")
		attrs.HasValue("tz", "Europe/Paris")
		attrs.HasValue("locale", "fr")
	})

	t.Run("UpdatePassphraseWithTwoFactorAuth", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		e.PUT("/settings/instance/auth_mode").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "application/json").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "auth_mode": "two_factor_mail"
      }`)).
			Expect().Status(204)

		mailPassCode, err := testInstance.GenerateMailConfirmationCode()
		require.NoError(t, err)

		e.PUT("/settings/instance/auth_mode").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "application/json").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(fmt.Sprintf(`{
        "auth_mode": "two_factor_mail",
        "two_factor_activation_code": "%s"
      }`, mailPassCode))).
			Expect().Status(204)

		obj := e.PUT("/settings/passphrase").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "current_passphrase": "MyPassphrase"
      }`)).
			Expect().Status(200).
			JSON().Object()

		obj.Value("two_factor_token").String().NotEmpty()

		twoFactorToken, twoFactorPasscode, err := testInstance.GenerateTwoFactorSecrets()
		require.NoError(t, err)

		e.PUT("/settings/passphrase").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithJSON(map[string]interface{}{
				"new_passphrase":      "MyLastPassphrase",
				"two_factor_token":    twoFactorToken,
				"two_factor_passcode": twoFactorPasscode,
			}).
			Expect().Status(204)
	})

	t.Run("ListClients", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		e.GET("/settings/clients").
			WithCookie(sessCookie, "connected").
			Expect().Status(401)

		client := &oauth.Client{
			RedirectURIs:    []string{"http:/localhost:4000/oauth/callback"},
			ClientName:      "Cozy-desktop on my-new-laptop",
			ClientKind:      "desktop",
			ClientURI:       "https://docs.cozy.io/en/mobile/desktop.html",
			LogoURI:         "https://docs.cozy.io/assets/images/cozy-logo-docs.svg",
			PolicyURI:       "https://cozy.io/policy",
			SoftwareID:      "/github.com/cozy-labs/cozy-desktop",
			SoftwareVersion: "0.16.0",
		}
		regErr := client.Create(testInstance)
		assert.Nil(t, regErr)
		oauthClientID = client.ClientID

		obj := e.GET("/settings/clients").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Array()
		data.Length().IsEqual(2)

		el := data.Value(1).Object()
		el.HasValue("type", "io.cozy.oauth.clients")
		el.HasValue("id", client.ClientID)

		links := el.Value("links").Object()
		links.HasValue("self", "/settings/clients/"+client.ClientID)

		attrs := el.Value("attributes").Object()
		attrs.HasValue("client_name", client.ClientName)
		attrs.HasValue("client_kind", client.ClientKind)
		attrs.HasValue("client_uri", client.ClientURI)
		attrs.HasValue("logo_uri", client.LogoURI)
		attrs.HasValue("policy_uri", client.PolicyURI)
		attrs.HasValue("software_id", client.SoftwareID)
		attrs.HasValue("software_version", client.SoftwareVersion)
		attrs.NotContainsKey("client_secret")

		redirectURIs := attrs.Value("redirect_uris").Array()
		redirectURIs.Length().IsEqual(1)
		redirectURIs.Value(0).String().IsEqual(client.RedirectURIs[0])
	})

	t.Run("RevokeClient", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		e.DELETE("/settings/clients/"+oauthClientID).
			WithCookie(sessCookie, "connected").
			Expect().Status(401)

		e.DELETE("/settings/clients/"+oauthClientID).
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(204)

		obj := e.GET("/settings/clients").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Array()
		data.Length().IsEqual(1)
	})

	t.Run("PatchInstanceSameParams", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		doc1, err := testInstance.SettingsDocument()
		require.NoError(t, err)

		e.PUT("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
          "data": {
            "type": "io.cozy.settings",
            "id": "io.cozy.settings.instance",
            "meta": {
              "rev": "%s"
            },
            "attributes": {
              "tz": "Europe/Paris",
              "email": "alice@example.net",
              "locale": "fr"
            }
          }
        }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().NotEmpty()

		doc2, err := testInstance.SettingsDocument()
		assert.NoError(t, err)

		// Assert no changes
		assert.Equal(t, doc1.Rev(), doc2.Rev())
	})

	t.Run("PatchInstanceChangeParams", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		doc, err := testInstance.SettingsDocument()
		require.NoError(t, err)

		e.PUT("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
          "data": {
            "type": "io.cozy.settings",
            "id": "io.cozy.settings.instance",
            "meta": {
              "rev": "%s"
            },
            "attributes": {
              "tz": "Antarctica/McMurdo",
              "email": "alice@expat.eu",
              "locale": "de"
            }
          }
        }`, doc.Rev()))).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().NotEmpty()

		doc, err = testInstance.SettingsDocument()
		assert.NoError(t, err)

		assert.Equal(t, "Antarctica/McMurdo", doc.M["tz"].(string))
		assert.Equal(t, "alice@expat.eu", doc.M["email"].(string))
	})

	t.Run("PatchInstanceAddParam", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		doc1, err := testInstance.SettingsDocument()
		assert.NoError(t, err)

		e.PUT("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
          "data": {
            "type": "io.cozy.settings",
            "id": "io.cozy.settings.instance",
            "meta": {
              "rev": "%s"
            },
            "attributes": {
              "tz": "Europe/Berlin",
              "email": "alice@example.com",
              "how_old_are_you": "42"
            }
          }
        }`, doc1.Rev()))).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().NotEmpty()

		doc2, err := testInstance.SettingsDocument()
		assert.NoError(t, err)
		assert.NotEqual(t, doc1.Rev(), doc2.Rev())
		assert.Equal(t, "42", doc2.M["how_old_are_you"].(string))
		assert.Equal(t, "Europe/Berlin", doc2.M["tz"].(string))
		assert.Equal(t, "alice@example.com", doc2.M["email"].(string))
	})

	t.Run("PatchInstanceRemoveParams", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		doc1, err := testInstance.SettingsDocument()
		assert.NoError(t, err)

		e.PUT("/settings/instance").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(fmt.Sprintf(`{
          "data": {
            "type": "io.cozy.settings",
            "id": "io.cozy.settings.instance",
            "meta": {
              "rev": "%s"
            },
            "attributes": {
              "tz": "Europe/Berlin"
            }
          }
        }`, doc1.Rev()))).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().NotEmpty()

		doc2, err := testInstance.SettingsDocument()
		assert.NoError(t, err)
		assert.NotEqual(t, doc1.Rev(), doc2.Rev())
		assert.Equal(t, "Europe/Berlin", doc2.M["tz"].(string))
		_, ok := doc2.M["email"]
		assert.False(t, ok)
	})

	t.Run("FeatureFlags", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		_ = couchdb.DeleteDB(prefixer.GlobalPrefixer, consts.Settings)
		t.Cleanup(func() { _ = couchdb.DeleteDB(prefixer.GlobalPrefixer, consts.Settings) })

		obj := e.GET("/settings/flags").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.HasValue("type", "io.cozy.settings")
		data.HasValue("id", "io.cozy.settings.flags")

		data.Value("attributes").Object().IsEmpty()

		testInstance.FeatureFlags = map[string]interface{}{
			"from_instance_flag":   true,
			"from_multiple_source": "instance_flag",
			"json_object":          map[string]interface{}{"foo": "bar"},
		}
		testInstance.FeatureSets = []string{"set1", "set2"}
		require.NoError(t, instance.Update(testInstance))

		cache := config.GetConfig().CacheStorage

		cacheKey := fmt.Sprintf("flags:%s:%v", testInstance.ContextName, testInstance.FeatureSets)
		buf, err := json.Marshal(map[string]interface{}{
			"from_feature_sets":    true,
			"from_multiple_source": "manager",
		})
		assert.NoError(t, err)
		cache.Set(cacheKey, buf, 5*time.Second)
		ctxFlags := couchdb.JSONDoc{Type: consts.Settings}
		ctxFlags.M = map[string]interface{}{
			"ratio_0": []map[string]interface{}{
				{"ratio": 0, "value": "context"},
			},
			"ratio_1": []map[string]interface{}{
				{"ratio": 1, "value": "context"},
			},
			"ratio_0.000001": []map[string]interface{}{
				{"ratio": 0.000001, "value": "context"},
			},
			"ratio_0.999999": []map[string]interface{}{
				{"ratio": 0.999999, "value": "context"},
			},
		}

		id := fmt.Sprintf("%s.%s", consts.ContextFlagsSettingsID, testInstance.ContextName)
		ctxFlags.SetID(id)
		err = couchdb.CreateNamedDocWithDB(prefixer.GlobalPrefixer, &ctxFlags)
		assert.NoError(t, err)
		defFlags := couchdb.JSONDoc{Type: consts.Settings}
		defFlags.M = map[string]interface{}{
			"ratio_0":              "defaults",
			"ratio_1":              "defaults",
			"ratio_0.000001":       "defaults",
			"ratio_0.999999":       "defaults",
			"from_multiple_source": "defaults",
			"from_defaults":        true,
		}
		defFlags.SetID(consts.DefaultFlagsSettingsID)
		err = couchdb.CreateNamedDocWithDB(prefixer.GlobalPrefixer, &defFlags)
		assert.NoError(t, err)

		obj = e.GET("/settings/flags").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data = obj.Value("data").Object()
		data.HasValue("type", "io.cozy.settings")
		data.HasValue("id", "io.cozy.settings.flags")

		attrs := data.Value("attributes").Object()
		attrs.HasValue("from_instance_flag", true)
		attrs.HasValue("from_feature_sets", true)
		attrs.HasValue("from_defaults", true)
		attrs.HasValue("json_object", testInstance.FeatureFlags["json_object"])
		attrs.HasValue("from_multiple_source", "instance_flag")
		attrs.HasValue("ratio_0", "defaults")
		attrs.HasValue("ratio_0.000001", "defaults")
		attrs.HasValue("ratio_0.999999", "context")
		attrs.HasValue("ratio_1", "context")
	})

	t.Run("ClientsLimitExceededWithoutSession", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		e.GET("/settings/clients/limit-exceeded").
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(401)
	})

	t.Run("ClientsLimitExceededWithoutLimit", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		e.GET("/settings/clients/limit-exceeded").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8").
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(302).
			Header("location").IsEqual(testInstance.DefaultRedirection().String())

		redirect := "cozy://my-app"
		e.GET("/settings/clients/limit-exceeded").
			WithCookie(sessCookie, "connected").
			WithQuery("redirect", redirect).
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8").
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(302).
			Header("location").IsEqual(redirect)
	})

	t.Run("ClientsLimitExceededWithLimitExceeded", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		testutils.WithFlag(t, testInstance, "cozy.oauthclients.max", float64(0))

		// Create the OAuth client for the flagship app
		flagship := oauth.Client{
			RedirectURIs: []string{"cozy://flagship"},
			ClientName:   "flagship-app",
			ClientKind:   "mobile",
			SoftwareID:   "github.com/cozy/cozy-stack/testing/flagship",
			Flagship:     true,
		}
		require.Nil(t, flagship.Create(testInstance, oauth.NotPending))
		defer flagship.Delete(testInstance)

		e.GET("/settings/clients/limit-exceeded").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(200).
			HasContentType("text/html", "utf-8").
			Body().
			Contains("Disconnect one of your devices or change your Cozy offer to access your Cozy from this device.").
			Contains("/#/connectedDevices").
			NotContains("http://manager.example.org")

		testutils.WithManager(t, testInstance, testutils.ManagerConfig{URL: "http://manager.example.org", WithPremiumLinks: true})

		e.GET("/settings/clients/limit-exceeded").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(200).
			HasContentType("text/html", "utf-8").
			Body().
			Contains("Disconnect one of your devices or change your Cozy offer to access your Cozy from this device.").
			Contains("/#/connectedDevices").
			Contains("http://manager.example.org")

		testutils.WithFlag(t, testInstance, "flagship.iap.enabled", true)

		e.GET("/settings/clients/limit-exceeded").
			WithCookie(sessCookie, "connected").
			WithQuery("isFlagship", true).
			WithHeader("Authorization", "Bearer "+token).
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(200).
			HasContentType("text/html", "utf-8").
			Body().
			Contains("Disconnect one of your devices or change your Cozy offer to access your Cozy from this device.").
			Contains("/#/connectedDevices").
			NotContains("http://manager.example.org")

		e.GET("/settings/clients/limit-exceeded").
			WithCookie(sessCookie, "connected").
			WithQuery("isFlagship", true).
			WithQuery("isIapAvailable", true).
			WithHeader("Authorization", "Bearer "+token).
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(200).
			HasContentType("text/html", "utf-8").
			Body().
			Contains("Disconnect one of your devices or change your Cozy offer to access your Cozy from this device.").
			Contains("/#/connectedDevices").
			Contains("http://manager.example.org")
	})

	t.Run("ClientsLimitExceededWithLimitReached", func(t *testing.T) {
		e := testutils.CreateTestClient(t, tsURL)

		clients, _, err := oauth.GetConnectedUserClients(testInstance, 100, "")
		require.NoError(t, err)

		testutils.WithFlag(t, testInstance, "cozy.oauthclients.max", float64(len(clients)))

		e.GET("/settings/clients/limit-exceeded").
			WithCookie(sessCookie, "connected").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8").
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(302).
			Header("location").IsEqual(testInstance.DefaultRedirection().String())
	})

	t.Run("Avatar", func(t *testing.T) {
		t.Run("Put", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)
			sessCookie := session.CookieName(testInstance)

			// Create a sample avatar image
			avatarContent := "fake image content"

			e.PUT("/settings/avatar").
				WithCookie(sessCookie, "connected").
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-Type", "image/png").
				WithBytes([]byte(avatarContent)).
				Expect().Status(204)
		})

		t.Run("Delete", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)
			sessCookie := session.CookieName(testInstance)
			e.DELETE("/settings/avatar").
				WithCookie(sessCookie, "connected").
				WithHeader("Authorization", "Bearer "+token).
				Expect().Status(204)
		})
	})
}

func TestRegisterPassphraseForFlagshipApp(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	oauthClient := &oauth.Client{
		RedirectURIs:    []string{"http:/localhost:4000/oauth/callback"},
		ClientName:      "Cozy-desktop on my-new-laptop",
		ClientKind:      "desktop",
		ClientURI:       "https://docs.cozy.io/en/mobile/desktop.html",
		LogoURI:         "https://docs.cozy.io/assets/images/cozy-logo-docs.svg",
		PolicyURI:       "https://cozy.io/policy",
		SoftwareID:      "/github.com/cozy-labs/cozy-desktop",
		SoftwareVersion: "0.16.0",
	}

	setupFlagship := testutils.NewSetup(t, t.Name())
	testInstance := setupFlagship.GetTestInstance(&lifecycle.Options{
		Locale:      "en",
		Timezone:    "Europe/Berlin",
		Email:       "alice2@example.com",
		ContextName: "test-context",
	})

	svc := csettings.NewServiceMock(t)
	tsURL := setupRouter(t, testInstance, svc).URL

	require.Nil(t, oauthClient.Create(testInstance))
	client, err := oauth.FindClient(testInstance, oauthClient.ClientID)
	require.NoError(t, err)
	require.NoError(t, client.SetFlagship(testInstance))

	e := httpexpect.Default(t, tsURL)
	obj := e.POST("/settings/passphrase/flagship").
		WithJSON(map[string]interface{}{
			"passphrase":     "MyFirstPassphrase",
			"iterations":     50000,
			"register_token": hex.EncodeToString(testInstance.RegisterToken),
			"key":            "xxx-key-xxx",
			"public_key":     "xxx-public-key-xxx",
			"private_key":    "xxx-private-key-xxx",
			"client_id":      client.CouchID,
			"client_secret":  client.ClientSecret,
		}).
		Expect().Status(200).
		JSON().Object()

	obj.Value("access_token").String().NotEmpty()
	obj.Value("refresh_token").String().NotEmpty()
	obj.HasValue("scope", "*")
	obj.HasValue("token_type", "bearer")
}
