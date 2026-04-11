package webdav

import (
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// mountRealRoutes is a passthrough registrar that uses the real Routes
// function. Integration tests in this file need the real router so they
// exercise OPTIONS (unauthenticated) plus the handlePath dispatcher.
func mountRealRoutes(g *echo.Group) {
	Routes(g)
}

// registerNextcloudRoutes wires /remote.php/webdav(/*) onto the same
// Echo instance that httptest is serving. Adds the same instance-injecting
// middleware that GetTestServer uses for /dav (sets "instance" on context).
func registerNextcloudRoutes(t *testing.T, env *webdavTestEnv) {
	t.Helper()
	e, ok := env.TS.Config.Handler.(*echo.Echo)
	if !ok {
		t.Fatalf("httptest handler is not *echo.Echo: %T", env.TS.Config.Handler)
	}
	inst := env.Inst
	NextcloudRoutes(e.Group("/remote.php", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("instance", inst)
			return next(c)
		}
	}))
}

// --- OPTIONS tests -------------------------------------------------------

func TestOptions_FilesRoot_NoAuth(t *testing.T) {
	env := newWebdavTestEnv(t, mountRealRoutes)

	r := env.E.OPTIONS("/dav/files/").
		Expect().
		Status(http.StatusOK)
	r.Header("DAV").Contains("1")
	allow := r.Header("Allow").Raw()
	for _, m := range []string{"OPTIONS", "PROPFIND", "GET", "HEAD", "PUT", "DELETE", "MKCOL", "COPY", "MOVE"} {
		if !strings.Contains(allow, m) {
			t.Errorf("Allow header %q missing method %q", allow, m)
		}
	}
}

func TestOptions_FilesSubpath_NoAuth(t *testing.T) {
	env := newWebdavTestEnv(t, mountRealRoutes)

	r := env.E.OPTIONS("/dav/files/any/deep/path").
		Expect().
		Status(http.StatusOK)
	r.Header("DAV").Contains("1")
	allow := r.Header("Allow").Raw()
	for _, m := range []string{"OPTIONS", "PROPFIND", "GET", "HEAD", "PUT", "DELETE", "MKCOL", "COPY", "MOVE"} {
		if !strings.Contains(allow, m) {
			t.Errorf("Allow header %q missing method %q", allow, m)
		}
	}
}

// TestOptions_DoesNotCallVFS is a sanity check: the OPTIONS handler must
// respond with 200 + DAV headers even for nonsensical paths, without
// reaching the VFS. Authentication is NOT required.
func TestOptions_DoesNotCallVFS(t *testing.T) {
	env := newWebdavTestEnv(t, mountRealRoutes)

	env.E.OPTIONS("/dav/files/this/does/not/exist/and/has/..%2fweird%2fchars").
		Expect().
		Status(http.StatusOK).
		Header("DAV").Contains("1")
}

// --- Nextcloud /remote.php/webdav/* compatibility tests ------------------

func TestNextcloud_OptionsOnWebdavRoot(t *testing.T) {
	env := newWebdavTestEnv(t, mountRealRoutes)
	registerNextcloudRoutes(t, env)

	r := env.E.OPTIONS("/remote.php/webdav/").
		Expect().
		Status(http.StatusOK)
	r.Header("DAV").Contains("1")
	r.Header("Allow").Contains("PROPFIND")
}

func TestNextcloud_PropfindServesDirectly(t *testing.T) {
	env := newWebdavTestEnv(t, mountRealRoutes)
	registerNextcloudRoutes(t, env)

	// PROPFIND on /remote.php/webdav/ should return 207 directly (not 308).
	// Auth required — without token we get 401.
	env.E.Request("PROPFIND", "/remote.php/webdav/").
		WithHeader("Depth", "0").
		Expect().
		Status(http.StatusUnauthorized)
}

func TestNextcloud_AuthenticatedPropfind(t *testing.T) {
	env := newWebdavTestEnv(t, mountRealRoutes)
	registerNextcloudRoutes(t, env)

	// With valid auth, PROPFIND should return 207 Multi-Status directly.
	env.E.Request("PROPFIND", "/remote.php/webdav/").
		WithHeader("Depth", "0").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(http.StatusMultiStatus)
}
