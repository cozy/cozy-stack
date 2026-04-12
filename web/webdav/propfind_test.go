package webdav

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedDir creates a directory at the given absolute VFS path (e.g. "/Docs")
// under the instance root. Minimal helper for Phase 1 PROPFIND tests.
func seedDir(t *testing.T, inst *instance.Instance, absPath string) *vfs.DirDoc {
	t.Helper()
	fs := inst.VFS()
	dir, err := vfs.Mkdir(fs, absPath, nil)
	require.NoError(t, err)
	return dir
}

// --- Depth: 0 ------------------------------------------------------------

// TestPropfind_Depth0_Root asserts PROPFIND on the VFS root with Depth: 0
// returns a 207 Multi-Status carrying exactly one D:response that describes
// the root collection.
func TestPropfind_Depth0_Root(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	r := env.E.Request("PROPFIND", "/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "0").
		Expect().
		Status(http.StatusMultiStatus)

	body := r.Body().Raw()
	// Exactly one <D:response> element.
	assert.Equal(t, 1, strings.Count(body, "<D:response>"),
		"Depth:0 on root must return exactly one D:response")
	// Href points at /dav/files/ (with trailing slash — collections).
	assert.Regexp(t, `<D:href>/dav/files/?</D:href>`, body)
	// resourcetype carries D:collection. encoding/xml emits the long form
	// <D:collection></D:collection>, which is semantically identical to
	// the self-closing form per XML 1.0 §3.1 — both are accepted.
	assert.Contains(t, body, "D:collection")
	assert.Contains(t, body, "<D:resourcetype>")
}

// TestPropfind_Depth0_File seeds a file at the VFS root and asserts
// PROPFIND on it with Depth: 0 returns one response with all the file-
// specific live properties in the expected formats.
func TestPropfind_Depth0_File(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "hello.txt", []byte("Hello, WebDAV!"))

	r := env.E.Request("PROPFIND", "/dav/files/hello.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "0").
		Expect().
		Status(http.StatusMultiStatus)

	body := r.Body().Raw()
	assert.Equal(t, 1, strings.Count(body, "<D:response>"),
		"Depth:0 on a file must return exactly one D:response")
	// No collection marker on a plain file.
	assert.NotContains(t, body, "D:collection")
	// Content length matches the seeded byte count.
	assert.Contains(t, body, "<D:getcontentlength>14</D:getcontentlength>")
	// ETag is a double-quoted base64-ish string (buildETag uses base64(md5sum)).
	// encoding/xml escapes the surrounding quotes as &#34; inside element
	// text content — that's valid XML and clients decode entities before
	// comparing ETags.
	etagRE := regexp.MustCompile(`<D:getetag>(&#34;|")[A-Za-z0-9+/=]+(&#34;|")</D:getetag>`)
	assert.Regexp(t, etagRE, body)
	// getlastmodified is RFC 1123 (http.TimeFormat) — day-name, DD Mon YYYY HH:MM:SS GMT.
	lmRE := regexp.MustCompile(`<D:getlastmodified>[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} GMT</D:getlastmodified>`)
	assert.Regexp(t, lmRE, body)
}

// --- Depth: 1 ------------------------------------------------------------

// TestPropfind_Depth1_DirectoryWithChildren asserts PROPFIND with Depth: 1
// on a directory with 3 files returns 4 D:response elements: the dir + 3
// children.
func TestPropfind_Depth1_DirectoryWithChildren(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedDir(t, env.Inst, "/Docs")
	// Children seeded under /Docs via NewFileDoc with the Docs DirID.
	fs := env.Inst.VFS()
	dir, err := fs.DirByPath("/Docs")
	require.NoError(t, err)
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		doc, err := vfs.NewFileDoc(name, dir.DocID, int64(len("x")), nil,
			"text/plain", "text", dir.UpdatedAt, false, false, false, nil)
		require.NoError(t, err)
		f, err := fs.CreateFile(doc, nil)
		require.NoError(t, err)
		_, err = f.Write([]byte("x"))
		require.NoError(t, err)
		require.NoError(t, f.Close())
	}

	r := env.E.Request("PROPFIND", "/dav/files/Docs").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "1").
		Expect().
		Status(http.StatusMultiStatus)

	body := r.Body().Raw()
	assert.Equal(t, 4, strings.Count(body, "<D:response>"),
		"Depth:1 on a dir with 3 children must return 4 D:response elements (self + 3 children)")
	// Each child filename appears in an href.
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		assert.Contains(t, body, name, "child %q missing from Depth:1 response", name)
	}
}

// --- Depth: infinity -----------------------------------------------------

// TestPropfind_DepthInfinity_Returns403 asserts that a Depth:infinity
// request is rejected with 403 Forbidden carrying a
// <D:propfind-finite-depth/> body per RFC 4918 §9.1.
func TestPropfind_DepthInfinity_Returns403(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	r := env.E.Request("PROPFIND", "/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "infinity").
		Expect().
		Status(http.StatusForbidden)

	r.Body().Contains("propfind-finite-depth")
}

// --- 404 path ------------------------------------------------------------

// TestPropfind_NonexistentPath_Returns404 asserts PROPFIND on an unknown
// path returns 404 (not 403 — 403 is reserved for traversal / out-of-scope).
func TestPropfind_NonexistentPath_Returns404(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.Request("PROPFIND", "/dav/files/does-not-exist").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "0").
		Expect().
		Status(http.StatusNotFound)
}

