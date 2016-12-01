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
	"github.com/cozy/cozy-stack/web"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

const domain = "cozy-with-apps.example.net"
const slug = "mini"

var ts *httptest.Server
var testInstance *instance.Instance

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

	err = createFile(appdir, "index.html", "this is index.html")
	if err != nil {
		return err
	}
	err = createFile(appdir, "hello.html", "world")
	if err != nil {
		return err
	}
	err = createFile(pubdir, "index.html", "this is a file in public/")
	return err
}

func doGet(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", ts.URL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Host = slug + "." + domain
	return http.DefaultClient.Do(req)
}

func assertGet(t *testing.T, path, contentType, content string) {
	res, err := doGet(path)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	assert.Equal(t, contentType, res.Header.Get("Content-Type"))
	body, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, content, string(body))
}

func assertNotFound(t *testing.T, path string) {
	res, err := doGet(path)
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

	router, err := web.Create(&web.Config{
		Domain:    "example.net",
		Router:    echo.New(),
		ServeApps: Serve,
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ts = httptest.NewServer(router)

	res := m.Run()
	ts.Close()
	instance.Destroy(domain)
	os.RemoveAll(tempdir)

	os.Exit(res)
}
