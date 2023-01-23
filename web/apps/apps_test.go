// spec package is introduced to avoid circular dependencies since this
// particular test requires to depend on routing directly to expose the API and
// the APP server.
package apps_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apps "github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/intent"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/assets"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/filetype"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	webApps "github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const domain = "cozywithapps.example.net"

var ts *httptest.Server
var testInstance *instance.Instance
var token string

var jar *testutils.CookieJar
var client *http.Client

func TestApps(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	require.NoError(t, setup.SetupSwiftTest(), "Could not init Swift test")

	require.NoError(t, dynamic.InitDynamicAssetFS(), "Could not init dynamic FS")
	tempdir := t.TempDir()

	cfg := config.GetConfig()
	cfg.Fs.URL = &url.URL{
		Scheme: "file",
		Host:   "localhost",
		Path:   tempdir,
	}
	was := cfg.Subdomains
	cfg.Subdomains = config.NestedSubdomains
	defer func() { cfg.Subdomains = was }()

	pass := "aephe2Ei"
	testInstance = setup.GetTestInstance(&lifecycle.Options{Domain: domain})
	params := lifecycle.PassParameters{
		Key:        "fake-encrypt-key",
		Iterations: 0,
	}
	_ = lifecycle.ForceUpdatePassphrase(testInstance, []byte(pass), params)
	testInstance.RegisterToken = nil
	testInstance.OnboardingFinished = true
	_ = testInstance.Update()

	slug, err := setup.InstallMiniApp()
	require.NoError(t, err, "Could not install mini app")

	_, err = setup.InstallMiniKonnector()
	require.NoError(t, err, "Could not install mini konnector")

	ts = setup.GetTestServer("/apps", webApps.WebappsRoutes, func(r *echo.Echo) *echo.Echo {
		r.POST("/login", func(c echo.Context) error {
			sess, _ := session.New(testInstance, session.LongRun)
			cookie, _ := sess.ToCookie()
			c.SetCookie(cookie)
			return c.HTML(http.StatusOK, "OK")
		})
		r.POST("/auth/session_code", auth.CreateSessionCode)
		router, err := web.CreateSubdomainProxy(r, webApps.Serve)
		require.NoError(t, err, "Cant start subdoman proxy")
		return router
	})

	jar = setup.GetCookieJar()
	client = &http.Client{Jar: jar}

	// Login
	req, _ := http.NewRequest("POST", ts.URL+"/login", bytes.NewBufferString("passphrase="+pass))
	req.Host = testInstance.Domain
	_, _ = client.Do(req)

	_, token = setup.GetTestClient(consts.Apps + " " + consts.Konnectors)

	t.Run("Serve", func(t *testing.T) {
		assertAuthGet(t, slug, "/foo/", "text/html; charset=utf-8", `this is index.html. <a lang="en" href="https://cozywithapps.example.net/status/">Status</a>`)
		assertAuthGet(t, slug, "/foo/hello.html", "text/html; charset=utf-8", "world {{.Token}}")
		assertAuthGet(t, slug, "/public", "text/html; charset=utf-8", "this is a file in public/")
		assertAuthGet(t, slug, "/public/index.html", "text/html; charset=utf-8", "this is a file in public/")
		assertAnonGet(t, slug, "/public", "text/html; charset=utf-8", "this is a file in public/")
		assertAnonGet(t, slug, "/public/index.html", "text/html; charset=utf-8", "this is a file in public/")
		assertNotPublic(t, slug, "/foo", 302, "https://cozywithapps.example.net/auth/login?redirect=https%3A%2F%2Fmini.cozywithapps.example.net%2Ffoo")
		assertNotPublic(t, slug, "/foo/hello.tml", 401, "")
		assertNotFound(t, slug, "/404")
		assertNotFound(t, slug, "/")
		assertNotFound(t, slug, "/index.html")
		assertNotFound(t, slug, "/public/hello.html")
		assertInternalServerError(t, slug, "/invalid")
	})

	t.Run("CozyBar", func(t *testing.T) {
		body := doGetAll(t, slug, "/bar/", true)
		assert.Contains(t, string(body), `<link rel="stylesheet" type="text/css" href="//cozywithapps.example.net/assets/css/cozy-bar`)
		assert.Contains(t, string(body), `<script src="//cozywithapps.example.net/assets/js/cozy-bar`)
	})

	t.Run("ServeWithAnIntents", func(t *testing.T) {
		intent := &intent.Intent{
			Action: "PICK",
			Type:   "io.cozy.foos",
			Client: "io.cozy.apps/test-app",
		}
		err := intent.Save(testInstance)
		assert.NoError(t, err)
		err = intent.FillServices(testInstance)
		assert.NoError(t, err)
		assert.Len(t, intent.Services, 1)
		err = intent.Save(testInstance)
		assert.NoError(t, err)

		path := strings.Replace(intent.Services[0].Href, "https://mini.cozywithapps.example.net", "", 1)
		res, err := doGet(slug, path, true)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		h := res.Header.Get(echo.HeaderContentSecurityPolicy)
		assert.Contains(t, h, "frame-ancestors 'self' https://test-app.cozywithapps.example.net/;")
	})

	t.Run("FaviconWithContext", func(t *testing.T) {
		context := "foo"

		asset, ok := assets.Get("/favicon.ico", context)
		if ok {
			_ = assets.Remove(asset.Name, asset.Context)
		}
		// Create and insert an asset in foo context
		tmpdir := t.TempDir()
		_, err := os.OpenFile(filepath.Join(tmpdir, "custom_favicon.png"), os.O_RDWR|os.O_CREATE, 0600)
		assert.NoError(t, err)

		assetsOptions := []model.AssetOption{{
			URL:     fmt.Sprintf("file://%s", filepath.Join(tmpdir, "custom_favicon.png")),
			Name:    "/favicon.ico",
			Context: context,
		}}
		err = dynamic.RegisterCustomExternals(assetsOptions, 1)
		assert.NoError(t, err)

		// Test the theme
		assert.NoError(t, lifecycle.Patch(testInstance, &lifecycle.Options{
			ContextName: context,
		}))
		assert.NoError(t, err)

		res, err := doGet(slug, "/foo", true)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		body, _ := io.ReadAll(res.Body)
		expected := `this is index.html. <a lang="en" href="https://cozywithapps.example.net/status/">Status</a>`
		assert.Contains(t, string(body), expected)
		assert.Contains(t, string(body), fmt.Sprintf("/assets/ext/%s/favicon.ico", context))
		assert.NotContains(t, string(body), "/assets/favicon.ico")
	})

	t.Run("SessionCode", func(t *testing.T) {
		// Create the OAuth client for the flagship app
		flagship := oauth.Client{
			RedirectURIs: []string{"cozy://flagship"},
			ClientName:   "flagship-app",
			SoftwareID:   "github.com/cozy/cozy-stack/testing/flagship",
			Flagship:     true,
		}
		assert.Nil(t, flagship.Create(testInstance, oauth.NotPending))

		// Create a maximal permission for it
		token, err := testInstance.MakeJWT(consts.AccessTokenAudience,
			flagship.ClientID, "*", "", time.Now())
		assert.NoError(t, err)

		// Create the session code
		req, err := http.NewRequest("POST", ts.URL+"/auth/session_code", nil)
		assert.NoError(t, err)
		req.Host = testInstance.Domain
		req.Header.Add("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 201, res.StatusCode)
		var payload map[string]string
		assert.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
		code := payload["session_code"]
		assert.NotEmpty(t, code)

		// Load a non-public page
		assert.NoError(t, jar.Reset())
		webview := &http.Client{Jar: jar}
		req, err = http.NewRequest("GET", ts.URL+"/foo/?session_code="+code, nil)
		assert.NoError(t, err)
		req.Host = slug + "." + testInstance.Domain
		res, err = webview.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		body, _ := io.ReadAll(res.Body)
		assert.Contains(t, string(body), "this is index.html")

		// Try again and check that the session code cannot be reused
		assert.NoError(t, jar.Reset())
		webview = &http.Client{Jar: jar, CheckRedirect: noRedirect}
		req, err = http.NewRequest("GET", ts.URL+"/foo/?session_code="+code, nil)
		assert.NoError(t, err)
		req.Host = slug + "." + testInstance.Domain
		res, err = webview.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 302, res.StatusCode)
		assert.Contains(t, res.Header.Get("location"), "/auth/login")
	})

	t.Run("ServeAppsWithJWTNotLogged", func(t *testing.T) {
		config.GetConfig().Subdomains = config.FlatSubdomains
		appHost := "cozywithapps-mini.example.net"

		req, _ := http.NewRequest("GET", ts.URL+"/foo?jwt=abc", nil)
		req.Host = appHost
		c := &http.Client{CheckRedirect: noRedirect}
		res, err := c.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 302, res.StatusCode)
		location, err := url.Parse(res.Header.Get("Location"))
		assert.NoError(t, err)
		assert.Equal(t, "/auth/login", location.Path)

		assert.Equal(t, testInstance.Domain, location.Host)
		assert.NotEmpty(t, location.Query().Get("redirect"))
		assert.Equal(t, "abc", location.Query().Get("jwt"))
	})

	t.Run("OauthAppCantInstallApp", func(t *testing.T) {
		req, _ := http.NewRequest("POST", ts.URL+"/apps/mini-bis?Source=git://github.com/nono/cozy-mini.git", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Host = testInstance.Domain
		res, err := client.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 403, res.StatusCode)
	})

	t.Run("OauthAppCantUpdateApp", func(t *testing.T) {
		req, _ := http.NewRequest("PUT", ts.URL+"/apps/mini", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Host = testInstance.Domain
		res, err := client.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 403, res.StatusCode)
	})

	t.Run("ListApps", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/apps/", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Host = testInstance.Domain
		res, err := client.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)

		var results map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&results)
		assert.NoError(t, err)
		objs := results["data"].([]interface{})
		assert.Len(t, objs, 1)
		data := objs[0].(map[string]interface{})
		id := data["id"].(string)
		assert.NotEmpty(t, id)
		typ := data["type"].(string)
		assert.Equal(t, "io.cozy.apps", typ)

		attrs := data["attributes"].(map[string]interface{})
		name := attrs["name"].(string)
		assert.Equal(t, "Mini", name)
		slug := attrs["slug"].(string)
		assert.Equal(t, "mini", slug)

		links := data["links"].(map[string]interface{})
		self := links["self"].(string)
		assert.Equal(t, "/apps/mini", self)
		related := links["related"].(string)
		assert.Equal(t, "https://cozywithapps-mini.example.net/", related)
		icon := links["icon"].(string)
		assert.Equal(t, "/apps/mini/icon/1.0.0", icon)
	})

	t.Run("IconForApp", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/apps/mini/icon", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Host = testInstance.Domain
		res, err := client.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		body, _ := io.ReadAll(res.Body)
		assert.Equal(t, "<svg>...</svg>", string(body))
	})

	t.Run("DownloadApp", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/apps/mini/download", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Host = testInstance.Domain
		res, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, 200, res.StatusCode)

		mimeType, reader := filetype.FromReader(res.Body)
		require.Equal(t, "application/gzip", mimeType)
		gr, err := gzip.NewReader(reader)
		require.NoError(t, err)
		tr := tar.NewReader(gr)
		indexFound := false
		for {
			header, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			if header.Name == "/index.html" {
				indexFound = true
			}
		}
		require.True(t, indexFound)
	})

	t.Run("DownloadKonnectorVersion", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/konnectors/mini/download/1.0.0", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Host = testInstance.Domain
		res, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, 200, res.StatusCode)

		mimeType, reader := filetype.FromReader(res.Body)
		require.Equal(t, "application/gzip", mimeType)
		gr, err := gzip.NewReader(reader)
		require.NoError(t, err)
		tr := tar.NewReader(gr)
		iconFound := false
		for {
			header, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			if header.Name == "/icon.svg" {
				iconFound = true
			}
		}
		require.True(t, iconFound)
	})

	t.Run("OpenWebapp", func(t *testing.T) {
		// Create the OAuth client for the flagship app
		flagship := oauth.Client{
			RedirectURIs: []string{"cozy://flagship"},
			ClientName:   "flagship-app",
			SoftwareID:   "github.com/cozy/cozy-stack/testing/flagship",
			Flagship:     true,
		}
		require.Nil(t, flagship.Create(testInstance, oauth.NotPending))

		// Create a maximal permission for it
		token, err := testInstance.MakeJWT(consts.AccessTokenAudience,
			flagship.ClientID, "*", "", time.Now())
		require.NoError(t, err)

		req, err := http.NewRequest("GET", ts.URL+"/apps/mini/open", nil)
		require.NoError(t, err)
		req.Host = testInstance.Domain
		req.Header.Add("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, 200, res.StatusCode)

		var results map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&results)
		assert.NoError(t, err)
		data := results["data"].(map[string]interface{})
		id := data["id"].(string)
		assert.NotEmpty(t, id)
		typ := data["type"].(string)
		assert.Equal(t, "io.cozy.apps.open", typ)

		attrs := data["attributes"].(map[string]interface{})
		name := attrs["AppName"].(string)
		assert.Equal(t, "Mini", name)
		slug := attrs["AppSlug"].(string)
		assert.Equal(t, "mini", slug)
		icon := attrs["IconPath"].(string)
		assert.Equal(t, "icon.svg", icon)
		tracking := attrs["Tracking"].(string)
		assert.Equal(t, "false", tracking)
		subdomain := attrs["SubDomain"].(string)
		assert.Equal(t, "flat", subdomain)
		cookie := attrs["Cookie"].(string)
		assert.Contains(t, cookie, "HttpOnly")
		appToken := attrs["Token"].(string)
		assert.NotEmpty(t, appToken)
		flags := attrs["Flags"].(string)
		assert.Equal(t, "{}", flags)

		links := data["links"].(map[string]interface{})
		self := links["self"].(string)
		assert.Equal(t, "/apps/mini/open", self)
	})

	t.Run("UninstallAppWithLinkedClient", func(t *testing.T) {
		// Install drive app
		installer, err := apps.NewInstaller(testInstance, apps.Copier(consts.WebappType, testInstance),
			&apps.InstallerOptions{
				Operation:  apps.Install,
				Type:       consts.WebappType,
				Slug:       "drive",
				SourceURL:  "registry://drive",
				Registries: testInstance.Registries(),
			},
		)
		assert.NoError(t, err)
		_, err = installer.RunSync()
		assert.NoError(t, err)

		// Link an OAuthClient to drive
		oauthClient := &oauth.Client{
			ClientName:   "test-linked",
			RedirectURIs: []string{"https://foobar"},
			SoftwareID:   "registry://drive",
		}

		oauthClient.Create(testInstance)
		// Forcing the oauthClient to get a couchID for the purpose of later deletion
		oauthClient, err = oauth.FindClient(testInstance, oauthClient.ClientID)
		assert.NoError(t, err)

		scope := "io.cozy.apps:ALL"
		token, err := testInstance.MakeJWT("cli", "drive", scope, "", time.Now())
		assert.NoError(t, err)

		// Trying to remove this app
		req, _ := http.NewRequest("DELETE", ts.URL+"/apps/drive", nil)
		req.Host = testInstance.Domain

		req.Header.Add("Authorization", "Bearer "+token)
		res, err := client.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 400, res.StatusCode)
		body, err := io.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Contains(t, string(body), "linked OAuth client exists")

		// Cleaning
		uninstaller, err := apps.NewInstaller(testInstance, apps.Copier(consts.WebappType, testInstance),
			&apps.InstallerOptions{
				Operation:  apps.Delete,
				Type:       consts.WebappType,
				Slug:       "drive",
				SourceURL:  "registry://drive",
				Registries: testInstance.Registries(),
			},
		)
		assert.NoError(t, err)
		_, err = uninstaller.RunSync()
		assert.NoError(t, err)
		errc := oauthClient.Delete(testInstance)
		assert.Nil(t, errc)
	})

	t.Run("UninstallAppWithoutLinkedClient", func(t *testing.T) {
		// Install drive app
		installer, err := apps.NewInstaller(testInstance, apps.Copier(consts.WebappType, testInstance),
			&apps.InstallerOptions{
				Operation:  apps.Install,
				Type:       consts.WebappType,
				Slug:       "drive",
				SourceURL:  "registry://drive",
				Registries: testInstance.Registries(),
			},
		)
		assert.NoError(t, err)
		_, err = installer.RunSync()
		assert.NoError(t, err)

		// Create an OAuth client not linked to drive
		oauthClient := &oauth.Client{
			ClientName:   "test-linked",
			RedirectURIs: []string{"https://foobar"},
			SoftwareID:   "foobarclient",
		}
		oauthClient.Create(testInstance)
		// Forcing the oauthClient to get a couchID for the purpose of later deletion
		oauthClient, err = oauth.FindClient(testInstance, oauthClient.ClientID)
		assert.NoError(t, err)

		scope := "io.cozy.apps:ALL"
		token, err := testInstance.MakeJWT("cli", "drive", scope, "", time.Now())
		assert.NoError(t, err)

		// Trying to remove this app
		req, _ := http.NewRequest("DELETE", ts.URL+"/apps/drive", nil)
		req.Host = testInstance.Domain

		req.Header.Add("Authorization", "Bearer "+token)
		res, err := client.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)

		// Cleaning
		errc := oauthClient.Delete(testInstance)
		assert.Nil(t, errc)
	})

	t.Run("SendKonnectorLogsFromFlagshipApp", func(t *testing.T) {
		initialOutput := logrus.New().Out
		defer logrus.SetOutput(initialOutput)

		testOutput := new(bytes.Buffer)
		logrus.SetOutput(testOutput)

		// Create the OAuth client for the flagship app
		flagship := oauth.Client{
			RedirectURIs: []string{"cozy://flagship"},
			ClientName:   "flagship-app",
			SoftwareID:   "github.com/cozy/cozy-stack/testing/flagship",
			Flagship:     true,
		}
		require.Nil(t, flagship.Create(testInstance, oauth.NotPending))

		// Give it the maximal permission
		token, err := testInstance.MakeJWT(consts.AccessTokenAudience,
			flagship.ClientID, "*", "", time.Now())
		require.NoError(t, err)

		// Send logs for a konnector
		konnectorLogs := `[
		{ "timestamp": "2022-10-27T17:13:38.382Z", "level": "error", "msg": "This is an error message" }
	]`
		req, _ := http.NewRequest("POST", ts.URL+"/konnectors/"+slug+"/logs", bytes.NewBufferString(konnectorLogs))
		req.Host = testInstance.Domain
		req.Header.Add("Authorization", "Bearer "+token)

		res, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, 204, res.StatusCode)

		assert.Equal(t, `time="2022-10-27T17:13:38Z" level=error msg="This is an error message" domain=`+domain+" nspace=konnectors slug="+slug+"\n", testOutput.String())

		// Send logs for a webapp
		testOutput.Reset()
		appLogs := `[
		{ "timestamp": "2022-10-27T17:13:38.382Z", "level": "error", "msg": "This is an error message" }
	]`
		req, _ = http.NewRequest("POST", ts.URL+"/apps/"+slug+"/logs", bytes.NewBufferString(appLogs))
		req.Host = testInstance.Domain
		req.Header.Add("Authorization", "Bearer "+token)

		res, err = client.Do(req)
		require.NoError(t, err)
		require.Equal(t, 204, res.StatusCode)

		assert.Equal(t, `time="2022-10-27T17:13:38Z" level=error msg="This is an error message" domain=`+domain+" nspace=apps slug="+slug+"\n", testOutput.String())
	})

	t.Run("SendKonnectorLogsFromKonnector", func(t *testing.T) {
		initialOutput := logrus.New().Out
		defer logrus.SetOutput(initialOutput)

		testOutput := new(bytes.Buffer)
		logrus.SetOutput(testOutput)

		token := testInstance.BuildKonnectorToken(slug)

		konnectorLogs := `[
		{ "timestamp": "2022-10-27T17:13:38.382Z", "level": "error", "msg": "This is an error message" }
	]`
		req, _ := http.NewRequest("POST", ts.URL+"/konnectors/"+slug+"/logs", bytes.NewBufferString(konnectorLogs))
		req.Host = testInstance.Domain
		req.Header.Add("Authorization", "Bearer "+token)

		res, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, 204, res.StatusCode)

		assert.Equal(t, `time="2022-10-27T17:13:38Z" level=error msg="This is an error message" domain=`+domain+" nspace=konnectors slug="+slug+"\n", testOutput.String())

		// Sending logs for a webapp should fail
		req, _ = http.NewRequest("POST", ts.URL+"/apps/"+slug+"/logs", bytes.NewBufferString(konnectorLogs))
		req.Host = testInstance.Domain
		req.Header.Add("Authorization", "Bearer "+token)

		res, err = client.Do(req)
		require.NoError(t, err)
		require.Equal(t, 403, res.StatusCode)
	})

	t.Run("SendAppLogsFromWebApp", func(t *testing.T) {
		initialOutput := logrus.New().Out
		defer logrus.SetOutput(initialOutput)

		testOutput := new(bytes.Buffer)
		logrus.SetOutput(testOutput)

		token := testInstance.BuildAppToken(slug, "")

		appLogs := `[
		{ "timestamp": "2022-10-27T17:13:38.382Z", "level": "error", "msg": "This is an error message" }
	]`
		req, _ := http.NewRequest("POST", ts.URL+"/apps/"+slug+"/logs", bytes.NewBufferString(appLogs))
		req.Host = testInstance.Domain
		req.Header.Add("Authorization", "Bearer "+token)

		res, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, 204, res.StatusCode)

		assert.Equal(t, `time="2022-10-27T17:13:38Z" level=error msg="This is an error message" domain=`+domain+" nspace=apps slug="+slug+"\n", testOutput.String())

		// Sending logs for a konnector should fail
		req, _ = http.NewRequest("POST", ts.URL+"/konnectors/"+slug+"/logs", bytes.NewBufferString(appLogs))
		req.Host = testInstance.Domain
		req.Header.Add("Authorization", "Bearer "+token)

		res, err = client.Do(req)
		require.NoError(t, err)
		require.Equal(t, 403, res.StatusCode)
	})
}

