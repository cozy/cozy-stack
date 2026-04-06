package webdav

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
)

// mountRealRoutes is a passthrough registrar that uses the real Routes
// function. Integration tests in this file need the real router so they
// exercise OPTIONS (unauthenticated) plus the handlePath dispatcher.
func mountRealRoutes(g *echo.Group) {
	Routes(g)
}

// registerNextcloudRedirect wires /remote.php/webdav(/*) onto the same
// Echo instance that httptest is serving. newWebdavTestEnv mounts the
// /dav group via setup.GetTestServer; to also test the 308 redirect we
// register the redirect routes directly on the underlying *echo.Echo.
func registerNextcloudRedirect(t *testing.T, env *webdavTestEnv) {
	t.Helper()
	e, ok := env.TS.Config.Handler.(*echo.Echo)
	if !ok {
		t.Fatalf("httptest handler is not *echo.Echo: %T", env.TS.Config.Handler)
	}
	for _, m := range webdavMethods {
		e.Add(m, "/remote.php/webdav", NextcloudRedirect)
		e.Add(m, "/remote.php/webdav/*", NextcloudRedirect)
	}
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

// --- Nextcloud 308 redirect tests ----------------------------------------

func TestNextcloudRedirect_PreservesMethod(t *testing.T) {
	env := newWebdavTestEnv(t, mountRealRoutes)
	registerNextcloudRedirect(t, env)

	// httpexpect follows redirects by default; disable to observe the 308.
	env.E.GET("/remote.php/webdav/Foo/bar.txt").
		WithRedirectPolicy(httpexpect.DontFollowRedirects).
		Expect().
		Status(http.StatusPermanentRedirect).
		Header("Location").IsEqual("/dav/files/Foo/bar.txt")
}

func TestNextcloudRedirect_RootPath(t *testing.T) {
	env := newWebdavTestEnv(t, mountRealRoutes)
	registerNextcloudRedirect(t, env)

	env.E.GET("/remote.php/webdav").
		WithRedirectPolicy(httpexpect.DontFollowRedirects).
		Expect().
		Status(http.StatusPermanentRedirect).
		Header("Location").IsEqual("/dav/files")
}

func TestNextcloudRedirect_PropfindMethod(t *testing.T) {
	env := newWebdavTestEnv(t, mountRealRoutes)
	registerNextcloudRedirect(t, env)

	// 308 (unlike 301/302) preserves the request method — critical for
	// PROPFIND, which would otherwise be downgraded to GET by the client.
	env.E.Request("PROPFIND", "/remote.php/webdav/Foo").
		WithRedirectPolicy(httpexpect.DontFollowRedirects).
		Expect().
		Status(http.StatusPermanentRedirect).
		Header("Location").IsEqual("/dav/files/Foo")
}
