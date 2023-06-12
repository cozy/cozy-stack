package settings

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/cozy/cozy-stack/worker/mails"
)

func TestSettings(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var instanceRev string
	var oauthClientID string

	config.UseTestFile()
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

	ts := setup.GetTestServer("/settings", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	tsB := setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/auth": func(g *echo.Group) {
			g.Use(fakeAuthentication)
			auth.Routes(g)
		},
		"/settings": func(g *echo.Group) {
			g.Use(fakeAuthentication)
			Routes(g)
		},
	})
	tsB.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(tsB.Close)

	setupFlagship := testutils.NewSetup(t, t.Name())
	testInstanceFlagship := setupFlagship.GetTestInstance(&lifecycle.Options{
		Locale:      "en",
		Timezone:    "Europe/Berlin",
		Email:       "alice2@example.com",
		ContextName: "test-context",
	})
	tsC := setupFlagship.GetTestServer("/settings", Routes)
	t.Cleanup(tsC.Close)
	tsC.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	t.Run("GetContext", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/settings/context").
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200)
	})

	t.Run("PatchWithGoodRev", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		doc1, err := testInstance.SettingsDocument()
		require.NoError(t, err)

		// We are going to patch an instance with newer values, and give the good rev
		e.PUT("/settings/instance").
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
		e := testutils.CreateTestClient(t, ts.URL)

		// We are going to patch an instance with newer values, but with a totally
		// random rev
		rev := "6-2d9b7ef014d10549c2b4e206672d3e44"

		e.PUT("/settings/instance").
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
		e := testutils.CreateTestClient(t, ts.URL)

		// We are defining a random rev, but make no changes in the instance values
		rev := "6-2d9b7ef014d10549c2b4e206672d3e44"

		e.PUT("/settings/instance").
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
		e := testutils.CreateTestClient(t, ts.URL)

		// We are defining a random rev, but make changes in the instance values
		rev := "6-2d9b7ef014d10549c2b4e206672d3e44"

		e.PUT("/settings/instance").
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
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/settings/disk-usage").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		e.GET("/settings/disk-usage").
			Expect().Status(401)

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.disk-usage")

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("used", "0")
		attrs.ValueEqual("files", "0")
		attrs.ValueEqual("versions", "0")
	})

	t.Run("RegisterPassphraseWrongToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/settings/passphrase").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "passphrase":     "MyFirstPassphrase",
        "iterations":     5000,
        "register_token": "BADBEEF",
      }`)).
			Expect().Status(400)

		e.POST("/settings/passphrase").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "passphrase":     "MyFirstPassphrase",
        "iterations":     5000,
        "register_token": "XYZ",
      }`)).
			Expect().Status(400)
	})

	t.Run("RegisterPassphraseCorrectToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		res := e.POST("/settings/passphrase").
			WithJSON(map[string]interface{}{
				"passphrase":     "MyFirstPassphrase",
				"iterations":     5000,
				"register_token": hex.EncodeToString(testInstance.RegisterToken),
				"key":            "xxx",
			}).
			Expect().Status(200)

		res.Cookies().Length().Equal(1)
		res.Cookie("cozysessid").Value().NotEmpty()
	})

	t.Run("RegisterPassphraseForFlagshipApp", func(t *testing.T) {
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
		require.Nil(t, oauthClient.Create(testInstanceFlagship))
		client, err := oauth.FindClient(testInstanceFlagship, oauthClient.ClientID)
		require.NoError(t, err)
		require.NoError(t, client.SetFlagship(testInstanceFlagship))

		e := httpexpect.Default(t, tsC.URL)
		obj := e.POST("/settings/passphrase/flagship").
			WithJSON(map[string]interface{}{
				"passphrase":     "MyFirstPassphrase",
				"iterations":     5000,
				"register_token": hex.EncodeToString(testInstanceFlagship.RegisterToken),
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
		obj.ValueEqual("scope", "*")
		obj.ValueEqual("token_type", "bearer")
	})

	t.Run("UpdatePassphraseWithWrongPassphrase", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.PUT("/settings/passphrase").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "new_passphrase":     "MyPassphrase",
        "current_passphrase": "BADBEEF",
        "iterations":         5000
      }`)).
			Expect().Status(400)
	})

	t.Run("UpdatePassphraseSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		res := e.PUT("/settings/passphrase").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "new_passphrase":     "MyUpdatedPassphrase",
        "current_passphrase": "MyFirstPassphrase",
        "iterations":         5000
      }`)).
			Expect().Status(204)

		res.Cookies().Length().Equal(1)
		res.Cookie("cozysessid").Value().NotEmpty()
	})

	t.Run("UpdatePassphraseWithForce", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.PUT("/settings/passphrase").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "new_passphrase": "MyPassphrase",
        "iterations":     5000,
        "force":          true
      }`)).
			Expect().Status(400)

		passwordDefined := false
		testInstance.PasswordDefined = &passwordDefined

		e.PUT("/settings/passphrase").
			WithQuery("Force", true).
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "new_passphrase": "MyPassphrase",
        "iterations":     5000,
        "force":          true
      }`)).
			Expect().Status(204)
	})

	t.Run("CheckPassphrase", func(t *testing.T) {
		t.Run("invalid", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.POST("/settings/passphrase/check").
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-Type", "application/json").
				WithBytes([]byte(`{
        "passphrase": "Invalid Passphrase"
      }`)).
				Expect().Status(403)
		})

		t.Run("valid", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.POST("/settings/passphrase/check").
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
			e := testutils.CreateTestClient(t, ts.URL)

			e.GET("/settings/hint").
				WithHeader("Authorization", "Bearer "+token).
				Expect().Status(404)
		})

		t.Run("WithHint", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			setting, err := settings.Get(testInstance)
			assert.NoError(t, err)
			setting.PassphraseHint = "my hint"
			err = couchdb.UpdateDoc(testInstance, setting)
			assert.NoError(t, err)

			e.GET("/settings/hint").
				WithHeader("Authorization", "Bearer "+token).
				Expect().Status(204)
		})
	})

	t.Run("UpdateHint", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.PUT("/settings/hint").
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
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/settings/passphrase").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.passphrase")

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("salt", "me@"+testInstance.Domain)
		attrs.ValueEqual("kdf", 0.0)
		attrs.ValueEqual("iterations", 5000.0)
	})

	t.Run("GetCapabilities", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/settings/instance").
			Expect().Status(401)

		obj := e.GET("/settings/capabilities").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.capabilities")

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("file_versioning", true)
		attrs.ValueEqual("can_auth_with_password", true)
		attrs.ValueEqual("can_auth_with_magic_links", false)
		attrs.ValueEqual("can_auth_with_oidc", false)
	})

	t.Run("GetInstance", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/settings/instance").
			Expect().Status(401)

		testInstance.RegisterToken = []byte("test")

		e.GET("/settings/instance").
			WithQuery("registerToken", "74657374").
			Expect().Status(200)

		testInstance.RegisterToken = []byte{}

		obj := e.GET("/settings/instance").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.instance")

		meta := data.Value("meta").Object()
		instanceRev = meta.Value("rev").String().NotEmpty().Raw()

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("email", "alice@example.org")
		attrs.ValueEqual("tz", "Europe/London")
		attrs.ValueEqual("locale", "en")
		attrs.ValueEqual("password_defined", true)
	})

	t.Run("UpdateInstance", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.PUT("/settings/instance").
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
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.instance")

		meta := data.Value("meta").Object()
		instanceRev = meta.Value("rev").String().NotEmpty().NotEqual(instanceRev).Raw()

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("email", "alice@example.net")
		attrs.ValueEqual("tz", "Europe/Paris")
		attrs.ValueEqual("locale", "fr")
	})

	t.Run("GetUpdatedInstance", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/settings/instance").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "application/vnd.api+json").
			WithHeader("Content-Type", "application/vnd.api+json").
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.instance")

		meta := data.Value("meta").Object()
		meta.ValueEqual("rev", instanceRev)

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("email", "alice@example.net")
		attrs.ValueEqual("tz", "Europe/Paris")
		attrs.ValueEqual("locale", "fr")
	})

	t.Run("UpdatePassphraseWithTwoFactorAuth", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.PUT("/settings/instance/auth_mode").
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
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Accept", "application/json").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(fmt.Sprintf(`{
        "auth_mode": "two_factor_mail",
        "two_factor_activation_code": "%s"
      }`, mailPassCode))).
			Expect().Status(204)

		obj := e.PUT("/settings/passphrase").
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
			WithHeader("Authorization", "Bearer "+token).
			WithJSON(map[string]interface{}{
				"new_passphrase":      "MyLastPassphrase",
				"two_factor_token":    twoFactorToken,
				"two_factor_passcode": twoFactorPasscode,
			}).
			Expect().Status(204)
	})

	t.Run("ListClients", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/settings/clients").
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
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Array()
		data.Length().Equal(2)

		el := data.Element(1).Object()
		el.ValueEqual("type", "io.cozy.oauth.clients")
		el.ValueEqual("id", client.ClientID)

		links := el.Value("links").Object()
		links.ValueEqual("self", "/settings/clients/"+client.ClientID)

		attrs := el.Value("attributes").Object()
		attrs.ValueEqual("client_name", client.ClientName)
		attrs.ValueEqual("client_kind", client.ClientKind)
		attrs.ValueEqual("client_uri", client.ClientURI)
		attrs.ValueEqual("logo_uri", client.LogoURI)
		attrs.ValueEqual("policy_uri", client.PolicyURI)
		attrs.ValueEqual("software_id", client.SoftwareID)
		attrs.ValueEqual("software_version", client.SoftwareVersion)
		attrs.NotContainsKey("client_secret")

		redirectURIs := attrs.Value("redirect_uris").Array()
		redirectURIs.Length().Equal(1)
		redirectURIs.First().String().Equal(client.RedirectURIs[0])
	})

	t.Run("RevokeClient", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.DELETE("/settings/clients/" + oauthClientID).
			Expect().Status(401)

		e.DELETE("/settings/clients/"+oauthClientID).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(204)

		obj := e.GET("/settings/clients").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Array()
		data.Length().Equal(1)
	})

	t.Run("RedirectOnboardingSecret", func(t *testing.T) {
		e := httpexpect.Default(t, tsB.URL)

		// Without onboarding
		e.GET("/settings/onboarded").
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Equal(testInstance.OnboardedRedirection().String())

		// With onboarding
		deeplink := "cozydrive://testinstance.com"
		oauthClient := &oauth.Client{
			RedirectURIs:     []string{deeplink},
			ClientName:       "CozyTest",
			SoftwareID:       "/github.com/cozy-labs/cozy-desktop",
			OnboardingSecret: "foobar",
			OnboardingApp:    "test",
		}

		oauthClient.Create(testInstance)

		redirectURL := e.GET("/settings/onboarded").
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").
			NotEqual(testInstance.OnboardedRedirection().String()).
			Contains("/auth/authorize").Raw()

		u, err := url.Parse(redirectURL)
		require.NoError(t, err)

		values := u.Query()
		assert.Equal(t, values.Get("redirect_uri"), deeplink)
	})

	t.Run("PatchInstanceSameParams", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		doc1, err := testInstance.SettingsDocument()
		require.NoError(t, err)

		e.PUT("/settings/instance").
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
		e := testutils.CreateTestClient(t, ts.URL)

		doc, err := testInstance.SettingsDocument()
		require.NoError(t, err)

		e.PUT("/settings/instance").
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
		e := testutils.CreateTestClient(t, ts.URL)

		doc1, err := testInstance.SettingsDocument()
		assert.NoError(t, err)

		e.PUT("/settings/instance").
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
		e := testutils.CreateTestClient(t, ts.URL)

		doc1, err := testInstance.SettingsDocument()
		assert.NoError(t, err)

		e.PUT("/settings/instance").
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
		e := testutils.CreateTestClient(t, ts.URL)

		_ = couchdb.DeleteDB(prefixer.GlobalPrefixer, consts.Settings)
		t.Cleanup(func() { _ = couchdb.DeleteDB(prefixer.GlobalPrefixer, consts.Settings) })

		obj := e.GET("/settings/flags").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.flags")

		data.Value("attributes").Object().Empty()

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
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data = obj.Value("data").Object()
		data.ValueEqual("type", "io.cozy.settings")
		data.ValueEqual("id", "io.cozy.settings.flags")

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("from_instance_flag", true)
		attrs.ValueEqual("from_feature_sets", true)
		attrs.ValueEqual("from_defaults", true)
		attrs.ValueEqual("json_object", testInstance.FeatureFlags["json_object"])
		attrs.ValueEqual("from_multiple_source", "instance_flag")
		attrs.ValueEqual("ratio_0", "defaults")
		attrs.ValueEqual("ratio_0.000001", "defaults")
		attrs.ValueEqual("ratio_0.999999", "context")
		attrs.ValueEqual("ratio_1", "context")
	})
}

func fakeAuthentication(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := c.Get("instance").(*instance.Instance)
		sess, _ := session.New(instance, session.LongRun)
		c.Set("session", sess)
		return next(c)
	}
}