func doGet(slug, path string, auth bool) (*http.Response, error) {
	c := client
	if !auth {
		c = &http.Client{CheckRedirect: noRedirect}
	}
	req, err := http.NewRequest("GET", ts.URL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Host = slug + "." + testInstance.Domain
	return c.Do(req)
}

func doGetAll(t *testing.T, slug, path string, auth bool) []byte {
	res, err := doGet(slug, path, auth)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	assert.NoError(t, err)
	return body
}

func assertGet(t *testing.T, contentType, content string, res *http.Response) {
	assert.Equal(t, 200, res.StatusCode)
	actual := strings.ToLower(res.Header.Get("Content-Type"))
	assert.Equal(t, contentType, actual)
	body, _ := io.ReadAll(res.Body)
	assert.Contains(t, string(body), content)
}

func assertAuthGet(t *testing.T, slug, path, contentType, content string) {
	res, err := doGet(slug, path, true)
	assert.NoError(t, err)
	assertGet(t, contentType, content, res)
}

func assertAnonGet(t *testing.T, slug, path, contentType, content string) {
	res, err := doGet(slug, path, false)
	assert.NoError(t, err)
	assertGet(t, contentType, content, res)
}

func assertNotPublic(t *testing.T, slug, path string, code int, location string) {
	res, err := doGet(slug, path, false)
	assert.NoError(t, err)
	assert.Equal(t, code, res.StatusCode)
	if 300 <= code && code < 400 {
		assert.Equal(t, location, res.Header.Get("location"))
	}
}

func assertNotFound(t *testing.T, slug, path string) {
	res, err := doGet(slug, path, true)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func assertInternalServerError(t *testing.T, slug, path string) {
	res, err := doGet(slug, path, true)
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}
