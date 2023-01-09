package oidc

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testInstance *instance.Instance
var ts *httptest.Server

func TestOidc(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(nil, t.Name())
	t.Cleanup(setup.Cleanup)
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	// Declaring a dummy worker for the 2FA (sendmail)
	wl := &job.WorkerConfig{
		WorkerType:  "sendmail",
		Concurrency: 4,
		WorkerFunc: func(ctx *job.WorkerContext) error {
			return nil
		},
	}
	job.AddWorker(wl)

	testInstance = setup.GetTestInstance(&lifecycle.Options{ContextName: "foocontext"})

	// Mocking API endpoint to validate token
	ts = setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/oidc": Routes,
		"/token": func(g *echo.Group) {
			g.POST("/getToken", handleToken)
			g.GET("/:domain", handleUserInfo)
		},
	})

	ts.Config.Handler.(*echo.Echo).Renderer = render
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	// Creating a custom context with oidc authentication
	tokenURL := ts.URL + "/token/getToken"
	userInfoURL := ts.URL + "/token/" + testInstance.Domain
	authentication := map[string]interface{}{
		"oidc": map[string]interface{}{
			"redirect_uri":            "http://foobar.com/redirect",
			"client_id":               "foo",
			"client_secret":           "bar",
			"scope":                   "foo",
			"authorize_url":           "http://foobar.com/authorize",
			"token_url":               tokenURL,
			"userinfo_url":            userInfoURL,
			"userinfo_instance_field": "domain",
		},
	}
	conf := config.GetConfig()
	conf.Authentication = map[string]interface{}{
		"foocontext": authentication,
	}

	require.NoError(t, dynamic.InitDynamicAssetFS(), "Could not init dynamic FS")

	t.Run("StartWithOnboardingNotFinished", func(t *testing.T) {
		// Should get a 200 with body "activate your cozy"
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/oidc/start", nil)
		req.Host = testInstance.Domain
		assert.NoError(t, err)

		// Preventing redirects
		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		content, err := io.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Contains(t, string(content), "Onboarding Not activated")
	})

	t.Run("StartWithOnboardingFinished", func(t *testing.T) {
		onboardingFinished := true
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{OnboardingFinished: &onboardingFinished})

		// Should return a 303 redirect
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/oidc/start", nil)
		req.Host = testInstance.Domain
		assert.NoError(t, err)

		// Preventing redirection to assert we are effectively redirected
		c := &http.Client{CheckRedirect: noRedirect}
		res, err := c.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 303, res.StatusCode)
		location := res.Header["Location"][0]
		redirected, err := url.Parse(location)
		assert.NoError(t, err)
		assert.Equal(t, "foobar.com", redirected.Host)
		assert.Equal(t, "/authorize", redirected.Path)

		values, err := url.ParseQuery(redirected.RawQuery)
		assert.NoError(t, err)
		assert.NotNil(t, values.Get("client_id"))
		assert.NotNil(t, values.Get("nonce"))
		assert.NotNil(t, values.Get("redirect_uri"))
		assert.NotNil(t, values.Get("response_type"))
		assert.NotNil(t, values.Get("state"))
		assert.NotNil(t, values.Get("scope"))
	})

	t.Run("StateIsMandatoryForLogin", func(t *testing.T) {
		onboardingFinished := true
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{OnboardingFinished: &onboardingFinished})

		// Should return a 303 redirect
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/oidc/start", nil)
		req.Host = testInstance.Domain
		assert.NoError(t, err)

		// Preventing redirection to assert we are effectively redirected
		c := &http.Client{CheckRedirect: noRedirect}
		res, err := c.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 303, res.StatusCode)
		location := res.Header["Location"][0]
		redirected, err := url.Parse(location)
		assert.NoError(t, err)
		assert.Equal(t, "foobar.com", redirected.Host)
		assert.Equal(t, "/authorize", redirected.Path)

		values, err := url.ParseQuery(redirected.RawQuery)
		assert.NoError(t, err)
		assert.NotNil(t, values.Get("client_id"))
		assert.NotNil(t, values.Get("nonce"))
		assert.NotNil(t, values.Get("redirect_uri"))
		assert.NotNil(t, values.Get("response_type"))
		assert.NotNil(t, values.Get("state"))
		assert.NotNil(t, values.Get("scope"))

		// Get the login page, assert we have an error if state is missing
		stateID := values.Get("state")
		values.Del("state")
		v := values.Encode()
		reqLogin, err := http.NewRequest(http.MethodGet, ts.URL+"/oidc/login?"+v, nil)
		assert.NoError(t, err)
		reqLogin.Host = testInstance.Domain
		resLogin, err := c.Do(reqLogin)
		assert.NoError(t, err)
		assert.Equal(t, 404, resLogin.StatusCode)

		// And the login page should work with the state
		values.Add("state", stateID)
		v = values.Encode()
		reqLogin, err = http.NewRequest(http.MethodGet, ts.URL+"/oidc/login?"+v, nil)
		assert.NoError(t, err)
		reqLogin.Host = testInstance.Domain
		resLogin, err = c.Do(reqLogin)
		assert.NoError(t, err)
		assert.Equal(t, 303, resLogin.StatusCode)
		location = resLogin.Header["Location"][0]
		assert.Equal(t, location, testInstance.DefaultRedirection().String())
	})

	t.Run("LoginWith2FA", func(t *testing.T) {
		onboardingFinished := true
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{OnboardingFinished: &onboardingFinished, AuthMode: "two_factor_mail"})

		// Should return a 303 redirect
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/oidc/start", nil)
		req.Host = testInstance.Domain
		assert.NoError(t, err)

		// Preventing redirection to assert we are effectively redirected
		c := &http.Client{CheckRedirect: noRedirect}
		res, err := c.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 303, res.StatusCode)
		location := res.Header["Location"][0]
		redirected, err := url.Parse(location)
		assert.NoError(t, err)
		assert.Equal(t, "foobar.com", redirected.Host)
		assert.Equal(t, "/authorize", redirected.Path)

		values, err := url.ParseQuery(redirected.RawQuery)
		assert.NoError(t, err)
		assert.NotNil(t, values.Get("client_id"))
		assert.NotNil(t, values.Get("nonce"))
		assert.NotNil(t, values.Get("redirect_uri"))
		assert.NotNil(t, values.Get("response_type"))
		assert.NotNil(t, values.Get("state"))
		assert.NotNil(t, values.Get("scope"))

		// Get the login page, assert we have the 2FA activated
		values.Add("token", "foo")
		v := values.Encode()
		reqLogin, err := http.NewRequest(http.MethodGet, ts.URL+"/oidc/login?"+v, nil)
		assert.NoError(t, err)
		reqLogin.Host = testInstance.Domain
		resLogin, err := c.Do(reqLogin)
		assert.NoError(t, err)
		assert.Equal(t, 200, resLogin.StatusCode)
		content, err := io.ReadAll(resLogin.Body)
		assert.NoError(t, err)
		assert.Contains(t, string(content), `<form id="oidc-twofactor-form"`)
		re := regexp.MustCompile(`name="access-token" value="(\w+)"`)
		matches := re.FindStringSubmatch(string(content))
		assert.Len(t, matches, 2)
		accessToken := matches[1]

		// Check that the user is redirected to the 2FA page
		values = url.Values{
			"access-token":         {accessToken},
			"trusted-device-token": {""},
			"redirect":             {""},
			"confirm":              {""},
		}
		body := bytes.NewReader([]byte(values.Encode()))
		req2FA, err := http.NewRequest(http.MethodPost, ts.URL+"/oidc/twofactor", body)
		assert.NoError(t, err)
		req2FA.Host = testInstance.Domain
		res2FA, err := c.Do(req2FA)
		assert.NoError(t, err)
		assert.Equal(t, 303, res2FA.StatusCode)

		locationLogin := res2FA.Header["Location"][0]
		redirected, err = url.Parse(locationLogin)
		assert.NoError(t, err)
		assert.Equal(t, "/auth/twofactor", redirected.Path)
		assert.NotNil(t, redirected.Query().Get("two_factor_token"))
	})
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func handleToken(c echo.Context) error {
	return c.JSON(http.StatusOK, echo.Map{"access_token": "foobar"})
}

func handleUserInfo(c echo.Context) error {
	return c.JSON(http.StatusOK, echo.Map{"domain": c.Param("domain")})
}
