// spec package is introduced to avoid circular dependencies since this
// particular test requires to depend on routing directly to expose the API and
// the APP server.
package apps_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/intents"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	webApps "github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
)

const domain = "cozywithapps.example.net"
const slug = "mini"

var ts *httptest.Server
var testInstance *instance.Instance
var token string
var manifest *apps.WebappManifest

var jar http.CookieJar
var client *http.Client

func createFile(dir, filename, content string) error {
	abs := path.Join(dir, filename)
	file, err := vfs.Create(testInstance.VFS(), abs)
	if err != nil {
		return err
	}
	defer file.Close()
	file.Write([]byte(content))
	return nil
}

func installMiniApp() error {
	manifest = &apps.WebappManifest{
		Name:      "Mini",
		Icon:      "icon.svg",
		DocSlug:   slug,
		DocSource: "git://github.com/cozy/mini.git",
		DocState:  apps.Ready,
		Intents: []apps.Intent{
			{
				Action: "PICK",
				Types:  []string{"io.cozy.foos"},
				Href:   "/foo",
			},
		},
		Routes: apps.Routes{
			"/foo": apps.Route{
				Folder: "/",
				Index:  "index.html",
				Public: false,
			},
			"/bar": apps.Route{
				Folder: "/bar",
				Index:  "index.html",
				Public: false,
			},
			"/public": apps.Route{
				Folder: "/public",
				Index:  "index.html",
				Public: true,
			},
		},
	}

	err := couchdb.CreateNamedDoc(testInstance, manifest)
	if err != nil {
		return err
	}

	appdir := path.Join(vfs.WebappsDirName, slug)
	_, err = vfs.MkdirAll(testInstance.VFS(), appdir)
	if err != nil {
		return err
	}
	bardir := path.Join(appdir, "bar")
	_, err = vfs.Mkdir(testInstance.VFS(), bardir, nil)
	if err != nil {
		return err
	}
	pubdir := path.Join(appdir, "public")
	_, err = vfs.Mkdir(testInstance.VFS(), pubdir, nil)
	if err != nil {
		return err
	}

	err = createFile(appdir, "icon.svg", "<svg>...</svg>")
	if err != nil {
		return err
	}
	err = createFile(appdir, "index.html", `this is index.html. <a lang="{{.Locale}}" href="https://{{.Domain}}/status/">Status</a>`)
	if err != nil {
		return err
	}
	err = createFile(bardir, "index.html", "{{.CozyBar}}")
	if err != nil {
		return err
	}
	err = createFile(appdir, "hello.html", "world {{.Token}}")
	if err != nil {
		return err
	}
	err = createFile(pubdir, "index.html", "this is a file in public/")
	return err
}

