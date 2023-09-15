package testutils

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
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
	"github.com/gavv/httpexpect/v2"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"github.com/ncw/swift/v2/swifttest"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// This flag avoid starting the stack twice.
var stackStarted bool
var useDebug bool

func init() {
	flag.BoolVar(&useDebug, "debug", false, "display the requests content")

	if useDebug {
		useDebug = true
	}
}

// CreateTestClient setup an httpexpect.Expect client used to make http tests.
//
// This init take allow to use the `--debug` flag in your tests in order to
// print the requests/responses content.
//
// example: `go test ./web/permissions --debug`.
func CreateTestClient(t testing.TB, url string) *httpexpect.Expect {
	var printer httpexpect.Printer

	t.Helper()

	flag.Parse()

	if useDebug {
		printer = httpexpect.NewDebugPrinter(t, true)
	} else {
		printer = httpexpect.NewCompactPrinter(t)
	}

	return httpexpect.WithConfig(httpexpect.Config{
		TestName: t.Name(),
		BaseURL:  url,
		Reporter: httpexpect.NewAssertReporter(t),
		Printers: []httpexpect.Printer{printer},
	})
}

// NeedCouchdb kill the process if there is no couchdb running
func NeedCouchdb(t *testing.T) {
	if _, err := couchdb.CheckStatus(context.Background()); err != nil {
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
func (c *TestSetup) SetupSwiftTest() {
	swiftSrv, err := swifttest.NewSwiftServer("localhost")
	require.NoError(c.t, err, "failed to create swift server")

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
}

// GetTestInstance creates an instance with a random host
// The instance will be removed on container cleanup
func (c *TestSetup) GetTestInstance(opts ...*lifecycle.Options) *instance.Instance {
	if c.inst != nil {
		return c.inst
	}
	var err error
	if !stackStarted {
		_, _, err = stack.Start(stack.NoGops, stack.NoDynAssets)
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
	if err != nil && !errors.Is(err, instance.ErrNotFound) {
		require.NoError(c.t, err, "Error while destroying instance")
	}

	i, err := lifecycle.Create(opts[0])
	require.NoError(c.t, err, "Cannot create test instance")

	c.t.Cleanup(func() { _ = lifecycle.Destroy(i.Domain) })
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
	c.t.Cleanup(ts.Close)
	c.ts = ts
	return ts
}

func (c *TestSetup) InstallMiniApp() (string, error) {
	slug := "mini"
	instance := c.GetTestInstance()
	c.t.Cleanup(func() { _ = permission.DestroyWebapp(instance, slug) })

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
				"/invalid": apps.Route{
					Folder: "/",
					Index:  "invalid.html",
					Public: false,
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
	err = createFile(instance, appdir, "index.html", `<html><body>this is index.html. <a lang="{{.Locale}}" href="https://{{.Domain}}/status/">Status</a> {{.Favicon}}</body></html>`)
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
	if err != nil {
		return "", err
	}
	err = createFile(instance, appdir, "invalid.html", "this is invalid.html. {{.InvalidHelper}}")
	return slug, err
}

func (c *TestSetup) InstallMiniKonnector() (string, error) {
	slug := "mini"
	instance := c.GetTestInstance()
	c.t.Cleanup(func() { _ = permission.DestroyKonnector(instance, slug) })

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

func WithManager(t *testing.T, inst *instance.Instance) (shouldRemoveUUID bool) {
	if inst.UUID == "" {
		uuid, err := uuid.NewV4()
		require.NoError(t, err, "Could not enable test instance manager")
		inst.UUID = uuid.String()
		shouldRemoveUUID = true
	}

	config, ok := inst.SettingsContext()
	require.True(t, ok, "Could not enable test instance manager: could not fetch test instance settings context")

	managerURL, ok := config["manager_url"].(string)
	require.True(t, ok, "Could not enable test instance manager: manager_url config is required")
	require.NotEmpty(t, managerURL, "Could not enable test instance manager: manager_url config is required")

	was := config["enable_premium_links"]
	config["enable_premium_links"] = true

	t.Cleanup(func() {
		config["enable_premium_links"] = was

		if shouldRemoveUUID {
			inst.UUID = ""
			require.NoError(t, instance.Update(inst))
		}
	})

	err := instance.Update(inst)
	require.NoError(t, err, "Could not enable test instance manager")

	return shouldRemoveUUID
}

func DisableManager(inst *instance.Instance, shouldRemoveUUID bool) error {
	config, ok := inst.SettingsContext()
	if !ok {
		return fmt.Errorf("Could not disable test instance manager: could not fetch test instance settings context")
	}

	config["enable_premium_links"] = false

	if shouldRemoveUUID {
		inst.UUID = ""
		return instance.Update(inst)
	}
	return nil
}

func WithOAuthClientsLimit(t *testing.T, inst *instance.Instance, limit float64) {
	flags := inst.FeatureFlags
	if flags == nil {
		flags = map[string]interface{}{}
	}

	was := flags["cozy.oauthclients.max"]

	flags["cozy.oauthclients.max"] = limit
	inst.FeatureFlags = flags
	err := instance.Update(inst)
	require.NoError(t, err, "Could not set OAuth clients limit")

	t.Cleanup(func() {
		flags["cozy.oauthclients.max"] = was
		inst.FeatureFlags = flags
		require.NoError(t, instance.Update(inst))
	})
}
