package apps

import (
	"bytes"
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
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

const domain = "cozy-with-apps.example.net"
const slug = "mini"

var ts *httptest.Server
var testInstance *instance.Instance
var manifest *apps.Manifest
var instanceURL *url.URL

// Stupid http.CookieJar which always returns all cookies.
// NOTE golang stdlib uses cookies for the URL (ie the testserver),
// not for the host (ie the instance), so we do it manually
type testJar struct {
	Jar *cookiejar.Jar
}

func (j *testJar) Cookies(u *url.URL) (cookies []*http.Cookie) {
	return j.Jar.Cookies(instanceURL)
}

func (j *testJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.Jar.SetCookies(instanceURL, cookies)
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
		Slug:   slug,
		Source: "git://github.com/cozy/mini.git",
		State:  apps.Ready,
		Contexts: apps.Contexts{
			"/foo": apps.Context{
				Folder: "/",
				Index:  "index.html",
				Public: false,
			},
			"/public": apps.Context{
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
	pubdir := path.Join(appdir, "public")
	_, err = vfs.Mkdir(testInstance, pubdir, nil)
	if err != nil {
		return err
	}

	err = createFile(appdir, "index.html", `this is index.html. <a href="https://{{.Domain}}/status/">Status</a>`)
	if err != nil {
		return err
	}
	err = createFile(appdir, "hello.html", "world {{.CtxToken}}")
	if err != nil {
		return err
	}
	err = createFile(pubdir, "index.html", "this is a file in public/")
	return err
}

func doGet(path string, auth bool) (*http.Response, error) {
	c := http.DefaultClient
	if auth {
		c = client
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

func assertNotPublic(t *testing.T, path string) {
	res, err := doGet(path, false)
	assert.NoError(t, err)
	assert.Equal(t, 401, res.StatusCode)
}

func assertNotFound(t *testing.T, path string) {
	res, err := doGet(path, true)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestServe(t *testing.T) {
	assertAuthGet(t, "/foo/", "text/html", `this is index.html. <a href="https://cozy-with-apps.example.net/status/">Status</a>`)
	assertAuthGet(t, "/foo/hello.html", "text/html", "world {{.CtxToken}}")
	assertAuthGet(t, "/public", "text/html", "this is a file in public/")
	assertAuthGet(t, "/public/index.html", "text/html", "this is a file in public/")
	assertAnonGet(t, "/public", "text/html", "this is a file in public/")
	assertAnonGet(t, "/public/index.html", "text/html", "this is a file in public/")
	assertNotPublic(t, "/foo")
	assertNotPublic(t, "/foo/hello.tml")
	assertNotFound(t, "/404")
	assertNotFound(t, "/")
	assertNotFound(t, "/index.html")
	assertNotFound(t, "/public/hello.html")
}

func TestBuildCtxToken(t *testing.T) {
	ctx := manifest.Contexts["/public"]
	tokenString := buildCtxToken(testInstance, manifest, ctx)
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		_, ok := token.Method.(*jwt.SigningMethodHMAC)
		assert.True(t, ok, "The signing method should be HMAC")
		return testInstance.SessionSecret, nil
	})
	assert.NoError(t, err)
	assert.True(t, token.Valid)

	claims, ok := token.Claims.(jwt.MapClaims)
	assert.True(t, ok, "Claims can be parsed as standard claims")
	assert.Equal(t, "context", claims["aud"])
	assert.Equal(t, "https://mini.cozy-with-apps.example.net/", claims["iss"])
	assert.Equal(t, "public", claims["sub"])
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		fmt.Println("Could not create temporary directory.")
		os.Exit(1)
	}

	config.GetConfig().Fs.URL = fmt.Sprintf("file://localhost%s", tempdir)

	instance.Destroy(domain)
	testInstance, err = instance.Create(domain, "en", nil)
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
	router, err := web.Create(r, Serve)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ts = httptest.NewServer(router)

	instanceURL, _ = url.Parse("https://" + domain + "/")
	j, _ := cookiejar.New(nil)
	jar = &testJar{Jar: j}
	client = &http.Client{Jar: jar}

	// Login
	req, _ := http.NewRequest("POST", ts.URL+"/login", bytes.NewBufferString("passphrase="+pass))
	req.Host = domain
	client.Do(req)

	res := m.Run()
	ts.Close()
	instance.Destroy(domain)
	os.RemoveAll(tempdir)

	os.Exit(res)
}
