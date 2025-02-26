package accounts

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/session"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOauth(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var testInstance *instance.Instance

	config.UseTestFile(t)
	build.BuildMode = build.ModeDev
	testutils.NeedCouchdb(t)

	setup := testutils.NewSetup(t, t.Name())
	ts := setup.GetTestServer("/accounts", Routes, func(r *echo.Echo) *echo.Echo {
		r.POST("/login", func(c echo.Context) error {
			sess, _ := session.New(testInstance, session.LongRun, "")
			cookie, _ := sess.ToCookie()
			t.Logf("cookie: %q", cookie)
			c.SetCookie(cookie)
			return c.HTML(http.StatusOK, "OK")
		})
		return r
	})
	t.Cleanup(ts.Close)

	testInstance = setup.GetTestInstance(&lifecycle.Options{
		Domain: strings.Replace(ts.URL, "http://127.0.0.1", "cozy.localhost", 1),
	})
	_ = couchdb.ResetDB(prefixer.SecretsPrefixer, consts.AccountTypes)
	t.Cleanup(func() { _ = couchdb.DeleteDB(prefixer.SecretsPrefixer, consts.AccountTypes) })

	t.Run("AccessCodeOauthFlow", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Retrieve the cozysessid cookie
		cozysessID := e.POST("/login").
			WithHost(testInstance.Domain).
			Expect().Status(200).
			Cookie("cozysessid").Value().Raw()

		redirectURI := ts.URL + "/accounts/test-service/redirect"

		// Register the client inside the database.
		service := makeTestACService(redirectURI)
		t.Cleanup(service.Close)

		serviceType := account.AccountType{
			DocID:                 "test-service",
			GrantMode:             account.AuthorizationCode,
			ClientID:              "the-client-id",
			ClientSecret:          "the-client-secret",
			AuthEndpoint:          service.URL + "/oauth2/v2/auth",
			TokenEndpoint:         service.URL + "/oauth2/v4/token",
			RegisteredRedirectURI: redirectURI,
		}
		err := couchdb.CreateNamedDoc(prefixer.SecretsPrefixer, &serviceType)
		require.NoError(t, err)
		t.Cleanup(func() { _ = couchdb.DeleteDoc(prefixer.SecretsPrefixer, &serviceType) })

		// Start the oauth flow
		rawURL := e.GET("/accounts/test-service/start").
			WithQuery("scope", "the world").
			WithQuery("state", "somesecretstate").
			WithCookie("cozysessid", cozysessID).
			Expect().Status(200).
			Body().Raw()

		okURL, err := url.Parse(rawURL)
		require.NoError(t, err)

		// the user click the oauth link
		rawFinalURL := e.GET(okURL.Path).
			WithQueryString(okURL.RawQuery).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").NotEmpty().Contains("home").
			Raw()

		finalURL, err := url.Parse(rawFinalURL)
		require.NoError(t, err)

		var out couchdb.JSONDoc
		err = couchdb.GetDoc(testInstance, consts.Accounts, finalURL.Query().Get("account"), &out)
		assert.NoError(t, err)
		assert.Equal(t, "the-access-token", out.M["oauth"].(map[string]interface{})["access_token"])
		out.Type = consts.Accounts
		out.M["manual_cleaning"] = true
		_ = couchdb.DeleteDoc(testInstance, &out)
	})

	t.Run("RedirectURLOauthFlow", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Retrieve the cozysessid cookie
		cozysessID := e.POST("/login").
			WithHost(testInstance.Domain).
			Expect().Status(200).
			Cookie("cozysessid").Value().Raw()

		redirectURI := "http://" + testInstance.Domain + "/accounts/test-service2/redirect"
		service := makeTestRedirectURLService(redirectURI)
		t.Cleanup(service.Close)

		serviceType := account.AccountType{
			DocID:        "test-service2",
			GrantMode:    account.ImplicitGrantRedirectURL,
			AuthEndpoint: service.URL + "/oauth2/v2/auth",
		}
		err := couchdb.CreateNamedDoc(prefixer.SecretsPrefixer, &serviceType)
		require.NoError(t, err)
		t.Cleanup(func() { _ = couchdb.DeleteDoc(prefixer.SecretsPrefixer, &serviceType) })

		// Start the oauth flow
		rawURL := e.GET("/accounts/test-service2/start").
			WithQuery("scope", "the world").
			WithQuery("state", "somesecretstate").
			WithCookie("cozysessid", cozysessID).
			Expect().Status(200).
			Body().Raw()

		okURL, err := url.Parse(rawURL)
		require.NoError(t, err)

		// the user click the oauth link
		rawFinalURL := e.GET(okURL.Path).
			WithQueryString(okURL.RawQuery).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithHost(testInstance.Domain).
			Expect().Status(303).
			Header("Location").NotEmpty().Contains("home").
			Raw()

		finalURL, err := url.Parse(rawFinalURL)
		require.NoError(t, err)

		var out couchdb.JSONDoc
		err = couchdb.GetDoc(testInstance, consts.Accounts, finalURL.Query().Get("account"), &out)
		assert.NoError(t, err)
		assert.Equal(t, "the-access-token2", out.M["oauth"].(map[string]interface{})["access_token"])
		out.Type = consts.Accounts
		out.M["manual_cleaning"] = true
		_ = couchdb.DeleteDoc(testInstance, &out)
	})

	t.Run("DoNotRecreateAccountIfItAlreadyExists", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Retrieve the cozysessid cookie
		cozysessID := e.POST("/login").
			WithHost(testInstance.Domain).
			Expect().Status(200).
			Cookie("cozysessid").Value().Raw()

		existingAccount := &couchdb.JSONDoc{
			Type: consts.Accounts,
			M: map[string]interface{}{
				"account_type": "test-service3",
				"oauth": map[string]interface{}{
					"query": map[string]interface{}{
						"connection_id": []interface{}{
							"1750",
						},
					},
				},
			},
		}
		err := couchdb.CreateDoc(testInstance, existingAccount)
		require.NoError(t, err)
		t.Cleanup(func() {
			existingAccount.M["manual_cleaning"] = true
			_ = couchdb.DeleteDoc(testInstance, existingAccount)
		})

		redirectURI := "http://" + testInstance.Domain + "/accounts/test-service3/redirect"
		service := makeTestRedirectURLService(redirectURI)
		t.Cleanup(service.Close)

		serviceType := account.AccountType{
			DocID:        "test-service3",
			GrantMode:    account.ImplicitGrantRedirectURL,
			AuthEndpoint: service.URL + "/oauth2/v2/auth",
		}
		err = couchdb.CreateNamedDoc(prefixer.SecretsPrefixer, &serviceType)
		require.NoError(t, err)
		t.Cleanup(func() { _ = couchdb.DeleteDoc(prefixer.SecretsPrefixer, &serviceType) })

		// Start the oauth flow
		rawURL := e.GET("/accounts/test-service3/start").
			WithQuery("scope", "the world").
			WithQuery("state", "somesecretstate").
			WithCookie("cozysessid", cozysessID).
			Expect().Status(200).
			Body().Raw()

		okURL, err := url.Parse(rawURL)
		require.NoError(t, err)

		// the user click the oauth link
		rawFinalURL := e.GET(okURL.Path).
			WithQueryString(okURL.RawQuery).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithHost(testInstance.Domain).
			Expect().Status(303).
			Header("Location").NotEmpty().Contains("home").
			Raw()

		finalURL, err := url.Parse(rawFinalURL)
		require.NoError(t, err)

		assert.Equal(t, finalURL.Query().Get("account"), existingAccount.ID())
	})

	t.Run("FixedRedirectURIOauthFlow", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Retrieve the cozysessid cookie
		cozysessID := e.POST("/login").
			WithHost(testInstance.Domain).
			Expect().Status(200).
			Cookie("cozysessid").Value().Raw()

		redirectURI := "http://oauth_callback.cozy.localhost/accounts/test-service3/redirect"
		service := makeTestACService(redirectURI)
		t.Cleanup(service.Close)

		serviceType := account.AccountType{
			DocID:                 "test-service3",
			GrantMode:             account.AuthorizationCode,
			ClientID:              "the-client-id",
			ClientSecret:          "the-client-secret",
			AuthEndpoint:          service.URL + "/oauth2/v2/auth",
			TokenEndpoint:         service.URL + "/oauth2/v4/token",
			RegisteredRedirectURI: redirectURI,
		}
		err := couchdb.CreateNamedDoc(prefixer.SecretsPrefixer, &serviceType)
		require.NoError(t, err)
		t.Cleanup(func() { _ = couchdb.DeleteDoc(prefixer.SecretsPrefixer, &serviceType) })

		// Start the oauth flow
		rawURL := e.GET("/accounts/test-service3/start").
			WithQuery("scope", "the world").
			WithQuery("state", "somesecretstate").
			WithCookie("cozysessid", cozysessID).
			Expect().Status(200).
			Body().Raw()

		okURL, err := url.Parse(rawURL)
		require.NoError(t, err)

		// hack, we want to speak with ts.URL but setting Host to _oauth_callback
		rawFinalURL := e.GET(okURL.Path).
			WithQueryString(okURL.RawQuery).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithHost(okURL.Host).
			Expect().Status(303).
			Header("Location").NotEmpty().Contains("home").
			Raw()

		finalURL, err := url.Parse(rawFinalURL)
		require.NoError(t, err)

		var out couchdb.JSONDoc
		err = couchdb.GetDoc(testInstance, consts.Accounts, finalURL.Query().Get("account"), &out)
		assert.NoError(t, err)
		assert.Equal(t, "the-access-token", out.M["oauth"].(map[string]interface{})["access_token"])
		out.Type = consts.Accounts
		out.M["manual_cleaning"] = true
		_ = couchdb.DeleteDoc(testInstance, &out)
	})

	t.Run("CheckLogin", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		serviceType := account.AccountType{
			DocID:                 "test-service4",
			GrantMode:             account.AuthorizationCode,
			ClientID:              "the-client-id",
			ClientSecret:          "the-client-secret",
			AuthEndpoint:          "https://test-service4/auth",
			TokenEndpoint:         "https://test-service4/token",
			RegisteredRedirectURI: "https://oauth_callback.cozy.localhost/accounts/test-service4/redirect",
		}
		err := couchdb.CreateNamedDoc(prefixer.SecretsPrefixer, &serviceType)
		require.NoError(t, err)
		t.Cleanup(func() { _ = couchdb.DeleteDoc(prefixer.SecretsPrefixer, &serviceType) })

		// Start the oauth flow without cookie
		e.GET("/accounts/test-service4/start").
			WithQuery("scope", "foo").
			WithQuery("state", "bar").
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(403)

		sessionCode, err := testInstance.CreateSessionCode()
		require.NoError(t, err)

		// Start again with a session_code query param.
		res := e.GET("/accounts/test-service4/start").
			WithQuery("session_code", sessionCode).
			WithQuery("scope", "foo").
			WithQuery("state", "bar").
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303)

		res.Header("Location").HasPrefix(serviceType.AuthEndpoint)
		res.Cookies().Length().Equal(1)
		res.Cookie("cozysessid").Value().NotEmpty()
	})
}

