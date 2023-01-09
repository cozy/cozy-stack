package testutils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	apps "github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/stack"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/labstack/echo/v4"
	"github.com/ncw/swift/v2/swifttest"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// This flag avoid starting the stack twice.
var stackStarted bool

// Fatal prints a message and immediately exit the process
func Fatal(msg ...interface{}) {
	fmt.Println(msg...)
	os.Exit(1)
}

// NeedCouchdb kill the process if there is no couchdb running
func NeedCouchdb(t *testing.T) {
	if _, err := couchdb.CheckStatus(); err != nil {
		t.Fatal("This test need couchdb to run.")
	}
}

// TODO can be used as a reminder to do something in the future. The test that
// calls TODO will fail after the limit date, which is an efficient way to not
// forget about it.
func TODO(t *testing.T, date string, args ...interface{}) {
	now := time.Now()
	limit, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Errorf("Invalid date for TODO: %s", err)
	} else if now.After(limit) {
		t.Error(args...)
	}
}

// TestSetup is a wrapper around a testing.M which handles
// setting up instance, client, VFSContext, testserver
// and cleaning up after itself
type TestSetup struct {
	t       testing.TB
	name    string
	host    string
	inst    *instance.Instance
	ts      *httptest.Server
	cleanup func()
}

// NewSetup returns a new TestSetup
// name is used to prevent bug when tests are run in parallel
func NewSetup(t testing.TB, name string) *TestSetup {
	setup := TestSetup{
		name:    name,
		t:       t,
		host:    name + "_" + utils.RandomString(10) + ".cozy.local",
		cleanup: func() {},
	}

	t.Cleanup(setup.cleanup)

	return &setup
}

// SetupSwiftTest can be used to start an in-memory Swift server for tests.
func (c *TestSetup) SetupSwiftTest() error {
	swiftSrv, err := swifttest.NewSwiftServer("localhost")
	if err != nil {
		fmt.Printf("failed to create swift server %s", err)
	}

	viper.Set("swift.username", "swifttest")
	viper.Set("swift.api_key", "swifttest")
	viper.Set("swift.auth_url", swiftSrv.AuthURL)

	swiftURL := &url.URL{
		Scheme:   "swift",
		Host:     "localhost",
		RawQuery: "UserName=swifttest&Password=swifttest&AuthURL=" + url.QueryEscape(swiftSrv.AuthURL),
	}

	err = config.InitSwiftConnection(config.Fs{
		URL: swiftURL,
	})
	require.NoError(c.t, err, "Could not init swift connection.")
	viper.Set("fs.url", swiftURL.String())

	ctx := context.Background()
	err = config.GetSwiftConnection().ContainerCreate(ctx, dynamic.DynamicAssetsContainerName, nil)
	require.NoError(c.t, err, "Could not create dynamic container.")

	return nil
}

// AddCleanup adds a function to be run when the test is finished.
func (c *TestSetup) AddCleanup(f func() error) {
	next := c.cleanup
	c.cleanup = func() {
		err := f()
		if err != nil {
			fmt.Println("Error while cleanup", err)
		}
		next()
	}
}

// GetTmpDirectory creates a temporary directory
// The directory will be removed on container cleanup
func (c *TestSetup) GetTmpDirectory() string {
	tempdir, err := os.MkdirTemp("", "cozy-stack")
	require.NoError(c.t, err, "Could not create temporary directory.")

	c.AddCleanup(func() error { return os.RemoveAll(tempdir) })
	return tempdir
}

// GetTestInstance creates an instance with a random host
// The instance will be removed on container cleanup
func (c *TestSetup) GetTestInstance(opts ...*lifecycle.Options) *instance.Instance {
	if c.inst != nil {
		return c.inst
	}
	var err error
	if !stackStarted {
		_, err = stack.Start(stack.NoGops, stack.NoDynAssets)
		require.NoError(c.t, err, "Error while starting job system")
		stackStarted = true
	}
	if len(opts) == 0 {
		opts = []*lifecycle.Options{{}}
	}
	if opts[0].Domain == "" {
		opts[0].Domain = c.host
	} else {
		c.host = opts[0].Domain
	}
	err = lifecycle.Destroy(c.host)
	require.NoError(c.t, err, "Error while destroying instance")

	i, err := lifecycle.Create(opts[0])
	require.NoError(c.t, err, "Cannot create test instance")

	c.AddCleanup(func() error { err := lifecycle.Destroy(i.Domain); return err })
	c.inst = i
	return i
}

