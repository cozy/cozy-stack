// spec package is introduced to avoid circular dependencies since this
// particular test requires to depend on routing directly to expose the API and
// the APP server.
package auth_test

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gavv/httpexpect/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const domain = "cozy.example.net"

func TestAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var JWTSecret = []byte("foobar")

	var sessionID string

	var clientID string
	var clientSecret string
	var registrationToken string
	var altClientID string
	var altRegistrationToken string
	var csrfToken string
	var code string
	var refreshToken string

	config.UseTestFile()
	conf := config.GetConfig()
	conf.Assets = "../../assets"
	conf.DeprecatedApps = config.DeprecatedAppsCfg{
		Apps: []config.DeprecatedApp{
			{
				SoftwareID: "github.com/some-deprecated-app",
				Name:       "some-deprecated-app",
				StoreURLs: map[string]string{
					// Must test "market://" url
					"android": "market://some-market-url",
					"iphone":  "https://some-basic-url",
				},
			},
		},
	}

	conf.Authentication = make(map[string]interface{})
	confAuth := make(map[string]interface{})
	confAuth["jwt_secret"] = base64.StdEncoding.EncodeToString(JWTSecret)
	conf.Authentication[config.DefaultInstanceContext] = confAuth

	_ = web.LoadSupportedLocales()
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())

	testInstance := setup.GetTestInstance(&lifecycle.Options{
		Domain:        domain,
		Email:         "test@spam.cozycloud.cc",
		Passphrase:    "MyPassphrase",
		KdfIterations: 5000,
		Key:           "xxx",
	})

	ts := setup.GetTestServer("/test", fakeAPI, func(r *echo.Echo) *echo.Echo {
		handler, err := web.CreateSubdomainProxy(r, apps.Serve)
		require.NoError(t, err, "Cant start subdomain proxy")
		return handler
	})
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	require.NoError(t, dynamic.InitDynamicAssetFS(config.FsURL().String()), "Could not init dynamic FS")

	t.Run("InstanceBlocked", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Block the instance
		testInstance.Blocked = true
		require.NoError(t, instance.Update(testInstance))

		e.GET("/auth/login").
			WithHost(testInstance.Domain).
			Expect().Status(http.StatusServiceUnavailable)

		// Trying with a Accept: text/html header to simulate a browser
		body := e.GET("/auth/login").
			WithHost(testInstance.Domain).
			WithHeader("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8").
			Expect().Status(http.StatusServiceUnavailable).
			Body()

		body.Contains("<title>Cozy</title>")
		body.Contains("Your Cozy has been blocked</h1>")

		// Unblock the instance
		testInstance.Blocked = false
		_ = instance.Update(testInstance)
	})

	t.Run("IsLoggedInWhenNotLoggedIn", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/test").
			WithHost(domain).
			Expect().Status(200).
			Text(httpexpect.ContentOpts{MediaType: "text/plain"}).
			Equal("who_are_you")
	})

	t.Run("HomeWhenNotLoggedIn", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/").
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Equal("https://cozy.example.net/auth/login")
	})

	t.Run("HomeWhenNotLoggedInWithJWT", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/").WithQuery("jwt", "foobar").
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Equal("https://cozy.example.net/auth/login?jwt=foobar")
	})

	t.Run("ShowLoginPage", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/login").
			WithHost(domain).
			Expect().Status(200).
			Text(httpexpect.ContentOpts{MediaType: "text/html"}).
			Contains("Log in")
	})

	t.Run("ShowLoginPageWithRedirectBadURL", func(t *testing.T) {
		testsRedirect := []string{" ", "foo.bar", "ftp://sub." + domain + "/foo"}

		for _, test := range testsRedirect {
			t.Run(test, func(t *testing.T) {
				e := testutils.CreateTestClient(t, ts.URL)

				e.GET("/auth/login").WithQuery("redirect", test).
					WithHost(domain).
					Expect().Status(400).
					Text(httpexpect.ContentOpts{MediaType: "text/plain"}).Contains("bad url")
			})
		}
	})

	t.Run("ShowLoginPageWithRedirectXSS", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/login").WithQuery("redirect", "https://sub."+domain+"/<script>alert('foo')</script>").
			WithHost(domain).
			Expect().Status(200).
			Text(httpexpect.ContentOpts{MediaType: "text/html"}).
			Contains("%3Cscript%3Ealert%28%27foo%27%29%3C/script%3E").
			NotContains("<script>")
	})

	t.Run("ShowLoginPageWithRedirectFragment", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/login").WithQuery("redirect", "https://"+domain+"/auth/authorize#myfragment").
			WithHost(domain).
			Expect().Status(200).
			Text(httpexpect.ContentOpts{MediaType: "text/html"}).
			NotContains("myfragment").
			Contains(`<input id="redirect" type="hidden" name="redirect" value="https://cozy.example.net/auth/authorize#=" />`)
	})

	t.Run("ShowLoginPageWithRedirectSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/login").WithQuery("redirect", "https://sub."+domain+"/foo/bar?query=foo#myfragment").
			WithHost(domain).
			Expect().Status(200).
			Text(httpexpect.ContentOpts{MediaType: "text/html"}).
			Contains(`<input id="redirect" type="hidden" name="redirect" value="https://sub.cozy.example.net/foo/bar?query=foo#myfragment" />`)
	})

	t.Run("LoginWithoutCSRFToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/login").WithFormField("passphrase", "MyPassphrase").
			WithHost(domain).
			Expect().Status(400)
	})

	t.Run("LoginWithBadPassphrase", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		token := getLoginCSRFToken(e)

		e.POST("/auth/login").
			WithHost(domain).
			WithCookie("_csrf", token).
			WithFormField("csrf_token", token).
			WithFormField("passphrase", "Nope").
			Expect().Status(401)
	})

	t.Run("LoginWithGoodPassphrase", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		token := getLoginCSRFToken(e)

		res := e.POST("/auth/login").
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithCookie("_csrf", token).
			WithFormField("csrf_token", token).
			WithFormField("passphrase", "MyPassphrase").
			Expect().Status(303)

		res.Header("Location").Equal("https://home.cozy.example.net/")
		res.Cookies().Length().Equal(2)
		res.Cookie("_csrf").Value().Equal(token)
		res.Cookie(session.CookieName(testInstance)).Value().NotEmpty()

		var results []*session.LoginEntry
		err := couchdb.GetAllDocs(
			testInstance,
			consts.SessionsLogins,
			&couchdb.AllDocsRequest{Limit: 100},
			&results,
		)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(results))
		assert.Equal(t, "Go-http-client/1.1", results[0].UA)
		assert.Equal(t, "127.0.0.1", results[0].IP)
		assert.False(t, results[0].CreatedAt.IsZero())
	})

	t.Run("LoginWithRedirect", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			token := getLoginCSRFToken(e)
			e.POST("/auth/login").
				WithHost(domain).
				WithRedirectPolicy(httpexpect.DontFollowRedirects).
				WithCookie("_csrf", token).
				WithFormField("csrf_token", token).
				WithFormField("passphrase", "MyPassphrase").
				WithFormField("redirect", "https://sub."+domain+"/#myfragment").
				Expect().Status(303).
				Header("Location").Equal("https://sub.cozy.example.net/#myfragment")
		})

		t.Run("invalid redirect field", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			token := getLoginCSRFToken(e)
			e.POST("/auth/login").
				WithHost(domain).
				WithRedirectPolicy(httpexpect.DontFollowRedirects).
				WithCookie("_csrf", token).
				WithFormField("csrf_token", token).
				WithFormField("passphrase", "MyPassphrase").
				WithFormField("redirect", "foo.bar").
				Expect().Status(400)
		})
	})

	t.Run("DelegatedJWTLoginWithRedirect", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, session.ExternalClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "sruti",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			Name:  domain,
			Email: "sruti@external.notmycozy.net",
			Code:  "student",
		})
		signed, err := token.SignedString(JWTSecret)
		require.NoError(t, err)

		sessionID = e.GET("/auth/login").WithQuery("jwt", signed).
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Cookie(session.CookieName(testInstance)).Value().NotEmpty().Raw()
	})

	t.Run("IsLoggedInAfterLogin", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/test").
			WithHost(domain).
			WithCookie(session.CookieName(testInstance), sessionID).
			Expect().Status(200).
			Text(httpexpect.ContentOpts{MediaType: "text/plain"}).
			Equal("logged_in")
	})

	t.Run("HomeWhenLoggedIn", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/").
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithCookie(session.CookieName(testInstance), sessionID).
			Expect().Status(303).
			Header("Location").Equal("https://home.cozy.example.net/")
	})

	t.Run("RegisterClientNotJSON", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/register").
			WithHost(domain).
			WithFormField("foo", "bar").
			Expect().Status(400)
	})

	t.Run("RegisterClientNoRedirectURI", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/auth/register").
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithJSON(map[string]string{
				"client_name": "cozy-test",
				"software_id": "github.com/cozy/cozy-test",
			}).
			Expect().Status(400).
			JSON().Object()

		obj.ValueEqual("error", "invalid_redirect_uri")
		obj.ValueEqual("error_description", "redirect_uris is mandatory")
	})

	t.Run("RegisterClient with error", func(t *testing.T) {
		tests := []struct {
			name           string
			body           map[string]interface{}
			err            string
			errDescription string
		}{
			{
				name: "InvalidRedirectURI",
				body: map[string]interface{}{
					"redirect_uris": []string{"http://example.org/foo#bar"},
					"client_name":   "cozy-test",
					"software_id":   "github.com/cozy/cozy-test",
				},
				err:            "invalid_redirect_uri",
				errDescription: "http://example.org/foo#bar is invalid",
			},
			{
				name: "NoClientName",
				body: map[string]interface{}{
					"redirect_uris": []string{"https://example.org/oauth/callback"},
					"software_id":   "github.com/cozy/cozy-test",
				},
				err:            "invalid_client_metadata",
				errDescription: "client_name is mandatory",
			},
			{
				name: "NoSoftwareID",
				body: map[string]interface{}{
					"redirect_uris": []string{"https://example.org/oauth/callback"},
					"client_name":   "cozy-test",
				},
				err:            "invalid_client_metadata",
				errDescription: "software_id is mandatory",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				e := testutils.CreateTestClient(t, ts.URL)

				obj := e.POST("/auth/register").
					WithHost(domain).
					WithHeader("Accept", "application/json").
					WithJSON(test.body).
					Expect().Status(400).
					JSON().Object()

				obj.ValueEqual("error", test.err)
				obj.ValueEqual("error_description", test.errDescription)
			})
		}
	})

	t.Run("RegisterClientSuccessWithJustMandatoryFields", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/auth/register").
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithJSON(map[string]interface{}{
				"redirect_uris": []string{"https://example.org/oauth/callback"},
				"client_name":   "cozy-test",
				"software_id":   "github.com/cozy/cozy-test",
			}).
			Expect().Status(201).
			JSON().Object()

		obj.ValueEqual("client_secret_expires_at", 0)
		obj.ValueEqual("redirect_uris", []string{"https://example.org/oauth/callback"})
		obj.ValueEqual("grant_types", []string{"authorization_code", "refresh_token"})
		obj.ValueEqual("response_types", []string{"code"})
		obj.ValueEqual("client_name", "cozy-test")
		obj.ValueEqual("software_id", "github.com/cozy/cozy-test")

		clientID = obj.Value("client_id").String().NotEmpty().NotEqual("ignored").Raw()
		clientSecret = obj.Value("client_secret").String().NotEmpty().NotEqual("ignored").Raw()
		registrationToken = obj.Value("registration_access_token").String().NotEmpty().NotEqual("ignored").Raw()
	})

	t.Run("RegisterClientSuccessWithAllFields", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/auth/register").
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithJSON(map[string]interface{}{
				"_id":                       "ignored",
				"_rev":                      "ignored",
				"client_id":                 "ignored",
				"client_secret":             "ignored",
				"client_secret_expires_at":  42,
				"registration_access_token": "ignored",
				"redirect_uris":             []string{"https://example.org/oauth/callback"},
				"grant_types":               []string{"ignored"},
				"response_types":            []string{"ignored"},
				"client_name":               "new-cozy-test",
				"client_kind":               "test",
				"client_uri":                "https://github.com/cozy/cozy-test",
				"logo_uri":                  "https://raw.github.com/cozy/cozy-setup/gh-pages/assets/images/happycloud.png",
				"policy_uri":                "https://github/com/cozy/cozy-test/master/policy.md",
				"software_id":               "github.com/cozy/cozy-test",
				"software_version":          "v0.1.2",
			}).
			Expect().Status(201).
			JSON().Object()

		obj.NotContainsKey("_id")
		obj.NotContainsKey("_rev")
		obj.Value("registration_access_token").String().NotEmpty().NotEqual("ignored").Raw()

		obj.ValueEqual("client_secret_expires_at", 0)
		obj.ValueEqual("redirect_uris", []string{"https://example.org/oauth/callback"})
		obj.ValueEqual("grant_types", []string{"authorization_code", "refresh_token"})
		obj.ValueEqual("response_types", []string{"code"})
		obj.ValueEqual("client_name", "new-cozy-test")
		obj.ValueEqual("client_kind", "test")
		obj.ValueEqual("client_uri", "https://github.com/cozy/cozy-test")
		obj.ValueEqual("logo_uri", "https://raw.github.com/cozy/cozy-setup/gh-pages/assets/images/happycloud.png")
		obj.ValueEqual("policy_uri", "https://github/com/cozy/cozy-test/master/policy.md")
		obj.ValueEqual("software_id", "github.com/cozy/cozy-test")
		obj.ValueEqual("software_version", "v0.1.2")

		altClientID = obj.Value("client_id").String().NotEmpty().NotEqual("ignored").Raw()
		altRegistrationToken = obj.Value("registration_access_token").String().NotEmpty().NotEqual("ignored").Raw()
	})

	t.Run("RegisterSharingClientSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/auth/register").
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithJSON(map[string]interface{}{
				"redirect_uris": []string{"https://cozy.example.org/sharings/answer"},
				"client_name":   "John",
				"software_id":   "github.com/cozy/cozy-stack",
				"client_kind":   "sharing",
				"client_uri":    "https://cozy.example.org",
			}).
			Expect().Status(201).
			JSON().Object()

		obj.Value("client_id").String().NotEmpty().NotEqual("ignored").Raw()
		obj.Value("client_secret").String().NotEmpty().NotEqual("ignored").Raw()
		obj.Value("registration_access_token").String().NotEmpty().NotEqual("ignored").Raw()

		obj.ValueEqual("client_secret_expires_at", 0)
		obj.ValueEqual("redirect_uris", []string{"https://cozy.example.org/sharings/answer"})
		obj.ValueEqual("client_name", "John")
		obj.ValueEqual("software_id", "github.com/cozy/cozy-stack")
	})

	t.Run("DeleteClientNoToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.DELETE("/auth/register/" + altClientID).
			WithHost(domain).
			Expect().Status(401)
	})

	t.Run("DeleteClientSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.DELETE("/auth/register/"+altClientID).
			WithHost(domain).
			WithHeader("Authorization", "Bearer "+altRegistrationToken).
			Expect().Status(204)

		// And next calls should return a 204 too
		e.DELETE("/auth/register/"+altClientID).
			WithHost(domain).
			WithHeader("Authorization", "Bearer "+altRegistrationToken).
			Expect().Status(204)
	})

	t.Run("ReadClientNoToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/register/"+clientID).
			WithHost(domain).
			WithHeader("Accept", "application/json").
			Expect().Status(401).
			Body().NotContains(clientSecret)
	})

	t.Run("ReadClientInvalidToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/register/"+clientID).
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+altRegistrationToken).
			Expect().Status(401).
			Body().NotContains(clientSecret)
	})

	t.Run("DeprecatedApp", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		deprecatedClientID := e.POST("/auth/register").
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithJSON(map[string]interface{}{
				"redirect_uris": []string{"https://example.org/oauth/callback"},
				"client_name":   "cozy-test",
				"software_id":   "github.com/some-deprecated-app",
			}).
			Expect().Status(201).
			JSON().Object().
			Value("client_id").String().NotEmpty().NotEqual("ignored").Raw()

		body := e.GET("/auth/authorize").
			WithHeader("User-Agent", "Mozilla/5.0 (Linux; U; Android 1.5; de-; HTC Magic Build/PLAT-RC33) AppleWebKit/528.5+ (KHTML, like Gecko) Version/3.1.2 Mobile Safari/525.20.1").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("scope", "files:read").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithQuery("client_id", deprecatedClientID).
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(200).
			Body()

		body.Contains(`href="market://some-market-url"`)
	})

	t.Run("ReadClientInvalidClientID", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/register/"+altClientID).
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+altRegistrationToken).
			Expect().Status(404)
	})

	t.Run("ReadClientSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/auth/register/"+clientID).
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+registrationToken).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("client_id", clientID)
		obj.ValueEqual("client_secret", clientSecret)
		obj.ValueEqual("client_secret_expires_at", 0)
		obj.NotContainsKey("registration_access_token")
		obj.ValueEqual("redirect_uris", []string{"https://example.org/oauth/callback"})
		obj.ValueEqual("grant_types", []string{"authorization_code", "refresh_token"})
		obj.ValueEqual("response_types", []string{"code"})
		obj.ValueEqual("client_name", "cozy-test")
		obj.ValueEqual("software_id", "github.com/cozy/cozy-test")
	})

	t.Run("UpdateClientDeletedClientID", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.PUT("/auth/register/"+altClientID).
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+registrationToken).
			WithJSON(map[string]string{
				"client_id": altClientID,
			}).
			Expect().Status(404)
	})

	t.Run("UpdateClientInvalidClientID", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.PUT("/auth/register/"+clientID).
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+registrationToken).
			WithJSON(map[string]string{
				"client_id": "123456789",
			}).
			Expect().Status(400).
			JSON().Object()

		obj.ValueEqual("error", "invalid_client_id")
		obj.ValueEqual("error_description", "client_id is mandatory")
	})

	t.Run("UpdateClientNoRedirectURI", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.PUT("/auth/register/"+clientID).
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+registrationToken).
			WithJSON(map[string]string{
				"client_id":   clientID,
				"client_name": "cozy-test",
				"software_id": "github.com/cozy/cozy-test",
			}).
			Expect().Status(400).
			JSON().Object()

		obj.ValueEqual("error", "invalid_redirect_uri")
		obj.ValueEqual("error_description", "redirect_uris is mandatory")
	})

	t.Run("UpdateClientSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.PUT("/auth/register/"+clientID).
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+registrationToken).
			WithJSON(map[string]interface{}{
				"client_id":        clientID,
				"redirect_uris":    []string{"https://example.org/oauth/callback"},
				"client_name":      "cozy-test",
				"software_id":      "github.com/cozy/cozy-test",
				"software_version": "v0.1.3",
			}).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("client_id", clientID)
		obj.ValueEqual("client_secret", clientSecret)
		obj.ValueEqual("client_secret_expires_at", 0)
		obj.NotContainsKey("registration_access_token")
		obj.ValueEqual("redirect_uris", []string{"https://example.org/oauth/callback"})
		obj.ValueEqual("grant_types", []string{"authorization_code", "refresh_token"})
		obj.ValueEqual("response_types", []string{"code"})
		obj.ValueEqual("client_name", "cozy-test")
		obj.ValueEqual("software_id", "github.com/cozy/cozy-test")
		obj.ValueEqual("software_version", "v0.1.3")
	})

	t.Run("UpdateClientSecret", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.PUT("/auth/register/"+clientID).
			WithHost(domain).
			WithHeader("Accept", "application/json").
			WithHeader("Authorization", "Bearer "+registrationToken).
			WithJSON(map[string]interface{}{
				"client_id":        clientID,
				"client_secret":    clientSecret,
				"redirect_uris":    []string{"https://example.org/oauth/callback"},
				"client_name":      "cozy-test",
				"software_id":      "github.com/cozy/cozy-test",
				"software_version": "v0.1.4",
			}).
			Expect().Status(200).
			JSON().Object()

		clientSecret = obj.Value("client_secret").String().NotEqual(clientSecret).Raw()

		obj.ValueEqual("client_id", clientID)
		obj.ValueEqual("client_secret_expires_at", 0)
		obj.NotContainsKey("registration_access_token")
		obj.ValueEqual("redirect_uris", []string{"https://example.org/oauth/callback"})
		obj.ValueEqual("grant_types", []string{"authorization_code", "refresh_token"})
		obj.ValueEqual("response_types", []string{"code"})
		obj.ValueEqual("client_name", "cozy-test")
		obj.ValueEqual("software_id", "github.com/cozy/cozy-test")
		obj.ValueEqual("software_version", "v0.1.4")
	})

	t.Run("AuthorizeFormRedirectsWhenNotLoggedIn", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("scope", "files:read").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithQuery("client_id", clientID).
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303)
	})

	t.Run("AuthorizeFormBadResponseType", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/authorize").
			WithQuery("response_type", "token"). // invalid
			WithQuery("state", "123456").
			WithQuery("scope", "files:read").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithQuery("client_id", clientID).
			WithHost(domain).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("Invalid response type")
	})

	t.Run("AuthorizeFormNoState", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("scope", "files:read").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithQuery("client_id", clientID).
			WithHost(domain).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The state parameter is mandatory")
	})

	t.Run("AuthorizeFormNoClientId", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("scope", "files:read").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithHost(domain).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The client_id parameter is mandatory")
	})

	t.Run("AuthorizeFormNoRedirectURI", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("scope", "files:read").
			WithQuery("client_id", clientID).
			WithHost(domain).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The redirect_uri parameter is mandatory")
	})

	t.Run("AuthorizeFormNoScope", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithQuery("client_id", clientID).
			WithHost(domain).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The scope parameter is mandatory")
	})

	t.Run("AuthorizeFormInvalidClient", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithQuery("scope", "files:read").
			WithQuery("client_id", "foo").
			WithHost(domain).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The client must be registered")
	})

	t.Run("AuthorizeFormInvalidRedirectURI", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("redirect_uri", "https://evil.com/").
			WithQuery("scope", "files:read").
			WithQuery("client_id", clientID).
			WithHost(domain).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The redirect_uri parameter doesn&#39;t match the registered ones")
	})

	t.Run("AuthorizeFormSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		resBody := e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithQuery("scope", "files:read").
			WithQuery("client_id", clientID).
			WithCookie(session.CookieName(testInstance), sessionID).
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(200).
			ContentType("text/html", "utf-8").
			Body()

		resBody.Contains("would like permission to access your Cozy")
		matches := resBody.Match(`<input type="hidden" name="csrf_token" value="(\w+)"`)
		matches.Length().Equal(2)
		csrfToken = matches.Index(1).Raw()
	})

	t.Run("AuthorizeFormClientMobileApp", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		var oauthClient oauth.Client

		u := "https://example.org/oauth/callback"
		oauthClient.RedirectURIs = []string{u}
		oauthClient.ClientName = "cozy-test-2"
		oauthClient.SoftwareID = "registry://drive"
		oauthClient.Create(testInstance)

		e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithQuery("client_id", oauthClient.ClientID).
			WithCookie(session.CookieName(testInstance), sessionID).
			WithHost(domain).
			Expect().Status(200).
			ContentType("text/html", "utf-8").
			Body().
			Contains("io.cozy.files")
	})

	t.Run("AuthorizeFormFlagshipApp", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		var oauthClient oauth.Client

		u := "https://example.org/oauth/callback"
		oauthClient.RedirectURIs = []string{u}
		oauthClient.ClientName = "cozy-test-2"
		oauthClient.SoftwareID = "registry://drive"
		oauthClient.Create(testInstance)

		e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithQuery("client_id", clientID).
			WithQuery("scope", "*").
			WithQuery("code_challenge", "w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI").
			WithQuery("code_challenge_method", "S256").
			WithCookie(session.CookieName(testInstance), sessionID).
			WithHost(domain).
			Expect().Status(200).
			ContentType("text/html", "utf-8").
			Body().
			NotContains("would like permission to access your Cozy").
			Contains("The origin of this application is not certified.")
	})

	t.Run("AuthorizeWhenNotLoggedIn", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/authorize").
			WithFormField("state", "123456").
			WithFormField("client_id", clientID).
			WithFormField("redirect_uri", "https://example.org/oauth/callback").
			WithFormField("scope", "files:read").
			WithFormField("csrf_token", csrfToken).
			WithFormField("response_type", "code").
			WithHost(domain).
			Expect().Status(403)
	})

	t.Run("AuthorizeWithInvalidCSRFToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/authorize").
			WithFormField("state", "123456").
			WithFormField("client_id", clientID).
			WithFormField("redirect_uri", "https://example.org/oauth/callback").
			WithFormField("scope", "files:read").
			WithFormField("csrf_token", "azertyuiop").
			WithFormField("response_type", "code").
			WithHost(domain).
			WithCookie("_csrf", csrfToken).
			Expect().Status(403).
			Text(httpexpect.ContentOpts{MediaType: "text/plain"}).
			Contains("invalid csrf token")
	})

	t.Run("AuthorizeWithNoState", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/authorize").
			WithFormField("client_id", clientID).
			WithFormField("redirect_uri", "https://example.org/oauth/callback").
			WithFormField("scope", "files:read").
			WithFormField("csrf_token", csrfToken).
			WithFormField("response_type", "code").
			WithHost(domain).
			WithCookie("_csrf", csrfToken).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The state parameter is mandatory")
	})

	t.Run("AuthorizeWithNoClientID", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/authorize").
			WithFormField("state", "123456").
			WithFormField("redirect_uri", "https://example.org/oauth/callback").
			WithFormField("scope", "files:read").
			WithFormField("csrf_token", csrfToken).
			WithFormField("response_type", "code").
			WithHost(domain).
			WithCookie("_csrf", csrfToken).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The client_id parameter is mandatory")
	})

	t.Run("AuthorizeWithInvalidClientID", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/authorize").
			WithFormField("state", "123456").
			WithFormField("client_id", "invalid").
			WithFormField("redirect_uri", "https://example.org/oauth/callback").
			WithFormField("scope", "files:read").
			WithFormField("csrf_token", csrfToken).
			WithFormField("response_type", "code").
			WithHost(domain).
			WithCookie("_csrf", csrfToken).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The client must be registered")
	})

	t.Run("AuthorizeWithNoRedirectURI", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/authorize").
			WithFormField("state", "123456").
			WithFormField("client_id", clientID).
			WithFormField("scope", "files:read").
			WithFormField("csrf_token", csrfToken).
			WithFormField("response_type", "code").
			WithHost(domain).
			WithCookie("_csrf", csrfToken).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The redirect_uri parameter is mandatory")
	})

	t.Run("AuthorizeWithInvalidURI", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/authorize").
			WithFormField("state", "123456").
			WithFormField("client_id", clientID).
			WithFormField("scope", "files:read").
			WithFormField("redirect_uri", "/oauth/callback").
			WithFormField("csrf_token", csrfToken).
			WithFormField("response_type", "code").
			WithHost(domain).
			WithCookie("_csrf", csrfToken).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The redirect_uri parameter doesn&#39;t match the registered ones")
	})

	t.Run("AuthorizeWithNoScope", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/authorize").
			WithFormField("state", "123456").
			WithFormField("client_id", clientID).
			WithFormField("redirect_uri", "https://example.org/oauth/callback").
			WithFormField("csrf_token", csrfToken).
			WithFormField("response_type", "code").
			WithHost(domain).
			WithCookie("_csrf", csrfToken).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains("The scope parameter is mandatory")
	})

	t.Run("AuthorizeSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		redirectURL := e.POST("/auth/authorize").
			WithFormField("state", "123456").
			WithFormField("client_id", clientID).
			WithFormField("scope", "files:read").
			WithFormField("redirect_uri", "https://example.org/oauth/callback").
			WithFormField("csrf_token", csrfToken).
			WithFormField("response_type", "code").
			WithHost(domain).
			WithCookie("_csrf", csrfToken).
			WithCookie(session.CookieName(testInstance), sessionID).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(302).
			Header("Location")

		var results []oauth.AccessCode
		req := &couchdb.AllDocsRequest{}
		err := couchdb.GetAllDocs(testInstance, consts.OAuthAccessCodes, req, &results)
		require.NoError(t, err)
		require.Len(t, results, 1)

		code = results[0].Code
		redirectURL.Equal(fmt.Sprintf("https://example.org/oauth/callback?access_code=%s&code=%s&state=123456#", code, code))
		assert.Equal(t, results[0].ClientID, clientID)
		assert.Equal(t, results[0].Scope, "files:read")
	})

	t.Run("AuthorizeSuccessOnboardingDeeplink", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		var oauthClient oauth.Client
		oauthClient.RedirectURIs = []string{"cozydrive://"}
		oauthClient.ClientName = "cozy-test-install-app"
		oauthClient.SoftwareID = "io.cozy.mobile.drive"
		oauthClient.OnboardingSecret = "toto"
		oauthClient.Create(testInstance)

		body := e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("scope", "files:read").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithQuery("client_id", clientID).
			WithHost(domain).
			WithCookie(session.CookieName(testInstance), sessionID).
			Expect().Status(200).
			ContentType("text/html", "utf-8").
			Body()

		body.Contains("would like permission to access your Cozy")
		matches := body.Match(`<input type="hidden" name="csrf_token" value="(\w+)"`)
		matches.Length().Equal(2)
		csrfToken = matches.Index(1).Raw()

		e.POST("/auth/authorize").
			WithHeader("Accept", "application/json").
			WithFormField("state", "123456").
			WithFormField("client_id", oauthClient.ClientID).
			WithFormField("redirect_uri", "cozydrive://").
			WithFormField("scope", "files:read").
			WithFormField("csrf_token", csrfToken).
			WithFormField("response_type", "code").
			WithHost(domain).
			WithCookie("_csrf", csrfToken).
			WithCookie(session.CookieName(testInstance), sessionID).
			Expect().Status(200).
			JSON().Object().
			Value("deeplink").
			String().HasPrefix("cozydrive:?")
	})

	t.Run("AuthorizeSuccessOnboarding", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		var oauthClient oauth.Client
		u := "https://example.org/oauth/callback"
		oauthClient.RedirectURIs = []string{u}
		oauthClient.ClientName = "cozy-test-install-app"
		oauthClient.SoftwareID = "io.cozy.mobile.drive"
		oauthClient.OnboardingSecret = "toto"
		oauthClient.Create(testInstance)

		e.POST("/auth/authorize").
			WithFormField("state", "123456").
			WithFormField("client_id", oauthClient.ClientID).
			WithFormField("redirect_uri", "https://example.org/oauth/callback").
			WithFormField("scope", "files:read").
			WithFormField("csrf_token", csrfToken).
			WithFormField("response_type", "code").
			WithHost(domain).
			WithCookie("_csrf", csrfToken).
			WithCookie(session.CookieName(testInstance), sessionID).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(302)
	})

	t.Run("InstallAppWithLinkedApp", func(t *testing.T) {
		var linkedClientID string
		var linkedClientSecret string
		var linkedCode string

		t.Run("Success", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			var oauthClient oauth.Client
			oauthClient.RedirectURIs = []string{"https://example.org/oauth/callback"}
			oauthClient.ClientName = "cozy-test-install-app"
			oauthClient.SoftwareID = "registry://drive"
			oauthClient.Create(testInstance)

			linkedClientID = oauthClient.ClientID         // Used for following tests
			linkedClientSecret = oauthClient.ClientSecret // Used for following tests

			e.POST("/auth/authorize").
				WithFormField("state", "123456").
				WithFormField("client_id", oauthClient.ClientID).
				WithFormField("redirect_uri", "https://example.org/oauth/callback").
				WithFormField("scope", "files:read").
				WithFormField("csrf_token", csrfToken).
				WithFormField("response_type", "code").
				WithHost(domain).
				WithCookie("_csrf", csrfToken).
				WithCookie(session.CookieName(testInstance), sessionID).
				WithRedirectPolicy(httpexpect.DontFollowRedirects).
				Expect().Status(302)

			couch := config.CouchCluster(testInstance.DBCluster())
			db := testInstance.DBPrefix() + "%2F" + consts.Apps
			err := couchdb.EnsureDBExist(testInstance, consts.Apps)
			assert.NoError(t, err)
			reqGetChanges, err := http.NewRequest("GET", couch.URL.String()+couchdb.EscapeCouchdbName(db)+"/_changes?feed=longpoll", nil)
			assert.NoError(t, err)
			if auth := couch.Auth; auth != nil {
				if p, ok := auth.Password(); ok {
					reqGetChanges.SetBasicAuth(auth.Username(), p)
				}
			}
			resGetChanges, err := config.CouchClient().Do(reqGetChanges)
			assert.NoError(t, err)
			defer resGetChanges.Body.Close()
			assert.Equal(t, resGetChanges.StatusCode, 200)
			body, err := io.ReadAll(resGetChanges.Body)
			assert.NoError(t, err)
			assert.Contains(t, string(body), "io.cozy.apps/drive")

			var results []oauth.AccessCode
			reqDocs := &couchdb.AllDocsRequest{}
			err = couchdb.GetAllDocs(testInstance, consts.OAuthAccessCodes, reqDocs, &results)
			assert.NoError(t, err)
			for _, result := range results {
				if result.ClientID == linkedClientID {
					linkedCode = result.Code
					break
				}
			}
		})

		t.Run("CheckLinkedAppInstalled", func(t *testing.T) {
			// We use the webapp drive installed from the previous test
			err := auth.CheckLinkedAppInstalled(testInstance, "drive")
			assert.NoError(t, err)
		})

		t.Run("AccessTokenLinkedAppInstalled", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			obj := e.POST("/auth/access_token").
				WithFormField("grant_type", "authorization_code").
				WithFormField("client_id", linkedClientID).
				WithFormField("client_secret", linkedClientSecret).
				WithFormField("code", linkedCode).
				WithHost(domain).
				Expect().Status(200).
				JSON().Object()

			obj.ValueEqual("token_type", "bearer")
			obj.ValueEqual("scope", "@io.cozy.apps/drive")

			assertValidToken(t, testInstance, obj.Value("access_token").String().Raw(), "access", linkedClientID, "@io.cozy.apps/drive")
			assertValidToken(t, testInstance, obj.Value("refresh_token").String().Raw(), "refresh", linkedClientID, "@io.cozy.apps/drive")
		})
	})

	t.Run("AccessTokenNoGrantType", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/access_token").
			WithFormField("client_id", clientID).
			WithFormField("client_secret", clientSecret).
			WithFormField("code", code).
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "the grant_type parameter is mandatory")
	})

	t.Run("AccessTokenInvalidGrantType", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/access_token").
			WithFormField("grant_type", "token"). // invalide
			WithFormField("client_id", clientID).
			WithFormField("client_secret", clientSecret).
			WithFormField("code", code).
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "invalid grant type")
	})

	t.Run("AccessTokenNoClientID", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/access_token").
			WithFormField("grant_type", "authorization_code").
			WithFormField("client_secret", clientSecret).
			WithFormField("code", code).
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "the client_id parameter is mandatory")
	})

	t.Run("AccessTokenInvalidClientID", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/access_token").
			WithFormField("grant_type", "authorization_code").
			WithFormField("client_id", "foo"). // invalid
			WithFormField("client_secret", clientSecret).
			WithFormField("code", code).
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "the client must be registered")
	})

	t.Run("AccessTokenNoClientSecret", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/access_token").
			WithFormField("grant_type", "authorization_code").
			WithFormField("client_id", clientID).
			WithFormField("code", code).
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "the client_secret parameter is mandatory")
	})

	t.Run("AccessTokenInvalidClientSecret", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/access_token").
			WithFormField("grant_type", "authorization_code").
			WithFormField("client_id", clientID).
			WithFormField("client_secret", "foo"). // invalid
			WithFormField("code", code).
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "invalid client_secret")
	})

	t.Run("AccessTokenNoCode", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/access_token").
			WithFormField("grant_type", "authorization_code").
			WithFormField("client_id", clientID).
			WithFormField("client_secret", clientSecret).
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "the code parameter is mandatory")
	})

	t.Run("AccessTokenInvalidCode", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/access_token").
			WithFormField("grant_type", "authorization_code").
			WithFormField("client_id", clientID).
			WithFormField("client_secret", clientSecret).
			WithFormField("code", "foo").
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "invalid code")
	})

	t.Run("AccessTokenSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/auth/access_token").
			WithFormField("grant_type", "authorization_code").
			WithFormField("client_id", clientID).
			WithFormField("client_secret", clientSecret).
			WithFormField("code", code).
			WithHost(domain).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("token_type", "bearer")
		obj.ValueEqual("scope", "files:read")

		assertValidToken(t, testInstance, obj.Value("access_token").String().Raw(), "access", clientID, "files:read")
		assertValidToken(t, testInstance, obj.Value("refresh_token").String().Raw(), "refresh", clientID, "files:read")

		refreshToken = obj.Value("refresh_token").String().NotEmpty().Raw()
	})

	t.Run("RefreshTokenNoToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/access_token").
			WithFormField("grant_type", "refresh_token").
			WithFormField("client_id", clientID).
			WithFormField("client_secret", clientSecret).
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "invalid refresh token")
	})

	t.Run("RefreshTokenInvalidToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/access_token").
			WithFormField("grant_type", "refresh_token").
			WithFormField("client_id", clientID).
			WithFormField("client_secret", clientSecret).
			WithFormField("refresh_token", "foo").
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "invalid refresh token")
	})

	t.Run("RefreshTokenInvalidSigningMethod", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		claims := permission.Claims{
			StandardClaims: crypto.StandardClaims{
				Audience: consts.RefreshTokenAudience,
				Issuer:   domain,
				IssuedAt: crypto.Timestamp(),
				Subject:  clientID,
			},
			Scope: "files:write",
		}
		token := jwt.NewWithClaims(jwt.GetSigningMethod("none"), claims)
		fakeToken, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		e.POST("/auth/access_token").
			WithFormField("grant_type", "refresh_token").
			WithFormField("client_id", clientID).
			WithFormField("client_secret", clientSecret).
			WithFormField("refresh_token", fakeToken).
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "invalid refresh token")
	})

	t.Run("RefreshTokenSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/auth/access_token").
			WithFormField("grant_type", "refresh_token").
			WithFormField("client_id", clientID).
			WithFormField("client_secret", clientSecret).
			WithFormField("refresh_token", refreshToken).
			WithHost(domain).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("token_type", "bearer")
		obj.ValueEqual("scope", "files:read")
		obj.NotContainsKey("refresh_token")

		assertValidToken(t, testInstance, obj.Value("access_token").String().Raw(), "access", clientID, "files:read")
	})

	t.Run("OAuthWithPKCE", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		/* Values taken from https://datatracker.ietf.org/doc/html/rfc7636#appendix-B */
		challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
		verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

		/* 1. GET /auth/authorize */
		resBody := e.GET("/auth/authorize").
			WithQuery("response_type", "code").
			WithQuery("state", "123456").
			WithQuery("redirect_uri", "https://example.org/oauth/callback").
			WithQuery("scope", "files:read").
			WithQuery("client_id", clientID).
			WithQuery("code_challenge", challenge).
			WithQuery("code_challenge_method", "S256").
			WithCookie(session.CookieName(testInstance), sessionID).
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(200).
			ContentType("text/html", "utf-8").
			Body()

		matches := resBody.Match(`<input type="hidden" name="csrf_token" value="(\w+)"`)
		matches.Length().Equal(2)
		csrfToken = matches.Index(1).Raw()

		/* 2. POST /auth/authorize */
		e.POST("/auth/authorize").
			WithFormField("state", "123456").
			WithFormField("client_id", clientID).
			WithFormField("scope", "files:read").
			WithFormField("redirect_uri", "https://example.org/oauth/callback").
			WithFormField("csrf_token", csrfToken).
			WithFormField("response_type", "code").
			WithFormField("code_challenge", challenge).
			WithFormField("code_challenge_method", "S256").
			WithHost(domain).
			WithCookie("_csrf", csrfToken).
			WithCookie(session.CookieName(testInstance), sessionID).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(302)

		var results []oauth.AccessCode
		allReq := &couchdb.AllDocsRequest{}
		err := couchdb.GetAllDocs(testInstance, consts.OAuthAccessCodes, allReq, &results)
		assert.NoError(t, err)
		var code string
		for _, result := range results {
			if result.Challenge != "" {
				code = result.Code
			}
		}
		require.NotEmpty(t, code)

		/* 3. POST /auth/access_token without code_verifier must fail */
		e.POST("/auth/access_token").
			WithFormField("grant_type", "authorization_code").
			WithFormField("client_id", clientID).
			WithFormField("client_secret", clientSecret).
			WithFormField("code", code).
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "invalid code_verifier")

		/* 4. POST /auth/access_token with code_verifier should succeed */
		obj := e.POST("/auth/access_token").
			WithFormField("grant_type", "authorization_code").
			WithFormField("client_id", clientID).
			WithFormField("client_secret", clientSecret).
			WithFormField("code", code).
			WithFormField("code_verifier", verifier).
			WithHost(domain).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("token_type", "bearer")
		obj.ValueEqual("scope", "files:read")

		assertValidToken(t, testInstance, obj.Value("access_token").String().Raw(), "access", clientID, "files:read")
		assertValidToken(t, testInstance, obj.Value("refresh_token").String().Raw(), "refresh", clientID, "files:read")
	})

	t.Run("ConfirmFlagship", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		token, code, err := oauth.GenerateConfirmCode(testInstance, clientID)
		require.NoError(t, err)

		e.POST("/auth/clients/"+clientID+"/flagship").
			WithFormField("code", code).
			WithFormField("token", string(token)).
			WithHost(domain).
			Expect().Status(204)

		client, err := oauth.FindClient(testInstance, clientID)
		require.NoError(t, err)
		assert.True(t, client.Flagship)
	})

	t.Run("LoginFlagship", func(t *testing.T) {
		oauthClient := &oauth.Client{
			RedirectURIs:    []string{"cozy://flagship"},
			ClientName:      "Cozy Flagship",
			ClientKind:      "mobile",
			SoftwareID:      "cozy-flagship",
			SoftwareVersion: "0.1.0",
		}

		require.Nil(t, oauthClient.Create(testInstance))
		client, err := oauth.FindClient(testInstance, oauthClient.ClientID)
		require.NoError(t, err)
		client.CertifiedFromStore = true
		require.NoError(t, client.SetFlagship(testInstance))

		t.Run("WithAnInvalidPassPhrase", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.POST("/auth/login/flagship").
				WithHeader("Accept", "application/json").
				WithJSON(map[string]string{
					"passphrase":    "InvalidPassphrase",
					"client_id":     client.CouchID,
					"client_secret": client.ClientSecret,
				}).
				WithHost(domain).
				Expect().Status(401)
		})

		t.Run("WithAnInvalidClientSecret", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.POST("/auth/login/flagship").
				WithHeader("Accept", "application/json").
				WithJSON(map[string]string{
					"passphrase":    "MyPassphrase",
					"client_id":     client.CouchID,
					"client_secret": "InvalidClientSecret",
				}).
				WithHost(domain).
				Expect().Status(400)
		})

		t.Run("Success", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			obj := e.POST("/auth/login/flagship").
				WithHeader("Accept", "application/json").
				WithJSON(map[string]string{
					"passphrase":    "MyPassphrase",
					"client_id":     client.CouchID,
					"client_secret": client.ClientSecret,
				}).
				WithHost(domain).
				Expect().Status(200).
				JSON().Object()

			obj.Value("access_token").String().NotEmpty()
			obj.Value("refresh_token").String().NotEmpty()
			obj.ValueEqual("scope", "*")
			obj.ValueEqual("token_type", "bearer")
		})
	})

	t.Run("AppRedirectionOnLogin", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/login").
			WithQuery("redirect", "drive/#/foobar").
			WithHost(domain).
			WithCookie(session.CookieName(testInstance), sessionID).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Equal("https://drive.cozy.example.net#/foobar")
	})

	t.Run("LogoutNoToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.DELETE("/auth/login").
			WithHost(domain).
			Expect().Status(401)
	})

	t.Run("LogoutSuccess", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		token := testInstance.BuildAppToken("home", "")

		_, err := permission.CreateWebappSet(testInstance, "home", permission.Set{}, "1.0.0")
		assert.NoError(t, err)

		e.DELETE("/auth/login").
			WithHeader("Authorization", "Bearer "+token).
			WithHost(domain).
			Expect().Status(204)

		err = permission.DestroyWebapp(testInstance, "home")
		require.NoError(t, err)
	})

	t.Run("LogoutOthers", func(t *testing.T) {
		// First two connexion
		e1 := testutils.CreateTestClient(t, ts.URL)
		e2 := testutils.CreateTestClient(t, ts.URL)

		// Third connexion closing e2 using the cookies from e1
		e3 := testutils.CreateTestClient(t, ts.URL)

		// Authenticate user 1
		csrfToken := getLoginCSRFToken(e1)
		res := e1.POST("/auth/login").
			WithFormField("passphrase", "MyPassphrase").
			WithFormField("csrf_token", csrfToken).
			WithCookie("_csrf", csrfToken).
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303)

		res.Cookie("_csrf").Value().NotEmpty()

		// Retrieve the session id from cozysessid
		rawSessID1 := res.Cookie("cozysessid").Value().NotEmpty().Raw()
		b, err := base64.RawURLEncoding.DecodeString(rawSessID1)
		require.NoError(t, err)
		sessionID1 := string(b[8 : 8+32])

		// Authenticate user 2
		csrfToken = getLoginCSRFToken(e2)
		res = e2.POST("/auth/login").
			WithFormField("passphrase", "MyPassphrase").
			WithFormField("csrf_token", csrfToken).
			WithCookie("_csrf", csrfToken).
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303)

		res.Cookie("_csrf").Value().NotEmpty()
		res.Cookie("cozysessid").Value().NotEmpty()

		token := testInstance.BuildAppToken("home", sessionID1)
		_, err = permission.CreateWebappSet(testInstance, "home", permission.Set{}, "1.0.0")
		assert.NoError(t, err)

		// Delete all the other sessions
		e3.DELETE("/auth/login/others").
			WithHost(domain).
			WithHeader("Authorization", "Bearer "+token).
			WithCookie("cozysessid", rawSessID1).
			Expect().Status(204)

		// Delete all the other sessions again give the same output
		e3.DELETE("/auth/login/others").
			WithHost(domain).
			WithHeader("Authorization", "Bearer "+token).
			WithCookie("cozysessid", rawSessID1).
			Expect().Status(204)

		err = permission.DestroyWebapp(testInstance, "home")
		assert.NoError(t, err)
	})

	t.Run("PassphraseResetLoggedIn", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		body := e.GET("/auth/passphrase_reset").
			WithHost(domain).
			Expect().Status(200).
			ContentType("text/html", "utf-8").
			Body()

		body.Contains("my password")
		body.Contains(`<input type="hidden" name="csrf_token"`)
	})

	t.Run("PassphraseReset", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		csrfToken := e.GET("/auth/passphrase_reset").
			WithHost(domain).
			Expect().Status(200).
			Cookie("_csrf").Value().Raw()

		e.POST("/auth/passphrase_reset").
			WithFormField("csrf_token", csrfToken).
			WithCookie("_csrf", csrfToken).
			WithHost(domain).
			Expect().Status(200).
			ContentType("text/html", "utf-8")
	})

	t.Run("PassphraseRenewFormNoToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/passphrase_renew").
			WithHost(domain).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains(`The link to reset the password is truncated or has expired`)
	})

	t.Run("PassphraseRenewFormBadToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/passphrase_renew").
			WithQuery("token", "invalid"). // invalid
			WithHost(domain).
			Expect().Status(400).
			ContentType("text/html", "utf-8").
			Body().Contains(`The link to reset the password is truncated or has expired`)
	})

	t.Run("PassphraseRenewFormWithToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/auth/passphrase_renew").
			WithQuery("token", "badbee"). // good format but invalid
			WithHost(domain).
			Expect().Status(400).
			JSON().Object().
			ValueEqual("error", "invalid_token")
	})

	t.Run("PassphraseRenew", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		d := "test.cozycloud.cc.web_reset_form"
		_ = lifecycle.Destroy(d)
		in1, err := lifecycle.Create(&lifecycle.Options{
			Domain: d,
			Locale: "en",
			Email:  "alice@example.com",
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = lifecycle.Destroy(d) })

		err = lifecycle.RegisterPassphrase(in1, in1.RegisterToken, lifecycle.PassParameters{
			Pass:       []byte("MyPass"),
			Iterations: 5000,
			Key:        "0.uRcMe+Mc2nmOet4yWx9BwA==|PGQhpYUlTUq/vBEDj1KOHVMlTIH1eecMl0j80+Zu0VRVfFa7X/MWKdVM6OM/NfSZicFEwaLWqpyBlOrBXhR+trkX/dPRnfwJD2B93hnLNGQ=",
		})
		require.NoError(t, err)

		csrfToken := e.GET("/auth/passphrase_reset").
			WithHost(d).
			Expect().Status(200).
			Cookie("_csrf").Value().Raw()

		e.POST("/auth/passphrase_reset").
			WithFormField("csrf_token", csrfToken).
			WithCookie("_csrf", csrfToken).
			WithHost(d).
			Expect().Status(200)

		in2, err := instance.GetFromCouch(d)
		require.NoError(t, err)

		e.POST("/auth/passphrase_renew").
			WithFormField("passphrase_reset_token", hex.EncodeToString(in2.PassphraseResetToken)).
			WithFormField("passphrase", "NewPassphrase").
			WithFormField("csrf_token", csrfToken).
			WithCookie("_csrf", csrfToken).
			WithHost(d).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").
			Equal("https://test.cozycloud.cc.web_reset_form/auth/login")
	})

	t.Run("IsLoggedOutAfterLogout", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/test").
			WithHost(domain).
			Expect().Status(200).
			Text(httpexpect.ContentOpts{MediaType: "text/plain"}).
			Equal("who_are_you")
	})

	t.Run("SecretExchangeGoodSecret", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		var oauthClient oauth.Client

		oauthClient.RedirectURIs = []string{"abc"}
		oauthClient.ClientName = "cozy-test"
		oauthClient.SoftwareID = "github.com/cozy/cozy-test"
		oauthClient.OnboardingSecret = "foobarsecret"
		oauthClient.Create(testInstance)

		e.POST("/auth/secret_exchange").
			WithHeader("Content-Type", "application/json; charset=utf-8").
			WithHeader("Accept", "application/json").
			WithHost(domain).
			WithBytes([]byte(`{ "secret": "foobarsecret" }`)).
			Expect().Status(200).
			JSON().Object().
			Value("client_secret").String().NotEmpty()
	})

	t.Run("SecretExchangeBadSecret", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		var oauthClient oauth.Client

		oauthClient.RedirectURIs = []string{"abc"}
		oauthClient.ClientName = "cozy-test"
		oauthClient.SoftwareID = "github.com/cozy/cozy-test"
		oauthClient.OnboardingSecret = "foobarsecret"
		oauthClient.Create(testInstance)

		e.POST("/auth/secret_exchange").
			WithHeader("Content-Type", "application/json; charset=utf-8").
			WithHeader("Accept", "application/json").
			WithHost(domain).
			WithBytes([]byte(`{ "secret": "bad secret" }`)). // invalid
			Expect().Status(404).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().
			Value("errors").Array().NotEmpty()
	})

	t.Run("SecretExchangeBadPayload", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		var oauthClient oauth.Client

		oauthClient.RedirectURIs = []string{"abc"}
		oauthClient.ClientName = "cozy-test"
		oauthClient.SoftwareID = "github.com/cozy/cozy-test"
		oauthClient.OnboardingSecret = "foobarsecret"
		oauthClient.Create(testInstance)

		e.POST("/auth/secret_exchange").
			WithHeader("Content-Type", "application/json; charset=utf-8").
			WithHeader("Accept", "application/json").
			WithHost(domain).
			WithBytes([]byte(`{ "foo": "bar" }`)). // invalid
			Expect().Status(400).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().
			Value("errors").Array().ContainsOnly(map[string]interface{}{
			"detail": "Missing secret",
			"source": map[string]interface{}{},
			"status": "400",
			"title":  "Bad request",
		})
	})

	t.Run("SecretExchangeNoPayload", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		var oauthClient oauth.Client

		oauthClient.RedirectURIs = []string{"abc"}
		oauthClient.ClientName = "cozy-test"
		oauthClient.SoftwareID = "github.com/cozy/cozy-test"
		oauthClient.OnboardingSecret = "foobarsecret"
		oauthClient.Create(testInstance)

		e.POST("/auth/secret_exchange").
			WithHeader("Content-Type", "application/json; charset=utf-8").
			WithHeader("Accept", "application/json").
			// No body
			WithHost(domain).
			Expect().Status(400).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object().
			Value("errors").Array().ContainsOnly(map[string]interface{}{
			"detail": "EOF",
			"source": map[string]interface{}{},
			"status": "400",
			"title":  "Bad Request",
		})
	})

	t.Run("PassphraseOnboarding", func(t *testing.T) {
		// e := testutils.CreateTestClient(t, ts.URL)
		e := httpexpect.WithConfig(httpexpect.Config{
			TestName: t.Name(),
			BaseURL:  ts.URL,
			Reporter: httpexpect.NewAssertReporter(t),
			Printers: []httpexpect.Printer{
				httpexpect.NewDebugPrinter(t, true),
			},
		})

		// Here we create a new instance without passphrase
		d := "test.cozycloud.cc.web_passphrase"
		_ = lifecycle.Destroy(d)
		inst, err := lifecycle.Create(&lifecycle.Options{
			Domain: d,
			Locale: "en",
			Email:  "alice@example.com",
		})
		require.NoError(t, err)
		require.False(t, inst.OnboardingFinished)

		// Should redirect to /auth/passphrase
		e.GET("/").
			WithQuery("registerToken", hex.EncodeToString(inst.RegisterToken)).
			WithHost(inst.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Contains("/auth/passphrase?registerToken=")

		// Adding a passphrase and check if we are redirected to home
		pass := []byte("passphrase")
		err = lifecycle.RegisterPassphrase(inst, inst.RegisterToken, lifecycle.PassParameters{
			Pass:       pass,
			Iterations: 5000,
			Key:        "0.uRcMe+Mc2nmOet4yWx9BwA==|PGQhpYUlTUq/vBEDj1KOHVMlTIH1eecMl0j80+Zu0VRVfFa7X/MWKdVM6OM/NfSZicFEwaLWqpyBlOrBXhR+trkX/dPRnfwJD2B93hnLNGQ=",
		})
		assert.NoError(t, err)

		inst.OnboardingFinished = true

		e.GET("/").
			WithQuery("registerToken", hex.EncodeToString(inst.RegisterToken)).
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Contains("/auth/login")
	})

	t.Run("PassphraseOnboardingFinished", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Using the testInstance which had already been onboarded
		// Should redirect to home
		e.GET("/auth/passphrase").
			WithHost(domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Equal("https://home.cozy.example.net/")
	})

	t.Run("PassphraseOnboardingBadRegisterToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Should render need_onboarding
		d := "test.cozycloud.cc.web_passphrase_bad_token"
		_ = lifecycle.Destroy(d)
		inst, err := lifecycle.Create(&lifecycle.Options{
			Domain: d,
			Locale: "en",
			Email:  "alice@example.com",
		})
		assert.NoError(t, err)
		assert.False(t, inst.OnboardingFinished)

		// Should redirect to /auth/passphrase
		e.GET("/auth/passphrase").
			WithQuery("registerToken", "coincoin").
			WithHost(inst.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(200).
			ContentType("text/html", "utf-8").
			Body().Contains("Your Cozy has not been yet activated.")
	})

	t.Run("LoginOnboardingNotFinished", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Should render need_onboarding
		d := "test.cozycloud.cc.web_login_onboarding_not_finished"
		_ = lifecycle.Destroy(d)
		inst, err := lifecycle.Create(&lifecycle.Options{
			Domain: d,
			Locale: "en",
			Email:  "alice@example.com",
		})
		assert.NoError(t, err)
		assert.False(t, inst.OnboardingFinished)

		// Should redirect to /auth/passphrase
		e.GET("/auth/login").
			WithHost(inst.Domain).
			Expect().Status(200).
			ContentType("text/html", "utf-8").
			Body().Contains("Your Cozy has not been yet activated.")
	})

	t.Run("ShowConfirmForm", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		body := e.GET("/auth/confirm").
			WithQuery("state", "342dd650-599b-0139-cfb0-543d7eb8149c").
			WithHost(domain).
			Expect().Status(200).
			ContentType("text/html", "utf-8").
			Body()

		body.Contains(`<input id="state" type="hidden" name="state" value="342dd650-599b-0139-cfb0-543d7eb8149c" />`)
		body.NotContains("myfragment")
	})

	t.Run("SendConfirmBadCSRFToken", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/auth/confirm").
			WithHeader("Accept", "application/json").
			WithFormField("passphrase", "MyPassphrase").
			WithFormField("csrf_token", "123456").
			WithFormField("state", "342dd650-599b-0139-cfb0-543d7eb8149c").
			WithCookie("_csrf", csrfToken).
			WithHost(domain).
			Expect().Status(403)
	})

	t.Run("SendConfirmBadPass", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		token := getConfirmCSRFToken(e)

		e.POST("/auth/confirm").
			WithHeader("Accept", "application/json").
			WithFormField("passphrase", "InvalidPassphrase"). // invalid
			WithFormField("csrf_token", token).
			WithFormField("state", "342dd650-599b-0139-cfb0-543d7eb8149c").
			WithCookie("_csrf", token).
			WithHost(domain).
			Expect().Status(401)
	})

	t.Run("SendConfirmOK", func(t *testing.T) {
		var confirmCode string

		t.Run("GetConfirmCode", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			token := getConfirmCSRFToken(e)

			obj := e.POST("/auth/confirm").
				WithHeader("Accept", "application/json").
				WithFormField("passphrase", "MyPassphrase").
				WithFormField("csrf_token", token).
				WithFormField("state", "342dd650-599b-0139-cfb0-543d7eb8149c").
				WithCookie("_csrf", token).
				WithHost(domain).
				Expect().Status(200).
				JSON().Object()

			redirect := obj.Value("redirect").String().Raw()

			u, err := url.Parse(redirect)
			assert.NoError(t, err)

			confirmCode = u.Query().Get("code")
			assert.NotEmpty(t, confirmCode)
			state := u.Query().Get("state")
			assert.Equal(t, "342dd650-599b-0139-cfb0-543d7eb8149c", state)
		})

		t.Run("ConfirmBadCode", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.GET("/auth/confirm/123456").
				WithHost(domain).
				Expect().Status(401)
		})

		t.Run("ConfirmCodeOK", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.GET("/auth/confirm/" + confirmCode).
				WithHost(domain).
				Expect().Status(204)
		})
	})

	t.Run("KonnectorTokens", func(t *testing.T) {
		konnSlug, err := setup.InstallMiniKonnector()
		require.NoError(t, err, "Could not install mini konnector.")

		t.Run("BuildKonnectorToken", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			// Create an flagship OAuth client
			oauthClient := oauth.Client{
				RedirectURIs: []string{"cozy://client"},
				ClientName:   "oauth-client",
				SoftwareID:   "github.com/cozy/cozy-stack/testing/client",
				Flagship:     true,
			}
			require.Nil(t, oauthClient.Create(testInstance, oauth.NotPending))

			// Give it the maximal permission
			token, err := testInstance.MakeJWT(consts.AccessTokenAudience,
				oauthClient.ClientID, "*", "", time.Now())
			require.NoError(t, err)

			// Get konnector access_token
			konnToken := e.POST("/auth/tokens/konnectors/"+konnSlug).
				WithHeader("Accept", "application/json").
				WithHeader("Authorization", "Bearer "+token).
				WithHost(domain).
				Expect().Status(201).
				JSON().String().Raw()

			// Validate token
			claims := permission.Claims{}
			err = crypto.ParseJWT(konnToken, func(token *jwt.Token) (interface{}, error) {
				return testInstance.SessionSecret(), nil
			}, &claims)
			assert.NoError(t, err)
			assert.Equal(t, consts.KonnectorAudience, claims.Audience)
			assert.Equal(t, domain, claims.Issuer)
			assert.Equal(t, konnSlug, claims.Subject)
			assert.Equal(t, "", claims.Scope)
		})

		t.Run("BuildKonnectorTokenNotFlagshipApp", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			// Create an OAuth client
			oauthClient := oauth.Client{
				RedirectURIs: []string{"cozy://client"},
				ClientName:   "oauth-client",
				SoftwareID:   "github.com/cozy/cozy-stack/testing/client",
				Flagship:     false,
			}
			require.Nil(t, oauthClient.Create(testInstance, oauth.NotPending))

			// Give it the maximal permission
			token, err := testInstance.MakeJWT(consts.AccessTokenAudience,
				oauthClient.ClientID, "*", "", time.Now())
			require.NoError(t, err)

			e.POST("/auth/tokens/konnectors/"+konnSlug).
				WithHeader("Accept", "application/json").
				WithHeader("Authorization", "Bearer "+token).
				WithHost(domain).
				Expect().Status(403)
		})

		t.Run("BuildKonnectorTokenInvalidSlug", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			// Create an flagship OAuth client
			oauthClient := oauth.Client{
				RedirectURIs: []string{"cozy://client"},
				ClientName:   "oauth-client",
				SoftwareID:   "github.com/cozy/cozy-stack/testing/client",
				Flagship:     true,
			}
			require.Nil(t, oauthClient.Create(testInstance, oauth.NotPending))

			// Give it the maximal permission
			token, err := testInstance.MakeJWT(consts.AccessTokenAudience,
				oauthClient.ClientID, "*", "", time.Now())
			require.NoError(t, err)

			e.POST("/auth/tokens/konnectors/missin").
				WithHeader("Accept", "application/json").
				WithHeader("Authorization", "Bearer "+token).
				WithHost(domain).
				Expect().Status(404)
		})
	})

	t.Run("MagicLink", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		d := "test.cozycloud.cc.magic_link"
		_ = lifecycle.Destroy(d)
		magicLink := true
		inst, err := lifecycle.Create(&lifecycle.Options{
			Domain:    d,
			Locale:    "en",
			Email:     "alice@example.com",
			MagicLink: &magicLink,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = lifecycle.Destroy(d) })

		t.Run("Failure", func(t *testing.T) {
			code := "badcode"

			e.GET("/auth/magic_link").
				WithHost(d).
				WithRedirectPolicy(httpexpect.DontFollowRedirects).
				WithQuery("code", code).
				Expect().Status(400)
		})

		t.Run("Success", func(t *testing.T) {
			code, err := lifecycle.CreateMagicLinkCode(inst)
			require.NoError(t, err)

			e.GET("/auth/magic_link").
				WithHost(d).
				WithRedirectPolicy(httpexpect.DontFollowRedirects).
				WithQuery("code", code).
				Expect().Status(303).
				Header("Location").Equal("https://home." + d + "/")
		})

		t.Run("Flagship", func(t *testing.T) {
			oauthClient := &oauth.Client{
				RedirectURIs:    []string{"cozy://flagship"},
				ClientName:      "Cozy Flagship",
				ClientKind:      "mobile",
				SoftwareID:      "cozy-flagship",
				SoftwareVersion: "0.1.0",
			}

			require.Nil(t, oauthClient.Create(inst))
			client, err := oauth.FindClient(inst, oauthClient.ClientID)
			require.NoError(t, err)
			client.CertifiedFromStore = true
			require.NoError(t, client.SetFlagship(inst))

			code, err := lifecycle.CreateMagicLinkCode(inst)
			require.NoError(t, err)

			obj := e.POST("/auth/magic_link/flagship").
				WithHost(d).
				WithHeader("Accept", "application/json").
				WithJSON(map[string]string{
					"magic_code":    code,
					"client_id":     client.CouchID,
					"client_secret": client.ClientSecret,
				}).
				Expect().Status(200).
				JSON().Object()

			obj.Value("access_token").String().NotEmpty()
			obj.Value("refresh_token").String().NotEmpty()
			obj.ValueEqual("scope", "*")
			obj.ValueEqual("token_type", "bearer")
		})
	})
}

