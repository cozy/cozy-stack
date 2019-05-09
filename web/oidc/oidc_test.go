package oidc

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
)

var testInstance *instance.Instance
var ts *httptest.Server

func TestStartWithOnboardingNotFinished(t *testing.T) {
	// Should get a 200 with body "activate your cozy"
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/oidc/start", nil)
	req.Host = testInstance.Domain
	assert.NoError(t, err)

	// Preventing redirects
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	content, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "Onboarding Not activated")
}

func TestStartWithOnboardingFinished(t *testing.T) {
	// Creating a custom context with oidc authentication
	authentication := map[string]interface{}{
		"oidc": map[string]interface{}{
			"redirect_uri":            "http://foobar.com/redirect",
			"client_id":               "foo",
			"client_secret":           "bar",
			"scope":                   "foo",
			"authorize_url":           "http://foobar.com/authorize",
			"token_url":               "http://foobar.com/token",
			"userinfo_url":            "http://foobar.com/userinfos",
			"userinfo_instance_field": "foooo",
		},
	}
	conf := config.GetConfig()
	conf.Authentication = map[string]interface{}{
		"foocontext": authentication,
	}

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
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"
	testutils.NeedCouchdb()
	testSetup := testutils.NewSetup(m, "oidc_test")
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	testInstance = testSetup.GetTestInstance(&lifecycle.Options{ContextName: "foocontext"})
	ts = testSetup.GetTestServer("/oidc", Routes)
	ts.Config.Handler.(*echo.Echo).Renderer = render
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	os.Exit(testSetup.Run())
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}
