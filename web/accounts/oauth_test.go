package accounts

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ts *httptest.Server
var testInstance *instance.Instance
var setup *testutils.TestSetup
var jar *testutils.CookieJar
var client *http.Client

func TestAccessCodeOauthFlow(t *testing.T) {
	redirectURI := ts.URL + "/accounts/test-service/redirect"

	service := makeTestACService(redirectURI)
	defer service.Close()

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
	if !assert.NoError(t, err) {
		return
	}
	defer func() {
		_ = couchdb.DeleteDoc(prefixer.SecretsPrefixer, &serviceType)
	}()

	u := ts.URL + "/accounts/test-service/start?scope=the+world&state=somesecretstate"

	res, err := client.Get(u)
	if !assert.NoError(t, err) {
		return
	}

	bb, err := ioutil.ReadAll(res.Body)
	if !assert.NoError(t, err) {
		return
	}
	res.Body.Close()
	okURL := string(bb)

	if !assert.Equal(t, 200, res.StatusCode) {
		fmt.Println("Bad response", res, okURL)
		return
	}

	// the user click the oauth link
	res2, err := (&http.Client{CheckRedirect: stopBeforeDataCollectFail}).Get(okURL)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusSeeOther, res2.StatusCode)
	finalURL, err := res2.Location()
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Contains(t, finalURL.String(), "home") {
		return
	}

	var out couchdb.JSONDoc
	err = couchdb.GetDoc(testInstance, consts.Accounts, finalURL.Query().Get("account"), &out)
	assert.NoError(t, err)
	assert.Equal(t, "the-access-token", out.M["oauth"].(map[string]interface{})["access_token"])
	out.Type = consts.Accounts
	out.M["manual_cleaning"] = true
	_ = couchdb.DeleteDoc(testInstance, &out)
}

func TestRedirectURLOauthFlow(t *testing.T) {
	redirectURI := "http://" + testInstance.Domain + "/accounts/test-service2/redirect"
	service := makeTestRedirectURLService(redirectURI)
	defer service.Close()

	serviceType := account.AccountType{
		DocID:        "test-service2",
		GrantMode:    account.ImplicitGrantRedirectURL,
		AuthEndpoint: service.URL + "/oauth2/v2/auth",
	}
	err := couchdb.CreateNamedDoc(prefixer.SecretsPrefixer, &serviceType)
	if !assert.NoError(t, err) {
		return
	}
	defer func() {
		_ = couchdb.DeleteDoc(prefixer.SecretsPrefixer, &serviceType)
	}()

	u := ts.URL + "/accounts/test-service2/start?scope=the+world"

	res, err := client.Get(u)
	if !assert.NoError(t, err) {
		return
	}

	bb, err := ioutil.ReadAll(res.Body)
	if !assert.NoError(t, err) {
		return
	}
	res.Body.Close()
	okURL := string(bb)

	if !assert.Equal(t, 200, res.StatusCode) {
		fmt.Println("Bad response", res, okURL)
		return
	}

	res2, err := (&http.Client{CheckRedirect: stopBeforeDataCollectFail}).Get(okURL)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusSeeOther, res2.StatusCode)
	finalURL, err := res2.Location()
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Contains(t, finalURL.String(), "home") {
		return
	}

	var out couchdb.JSONDoc
	err = couchdb.GetDoc(testInstance, consts.Accounts, finalURL.Query().Get("account"), &out)
	assert.NoError(t, err)
	assert.Equal(t, "the-access-token2", out.M["oauth"].(map[string]interface{})["access_token"])
	out.Type = consts.Accounts
	out.M["manual_cleaning"] = true
	_ = couchdb.DeleteDoc(testInstance, &out)
}

func TestDoNotRecreateAccountIfItAlreadyExists(t *testing.T) {
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
	defer func() {
		existingAccount.M["manual_cleaning"] = true
		_ = couchdb.DeleteDoc(testInstance, existingAccount)
	}()

	redirectURI := "http://" + testInstance.Domain + "/accounts/test-service3/redirect"
	service := makeTestRedirectURLService(redirectURI)
	defer service.Close()

	serviceType := account.AccountType{
		DocID:        "test-service3",
		GrantMode:    account.ImplicitGrantRedirectURL,
		AuthEndpoint: service.URL + "/oauth2/v2/auth",
	}
	err = couchdb.CreateNamedDoc(prefixer.SecretsPrefixer, &serviceType)
	require.NoError(t, err)
	defer func() {
		_ = couchdb.DeleteDoc(prefixer.SecretsPrefixer, &serviceType)
	}()

	u := ts.URL + "/accounts/test-service3/start?scope=the+world"
	res, err := client.Get(u)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	bb, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err)
	okURL := string(bb)

	res2, err := (&http.Client{CheckRedirect: stopBeforeDataCollectFail}).Get(okURL)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, res2.StatusCode)
	finalURL, err := res2.Location()
	require.NoError(t, err)
	assert.Equal(t, finalURL.Query().Get("account"), existingAccount.ID())
}