func makeTestRedirectURLService(redirectURI string) *httptest.Server {
	serviceHandler := echo.New()
	serviceHandler.GET("/oauth2/v2/auth", func(c echo.Context) error {
		ok := c.QueryParam("scope") == "the world" &&
			c.QueryParam("response_type") == "token" &&
			c.QueryParam("redirect_url") == redirectURI

		if !ok {
			return echo.NewHTTPError(400, "Bad Params "+c.QueryParams().Encode())
		}
		opts := &url.Values{}
		opts.Add("access_token", "the-access-token2")
		opts.Add("connection_id", "1750")
		return c.String(200, c.QueryParam("redirect_url")+"?"+opts.Encode())
	})
	return httptest.NewServer(serviceHandler)
}

func makeTestACService(redirectURI string) *httptest.Server {
	serviceHandler := echo.New()
	serviceHandler.GET("/oauth2/v2/auth", func(c echo.Context) error {
		ok := c.QueryParam("scope") == "the world" &&
			c.QueryParam("client_id") == "the-client-id" &&
			c.QueryParam("response_type") == "code" &&
			c.QueryParam("redirect_uri") == redirectURI

		if !ok {
			return echo.NewHTTPError(400, "Bad Params "+c.QueryParams().Encode())
		}
		opts := &url.Values{}
		opts.Add("code", "myaccesscode")
		opts.Add("state", c.QueryParam("state"))
		return c.String(200, c.QueryParam("redirect_uri")+"?"+opts.Encode())
	})
	serviceHandler.POST("/oauth2/v4/token", func(c echo.Context) error {
		ok := c.FormValue("code") == "myaccesscode" &&
			c.FormValue("client_id") == "the-client-id" &&
			c.FormValue("client_secret") == "the-client-secret"

		if !ok {
			vv, _ := c.FormParams()
			return echo.NewHTTPError(400, "Bad Params "+vv.Encode())
		}
		return c.JSON(200, map[string]interface{}{
			"access_token":  "the-access-token",
			"refresh_token": "the-refresh-token",
			"expires_in":    3600,
			"token_type":    "Bearer",
		})
	})
	return httptest.NewServer(serviceHandler)
}
