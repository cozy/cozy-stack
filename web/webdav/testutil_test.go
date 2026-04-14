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
// If overrideRoutes is nil, the canonical webdav.Routes is used — this
// became possible once plan 01-06 landed Routes. Tests that need to
// exercise the middleware in isolation (auth_test.go) or mount extra
// routes alongside the real ones can still pass an explicit registrar.
func newWebdavTestEnv(t *testing.T, overrideRoutes func(*echo.Group)) *webdavTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("webdav integration tests require a cozy test instance")
	}
	if overrideRoutes == nil {
		overrideRoutes = Routes
	}
	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	// Prevent the AntivirusTrigger goroutine from being registered in test
	// runs. This closes the FOLLOWUP-01 race between config.UseViper and the
	// AV trigger's long-lived goroutine. See
	// .planning/phases/01-foundation/01-VALIDATION.md Gap 1.
	t.Setenv("COZY_DISABLE_AV_TRIGGER", "1")

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