// --- Namespace ----------------------------------------------------------

// TestPropfind_NamespacePrefixInBody asserts the response body uses the
// xmlns:D="DAV:" declaration and the D: prefix on every element — required
// for Windows Mini-Redirector compatibility.
func TestPropfind_NamespacePrefixInBody(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	body := env.E.Request("PROPFIND", "/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "0").
		Expect().
		Status(http.StatusMultiStatus).
		Body().Raw()

	assert.Contains(t, body, `xmlns:D="DAV:"`)
	assert.Contains(t, body, "<D:multistatus")
	assert.Contains(t, body, "<D:response>")
	assert.Contains(t, body, "<D:href>")
	assert.Contains(t, body, "<D:propstat>")
	assert.Contains(t, body, "<D:prop>")
	// No default-namespace form must leak onto children.
	assert.NotContains(t, body, `xmlns="DAV:"`)
}

// --- Malformed XML request bodies (litmus propfind_invalid, propfind_invalid2) -----------

// TestPropfind_MalformedXMLBody_Returns400 asserts that a PROPFIND request
// carrying a non-well-formed XML body returns 400 Bad Request, not 207.
// Reproduces litmus propfind_invalid: "PROPFIND with non-well-formed XML
// request body got 207 response not 400".
func TestPropfind_MalformedXMLBody_Returns400(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	// Non-well-formed XML — unclosed tag
	env.E.Request("PROPFIND", "/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "0").
		WithHeader("Content-Type", "application/xml").
		WithBytes([]byte(`<?xml version="1.0" encoding="utf-8"?><D:propfind xmlns:D="DAV:`)).
		Expect().
		Status(http.StatusBadRequest)
}

// TestPropfind_InvalidNamespaceBody_Returns400 asserts that a PROPFIND request
// carrying an invalid namespace declaration in the body returns 400 Bad Request.
// Reproduces litmus propfind_invalid2: "PROPFIND with invalid namespace
// declaration in body got 207 response not 400".
func TestPropfind_InvalidNamespaceBody_Returns400(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	// Invalid namespace prefix in XML body — litmus sends this specific body
	env.E.Request("PROPFIND", "/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "0").
		WithHeader("Content-Type", "application/xml").
		WithBytes([]byte(`<?xml version="1.0" encoding="utf-8"?><D:propfind xmlns:D="DAV:"><D:prop><D:getetag/></prop></D:propfind>`)).
		Expect().
		Status(http.StatusBadRequest)
}

// TestPropfind_EmptyBody_Returns207 asserts that a PROPFIND with no body
// still returns 207 (empty body = allprop behavior should be preserved).
func TestPropfind_EmptyBody_Returns207(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.Request("PROPFIND", "/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "0").
		Expect().
		Status(http.StatusMultiStatus)
}

// --- Nextcloud route href prefix (litmus propfind_d0 on /remote.php/webdav/) ---

// TestPropfind_NextcloudRoute_HrefPrefix asserts that PROPFIND via the
// /remote.php/webdav/ route returns hrefs prefixed with /remote.php/webdav/,
// not /dav/files/. Reproduces the litmus propfind_d0 WARNING on the Nextcloud
// route: "response href for wrong resource".
//
// The test uses GetTestServerMultipleRoutes to mount both route sets so that
// both /dav/files/ and /remote.php/webdav/ are reachable on the same test server.
func TestPropfind_NextcloudRoute_HrefPrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("webdav integration tests require a cozy test instance")
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

	ts := setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/dav":        Routes,
		"/remote.php": NextcloudRoutes,
	})
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	e := testutils.CreateTestClient(t, ts.URL)

	_ = inst // instance is used implicitly via middleware

	body := e.Request("PROPFIND", "/remote.php/webdav/").
		WithHeader("Authorization", "Bearer "+token).
		WithHeader("Depth", "0").
		Expect().
		Status(http.StatusMultiStatus).
		Body().Raw()

	// href must use the Nextcloud prefix
	assert.Contains(t, body, "/remote.php/webdav/",
		"PROPFIND via /remote.php/webdav/ must produce hrefs with /remote.php/webdav/ prefix")
	assert.NotContains(t, body, "/dav/files/",
		"PROPFIND via /remote.php/webdav/ must NOT produce /dav/files/ hrefs")
}

// --- All 9 live properties ----------------------------------------------

// TestPropfind_AllNineLiveProperties asserts a Depth:0 response on a seeded
// file carries all 9 live property element names in the XML body.
func TestPropfind_AllNineLiveProperties(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "hello.txt", []byte("Hello, WebDAV!"))

	body := env.E.Request("PROPFIND", "/dav/files/hello.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "0").
		Expect().
		Status(http.StatusMultiStatus).
		Body().Raw()

	for _, elt := range []string{
		"D:resourcetype",
		"D:getlastmodified",
		"D:getcontentlength",
		"D:getetag",
		"D:getcontenttype",
		"D:displayname",
		"D:creationdate",
		"D:supportedlock",
		"D:lockdiscovery",
	} {
		assert.Contains(t, body, elt, "response body missing live property %q", elt)
	}
}
