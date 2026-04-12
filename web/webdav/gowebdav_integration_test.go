package webdav

// End-to-end integration test driving the full WebDAV read-only surface
// via a real github.com/studio-b12/gowebdav client + raw HTTP requests for
// the parts of Phase 1's success criteria that gowebdav's high-level API
// does not directly expose (401 WWW-Authenticate, OPTIONS without auth,
// Depth:infinity rejection, 308 Nextcloud redirect).
//
// The five subtests map 1:1 to the five Phase 1 success criteria listed
// in .planning/ROADMAP.md. This is the authoritative "would a real
// WebDAV client work end-to-end?" test — the final gate before
// /gsd:verify-work.

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/studio-b12/gowebdav"
)

// TestE2E_GowebdavClient exercises the full /dav/files read-only surface
// through a real gowebdav client and asserts every Phase 1 success
// criterion from .planning/ROADMAP.md.
//
// Subtests:
//
//	SuccessCriterion1_BrowseWithBearerToken
//	SuccessCriterion2_AuthRequiredExceptOptions
//	SuccessCriterion3_SecurityGuards
//	SuccessCriterion4_GetFileAndCollection
//	SuccessCriterion5_NextcloudRedirect
func TestE2E_GowebdavClient(t *testing.T) {
	// ---------------------------------------------------------------
	// Success criterion 1: valid Bearer-token client can browse, stat,
	// and read files via the gowebdav library.
	// ---------------------------------------------------------------
	t.Run("SuccessCriterion1_BrowseWithBearerToken", func(t *testing.T) {
		env := newWebdavTestEnv(t, nil)

		// Seed a file at the root and a populated subdirectory.
		seedFile(t, env.Inst, "hello.txt", []byte("Hello, WebDAV!"))
		seedDir(t, env.Inst, "/Docs")

		// gowebdav: token is passed as the Basic-auth password (empty
		// username is the Cozy convention — see plan 01-05).
		c := gowebdav.NewClient(env.TS.URL+"/dav/files", "", env.Token)
		require.NoError(t, c.Connect(), "gowebdav client must connect")

		// ReadDir("/") → PROPFIND Depth:1 under the hood.
		infos, err := c.ReadDir("/")
		require.NoError(t, err)

		names := map[string]bool{}
		for _, info := range infos {
			names[info.Name()] = true
		}
		assert.True(t, names["hello.txt"],
			"ReadDir must surface seeded file hello.txt, got %v", names)
		assert.True(t, names["Docs"],
			"ReadDir must surface seeded directory Docs, got %v", names)

		// Stat → PROPFIND Depth:0 on a single file.
		info, err := c.Stat("/hello.txt")
		require.NoError(t, err)
		assert.Equal(t, int64(len("Hello, WebDAV!")), info.Size(),
			"Stat must return the correct byte size")
		assert.False(t, info.IsDir(), "Stat on a file must report IsDir=false")
		assert.False(t, info.ModTime().IsZero(),
			"Stat must return a non-zero ModTime")

		// Read → GET streams the exact bytes back.
		data, err := c.Read("/hello.txt")
		require.NoError(t, err)
		assert.Equal(t, "Hello, WebDAV!", string(data),
			"Read must return the exact seeded bytes")
	})

	// ---------------------------------------------------------------
	// Success criterion 2: unauthenticated non-OPTIONS → 401 with
	// WWW-Authenticate: Basic realm="Cozy"; OPTIONS bypasses auth and
	// advertises DAV: 1 + full Allow list.
	// ---------------------------------------------------------------
	t.Run("SuccessCriterion2_AuthRequiredExceptOptions", func(t *testing.T) {
		env := newWebdavTestEnv(t, nil)

		// PROPFIND without Authorization → 401 + Basic realm="Cozy".
		env.E.Request("PROPFIND", "/dav/files/").
			Expect().
			Status(http.StatusUnauthorized).
			Header("WWW-Authenticate").IsEqual(`Basic realm="Cozy"`)

		// OPTIONS without Authorization → 200 + DAV: 1 + Allow list.
		r := env.E.OPTIONS("/dav/files/").
			Expect().
			Status(http.StatusOK)
		r.Header("DAV").Contains("1")
		allow := r.Header("Allow").Raw()
		for _, m := range []string{"OPTIONS", "PROPFIND", "GET", "HEAD"} {
			assert.Contains(t, allow, m,
				"Allow header %q missing method %q", allow, m)
		}
	})

	// ---------------------------------------------------------------
	// Success criterion 3: Depth:infinity → 403, path traversal → 403
	// (VFS never reached, via the path_mapper encoded-escape guard).
	// ---------------------------------------------------------------
	t.Run("SuccessCriterion3_SecurityGuards", func(t *testing.T) {
		env := newWebdavTestEnv(t, nil)

		// Depth:infinity on root → 403 propfind-finite-depth.
		env.E.Request("PROPFIND", "/dav/files/").
			WithHeader("Authorization", "Bearer "+env.Token).
			WithHeader("Depth", "infinity").
			Expect().
			Status(http.StatusForbidden).
			Body().Contains("propfind-finite-depth")

		// Percent-encoded traversal (/dav/files/..%2fsettings) — Echo
		// decodes once, leaving a residual %, which the path mapper
		// rejects before touching VFS. 403 forbidden.
		env.E.Request("PROPFIND", "/dav/files/..%252fsettings").
			WithHeader("Authorization", "Bearer "+env.Token).
			WithHeader("Depth", "0").
			Expect().
			Status(http.StatusForbidden)

		// Null-byte smuggling — the path mapper rejects \x00 outright.
		env.E.Request("PROPFIND", "/dav/files/foo%00bar").
			WithHeader("Authorization", "Bearer "+env.Token).
			WithHeader("Depth", "0").
			Expect().
			Status(http.StatusForbidden)
	})

	// ---------------------------------------------------------------
	// Success criterion 4: GET streams with Content-Length + ETag +
	// Last-Modified; Range works; HEAD is headers-only; GET on
	// collection returns 405 with Allow.
	// ---------------------------------------------------------------
	t.Run("SuccessCriterion4_GetFileAndCollection", func(t *testing.T) {
		env := newWebdavTestEnv(t, nil)
		seedFile(t, env.Inst, "range.txt", []byte("Hello, WebDAV!"))

		// GET file → 200 + Content-Length + ETag + Last-Modified.
		resp := env.E.GET("/dav/files/range.txt").
			WithHeader("Authorization", "Bearer "+env.Token).
			Expect().
			Status(http.StatusOK)
		resp.Header("Content-Length").IsEqual("14")
		assert.NotEmpty(t, resp.Header("Etag").Raw(),
			"GET must set Etag header")
		assert.NotEmpty(t, resp.Header("Last-Modified").Raw(),
			"GET must set Last-Modified header")
		resp.Body().IsEqual("Hello, WebDAV!")

		// Range request → 206 Partial Content.
		rr := env.E.GET("/dav/files/range.txt").
			WithHeader("Authorization", "Bearer "+env.Token).
			WithHeader("Range", "bytes=0-4").
			Expect().
			Status(http.StatusPartialContent)
		rr.Header("Content-Range").IsEqual("bytes 0-4/14")
		rr.Body().IsEqual("Hello")

		// HEAD → 200, headers set, empty body.
		hr := env.E.HEAD("/dav/files/range.txt").
			WithHeader("Authorization", "Bearer "+env.Token).
			Expect().
			Status(http.StatusOK)
		hr.Header("Content-Length").IsEqual("14")
		assert.NotEmpty(t, hr.Header("Etag").Raw(),
			"HEAD must set Etag header")
		hr.Body().IsEmpty()

		// GET on collection → 405 Method Not Allowed + Allow header.
		cr := env.E.GET("/dav/files/").
			WithHeader("Authorization", "Bearer "+env.Token).
			Expect().
			Status(http.StatusMethodNotAllowed)
		allow := cr.Header("Allow").Raw()
		for _, m := range []string{"OPTIONS", "PROPFIND", "HEAD"} {
			assert.Contains(t, allow, m,
				"405 Allow header %q missing method %q", allow, m)
		}
	})

	// ---------------------------------------------------------------
	// Success criterion 5: /remote.php/webdav/* serves the same
	// handlers as /dav/files/* directly (no redirect — HTTP clients
	// strip Authorization on redirects, breaking auth).
	// ---------------------------------------------------------------
	t.Run("SuccessCriterion5_NextcloudCompat", func(t *testing.T) {
		env := newWebdavTestEnv(t, nil)
		seedFile(t, env.Inst, "hello.txt", []byte("Hello, WebDAV!"))

		// Register Nextcloud routes with instance middleware.
		e, ok := env.TS.Config.Handler.(*echo.Echo)
		require.True(t, ok, "httptest handler must be *echo.Echo, got %T",
			env.TS.Config.Handler)
		inst := env.Inst
		NextcloudRoutes(e.Group("/remote.php", func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				c.Set("instance", inst)
				return next(c)
			}
		}))

		// GET on /remote.php/webdav/hello.txt should serve the file
		// directly (no redirect) with auth preserved.
		env.E.GET("/remote.php/webdav/hello.txt").
			WithHeader("Authorization", "Bearer "+env.Token).
			Expect().
			Status(http.StatusOK).
			Body().IsEqual("Hello, WebDAV!")

		// PROPFIND on /remote.php/webdav/ should return 207 directly.
		env.E.Request("PROPFIND", "/remote.php/webdav/").
			WithHeader("Authorization", "Bearer "+env.Token).
			WithHeader("Depth", "0").
			Expect().
			Status(http.StatusMultiStatus)
	})

	// ---------------------------------------------------------------
	// Success criterion 6: COPY command via gowebdav client produces a
	// replica visible via a subsequent PROPFIND/Read. Validates wire-level
	// compatibility between the gowebdav client and handleCopy for both
	// the file and directory happy paths.
	// ---------------------------------------------------------------
	t.Run("SuccessCriterion6_Copy", func(t *testing.T) {
		env := newWebdavTestEnv(t, nil)
		client := gowebdav.NewClient(env.TS.URL+"/dav/files", "", env.Token)

		// --- Part A: file COPY ---
		srcContent := []byte("hello copy world")
		if err := client.Write("/srcfile.txt", srcContent, 0644); err != nil {
			t.Fatalf("seed srcfile.txt: %v", err)
		}
		// Copy the file to a new destination with overwrite=true.
		if err := client.Copy("/srcfile.txt", "/copiedfile.txt", true); err != nil {
			t.Fatalf("Copy file: %v", err)
		}
		// Verify the replica exists and has identical content.
		got, err := client.Read("/copiedfile.txt")
		if err != nil {
			t.Fatalf("Read copiedfile.txt: %v", err)
		}
		if !bytes.Equal(got, srcContent) {
			t.Errorf("copied content mismatch: got %q want %q", got, srcContent)
		}
		// Verify the source is UNTOUCHED (copy, not move).
		srcStat, err := client.Stat("/srcfile.txt")
		if err != nil || srcStat == nil {
			t.Errorf("source file disappeared after COPY: %v", err)
		}

		// --- Part B: directory COPY ---
		if err := client.Mkdir("/srcdir", 0755); err != nil {
			t.Fatalf("mkdir srcdir: %v", err)
		}
		if err := client.Write("/srcdir/child.txt", []byte("child content"), 0644); err != nil {
			t.Fatalf("seed srcdir/child.txt: %v", err)
		}
		if err := client.Mkdir("/srcdir/nested", 0755); err != nil {
			t.Fatalf("mkdir srcdir/nested: %v", err)
		}
		if err := client.Write("/srcdir/nested/leaf.txt", []byte("leaf content"), 0644); err != nil {
			t.Fatalf("seed srcdir/nested/leaf.txt: %v", err)
		}
		// Recursive copy (gowebdav Copy on a collection sends Depth:infinity by default).
		if err := client.Copy("/srcdir", "/copieddir", true); err != nil {
			t.Fatalf("Copy dir: %v", err)
		}
		// Verify every file is present in the destination.
		childGot, err := client.Read("/copieddir/child.txt")
		if err != nil || !bytes.Equal(childGot, []byte("child content")) {
			t.Errorf("copied child content mismatch: got %q err %v", childGot, err)
		}
		leafGot, err := client.Read("/copieddir/nested/leaf.txt")
		if err != nil || !bytes.Equal(leafGot, []byte("leaf content")) {
			t.Errorf("copied leaf content mismatch: got %q err %v", leafGot, err)
		}
		// Verify the source directory is untouched.
		srcChildren, err := client.ReadDir("/srcdir")
		if err != nil || len(srcChildren) < 2 {
			t.Errorf("source dir disappeared or changed: err %v len %d", err, len(srcChildren))
		}
	})
}
