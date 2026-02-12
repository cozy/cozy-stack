package oidc

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/auth"
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

	config.UseTestFile(t)
	config.GetConfig().Assets = "../../assets"
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	// Declaring a dummy worker for the 2FA (sendmail)
	wl := &job.WorkerConfig{
		WorkerType:  "sendmail",
		Concurrency: 4,
		WorkerFunc: func(ctx *job.TaskContext) error {
			return nil
		},
	}
	job.AddWorker(wl)

	testInstance := setup.GetTestInstance(&lifecycle.Options{ContextName: "foocontext"})

	// Mocking API endpoint to validate token
	ts := setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/oidc":       Routes,
		"/admin-oidc": AdminRoutes,
		"/token": func(g *echo.Group) {
			g.POST("/getToken", func(c echo.Context) error {
				return c.JSON(http.StatusOK, echo.Map{
					"access_token": "foobar",
					"id_token":     makeUnsignedJWT(map[string]interface{}{"sid": "login-session-456"}),
				})
			})
			g.GET("/:domain", func(c echo.Context) error {
				return c.JSON(http.StatusOK, echo.Map{"domain": c.Param("domain")})
			})
		},
		"/api": func(g *echo.Group) {
			g.GET("/v1/userinfo", func(c echo.Context) error {
				auth := c.Request().Header.Get(echo.HeaderAuthorization)
				if auth != "Bearer fc_token" {
					return c.NoContent(http.StatusBadRequest)
				}
				return c.JSON(http.StatusOK, echo.Map{
					"sub":   "fc_sub",
					"email": "jerome@example.org",
				})
			})
		},
	})

	ts.Config.Handler.(*echo.Echo).Renderer = render
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	// Creating a custom context with oidc and franceconnect authentication
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
		"franceconnect": map[string]interface{}{
			"redirect_uri":  "http://foobar.com/redirect",
			"client_id":     "fc_client_id",
			"client_secret": "fc_client_secret",
			"scope":         "openid profile",
			"authorize_url": "https://franceconnect.gouv.fr/api/v1/authorize",
			"token_url":     "https://franceconnect.gouv.fr/api/v1/token",
			"userinfo_url":  ts.URL + "/api/v1/userinfo",
		},
	}
	conf := config.GetConfig()
	conf.Authentication = map[string]interface{}{
		"foocontext": authentication,
	}

	require.NoError(t, dynamic.InitDynamicAssetFS(config.FsURL().String()), "Could not init dynamic FS")

	t.Run("StartWithOnboardingNotFinished", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Should get a 200 with body "activate your cozy"
		e.GET("/oidc/start").
			WithHost(testInstance.Domain).
			Expect().Status(200).
			ContentType("text/html").
			Body().Contains("Onboarding Not activated")
	})

	t.Run("StartWithOnboardingFinished", func(t *testing.T) {
		var err error

		e := testutils.CreateTestClient(t, ts.URL)

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
		// Verify login_hint logic: OrgDomain+email > old_domain > OIDCID > nothing
		loginHint := redirectURL.Query().Get("login_hint")
		expectedLoginHint := ""
		if testInstance.OrgDomain != "" {
			if email, err := testInstance.SettingsEMail(); err == nil && email != "" {
				expectedLoginHint = email
			}
		}
		if expectedLoginHint == "" && testInstance.OldDomain != "" {
			expectedLoginHint = testInstance.OldDomain
		} else if expectedLoginHint == "" && testInstance.OIDCID != "" {
			expectedLoginHint = testInstance.OIDCID
		}
		if expectedLoginHint != "" {
			assert.NotEmpty(t, loginHint, "login_hint should be present in redirect URL")
			assert.Equal(t, expectedLoginHint, loginHint, "login_hint should match email, old_domain or OIDCID")
		} else {
			assert.Empty(t, loginHint, "login_hint should not be present when no hint source is available")
		}
	})

	// Get the login page, assert we have an error if state is missing
	t.Run("Success", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/oidc/login").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithQueryString(redirectURL.RawQuery). // Reuse the query for the request above.
			Expect().Status(303).
			Header("Location").Equal(testInstance.DefaultRedirection().String())
	})

	t.Run("WithoutState", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		queryWithoutState := redirectURL.Query()
		queryWithoutState.Del("state")

		e.GET("/oidc/login").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			WithQueryString(queryWithoutState.Encode()).
			Expect().Status(404)
	})

	t.Run("LoginWith2FA", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		onboardingFinished := true
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{OnboardingFinished: &onboardingFinished, AuthMode: "two_factor_mail"})

		u := e.GET("/oidc/start").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Raw()

		redirectURL, err := url.Parse(u)
		require.NoError(t, err)

		// Get the login page, assert we have the 2FA activated
		queryWithToken := redirectURL.Query()
		queryWithToken.Add("token", "foo")

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

	t.Run("DelegatedCode", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		sub := "fc_sub"
		email := "jerome@example.org"

		onboardingFinished := true
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{
			OnboardingFinished: &onboardingFinished,
			FranceConnectID:    sub,
			AuthMode:           "basic",
		})

		obj := e.POST("/admin-oidc/"+testInstance.ContextName+"/franceconnect/code").
			WithHeader("Content-Type", "application/json").
			WithJSON(map[string]string{
				"access_token": "fc_token",
				"id_token":     makeUnsignedJWT(map[string]interface{}{"sid": "cloudery-session-789"}),
			}).
			Expect().Status(200).
			JSON().
			Object()
		obj.Value("sub").String().Equal(sub)
		obj.Value("email").String().Equal(email)
		code := obj.Value("delegated_code").String().NotEmpty().Raw()

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

		obj2 := e.POST("/oidc/access_token").
			WithHost(testInstance.Domain).
			WithJSON(map[string]string{
				"client_id":     oauthClient.ClientID,
				"client_secret": oauthClient.ClientSecret,
				"scope":         "*",
				"code":          code,
			}).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/json"}).
			Object()
		obj2.Value("token_type").String().Equal("bearer")
		obj2.Value("scope").String().Equal("*")
		obj2.Value("access_token").String().NotEmpty()
		obj2.Value("refresh_token").String().NotEmpty()

		// Verify the session ID was retrieved from the delegated code (not from id_token in AccessToken)
		storedClient, err := oauth.FindClient(testInstance, oauthClient.ClientID)
		require.NoError(t, err)
		require.Equal(t, "cloudery-session-789", storedClient.OIDCSessionID)
	})

	t.Run("LoginHintWithOIDCID", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		onboardingFinished := true
		oidcID := "user-oidc-sub-123"
		// Set OIDCID without old_domain to test OIDCID priority
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{
			OnboardingFinished: &onboardingFinished,
			OIDCID:             oidcID,
		})

		// Test that login_hint uses OIDCID when old_domain is not present
		u := e.GET("/oidc/start").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Raw()

		redirectURL, err := url.Parse(u)
		require.NoError(t, err)

		loginHint := redirectURL.Query().Get("login_hint")
		// If old_domain exists from previous test, it should take priority
		// Otherwise, OIDCID should be used
		if testInstance.OldDomain != "" {
			assert.Equal(t, testInstance.OldDomain, loginHint, "login_hint should use old_domain when present (priority)")
		} else {
			assert.NotEmpty(t, loginHint, "login_hint should be present when OIDCID is set")
			assert.Equal(t, oidcID, loginHint, "login_hint should use OIDCID when old_domain is not present")
		}
	})

	t.Run("LoginHintWithOrgDomain", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		onboardingFinished := true
		orgDomain := "company.com"
		email := "user@company.com"

		// Clear old_domain and OIDCID, set OrgDomain and email
		testInstance.OrgDomain = orgDomain
		testInstance.OldDomain = ""
		testInstance.OIDCID = ""
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{
			OnboardingFinished: &onboardingFinished,
			Email:              email,
		})

		// Test that login_hint uses email when OrgDomain is present (highest priority)
		u := e.GET("/oidc/start").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Raw()

		redirectURL, err := url.Parse(u)
		require.NoError(t, err)

		loginHint := redirectURL.Query().Get("login_hint")
		assert.NotEmpty(t, loginHint, "login_hint should be present when OrgDomain is set with email")
		assert.Equal(t, email, loginHint, "login_hint should use email when OrgDomain is present (highest priority)")

		// Clean up
		testInstance.OrgDomain = ""
	})

	t.Run("LoginHintWithOldDomain", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		onboardingFinished := true
		oldDomain := "old.example.com"
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{
			OnboardingFinished: &onboardingFinished,
			OldDomain:          oldDomain,
		})

		// Test that login_hint uses old_domain when present (highest priority)
		u := e.GET("/oidc/start").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Raw()

		redirectURL, err := url.Parse(u)
		require.NoError(t, err)

		loginHint := redirectURL.Query().Get("login_hint")
		assert.NotEmpty(t, loginHint, "login_hint should be present when old_domain is set")
		assert.Equal(t, oldDomain, loginHint, "login_hint should use old_domain when present (highest priority)")
	})

	t.Run("LoginHintWithFranceConnect", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		onboardingFinished := true
		oldDomain := "old.franceconnect.example.com"
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{
			OnboardingFinished: &onboardingFinished,
			OldDomain:          oldDomain,
		})

		// Test that login_hint is present for FranceConnect flow
		u := e.GET("/oidc/franceconnect").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Raw()

		redirectURL, err := url.Parse(u)
		require.NoError(t, err)

		loginHint := redirectURL.Query().Get("login_hint")
		assert.NotEmpty(t, loginHint, "login_hint should be present for FranceConnect flow")
		assert.Equal(t, oldDomain, loginHint, "login_hint should use old_domain for FranceConnect")
	})

	t.Run("LoginHintWithoutOldDomainOrOIDCID", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		onboardingFinished := true
		_ = lifecycle.Patch(testInstance, &lifecycle.Options{
			OnboardingFinished: &onboardingFinished,
		})

		// Test that login_hint behavior depends on current instance state
		// If old_domain or OIDCID exist from previous tests, they will be used
		// Otherwise, no login_hint should be present
		u := e.GET("/oidc/start").
			WithHost(testInstance.Domain).
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Raw()

		redirectURL, err := url.Parse(u)
		require.NoError(t, err)

		loginHint := redirectURL.Query().Get("login_hint")
		// Check current state - if neither old_domain nor OIDCID is set, login_hint should be empty
		if testInstance.OldDomain == "" && testInstance.OIDCID == "" {
			assert.Empty(t, loginHint, "login_hint should not be present when neither old_domain nor OIDCID is set")
		} else {
			// If one of them exists, it should be used (this tests the priority logic)
			assert.NotEmpty(t, loginHint, "login_hint should be present if old_domain or OIDCID is set")
		}
	})

	t.Run("LoginHintWithoutDomain", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Test Bitwarden flow where domain is empty - login_hint should not be present
		// Note: This tests the BitwardenStart endpoint which passes empty domain
		u := e.GET("/oidc/bitwarden/foocontext").
			WithQuery("redirect_uri", "cozypass://login").
			WithRedirectPolicy(httpexpect.DontFollowRedirects).
			Expect().Status(303).
			Header("Location").Raw()

		redirectURL, err := url.Parse(u)
		require.NoError(t, err)

		loginHint := redirectURL.Query().Get("login_hint")
		assert.Empty(t, loginHint, "login_hint should not be present when domain is empty (Bitwarden case)")
	})
}

func TestFlagshipOIDCLoginLogoutIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	config.GetConfig().Assets = "../../assets"
	testutils.NeedCouchdb(t)

	// Reset state storage to avoid leaking codes between tests
	globalStorageMutex.Lock()
	globalStorage = nil
	globalStorageMutex.Unlock()

	setup := testutils.NewSetup(t, t.Name())

	// Prepare mock OIDC provider
	sub := "flagship-user-sub"
	email := "flagship@example.org"
	sessionID := "flagship-session-123"

	logoutCh := make(chan string, 1)

	var oidcServer *httptest.Server
	oidcServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			resp := map[string]string{
				"issuer":                 oidcServer.URL,
				"authorization_endpoint": oidcServer.URL + "/authorize",
				"token_endpoint":         oidcServer.URL + "/token",
				"userinfo_endpoint":      oidcServer.URL + "/userinfo",
				"end_session_endpoint":   oidcServer.URL + "/logout",
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"sub":   sub,
				"email": email,
			})
		case "/logout":
			logoutCh <- r.URL.Query().Get("session_id")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(oidcServer.Close)

	cfg := config.GetConfig()
	cfg.Authentication = map[string]interface{}{
		"flagship-ctx": map[string]interface{}{
			"oidc": map[string]interface{}{
				"client_id":             "provider-client-id",
				"client_secret":         "provider-secret",
				"scope":                 "openid profile email",
				"redirect_uri":          "https://example.org/callback",
				"authorize_url":         oidcServer.URL + "/authorize",
				"token_url":             oidcServer.URL + "/token",
				"userinfo_url":          oidcServer.URL + "/userinfo",
				"allow_custom_instance": true,
			},
		},
	}

	cache := cfg.CacheStorage
	cache.Clear("oidc-config:" + oidcServer.URL + "/.well-known/openid-configuration")

	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	ts := setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/oidc":       Routes,
		"/admin-oidc": AdminRoutes,
		"/auth":       auth.Routes,
	})
	ts.Config.Handler.(*echo.Echo).Renderer = render
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	onboardingFinished := true
	testInstance := setup.GetTestInstance(&lifecycle.Options{
		ContextName:        "flagship-ctx",
		OnboardingFinished: &onboardingFinished,
		OIDCID:             sub,
	})

	flagshipClient := &oauth.Client{
		RedirectURIs:    []string{"cozy://flagship"},
		ClientName:      "Cozy Flagship",
		ClientKind:      "mobile",
		SoftwareID:      "cozy-flagship",
		SoftwareVersion: "1.0.0",
	}
	require.Nil(t, flagshipClient.Create(testInstance))

	clientID := flagshipClient.ClientID
	clientSecret := flagshipClient.ClientSecret
	registrationToken := flagshipClient.RegistrationToken

	client, err := oauth.FindClient(testInstance, clientID)
	require.NoError(t, err)
	client.CertifiedFromStore = true
	require.NoError(t, client.SetFlagship(testInstance))

	client, err = oauth.FindClient(testInstance, clientID)
	require.NoError(t, err)
	require.True(t, client.Flagship)

	e := testutils.CreateTestClient(t, ts.URL)

	// Call GetDelegatedCode endpoint (simulating cloudery)
	delegatedCodeResp := e.POST("/admin-oidc/flagship-ctx/generic/code").
		WithHeader("Content-Type", "application/json").
		WithJSON(map[string]string{
			"access_token": "cloudery-access-token",
			"id_token":     makeUnsignedJWT(map[string]interface{}{"sid": sessionID}),
		}).
		Expect().Status(http.StatusOK).
		JSON().
		Object()
	delegatedCodeResp.Value("sub").String().IsEqual(sub)
	delegatedCodeResp.Value("email").String().IsEqual(email)
	delegatedCode := delegatedCodeResp.Value("delegated_code").String().NotEmpty().Raw()

	// Call AccessToken with only the delegated code (no id_token)
	accessTokenResp := e.POST("/oidc/access_token").
		WithHost(testInstance.Domain).
		WithJSON(map[string]string{
			"client_id":     clientID,
			"client_secret": clientSecret,
			"scope":         "*",
			"code":          delegatedCode,
		}).
		Expect().Status(http.StatusOK).
		JSON().
		Object()
	accessTokenResp.Value("token_type").String().Equal("bearer")
	accessTokenResp.Value("scope").String().Equal("*")
	accessTokenResp.Value("access_token").String().NotEmpty()
	accessTokenResp.Value("refresh_token").String().NotEmpty()

	// Verify the session ID was stored in the OAuth client
	client, err = oauth.FindClient(testInstance, clientID)
	require.NoError(t, err)
	require.Equal(t, sessionID, client.OIDCSessionID)

	e.DELETE("/auth/register/"+clientID).
		WithHost(testInstance.Domain).
		WithHeader("Authorization", "Bearer "+registrationToken).
		Expect().
		Status(http.StatusNoContent)

	select {
	case sid := <-logoutCh:
		require.Equal(t, sessionID, sid)
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for OIDC end_session call")
	}
}

