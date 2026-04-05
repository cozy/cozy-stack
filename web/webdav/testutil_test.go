package webdav

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
)

// webdavTestEnv bundles the common test fixtures for WebDAV integration tests.
type webdavTestEnv struct {
	Inst  *instance.Instance
	Token string
	TS    *httptest.Server
	E     *httpexpect.Expect
}

// newWebdavTestEnv sets up a fresh test instance, OAuth client with
// io.cozy.files scope, and an httptest server mounted at /dav with the
// routes registered by overrideRoutes.
//
// overrideRoutes MUST be non-nil for now: the package's canonical Routes
// function is implemented in plan 01-06. Until it lands, every test in this
// package must supply its own route registrar (typically: attach
// resolveWebDAVAuth + a trivial 200 handler). Once Routes exists, callers may
// pass nil and the helper can be extended to default to it.
func newWebdavTestEnv(t *testing.T, overrideRoutes func(*echo.Group)) *webdavTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("webdav integration tests require a cozy test instance")
	}
	if overrideRoutes == nil {
		t.Fatal("newWebdavTestEnv: overrideRoutes is required until plan 01-06 introduces webdav.Routes")
	}
	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	setup := testutils.NewSetup(t, t.Name())
	config.GetConfig().Fs.URL = &url.URL{
		Scheme: "file",
		Host:   "localhost",
		Path:   t.TempDir(),
	}

	inst := setup.GetTestInstance()
	_, token := setup.GetTestClient(consts.Files)

	ts := setup.GetTestServer("/dav", overrideRoutes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	e := testutils.CreateTestClient(t, ts.URL)

	return &webdavTestEnv{
		Inst:  inst,
		Token: token,
		TS:    ts,
		E:     e,
	}
}
