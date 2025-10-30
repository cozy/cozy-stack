package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
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
	jwt "github.com/golang-jwt/jwt/v5"
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
				return c.JSON(http.StatusOK, echo.Map{"access_token": "foobar"})
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
			WithBytes([]byte(`{ "access_token": "fc_token" }`)).
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

		idToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sid": "delegated-session-123",
		})
		idTokenStr, err := idToken.SignedString([]byte("cozy-stack-test"))
		require.NoError(t, err)

		obj2 := e.POST("/oidc/access_token").
			WithHost(testInstance.Domain).
			WithJSON(map[string]string{
				"client_id":     oauthClient.ClientID,
				"client_secret": oauthClient.ClientSecret,
				"scope":         "*",
				"code":          code,
				"id_token":      idTokenStr,
			}).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/json"}).
			Object()
		obj2.Value("token_type").String().Equal("bearer")
		obj2.Value("scope").String().Equal("*")
		obj2.Value("access_token").String().NotEmpty()
		obj2.Value("refresh_token").String().NotEmpty()

		storedClient, err := oauth.FindClient(testInstance, oauthClient.ClientID)
		require.NoError(t, err)
		require.Equal(t, "delegated-session-123", storedClient.OIDCSessionID)
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
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kid := "flagship-oidc-key"
	modulus := base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes())
	exponent := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.E)).Bytes())

	logoutCh := make(chan string, 1)
	jwksPayload := map[string]interface{}{
		"keys": []map[string]string{{
			"kty": "RSA",
			"use": "sig",
			"kid": kid,
			"alg": "RS256",
			"n":   modulus,
			"e":   exponent,
		}},
	}

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
				"jwks_uri":               oidcServer.URL + "/jwks",
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/jwks":
			_ = json.NewEncoder(w).Encode(jwksPayload)
		case "/logout":
			logoutCh <- r.URL.Query().Get("session_id")
			w.WriteHeader(http.StatusOK)
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"domain": "example.test",
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(oidcServer.Close)

	cfg := config.GetConfig()
	cfg.Authentication = map[string]interface{}{
		"flagship-ctx": map[string]interface{}{
			"oidc": map[string]interface{}{
				"client_id":               "provider-client-id",
				"client_secret":           "provider-secret",
				"scope":                   "openid profile email",
				"redirect_uri":            "https://example.org/callback",
				"authorize_url":           oidcServer.URL + "/authorize",
				"token_url":               oidcServer.URL + "/token",
				"userinfo_url":            oidcServer.URL + "/userinfo",
				"userinfo_instance_field": "domain",
				"id_token_jwk_url":        oidcServer.URL + "/jwks",
				"allow_oauth_token":       true,
			},
		},
	}

	cache := cfg.CacheStorage
	cache.Clear("oidc-config:" + oidcServer.URL + "/.well-known/openid-configuration")
	cache.Clear("oidc-jwk:" + oidcServer.URL + "/jwks")

	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	ts := setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/oidc": Routes,
		"/auth": auth.Routes,
	})
	ts.Config.Handler.(*echo.Echo).Renderer = render
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	onboardingFinished := true
	testInstance := setup.GetTestInstance(&lifecycle.Options{
		ContextName:        "flagship-ctx",
		OnboardingFinished: &onboardingFinished,
		OIDCID:             "user-oidc-sub",
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

	globalStorageMutex.Lock()
	globalStorage = nil
	globalStorageMutex.Unlock()
	code := getStorage().CreateCode(testInstance.OIDCID)
	require.NotEmpty(t, code)

	claims := jwt.MapClaims{
		"iss": oidcServer.URL,
		"sub": testInstance.OIDCID,
		"sid": "flagship-session-123",
		"aud": []string{clientID},
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	idToken, err := token.SignedString(privateKey)
	require.NoError(t, err)

	e := testutils.CreateTestClient(t, ts.URL)

	obj := e.POST("/oidc/access_token").
		WithHost(testInstance.Domain).
		WithJSON(map[string]interface{}{
			"client_id":     clientID,
			"client_secret": clientSecret,
			"scope":         "*",
			"code":          code,
			"id_token":      idToken,
		}).
		Expect().Status(http.StatusOK).
		JSON().
		Object()
	obj.Value("token_type").String().Equal("bearer")
	obj.Value("scope").String().Equal("*")
	obj.Value("access_token").String().NotEmpty()
	obj.Value("refresh_token").String().NotEmpty()

	client, err = oauth.FindClient(testInstance, clientID)
	require.NoError(t, err)
	require.Equal(t, "flagship-session-123", client.OIDCSessionID)

	e.DELETE("/auth/register/"+clientID).
		WithHost(testInstance.Domain).
		WithHeader("Authorization", "Bearer "+registrationToken).
		Expect().
		Status(http.StatusNoContent)

	select {
	case sid := <-logoutCh:
		require.Equal(t, "flagship-session-123", sid)
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

	t.Run("NetworkErrorHandling", func(t *testing.T) {
		// Test with unreachable server
		testConfig := map[string]interface{}{
			"token_url": "http://unreachable.example.com/token",
		}
		config.GetConfig().Authentication = map[string]interface{}{
			"network-error-test": map[string]interface{}{
				"oidc": testConfig,
			},
		}

		err := oauth.PerformOIDCLogout("network-error-test", "session-123")
		// Should not error (best-effort)
		assert.NoError(t, err)
	})

	t.Run("MalformedConfigurationHandling", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/openid-configuration" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("invalid json"))
			}
		}))
		defer mockServer.Close()

		testConfig := map[string]interface{}{
			"token_url": mockServer.URL + "/token",
		}
		config.GetConfig().Authentication = map[string]interface{}{
			"malformed-test": map[string]interface{}{
				"oidc": testConfig,
			},
		}

		err := oauth.PerformOIDCLogout("malformed-test", "session-123")
		assert.NoError(t, err, "Should handle malformed configuration gracefully")
	})

	t.Run("EmptySessionID", func(t *testing.T) {
		err := oauth.PerformOIDCLogout("test-context", "")
		assert.NoError(t, err, "Should not error with empty session ID")
	})

	t.Run("InvalidContext", func(t *testing.T) {
		err := oauth.PerformOIDCLogout("nonexistent-context", "some-session")
		assert.NoError(t, err, "Should not error with invalid context (best-effort)")
	})
}