// GetTestClient creates an oauth client and associated token
func (c *TestSetup) GetTestClient(scopes string) (*oauth.Client, string) {
	inst := c.GetTestInstance()
	client := oauth.Client{
		RedirectURIs: []string{"http://localhost/oauth/callback"},
		ClientName:   "client-" + c.host,
		SoftwareID:   "github.com/cozy/cozy-stack/testing/" + c.name,
	}
	client.Create(inst, oauth.NotPending)
	token, err := c.inst.MakeJWT(consts.AccessTokenAudience, client.ClientID, scopes, "", time.Now())
	require.NoError(c.t, err, "Cannot create oauth token")

	return &client, token
}

// stupidRenderer is a renderer for echo that does nothing.
// It is used just to avoid the error "Renderer not registered" for rendering
// error pages.
type stupidRenderer struct{}

func (sr *stupidRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return nil
}

// GetTestServer start a testServer with a single group on prefix
// The server will be closed on container cleanup
func (c *TestSetup) GetTestServer(prefix string, routes func(*echo.Group),
	mws ...func(*echo.Echo) *echo.Echo) *httptest.Server {
	return c.GetTestServerMultipleRoutes(map[string]func(*echo.Group){prefix: routes}, mws...)
}

// GetTestServerMultipleRoutes starts a testServer and creates a group for each
// pair of (prefix, routes) given.
// The server will be closed on container cleanup.
func (c *TestSetup) GetTestServerMultipleRoutes(mpr map[string]func(*echo.Group), mws ...func(*echo.Echo) *echo.Echo) *httptest.Server {
	handler := echo.New()

	for prefix, routes := range mpr {
		group := handler.Group(prefix, func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(context echo.Context) error {
				context.Set("instance", c.inst)
				return next(context)
			}
		})

		routes(group)
	}

	for _, mw := range mws {
		handler = mw(handler)
	}
	handler.Renderer = &stupidRenderer{}
	ts := httptest.NewServer(handler)
	c.AddCleanup(func() error { ts.Close(); return nil })
	c.ts = ts
	return ts
}

// CookieJar is a http.CookieJar which always returns all cookies.
// NOTE golang stdlib uses cookies for the URL (ie the testserver),
// not for the host (ie the instance), so we do it manually
type CookieJar struct {
	Jar *cookiejar.Jar
	URL *url.URL
}

// Cookies implements http.CookieJar interface
func (j *CookieJar) Cookies(u *url.URL) (cookies []*http.Cookie) {
	return j.Jar.Cookies(j.URL)
}

// SetCookies implements http.CookieJar interface
func (j *CookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.Jar.SetCookies(j.URL, cookies)
}

// Reset clears all the cookie
func (j *CookieJar) Reset() error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	j.Jar = jar
	return nil
}

// GetCookieJar returns a cookie jar valable for test
// the jar discard the url passed to Cookies and SetCookies and always use
// the setup instance URL instead.
func (c *TestSetup) GetCookieJar() *CookieJar {
	instance := c.GetTestInstance()
	instanceURL, err := url.Parse("https://" + instance.Domain + "/auth")
	require.NoError(c.t, err, "Cant create cookie jar url")

	j, err := cookiejar.New(nil)
	require.NoError(c.t, err, "Cant create cookie jar url")

	return &CookieJar{
		Jar: j,
		URL: instanceURL,
	}
}