func doGet(path string, auth bool) (*http.Response, error) {
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

func doGetAll(t *testing.T, path string, auth bool) []byte {
	res, err := doGet(path, auth)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	body, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	return body
}

func assertGet(t *testing.T, contentType, content string, res *http.Response) {
	assert.Equal(t, 200, res.StatusCode)
	assert.Equal(t, contentType, res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, content, string(body))
}

func assertAuthGet(t *testing.T, path, contentType, content string) {
	res, err := doGet(path, true)
	assert.NoError(t, err)
	assertGet(t, contentType, content, res)
}

func assertAnonGet(t *testing.T, path, contentType, content string) {
	res, err := doGet(path, false)
	assert.NoError(t, err)
	assertGet(t, contentType, content, res)
}

func assertNotPublic(t *testing.T, path string, code int, location string) {
	res, err := doGet(path, false)
	assert.NoError(t, err)
	assert.Equal(t, code, res.StatusCode)
	if 300 <= code && code < 400 {
		assert.Equal(t, location, res.Header.Get("location"))
	}
}

func assertNotFound(t *testing.T, path string) {
	res, err := doGet(path, true)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestServe(t *testing.T) {
	assertAuthGet(t, "/foo/", "text/html; charset=utf-8", `this is index.html. <a lang="en" href="https://cozywithapps.example.net/status/">Status</a>`)
	assertAuthGet(t, "/foo/hello.html", "text/html; charset=utf-8", "world {{.Token}}")
	assertAuthGet(t, "/public", "text/html; charset=utf-8", "this is a file in public/")
	assertAuthGet(t, "/public/index.html", "text/html; charset=utf-8", "this is a file in public/")
	assertAnonGet(t, "/public", "text/html; charset=utf-8", "this is a file in public/")
	assertAnonGet(t, "/public/index.html", "text/html; charset=utf-8", "this is a file in public/")
	assertNotPublic(t, "/foo", 302, "https://cozywithapps.example.net/auth/login?redirect=https%3A%2F%2Fmini.cozywithapps.example.net%2Ffoo")
	assertNotPublic(t, "/foo/hello.tml", 401, "")
	assertNotFound(t, "/404")
	assertNotFound(t, "/")
	assertNotFound(t, "/index.html")
	assertNotFound(t, "/public/hello.html")
}

func TestCozyBar(t *testing.T) {
	body := doGetAll(t, "/bar/", true)
	assert.Contains(t, string(body), `<link rel="stylesheet" type="text/css" href="//cozywithapps.example.net/assets/css/cozy-bar`)
	assert.Contains(t, string(body), `<script defer src="//cozywithapps.example.net/assets/js/cozy-bar`)
}

func TestServeWithAnIntents(t *testing.T) {
	intent := &intents.Intent{
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
	res, err := doGet(path, true)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	h := res.Header.Get(echo.HeaderXFrameOptions)
	assert.Equal(t, "ALLOW-FROM https://test-app.cozywithapps.example.net/", h)
}

func TestServeAppsWithACode(t *testing.T) {
	config.GetConfig().Subdomains = config.FlatSubdomains
	appHost := "cozywithapps-mini.example.net"

	appURL, _ := url.Parse("https://" + appHost + "/")
	j, _ := cookiejar.New(nil)
	ja := &testutils.CookieJar{Jar: j, URL: appURL}
	c := &http.Client{Jar: ja, CheckRedirect: noRedirect}

	req, _ := http.NewRequest("GET", ts.URL+"/foo", nil)
	req.Host = appHost
	res, err := c.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 302, res.StatusCode)
	location, err := url.Parse(res.Header.Get("Location"))
	assert.NoError(t, err)
	assert.Equal(t, testInstance.Domain, location.Host)
	assert.Equal(t, "/auth/login", location.Path)
	assert.NotEmpty(t, location.Query().Get("redirect"))

	longRunSession := true
	session, _ := sessions.New(testInstance, longRunSession)
	code := sessions.BuildCode(session.ID(), appHost)

	req, _ = http.NewRequest("GET", ts.URL+"/foo?code="+code.Value, nil)
	req.Host = appHost
	res, err = c.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 302, res.StatusCode)
	location, err = url.Parse(res.Header.Get("Location"))
	assert.NoError(t, err)
	assert.Equal(t, appHost, location.Host)
	assert.Equal(t, "/foo", location.Path)
	assert.Empty(t, location.Query().Get("redirect"))
	assert.Empty(t, location.Query().Get("code"))
	cookies := res.Cookies()
	assert.Len(t, cookies, 1)
	assert.Equal(t, cookies[0].Name, sessions.SessionCookieName)
	assert.NotEmpty(t, cookies[0].Value)

	req, _ = http.NewRequest("GET", ts.URL+"/foo", nil)
	req.Host = appHost
	res, err = c.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	body, _ := ioutil.ReadAll(res.Body)
	expected := `this is index.html. <a lang="en" href="https://cozywithapps.example.net/status/">Status</a>`
	assert.Equal(t, expected, string(body))
}

func TestOauthAppCantInstallApp(t *testing.T) {
	req, _ := http.NewRequest("POST", ts.URL+"/apps/mini-bis?Source=git://github.com/nono/cozy-mini.git", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Host = testInstance.Domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, res.StatusCode)
}

func TestOauthAppCantUpdateApp(t *testing.T) {
	req, _ := http.NewRequest("PUT", ts.URL+"/apps/mini", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Host = testInstance.Domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, res.StatusCode)
}

func TestListApps(t *testing.T) {
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
	assert.Equal(t, "/apps/mini/icon", icon)
}

func TestIconForApp(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/apps/mini/icon", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Host = testInstance.Domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	body, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, "<svg>...</svg>", string(body))
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "apps_test")
	tempdir := setup.GetTmpDirectory()

	cfg := config.GetConfig()
	cfg.Fs.URL = &url.URL{
		Scheme: "file",
		Host:   "localhost",
		Path:   tempdir,
	}
	was := cfg.Subdomains
	cfg.Subdomains = config.NestedSubdomains
	defer func() { cfg.Subdomains = was }()

	testInstance = setup.GetTestInstance(&instance.Options{Domain: domain})
	pass := "aephe2Ei"
	hash, _ := crypto.GenerateFromPassphrase([]byte(pass))
	testInstance.PassphraseHash = hash
	testInstance.RegisterToken = nil
	testInstance.OnboardingFinished = true
	couchdb.UpdateDoc(couchdb.GlobalDB, testInstance)

	err := installMiniApp()
	if err != nil {
		setup.CleanupAndDie("Could not install mini app.", err)
	}

	ts = setup.GetTestServer("/apps", webApps.WebappsRoutes, func(r *echo.Echo) *echo.Echo {
		r.POST("/login", func(c echo.Context) error {
			longRunSession := true
			session, _ := sessions.New(testInstance, longRunSession)
			cookie, _ := session.ToCookie()
			c.SetCookie(cookie)
			return c.HTML(http.StatusOK, "OK")
		})
		router, err := web.CreateSubdomainProxy(r, webApps.Serve)
		if err != nil {
			setup.CleanupAndDie("Cant start subdoman proxy", err)
		}
		return router
	})

	jar = setup.GetCookieJar()
	client = &http.Client{Jar: jar}

	// Login
	req, _ := http.NewRequest("POST", ts.URL+"/login", bytes.NewBufferString("passphrase="+pass))
	req.Host = testInstance.Domain
	client.Do(req)

	_, token = setup.GetTestClient(consts.Apps + " io.cozy.registry.webapps " + consts.Versions)

	os.Exit(setup.Run())
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}