func TestFixedRedirectURIOauthFlow(t *testing.T) {
	redirectURI := "http://oauth_callback.cozy.localhost/accounts/test-service3/redirect"
	service := makeTestACService(redirectURI)
	defer service.Close()

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
	if !assert.NoError(t, err) {
		return
	}
	defer func() {
		_ = couchdb.DeleteDoc(prefixer.SecretsPrefixer, &serviceType)
	}()

	startURL, err := url.Parse(ts.URL + "/accounts/test-service3/start?scope=the+world")
	if !assert.NoError(t, err) {
		return
	}

	res, err := client.Get(startURL.String())
	if !assert.NoError(t, err) {
		return
	}

	bb, err := ioutil.ReadAll(res.Body)
	if !assert.NoError(t, err) {
		return
	}
	res.Body.Close()
	okURL := string(bb)

	if !assert.Equal(t, 200, res.StatusCode) {
		fmt.Println("Bad response", res, okURL)
		return
	}

	okURLObj, err := url.Parse(okURL)
	if !assert.NoError(t, err) {
		return
	}

	// hack, we want to speak with ts.URL but setting Host to _oauth_callback
	host := okURLObj.Host
	okURLObj.Host = startURL.Host
	req2, err := http.NewRequest("GET", okURLObj.String(), nil)
	if !assert.NoError(t, err) {
		return
	}
	req2.Host = host

	res2, err := (&http.Client{CheckRedirect: stopBeforeDataCollectFail}).Do(req2)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusSeeOther, res2.StatusCode)
	finalURL, err := res2.Location()
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Contains(t, finalURL.String(), "home") {
		return
	}

	var out couchdb.JSONDoc
	err = couchdb.GetDoc(testInstance, consts.Accounts, finalURL.Query().Get("account"), &out)
	assert.NoError(t, err)
	assert.Equal(t, "the-access-token", out.M["oauth"].(map[string]interface{})["access_token"])
	out.Type = consts.Accounts
	out.M["manual_cleaning"] = true
	_ = couchdb.DeleteDoc(testInstance, &out)
}

func TestCheckLogin(t *testing.T) {
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
	if !assert.NoError(t, err) {
		return
	}
	defer func() {
		_ = couchdb.DeleteDoc(prefixer.SecretsPrefixer, &serviceType)
	}()

	u := ts.URL + "/accounts/test-service4/start?scope=foo&state=bar"
	inAppBrowser := &http.Client{
		CheckRedirect: noRedirect,
		Jar:           setup.GetCookieJar(),
	}
	res, err := inAppBrowser.Get(u)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, res.StatusCode, 403)

	sessionCode, err := testInstance.CreateSessionCode()
	require.NoError(t, err)
	u2 := u + "&session_code=" + sessionCode
	res2, err := inAppBrowser.Get(u2)
	require.NoError(t, err)
	res2.Body.Close()
	require.Equal(t, res2.StatusCode, 303)
	assert.Contains(t, res2.Header.Get("Location"), serviceType.AuthEndpoint)
	cookies := res2.Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, cookies[0].Name, "cozysessid")
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	build.BuildMode = build.ModeDev
	testutils.NeedCouchdb()

	setup = testutils.NewSetup(m, "oauth-konnectors")
	ts = setup.GetTestServer("/accounts", Routes, func(r *echo.Echo) *echo.Echo {
		r.POST("/login", func(c echo.Context) error {
			sess, _ := session.New(testInstance, session.LongRun)
			cookie, _ := sess.ToCookie()
			c.SetCookie(cookie)
			return c.HTML(http.StatusOK, "OK")
		})
		return r
	})

	testInstance = setup.GetTestInstance(&lifecycle.Options{
		Domain: strings.Replace(ts.URL, "http://127.0.0.1", "cozy.localhost", 1),
	})
	_ = couchdb.ResetDB(prefixer.SecretsPrefixer, consts.AccountTypes)
	setup.AddCleanup(func() error {
		return couchdb.DeleteDB(prefixer.SecretsPrefixer, consts.AccountTypes)
	})

	// Login
	jar = setup.GetCookieJar()
	client = &http.Client{Jar: jar}
	req, _ := http.NewRequest("POST", ts.URL+"/login", nil)
	req.Host = testInstance.Domain
	_, _ = client.Do(req)

	os.Exit(setup.Run())
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func stopBeforeDataCollectFail(req *http.Request, via []*http.Request) error {
	if strings.Contains(req.URL.String(), "home") {
		return http.ErrUseLastResponse
	}
	return nil
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
