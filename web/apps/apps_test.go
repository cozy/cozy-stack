package apps

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"github.com/cozy/cozy-stack/apps"
	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

const domain = "cozy-with-apps.example.net"
const slug = "mini"

var ts *httptest.Server
var testInstance *instance.Instance

func injectInstanceAndSlug(i *instance.Instance, s string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("instance", i)
		c.Set("app_slug", s)
	}
}

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
	man := &apps.Manifest{
		Slug:   slug,
		Source: "git://github.com/cozy/mini.git",
		State:  apps.Ready,
	}

	err := couchdb.CreateNamedDoc(testInstance, man)
	if err != nil {
		return err
	}

	appdir := path.Join(apps.AppsDirectory, slug)
	err = vfs.MkdirAll(testInstance, appdir)
	if err != nil {
		return err
	}
	pubdir := path.Join(appdir, "public")
	err = vfs.Mkdir(testInstance, pubdir)
	if err != nil {
		return err
	}

	err = createFile(appdir, "index.html", "this is index.html")
	if err != nil {
		return err
	}
	err = createFile(appdir, "hello.html", "world")
	if err != nil {
		return err
	}
	err = createFile(pubdir, "index.html", "this is a file in public/")
	if err != nil {
		return err
	}

	return nil
}

func assertGet(t *testing.T, path, contentType, content string) {
	res, err := http.Get(ts.URL + path)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	assert.Equal(t, contentType, res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, content, string(body))
}

func assertNotFound(t *testing.T, path string) {
	res, err := http.Get(ts.URL + path)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestServe(t *testing.T) {
	assertGet(t, "/", "text/html", "this is index.html")
	assertGet(t, "/index.html", "text/html", "this is index.html")
	assertGet(t, "/hello.html", "text/html", "world")
	assertGet(t, "/public/", "text/html", "this is a file in public/")
	assertNotFound(t, "/404")
	assertNotFound(t, "/public/hello.html")
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		fmt.Println("Could not create temporary directory.")
		os.Exit(1)
	}

	gin.SetMode(gin.TestMode)

	config.GetConfig().Fs.URL = fmt.Sprintf("file://localhost%s", tempdir)

	instance.Destroy(domain)
	testInstance, err = instance.Create(domain, "en", nil)
	if err != nil {
		fmt.Println("Could not create test instance.", err)
		os.Exit(1)
	}

	err = installMiniApp()
	if err != nil {
		fmt.Println("Could not install mini app.", err)
		os.Exit(1)
	}

	router := gin.New()
	router.Use(injectInstanceAndSlug(testInstance, slug))
	router.Use(middlewares.ServeApp(Serve))

	ts = httptest.NewServer(router)

	res := m.Run()
	ts.Close()
	instance.Destroy(domain)
	os.RemoveAll(tempdir)

	os.Exit(res)
}