func (c *TestSetup) InstallMiniApp() (string, error) {
	slug := "mini"
	instance := c.GetTestInstance()
	c.AddCleanup(func() error { return permission.DestroyWebapp(instance, slug) })

	permissions := permission.Set{
		permission.Rule{
			Type:  "io.cozy.apps.logs",
			Verbs: permission.Verbs(permission.POST),
		},
	}
	version := "1.0.0"
	manifest := &couchdb.JSONDoc{
		Type: consts.Apps,
		M: map[string]interface{}{
			"_id":    consts.Apps + "/" + slug,
			"name":   "Mini",
			"icon":   "icon.svg",
			"slug":   slug,
			"source": "git://github.com/cozy/mini.git",
			"state":  apps.Ready,
			"intents": []apps.Intent{
				{
					Action: "PICK",
					Types:  []string{"io.cozy.foos"},
					Href:   "/foo",
				},
			},
			"routes": apps.Routes{
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
			"permissions": permissions,
			"version":     version,
		},
	}

	err := couchdb.CreateNamedDoc(instance, manifest)
	if err != nil {
		return "", err
	}

	_, err = permission.CreateWebappSet(instance, slug, permissions, version)
	if err != nil {
		return "", err
	}

	appdir := path.Join(vfs.WebappsDirName, slug, version)
	_, err = vfs.MkdirAll(instance.VFS(), appdir)
	if err != nil {
		return "", err
	}
	bardir := path.Join(appdir, "bar")
	_, err = vfs.Mkdir(instance.VFS(), bardir, nil)
	if err != nil {
		return "", err
	}
	pubdir := path.Join(appdir, "public")
	_, err = vfs.Mkdir(instance.VFS(), pubdir, nil)
	if err != nil {
		return "", err
	}

	err = createFile(instance, appdir, "icon.svg", "<svg>...</svg>")
	if err != nil {
		return "", err
	}
	err = createFile(instance, appdir, "index.html", `this is index.html. <a lang="{{.Locale}}" href="https://{{.Domain}}/status/">Status</a> {{.Favicon}}`)
	if err != nil {
		return "", err
	}
	err = createFile(instance, bardir, "index.html", "{{.CozyBar}}")
	if err != nil {
		return "", err
	}
	err = createFile(instance, appdir, "hello.html", "world {{.Token}}")
	if err != nil {
		return "", err
	}
	err = createFile(instance, pubdir, "index.html", "this is a file in public/")
	return slug, err
}

func (c *TestSetup) InstallMiniKonnector() (string, error) {
	slug := "mini"
	instance := c.GetTestInstance()
	c.AddCleanup(func() error { return permission.DestroyKonnector(instance, slug) })

	permissions := permission.Set{
		permission.Rule{
			Type:  "io.cozy.apps.logs",
			Verbs: permission.Verbs(permission.POST),
		},
	}
	version := "1.0.0"
	manifest := &couchdb.JSONDoc{
		Type: consts.Konnectors,
		M: map[string]interface{}{
			"_id":         consts.Konnectors + "/" + slug,
			"name":        "Mini",
			"icon":        "icon.svg",
			"slug":        slug,
			"source":      "git://github.com/cozy/mini.git",
			"state":       apps.Ready,
			"permissions": permissions,
			"version":     version,
		},
	}

	err := couchdb.CreateNamedDoc(instance, manifest)
	if err != nil {
		return "", err
	}

	_, err = permission.CreateKonnectorSet(instance, slug, permissions, version)
	if err != nil {
		return "", err
	}

	konnDir := path.Join(vfs.KonnectorsDirName, slug, version)
	_, err = vfs.MkdirAll(instance.VFS(), konnDir)
	if err != nil {
		return "", err
	}

	err = createFile(instance, konnDir, "icon.svg", "<svg>...</svg>")
	return slug, err
}

func createFile(instance *instance.Instance, dir, filename, content string) error {
	abs := path.Join(dir, filename+".br")
	file, err := vfs.Create(instance.VFS(), abs)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(compress(content))
	return err
}

func compress(content string) []byte {
	buf := &bytes.Buffer{}
	bw := brotli.NewWriter(buf)
	_, _ = bw.Write([]byte(content))
	_ = bw.Close()
	return buf.Bytes()
}
