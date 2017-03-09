// spec package is introduced to avoid circular dependencies since this
// particular test requires to depend on routing directly to expose the API and
// the APP server.
package apps_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"testing"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web"
	webApps "github.com/cozy/cozy-stack/web/apps"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

const domain = "cozywithapps.example.net"
const slug = "mini"

var ts *httptest.Server
var testInstance *instance.Instance
var clientID string
var manifest *apps.Manifest

// Stupid http.CookieJar which always returns all cookies.
// NOTE golang stdlib uses cookies for the URL (ie the testserver),
// not for the host (ie the instance), so we do it manually
type testJar struct {
	Jar *cookiejar.Jar
	URL *url.URL
}

func (j *testJar) Cookies(u *url.URL) (cookies []*http.Cookie) {
	return j.Jar.Cookies(j.URL)
}

func (j *testJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.Jar.SetCookies(j.URL, cookies)
}

var jar *testJar
var client *http.Client

func createFile(dir, filename, content string) error {
	abs := path.Join(dir, filename)
	file, err := vfs.Create(testInstance, abs)
	if err != nil {
		return err
	}
	defer file.Close()
	file.Write([]byte(content))
	return nil
}

func installMiniApp() error {
	manifest = &apps.Manifest{
		Name:   "Mini",
		Icon:   "icon.svg",
		Slug:   slug,
		Source: "git://github.com/cozy/mini.git",
		State:  apps.Ready,
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

	appdir := path.Join(vfs.AppsDirName, slug)
	_, err = vfs.MkdirAll(testInstance, appdir, nil)
	if err != nil {
		return err
	}
	bardir := path.Join(appdir, "bar")
	_, err = vfs.Mkdir(testInstance, bardir, nil)
	if err != nil {
		return err
	}
	pubdir := path.Join(appdir, "public")
	_, err = vfs.Mkdir(testInstance, pubdir, nil)
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
	req.Host = slug + "." + domain
	return c.Do(req)
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
	assertAuthGet(t, "/bar/", "text/html; charset=utf-8", ``+
		`<link rel="stylesheet" type="text/css" href="//cozywithapps.example.net/assets/css/cozy-bar.min.css">`+
		`<script defer src="//cozywithapps.example.net/assets/js/cozy-bar.min.js"></script>`)
}

func TestServeAppsWithACode(t *testing.T) {
	config.GetConfig().Subdomains = config.FlatSubdomains
	appHost := "cozywithapps-mini.example.net"

	appURL, _ := url.Parse("https://" + appHost + "/")
	j, _ := cookiejar.New(nil)
	ja := &testJar{Jar: j, URL: appURL}
	c := &http.Client{Jar: ja, CheckRedirect: noRedirect}

	req, _ := http.NewRequest("GET", ts.URL+"/foo", nil)
	req.Host = appHost
	res, err := c.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 302, res.StatusCode)
	location, err := url.Parse(res.Header.Get("Location"))
	assert.NoError(t, err)
	assert.Equal(t, domain, location.Host)
	assert.Equal(t, "/auth/login", location.Path)
	assert.NotEmpty(t, location.Query().Get("redirect"))

	session, _ := sessions.New(testInstance)
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
	req.Header.Add("Authorization", "Bearer "+testToken(testInstance))
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, res.StatusCode)
}

func TestOauthAppCantUpdateApp(t *testing.T) {
	req, _ := http.NewRequest("PUT", ts.URL+"/apps/mini", nil)
	req.Header.Add("Authorization", "Bearer "+testToken(testInstance))
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, res.StatusCode)
}

func TestListApps(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/apps/", nil)
	req.Header.Add("Authorization", "Bearer "+testToken(testInstance))
	req.Host = domain
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
	req.Header.Add("Authorization", "Bearer "+testToken(testInstance))
	req.Host = domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	body, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, "<svg>...</svg>", string(body))
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"

	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		fmt.Println("Could not create temporary directory.")
		os.Exit(1)
	}

	cfg := config.GetConfig()
	cfg.Fs.URL = fmt.Sprintf("file://localhost%s", tempdir)
	was := cfg.Subdomains
	cfg.Subdomains = config.NestedSubdomains
	defer func() { cfg.Subdomains = was }()

	instance.Destroy(domain)
	testInstance, err = instance.Create(&instance.Options{
		Domain: domain,
		Locale: "en",
	})
	if err != nil {
		fmt.Println("Could not create test instance.", err)
		os.Exit(1)
	}
	pass := "aephe2Ei"
	hash, _ := crypto.GenerateFromPassphrase([]byte(pass))
	testInstance.PassphraseHash = hash
	testInstance.RegisterToken = nil
	couchdb.UpdateDoc(couchdb.GlobalDB, testInstance)

	err = installMiniApp()
	if err != nil {
		fmt.Println("Could not install mini app.", err)
		os.Exit(1)
	}

	r := echo.New()
	r.POST("/login", func(c echo.Context) error {
		session, _ := sessions.New(testInstance)
		cookie, _ := session.ToCookie()
		c.SetCookie(cookie)
		return c.HTML(http.StatusOK, "OK")
	})
	webApps.Routes(r.Group("/apps"))
	router, err := web.CreateSubdomainProxy(r, webApps.Serve)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ts = httptest.NewServer(router)

	instanceURL, _ := url.Parse("https://" + domain + "/")
	j, _ := cookiejar.New(nil)
	jar = &testJar{Jar: j, URL: instanceURL}
	client = &http.Client{Jar: jar}

	// Login
	req, _ := http.NewRequest("POST", ts.URL+"/login", bytes.NewBufferString("passphrase="+pass))
	req.Host = domain
	client.Do(req)

	client := oauth.Client{
		RedirectURIs: []string{"http://localhost/oauth/callback"},
		ClientName:   "test-permissions",
		SoftwareID:   "github.com/cozy/cozy-stack/web/data",
	}
	client.Create(testInstance)
	clientID = client.ClientID

	res := m.Run()
	ts.Close()
	instance.Destroy(domain)
	os.RemoveAll(tempdir)

	os.Exit(res)
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func testToken(i *instance.Instance) string {
	t, _ := crypto.NewJWT(testInstance.OAuthSecret, permissions.Claims{
		StandardClaims: jwt.StandardClaims{
			Audience: permissions.AccessTokenAudience,
			Issuer:   testInstance.Domain,
			IssuedAt: crypto.Timestamp(),
			Subject:  clientID,
		},
		Scope: consts.Apps,
	})
	return t
}
