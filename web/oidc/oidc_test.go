package oidc

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOidc(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var redirectURL *url.URL

	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
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

	testInstance := setup.GetTestInstance(&lifecycle.Options{ContextName: "foocontext"})

	// Mocking API endpoint to validate token
	ts := setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/oidc": Routes,
		"/token": func(g *echo.Group) {
			g.POST("/getToken", func(c echo.Context) error {
				return c.JSON(http.StatusOK, echo.Map{"access_token": "foobar"})
			})
			g.GET("/:domain", func(c echo.Context) error {
				return c.JSON(http.StatusOK, echo.Map{"domain": c.Param("domain")})
			})
		},
	})

	ts.Config.Handler.(*echo.Echo).Renderer = render
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

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
		e := httpexpect.Default(t, ts.URL)

		// Should get a 200 with body "activate your cozy"
		e.GET("/oidc/start").
			WithHost(testInstance.Domain).
			Expect().Status(200).
			ContentType("text/html").
			Body().Contains("Onboarding Not activated")
	})

	t.Run("StartWithOnboardingFinished", func(t *testing.T) {
		var err error

		e := httpexpect.Default(t, ts.URL)

		onboardingFinished := true
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{OnboardingFinished: &onboardingFinished})

		// Should return a 303 redirect
		u := e.GET("/oidc/start").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Raw()

		redirectURL, err = url.Parse(u)
		require.NoError(t, err)

		assert.Equal(t, "foobar.com", redirectURL.Host)
		assert.Equal(t, "/authorize", redirectURL.Path)
		assert.NotNil(t, redirectURL.Query().Get("client_id"))
		assert.NotNil(t, redirectURL.Query().Get("nonce"))
		assert.NotNil(t, redirectURL.Query().Get("redirect_uri"))
		assert.NotNil(t, redirectURL.Query().Get("response_type"))
		assert.NotNil(t, redirectURL.Query().Get("state"))
		assert.NotNil(t, redirectURL.Query().Get("scope"))
	})

	// Get the login page, assert we have an error if state is missing
	t.Run("Success", func(t *testing.T) {
		e := httpexpect.Default(t, ts.URL)

		e.GET("/oidc/login").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithQueryString(redirectURL.RawQuery). // Reuse the query for the request above.
			Expect().Status(303).
			Header("Location").Equal(testInstance.DefaultRedirection().String())
	})

	t.Run("WithoutState", func(t *testing.T) {
		e := httpexpect.Default(t, ts.URL)

		queryWithoutState := redirectURL.Query()
		queryWithoutState.Del("state")

		e.GET("/oidc/login").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithQueryString(queryWithoutState.Encode()).
			Expect().Status(404)
	})

	t.Run("LoginWith2FA", func(t *testing.T) {
		var err error

		e := httpexpect.Default(t, ts.URL)

		onboardingFinished := true
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{OnboardingFinished: &onboardingFinished, AuthMode: "two_factor_mail"})

		u := e.GET("/oidc/start").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Raw()

		redirectURL, err = url.Parse(u)
		require.NoError(t, err)

		// Get the login page, assert we have the 2FA activated
		queryWithToken := redirectURL.Query()
		queryWithToken.Add("token", "foo")

		t.Logf("query: %q\n\n", queryWithToken.Encode())
		body := e.GET("/oidc/login").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithQueryString(queryWithToken.Encode()).
			Expect().Status(200).
			ContentType("text/html").
			Body()

		body.Contains(`<form id="oidc-twofactor-form"`)
		matches := body.Match(`name="access-token" value="(\w+)"`)
		matches.Length().Equal(2)
		accessToken := matches.Index(1).NotEmpty().Raw()

		// Check that the user is redirected to the 2FA page
		u = e.POST("/oidc/twofactor").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithFormField("access-token", accessToken).
			WithFormField("trusted-device-token", "").
			WithFormField("redirect", "").
			WithFormField("confirm", "").
			Expect().Status(303).
			Header("Location").Raw()

		redirectURL, err = url.Parse(u)
		require.NoError(t, err)

		assert.Equal(t, "/auth/twofactor", redirectURL.Path)
		assert.NotNil(t, redirectURL.Query().Get("two_factor_token"))
	})
}
