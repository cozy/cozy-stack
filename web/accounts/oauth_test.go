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
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var testInstance *instance.Instance
var setup *testutils.TestSetup

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
	err := couchdb.CreateNamedDoc(couchdb.GlobalSecretsDB, &serviceType)
	if !assert.NoError(t, err) {
		return
	}
	defer func() {
		_ = couchdb.DeleteDoc(couchdb.GlobalSecretsDB, &serviceType)
	}()

	u := ts.URL + "/accounts/test-service/start?scope=the+world&state=somesecretstate"

	res, err := http.Get(u)
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
	err := couchdb.CreateNamedDoc(couchdb.GlobalSecretsDB, &serviceType)
	if !assert.NoError(t, err) {
		return
	}
	defer func() {
		_ = couchdb.DeleteDoc(couchdb.GlobalSecretsDB, &serviceType)
	}()

	u := ts.URL + "/accounts/test-service2/start?scope=the+world"

	res, err := http.Get(u)
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
}

func TestFixedRedirectURIOauthFlow(t *testing.T) {
	redirectURI := "http://_oauth_callback.cozy.tools/accounts/test-service3/redirect"
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
	err := couchdb.CreateNamedDoc(couchdb.GlobalSecretsDB, &serviceType)
	if !assert.NoError(t, err) {
		return
	}
	defer func() {
		_ = couchdb.DeleteDoc(couchdb.GlobalSecretsDB, &serviceType)
	}()

	startURL, err := url.Parse(ts.URL + "/accounts/test-service3/start?scope=the+world")
	if !assert.NoError(t, err) {
		return
	}

	res, err := http.Get(startURL.String())
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
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	build.BuildMode = build.ModeDev
	testutils.NeedCouchdb()

	setup = testutils.NewSetup(m, "oauth-konnectors")
	ts = setup.GetTestServer("/accounts", Routes)
	testInstance = setup.GetTestInstance(&lifecycle.Options{
		Domain: strings.Replace(ts.URL, "http://127.0.0.1", "cozy.tools", 1),
	})
	_ = couchdb.ResetDB(couchdb.GlobalSecretsDB, consts.AccountTypes)
	setup.AddCleanup(func() error {
		return couchdb.DeleteDB(couchdb.GlobalSecretsDB, consts.AccountTypes)
	})

	os.Exit(setup.Run())
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