func getLoginCSRFToken(e *httpexpect.Expect) string {
	return e.GET("/auth/login").
		WithHost(domain).
		Expect().Status(200).
		Cookie("_csrf").Value().Raw()
}

func getConfirmCSRFToken(e *httpexpect.Expect) string {
	return e.GET("/auth/confirm").
		WithQuery("state", "123").
		WithHost(domain).
		Expect().Status(200).
		Cookie("_csrf").Value().Raw()
}

func fakeAPI(g *echo.Group) {
	g.Use(middlewares.NeedInstance, middlewares.LoadSession)
	g.GET("", func(c echo.Context) error {
		var content string
		if middlewares.IsLoggedIn(c) {
			content = "logged_in"
		} else {
			content = "who_are_you"
		}
		return c.String(http.StatusOK, content)
	})
}

func assertValidToken(t *testing.T, testInstance *instance.Instance, token, audience, subject, scope string) {
	claims := permission.Claims{}
	err := crypto.ParseJWT(token, func(token *jwt.Token) (interface{}, error) {
		return testInstance.OAuthSecret, nil
	}, &claims)
	assert.NoError(t, err)
	assert.Equal(t, audience, claims.Audience)
	assert.Equal(t, domain, claims.Issuer)
	assert.Equal(t, subject, claims.Subject)
	assert.Equal(t, scope, claims.Scope)
}
