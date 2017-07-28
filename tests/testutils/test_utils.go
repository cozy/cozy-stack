package testutils

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/echo"
)

// Fatal prints a message and immediately exit the process
func Fatal(msg ...interface{}) {
	fmt.Println(msg...)
	os.Exit(1)
}

// NeedCouchdb kill the process if there is no couchdb running
func NeedCouchdb() {
	db, err := checkup.HTTPChecker{URL: config.CouchURL().String()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		Fatal("This test need couchdb to run.")
	}
}

// TestSetup is a wrapper around a testing.M which handles
// setting up instance, client, VFSContext, testserver
// and cleaning up after itself
type TestSetup struct {
	testingM *testing.M
	name     string
	host     string
	inst     *instance.Instance
	ts       *httptest.Server
	cleanup  func()
}

// NewSetup returns a new TestSetup
// name is used to prevent bug when tests are run in parallel
func NewSetup(testingM *testing.M, name string) *TestSetup {
	setup := TestSetup{
		name:     name,
		testingM: testingM,
		host:     name + "_" + utils.RandomString(10) + ".cozy.local",
		cleanup:  func() {},
	}

	return &setup
}

// CleanupAndDie cleanup the TestSetup, prints a message and 	close the process
func (c *TestSetup) CleanupAndDie(msg ...interface{}) {
	c.cleanup()
	Fatal(msg...)
}

// Cleanup cleanup the TestSetup
func (c *TestSetup) Cleanup() {
	c.cleanup()
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
	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		c.CleanupAndDie("Could not create temporary directory.", err)
	}
	c.AddCleanup(func() error { return os.RemoveAll(tempdir) })
	return tempdir
}

// GetTestInstance creates an instance with a random host
// The instance will be removed on container cleanup
func (c *TestSetup) GetTestInstance(opts ...*instance.Options) *instance.Instance {
	if c.inst != nil {
		return c.inst
	}
	_, err := stack.Start()
	if err != nil {
		c.CleanupAndDie("Error while starting job system", err)
	}
	if len(opts) == 0 {
		opts = []*instance.Options{{}}
	}
	if opts[0].Domain == "" {
		opts[0].Domain = c.host
	} else {
		c.host = opts[0].Domain
	}
	err = instance.Destroy(c.host)
	if err != nil && err != instance.ErrNotFound {
		c.CleanupAndDie("Error while destroying instance", err)
	}
	i, err := instance.Create(opts[0])

	if err != nil {
		c.CleanupAndDie("Cannot create test instance", err)
	}
	c.AddCleanup(func() error { err := instance.Destroy(i.Domain); return err })
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
	client.Create(inst)
	token, err := c.inst.MakeJWT(permissions.AccessTokenAudience,
		client.ClientID, scopes, time.Now())

	if err != nil {
		c.CleanupAndDie("Cannot create oauth token", err)
	}

	return &client, token
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
	handler.HTTPErrorHandler = errors.ErrorHandler
	ts := httptest.NewServer(handler)
	c.AddCleanup(func() error { ts.Close(); return nil })
	c.ts = ts
	return ts
}

// Run runs the underlying testing.M and cleanup
func (c *TestSetup) Run() int {
	value := c.testingM.Run()
	c.cleanup()
	return value
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

// GetCookieJar returns a cookie jar valable for test
// the jar discard the url passed to Cookies and SetCookies and always use
// the setup instance URL instead.
func (c *TestSetup) GetCookieJar() http.CookieJar {
	instance := c.GetTestInstance()
	instanceURL, err := url.Parse("https://" + instance.Domain + "/")
	if err != nil {
		c.CleanupAndDie("Cant create cookie jar url", err)
	}
	j, err := cookiejar.New(nil)
	if err != nil {
		c.CleanupAndDie("Cant create cookie jar", err)
	}
	return &CookieJar{
		Jar: j,
		URL: instanceURL,
	}
}

// func resetDBAndViews(db couchdb.Database, doctype string) error {
// 	if err := couchdb.ResetDB(db, doctype); err != nil {
// 		return err
// 	}
//
// 	if err := couchdb.DefineIndexes(db, consts.IndexesByDoctype(doctype)); err != nil {
// 		return err
// 	}
//
// 	if err := couchdb.DefineViews(db, consts.ViewsByDoctype(doctype)); err != nil {
// 		return err
// 	}
//
// 	return nil
// }
//
// func (c *TestSetup) GetCleanDB(db couchdb.Database, doctype string) {
// 	err := resetDBAndViews()
// 	if err != nil {
// 		c.CleanupAndDie(err)
// 	}
// 	c.addCleanup(func() { couchdb.DeleteDB(db, doctype) })
//
// }