func TestOIDCLogout(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	setup := testutils.NewSetup(t, t.Name())
	_ = setup.GetTestInstance() // Not used in current tests, but needed for setup

	t.Run("PerformOIDCLogoutWithValidConfiguration", func(t *testing.T) {
		// Mock HTTP server for OIDC provider
		var mockServer *httptest.Server
		mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/openid-configuration":
				config := oauth.OIDCConfiguration{
					Issuer:             mockServer.URL,
					EndSessionEndpoint: mockServer.URL + "/logout",
				}
				json.NewEncoder(w).Encode(config)
			case "/logout":
				// Verify session_id parameter is present
				sessionID := r.URL.Query().Get("session_id")
				assert.Equal(t, "test-session-123", sessionID)
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer mockServer.Close()

		// Configure test context with mock server
		testConfig := map[string]interface{}{
			"token_url": mockServer.URL + "/token",
		}
		config.GetConfig().Authentication = map[string]interface{}{
			"test-context": map[string]interface{}{
				"oidc": testConfig,
			},
		}

		err := oauth.PerformOIDCLogout("test-context", "test-session-123")
		assert.NoError(t, err)
	})

	t.Run("CachingBehavior", func(t *testing.T) {
		requestCount := 0
		var mockServer *httptest.Server
		mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/openid-configuration" {
				requestCount++
				config := oauth.OIDCConfiguration{
					Issuer:             mockServer.URL,
					EndSessionEndpoint: mockServer.URL + "/logout",
				}
				json.NewEncoder(w).Encode(config)
			}
		}))
		defer mockServer.Close()

		// Configure test context
		testConfig := map[string]interface{}{
			"token_url": mockServer.URL + "/token",
		}
		config.GetConfig().Authentication = map[string]interface{}{
			"cache-test": map[string]interface{}{
				"oidc": testConfig,
			},
		}

		// First call should fetch from server
		_ = oauth.PerformOIDCLogout("cache-test", "session-1")
		assert.Equal(t, 1, requestCount)

		// Second call should use cache
		_ = oauth.PerformOIDCLogout("cache-test", "session-2")
		assert.Equal(t, 1, requestCount, "Configuration should be cached")
	})
}

// makeUnsignedJWT creates a simple unsigned JWT for testing.
func makeUnsignedJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + payloadB64 + "."
}
