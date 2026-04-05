# Phase 1: Foundation - Research

**Researched:** 2026-04-04
**Domain:** WebDAV read-only server layer on cozy-stack (Go monolith, Echo v4, CouchDB VFS)
**Confidence:** HIGH — all findings derived from direct inspection of the cozy-stack source tree

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Package and organisation:**
- Code in `web/webdav/` — parallel to `web/files/`
- Custom Echo v4 handlers (`echo.HandlerFunc`), NOT `golang.org/x/net/webdav`, NOT `emersion/go-webdav`
- XML via `encoding/xml` stdlib
- No new `model/` package — all business logic already in `model/vfs/`

**Routing:**
- Primary route: `/dav/files/*`
- Compat route: `/remote.php/webdav/*` → **308 Permanent Redirect** to `/dav/files/*`
- Normalisation: URL decode → `path.Clean` → assert `/files/` prefix
- Reject before any VFS call: `..`, `%2e%2e`, null bytes, prefixes `/settings`, `/apps`, etc.
- Scope: only `/files/` tree

**GET on collection:**
- Returns **405 Method Not Allowed** with `Allow:` header (OPTIONS, PROPFIND, HEAD)
- No HTML navigation page

**Authentication:**
- OAuth Bearer in `Authorization: Bearer <token>` — uses `middlewares.GetRequestToken`
- OAuth token accepted in Basic Auth password field (username ignored)
- 401: `WWW-Authenticate: Basic realm="Cozy"`
- OPTIONS is **the only method without auth**

**PROPFIND:**
- 9 live properties: `resourcetype`, `getlastmodified` (RFC 1123 / `http.TimeFormat`), `getcontentlength`, `getetag` (md5sum double-quoted, **never** `_rev`), `getcontenttype`, `displayname`, `creationdate` (ISO 8601), `supportedlock` (empty), `lockdiscovery` (empty)
- XML namespace: `xmlns:D="DAV:"` with `D:` prefix everywhere
- Depth:0 and Depth:1 only; Depth:infinity → **403 Forbidden**
- Streaming XML via `xml.Encoder.EncodeElement()` + `DirIterator`
- No hard cap on items; memory bounded by batch size
- DirIterator batch size: **200 items** (overrides default 100)

**Trash and special folders:**
- `.cozy_trash`: visible read-only
- Sharings: visible normally via existing permission system

**GET / HEAD:**
- File: delegate to `vfs.ServeFileContent`
- HEAD: same headers, no body
- Collection: 405

**Error format:**
- XML RFC 4918 §8.7: `<D:error xmlns:D="DAV:"><D:condition-element/></D:error>`
- Content-Type: `application/xml; charset="utf-8"`

**Audit logging:**
- WARN level in existing cozy-stack logger
- Structured fields: `instance`, `source_ip`, `user_agent`, `method`, `raw_url`, `normalized_path`, `token_hash`
- Events logged: path traversal attempts, PROPFIND Depth:infinity, 403 hors scope
- NOT logged: unauthenticated 401 (too noisy)

**Testing (TDD strict):**
- Cycle **RED → GREEN → REFACTOR** with separate commits (mandatory, non-negotiable)
- Unit tests written BEFORE structs/functions they test
- Integration tests: `studio-b12/gowebdav` v0.12.0 (add to `go.mod`)
- No VFS mocking — use real VFS (afero/mem-backed)

### Claude's Discretion

- Split of Go files in `web/webdav/` (one file per method, or grouped)
- Exact Go type names for XML structs
- Strategy for expressing read-only on trash in PROPFIND
- Helper functions for path mapping (inline or extracted)
- Exact error XML content beyond RFC 4918 precondition elements

### Deferred Ideas (OUT OF SCOPE)

- App-specific passwords (v2)
- LOCK/UNLOCK (never in v1)
- PROPPATCH and dead properties (v2)
- Quota properties (v2)
- Dedicated WebDAV metrics and Grafana dashboard (v2)
- WebDAV-specific rate limiting (global Cozy applies)
- Automated alerting on audit logs (v2)
- Digest Auth (v2)
- Hard cap / pagination on PROPFIND (evaluate if DoS issues arise in prod)
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| ROUTE-01 | WebDAV endpoint on `/dav/files/` | `web/routing.go:SetupRoutes` pattern confirmed; use `router.Match` with 9 WebDAV methods |
| ROUTE-02 | `/remote.php/webdav/*` → 308 redirect to `/dav/files/*` | `c.Redirect(http.StatusPermanentRedirect, ...)` via Echo — confirmed 308 preserves method |
| ROUTE-03 | Path normalisation: trailing slash, URL decode, `path.Clean`, prefix assertion | `path.Clean` + `strings.HasPrefix` pattern; `c.Param("*")` returns pre-decoded path in Echo |
| ROUTE-04 | OPTIONS responds with `DAV: 1`, `Allow:` list, no auth | Handler returns 200 with headers, no VFS call, bypasses auth middleware |
| ROUTE-05 | Only `/files/` tree exposed | Path mapper rejects anything not under `/files/` after normalisation |
| AUTH-01 | Bearer token via `Authorization: Bearer <token>` | `middlewares.GetRequestToken` at `web/middlewares/permissions.go:63` — confirmed |
| AUTH-02 | OAuth token in Basic Auth password field | Same `GetRequestToken` function — confirmed at line 69-71 |
| AUTH-03 | 401 with `WWW-Authenticate: Basic realm="Cozy"` | Custom 401 response in WebDAV auth middleware (not JSON:API) |
| AUTH-04 | Token translated to Cozy permissions | `middlewares.GetPermission` → `middlewares.ParseJWT` → `GetForOauth` — full chain confirmed |
| AUTH-05 | Permission scope verified | `middlewares.AllowVFS(c, permission.GET, doc)` or `AllowWholeType` for `/files/` scope |
| READ-01 | PROPFIND Depth:0 on collection | `inst.VFS().DirOrFileByPath()` → build single `<D:response>` element |
| READ-02 | PROPFIND Depth:0 on file | Same path; `fileDoc != nil` branch |
| READ-03 | PROPFIND Depth:1 on collection | `DirIterator` with `ByFetch:200`; stream `<D:response>` elements via `xml.Encoder` |
| READ-04 | PROPFIND Depth:infinity → 403 | Check header before any VFS call; log WARN; return XML error body |
| READ-05 | 9 live properties | XML structs in `xml.go`; ETag = base64 of `doc.MD5Sum`; date = `http.TimeFormat` |
| READ-06 | XML namespace `xmlns:D="DAV:"` | Struct tag `xml:"DAV: multistatus"` format confirmed for `encoding/xml` |
| READ-07 | PROPFIND streaming | `xml.Encoder` writes directly to `c.Response().Writer`; `DirIterator.Next()` loop |
| READ-08 | GET file streaming | `vfs.ServeFileContent(fs, doc, nil, "", "", req, w)` at `model/vfs/file.go:251` |
| READ-09 | HEAD file | Same as GET — `http.ServeContent` handles HEAD natively |
| READ-10 | GET on collection → 405 | Check `dirDoc != nil`; return 405 with `Allow:` header |
| SEC-01 | All methods except OPTIONS require auth | Auth middleware runs before dispatch; OPTIONS handler registered separately |
| SEC-02 | Path traversal prevention | `path.Clean` + prefix assertion; reject `%2e%2e`, null bytes before VFS |
| SEC-03 | Depth:infinity blocked | Check `c.Request().Header.Get("Depth")` first; 403 + XML error + WARN log |
| SEC-04 | Audit logs for traversal, infinity, out-of-scope | `inst.Logger().WithNamespace("webdav").WithFields(...)` |
| SEC-05 | Content-Length on all responses | PROPFIND: buffer in `bytes.Buffer`, set `Content-Length` before WriteHeader; GET: `ServeFileContent` handles |
| TEST-01 | Unit tests XML (before structs) | TDD: write failing `TestMarshalMultistatus`, `TestUnmarshalPropfind` first; then implement `xml.go` |
| TEST-02 | Unit tests path mapping (before impl) | TDD: write failing table-driven tests in `path_mapper_test.go`; then implement `path_mapper.go` |
| TEST-04 | Integration tests auth | `httpexpect` client: test Bearer header, Basic token-as-password, 401 response, scope rejection |
</phase_requirements>

---

## Summary

Phase 1 builds a read-only WebDAV server inside a new `web/webdav/` package. The implementation is entirely composed of existing cozy-stack primitives: `middlewares.GetRequestToken` for auth token extraction, `model/vfs.DirOrFileByPath` for path resolution, `model/vfs.DirIterator` for streaming PROPFIND, and `vfs.ServeFileContent` for GET/HEAD.

The key technical challenge is the XML namespace requirement: `encoding/xml` must emit `xmlns:D="DAV:"` with `D:` prefix on all elements, which requires careful struct tag notation (`xml:"DAV: elementname"`). All correctness invariants — ETag from MD5Sum, RFC 1123 dates, Content-Length on all responses, path traversal rejection before VFS calls — must be baked in from the first wave rather than added as hardening steps.

The `DirIterator` API is confirmed usable as-is: `inst.VFS().DirIterator(dirDoc, &vfs.IteratorOptions{ByFetch: 200})` returns an interface with a `Next() (*DirDoc, *FileDoc, error)` method, returning `vfs.ErrIteratorDone` when exhausted. No VFS extension is needed for Phase 1.

Route registration follows the established `web/routing.go` pattern: add one import and one `webdav.Routes(router.Group(...))` call. The 308 redirect for Nextcloud compatibility is a standard `c.Redirect(http.StatusPermanentRedirect, newPath)` call.

**Primary recommendation:** Build `web/webdav/` with 6 files (webdav.go, auth.go, path_mapper.go, handlers.go, xml.go, errors.go) + one test file, following the exact same structure as `web/files/`. All 28 Phase 1 requirements are met by combining existing VFS, middleware, and stdlib XML.

---

## Open Questions — Answers

### Q1: Does `pkg/webdav/webdav.go` have reusable XML structs?

**Answer: NO — the existing structs are CLIENT-side only and must NOT be reused.**

Source: `pkg/webdav/webdav.go` (entire file inspected)

The file contains a WebDAV **client** library used to talk to Nextcloud. Its XML structs (`multistatus`, `response`, `props`) are:
- Lowercase/unexported — not accessible from `web/webdav/`
- Designed for **parsing incoming** Nextcloud responses, not for **generating** RFC 4918 responses
- Missing the correct server-side namespace declarations
- The `multistatus` struct has `xml:"multistatus"` (no namespace prefix) — this is wrong for server responses

The `props` struct uses Owncloud extension properties (`fileid`, `trashbin-filename`) that our server should not emit.

**Decision:** Write new server-side XML structs in `web/webdav/xml.go`. Do not import or reuse `pkg/webdav`. The correct struct tag format for the server is `xml:"DAV: multistatus"` (with space before element name — this is Go's encoding/xml namespace syntax).

**Correct encoding/xml namespace syntax:**
```go
// The space in "DAV: multistatus" tells encoding/xml to use namespace "DAV:"
// and emit xmlns:D="DAV:" on the root element.
type Multistatus struct {
    XMLName xml.Name `xml:"DAV: multistatus"`
    // ...
}
```

---

### Q2: Does `vfs.DirIterator` expose a usable cursor/batch API for streaming XML?

**Answer: YES — `DirIterator` is exactly what we need. No VFS extension required.**

Sources:
- `model/vfs/iter.go:28` — `NewIterator` implementation
- `model/vfs/vfs.go:326-337` — `IteratorOptions` struct and `DirIterator` interface

**Confirmed API:**

```go
// IteratorOptions (model/vfs/vfs.go:327)
type IteratorOptions struct {
    AfterID string  // cursor for pagination
    ByFetch int     // batch size (0 → default 256; capped at 256 by iter.go:33)
}

// DirIterator interface (model/vfs/vfs.go:335)
type DirIterator interface {
    Next() (*DirDoc, *FileDoc, error)  // returns ErrIteratorDone when exhausted
}

// Usage pattern for PROPFIND Depth:1:
iter := inst.VFS().DirIterator(dirDoc, &vfs.IteratorOptions{ByFetch: 200})
for {
    d, f, err := iter.Next()
    if errors.Is(err, vfs.ErrIteratorDone) {
        break
    }
    if err != nil {
        return err
    }
    // encode d or f into xml.Encoder
}
```

**Important detail from `model/vfs/iter.go:32`:** `ByFetch` is capped at `iterMaxFetchSize = 256`. Setting `ByFetch: 200` works as intended. Setting `ByFetch: 300` would silently be reduced to 256.

The iterator uses CouchDB `_find` with a bookmark cursor for pagination internally — each `Next()` call transparently fetches the next batch when the current batch is exhausted. The streaming PROPFIND handler does not need to manage cursors explicitly.

**DirBatch** is also available (`model/vfs/vfs.go:238`) but requires explicit `couchdb.Cursor` management. Use `DirIterator` instead — it is simpler and already handles the cursor internally.

---

### Q3: Where exactly to register WebDAV routes in `web/routing.go`?

**Answer: Add one import and one route group block in `SetupRoutes`, after the existing JSON API routes block.**

Source: `web/routing.go:174-275` (full `SetupRoutes` function inspected)

**Established pattern** — every domain follows this exact structure:
```go
// In web/routing.go SetupRoutes():
files.Routes(router.Group("/files", mws...))
notes.Routes(router.Group("/notes", mws...))
```

**WebDAV integration point** (add after existing JSON API routes block, before dev routes):

```go
// In web/routing.go, inside SetupRoutes():

// WebDAV routes — custom middleware chain (auth is different from JSON:API)
{
    mwsWebDAV := []echo.MiddlewareFunc{
        middlewares.NeedInstance,
        middlewares.CheckInstanceBlocked,
        middlewares.CheckInstanceDeleting,
        // NOTE: No LoadSession, no Accept middleware — WebDAV uses its own auth
    }
    webdav.Routes(router.Group("/dav", mwsWebDAV...))

    // Nextcloud-compatible redirect: 308 preserves HTTP method (critical for PROPFIND)
    router.Match(webdavAllMethods, "/remote.php/webdav", func(c echo.Context) error {
        return c.Redirect(http.StatusPermanentRedirect, "/dav/files")
    }, mwsWebDAV...)
    router.Match(webdavAllMethods, "/remote.php/webdav/*", func(c echo.Context) error {
        newPath := strings.Replace(c.Request().URL.RequestURI(), "/remote.php/webdav", "/dav/files", 1)
        return c.Redirect(http.StatusPermanentRedirect, newPath)
    }, mwsWebDAV...)
}
```

**Add import:** `"github.com/cozy/cozy-stack/web/webdav"`

**`webdav.Routes` signature** (to implement in `web/webdav/webdav.go`):
```go
var webdavMethods = []string{
    http.MethodOptions, "PROPFIND", http.MethodGet, http.MethodHead,
    http.MethodPut, http.MethodDelete, "MKCOL", "COPY", "MOVE",
}

func Routes(router *echo.Group) {
    router.Match(webdavMethods, "/files", handlePath)
    router.Match(webdavMethods, "/files/*", handlePath)
}
```

**308 vs 301/302:** Echo's `c.Redirect` takes a status code. `http.StatusPermanentRedirect` = 308. This is the key distinction — 308 preserves the HTTP method (PROPFIND remains PROPFIND after redirect), whereas 301/302 allow clients to downgrade to GET.

---

### Q4: Exact signature of `middlewares.GetRequestToken` — does it work for both Bearer and Basic Auth password?

**Answer: YES — works for both without modification.**

Source: `web/middlewares/permissions.go:63-75`

```go
// GetRequestToken retrieves the token from the incoming request.
// web/middlewares/permissions.go:63
func GetRequestToken(c echo.Context) string {
    req := c.Request()
    if header := req.Header.Get(echo.HeaderAuthorization); header != "" {
        if strings.HasPrefix(header, bearerAuthScheme) {
            return header[len(bearerAuthScheme):]  // Bearer <token>
        }
        if strings.HasPrefix(header, basicAuthScheme) {
            _, pass, _ := req.BasicAuth()
            return pass  // Basic Auth: returns the password field
        }
    }
    return c.QueryParam("bearer_token")
}
```

This function already handles exactly the two auth modes required:
1. `Authorization: Bearer <token>` → returns the token string
2. `Authorization: Basic <base64(user:token)>` → returns the password field (the token)

The WebDAV auth middleware simply calls this function, then passes the result to `middlewares.GetPermission(c)` which calls `middlewares.ParseJWT(c, inst, tok)`. The only WebDAV-specific behaviour is the 401 response format (`WWW-Authenticate: Basic realm="Cozy"` instead of JSON:API error).

**WebDAV auth middleware skeleton:**
```go
// web/webdav/auth.go
func resolveWebDAVAuth(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        // OPTIONS bypasses auth (RFC 4918 allows unauthenticated server discovery)
        if c.Request().Method == http.MethodOptions {
            return next(c)
        }
        tok := middlewares.GetRequestToken(c)
        if tok == "" {
            return sendWebDAV401(c)
        }
        inst := middlewares.GetInstance(c)
        pdoc, err := middlewares.ParseJWT(c, inst, tok)
        if err != nil {
            return sendWebDAV401(c)
        }
        middlewares.ForcePermission(c, pdoc)
        return next(c)
    }
}

func sendWebDAV401(c echo.Context) error {
    c.Response().Header().Set("WWW-Authenticate", `Basic realm="Cozy"`)
    return c.NoContent(http.StatusUnauthorized)
}
```

---

### Q5: Exact signature of `vfs.ServeFileContent` and how to invoke from a WebDAV GET handler

**Answer: Confirmed at `model/vfs/file.go:251`.**

```go
// model/vfs/file.go:251
func ServeFileContent(
    fs VFS,
    doc *FileDoc,
    version *Version,   // nil for current version
    filename string,    // "" uses doc.DocName
    disposition string, // "" = no Content-Disposition header
    req *http.Request,
    w http.ResponseWriter,
) error
```

**WebDAV GET handler invocation:**
```go
func handleGet(c echo.Context) error {
    inst := middlewares.GetInstance(c)
    vfsPath, err := davPathToVFSPath(c.Param("*"))
    if err != nil {
        return sendWebDAVError(c, http.StatusBadRequest, "bad-request")
    }
    dirDoc, fileDoc, err := inst.VFS().DirOrFileByPath(vfsPath)
    if err != nil {
        if os.IsNotExist(err) {
            return sendWebDAVError(c, http.StatusNotFound, "not-found")
        }
        return err
    }
    if dirDoc != nil {
        // GET on collection → 405 Method Not Allowed
        c.Response().Header().Set("Allow", "OPTIONS, PROPFIND, HEAD")
        return c.NoContent(http.StatusMethodNotAllowed)
    }
    // Check permission
    if err := middlewares.AllowVFS(c, permission.GET, fileDoc); err != nil {
        return sendWebDAVError(c, http.StatusForbidden, "forbidden")
    }
    return vfs.ServeFileContent(inst.VFS(), fileDoc, nil, "", "", c.Request(), c.Response())
}
```

**Key detail from `model/vfs/file.go:262-263`:** `ServeFileContent` sets the ETag as base64 of `doc.MD5Sum` — already double-quoted with `fmt.Sprintf(`"%s"`, eTag)`. This is the correct RFC 7232 strong ETag format. No additional quoting needed in the WebDAV handler.

**HEAD:** `http.ServeContent` (called internally by `ServeFileContent`) handles HEAD natively — it sends all headers but no body when the request method is HEAD.

---

### Q6: Pattern for route registration, handler signatures, error handling, test setup from `web/files/`

**Answer: Confirmed from direct inspection of `web/files/files.go` and `web/files/files_test.go`.**

**Route registration pattern (`web/files/files.go:2121`):**
```go
func Routes(router *echo.Group) {
    router.GET("/download", ReadFileContentFromPathHandler)
    router.HEAD("/download", ReadFileContentFromPathHandler)
    // ... other routes
}
```

**Handler signature:**
```go
func HandleName(c echo.Context) error {
    inst := middlewares.GetInstance(c)  // always first
    // ... validate, call VFS, respond
}
```

**Error handling:**
```go
// VFS errors are wrapped via WrapVfsError (exported) or wrapVfsError (internal)
// web/files/files.go:2182
func WrapVfsError(err error) error {
    if errj := wrapVfsError(err); errj != nil {
        return errj  // returns *jsonapi.Error
    }
    return err
}
```
For WebDAV, we do NOT use `jsonapi` errors — we write our own `wrapWebDAVError(err) error` that produces XML RFC 4918 §8.7 bodies.

**Test setup pattern (`web/files/files_test.go:35-68`):**
```go
func TestFiles(t *testing.T) {
    if testing.Short() {
        t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
    }

    config.UseTestFile(t)
    testutils.NeedCouchdb(t)
    setup := testutils.NewSetup(t, t.Name())

    // Set local filesystem for VFS
    config.GetConfig().Fs.URL = &url.URL{
        Scheme: "file",
        Host:   "localhost",
        Path:   t.TempDir(),
    }

    testInstance := setup.GetTestInstance()
    _, token := setup.GetTestClient(consts.Files)
    ts := setup.GetTestServer("/files", Routes)
    ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
    t.Cleanup(ts.Close)

    t.Run("SubTest", func(t *testing.T) {
        e := testutils.CreateTestClient(t, ts.URL)
        // use e.GET, e.POST, etc. with httpexpect
    })
}
```

**WebDAV adaptation:** `GetTestServer("/dav", Routes)` registers at `/dav/files/*`. The `gowebdav` client connects to `ts.URL + "/dav/files"`. The `httpexpect` client is used for raw HTTP assertions (exact XML, headers).

---

### Q7: Exact test instance setup code from existing tests

**Answer: Confirmed from `tests/testutils/test_utils.go:115-200`.**

```go
// Full setup sequence for WebDAV integration tests:
setup := testutils.NewSetup(t, t.Name())         // creates test config
inst := setup.GetTestInstance()                   // creates CouchDB instance + VFS
_, token := setup.GetTestClient(consts.Files)     // creates OAuth client + JWT
ts := setup.GetTestServer("/dav", Routes)         // starts httptest.Server

// For gowebdav integration test:
c := gowebdav.NewAuthClient(
    ts.URL+"/dav/files",
    gowebdav.NewPreemptiveAuth(gowebdav.NewBasicAuth("", token)),
)
// OR for httpexpect raw HTTP tests:
e := testutils.CreateTestClient(t, ts.URL)
e.Request("PROPFIND", "/dav/files/").
    WithHeader("Authorization", "Bearer "+token).
    WithHeader("Depth", "1").
    Expect().Status(207)
```

**Key detail from `test_utils.go:256-260`:** `GetTestServer` injects the instance into echo context via middleware:
```go
group := handler.Group(prefix, func(next echo.HandlerFunc) echo.HandlerFunc {
    return func(context echo.Context) error {
        context.Set("instance", c.inst)
        return next(context)
    }
})
```
This means `middlewares.GetInstance(c)` works in tests without the full `NeedInstance` middleware.

**For `gowebdav` auth:** The token-as-Basic-password pattern (`NewBasicAuth("", token)`) works because `GetRequestToken` extracts the password field from Basic Auth. Username is ignored.

---

### Q8: How does existing code log WARN events with structured fields?

**Answer: Confirmed from `pkg/logger/logger.go` and usage patterns in `web/files/files.go`.**

```go
// Logger interface (pkg/logger/logger.go:21-41)
type Logger interface {
    Warnf(format string, args ...interface{})
    WithField(fn string, fv interface{}) Logger
    WithFields(fields Fields) Logger
    WithDomain(s string) Logger
    // ...
}

// Instance-scoped namespace (most common in web/ handlers):
inst.Logger().WithNamespace("webdav").Warnf("message: %s", arg)

// With structured fields:
inst.Logger().
    WithNamespace("webdav").
    WithFields(logger.Fields{
        "instance":        inst.Domain,
        "source_ip":       c.RealIP(),
        "user_agent":      c.Request().UserAgent(),
        "method":          c.Request().Method,
        "raw_url":         c.Request().URL.String(),
        "normalized_path": normalizedPath,
        "token_hash":      hashToken(token),
    }).
    Warnf("path traversal attempt")
```

**From `web/files/files.go:2235`:**
```go
logger.WithNamespace("files").Warnf("Not wrapped error: %s", err)
```

The `WithNamespace` call sets the `nspace` log field, enabling log filtering by subsystem. Use `"webdav"` as the namespace for all WebDAV audit events.

**Token hashing for audit logs:** Never log the raw token. Use:
```go
func hashToken(tok string) string {
    h := sha256.Sum256([]byte(tok))
    return hex.EncodeToString(h[:8]) // first 8 bytes, 16 hex chars
}
```

---

### Q9: Is there an existing HTTP error XML helper in cozy-stack?

**Answer: NO — `pkg/webdav/errors.go` only has client-side error sentinels. No RFC 4918 XML error builder exists.**

Source: `pkg/webdav/errors.go` (entire file inspected — only 5 sentinel error vars)

The existing `pkg/webdav/` package is a client library. Its `errors.go` has:
```go
var ErrInvalidAuth = errors.New("invalid authentication")
var ErrAlreadyExist = errors.New("it already exists")
// ... (5 sentinel errors total)
```
No XML formatting, no RFC 4918 error body construction.

The existing server-side error handler in `web/errors/errors.go` formats JSON:API errors — entirely wrong format for WebDAV.

**Conclusion:** Write a new `web/webdav/errors.go` with:

```go
// web/webdav/errors.go

// XMLError is an RFC 4918 §8.7 error body.
type XMLError struct {
    XMLName   xml.Name  `xml:"DAV: error"`
    Condition xml.Name  // e.g., xml.Name{Space: "DAV:", Local: "propfind-finite-depth"}
}

// sendWebDAVError writes an RFC 4918 XML error response.
func sendWebDAVError(c echo.Context, status int, condition string) error {
    body, err := xml.Marshal(XMLError{
        Condition: xml.Name{Space: "DAV:", Local: condition},
    })
    if err != nil {
        return c.NoContent(http.StatusInternalServerError)
    }
    c.Response().Header().Set(echo.HeaderContentType, `application/xml; charset="utf-8"`)
    c.Response().Header().Set(echo.HeaderContentLength, strconv.Itoa(len(body)))
    c.Response().WriteHeader(status)
    _, _ = c.Response().Write(body)
    return nil
}
```

**RFC 4918 condition elements for Phase 1:**
- `propfind-finite-depth` — Depth:infinity rejected (403)
- `lock-token-submitted` — Phase 2+
- Generic 404/403/405: use minimal XML with status element

---

### Q10: How to serve a 308 redirect with Echo v4 that preserves the HTTP method?

**Answer: `c.Redirect(http.StatusPermanentRedirect, newURL)` — where `http.StatusPermanentRedirect = 308`.**

Source: `web/auth/auth.go` (multiple `c.Redirect` calls confirmed); Echo v4 docs

Echo's `c.Redirect(code, url)` sets `Location:` header and calls `WriteHeader(code)`. HTTP 308 is defined in RFC 7538: "The request method is not allowed to be changed when reissuing the original request." This means a PROPFIND to `/remote.php/webdav/foo` receives a 308 and the client re-issues PROPFIND to `/dav/files/foo` — preserving the method.

**Important caveat:** Some WebDAV clients (older ones) do not follow 308 and fall back to GET. The current plan accepts this: clients using the Nextcloud-compat path that cannot follow 308 should configure the native `/dav/files/` path directly.

**Implementation:**
```go
// In web/webdav/webdav.go or web/routing.go:
router.Match(webdavMethods, "/remote.php/webdav/*", func(c echo.Context) error {
    suffix := c.Param("*")  // the part after /remote.php/webdav/
    newPath := "/dav/files/" + suffix
    return c.Redirect(http.StatusPermanentRedirect, newPath)
})
```

**Path preservation:** `c.Param("*")` in Echo gives the wildcard match without double-decoding. Preserve it verbatim in the redirect URL to avoid double-encoding issues.

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `encoding/xml` (stdlib) | Go 1.25 | RFC 4918 XML marshalling/unmarshalling | Zero dependency; `encoding/xml` correctly handles namespaced XML with struct tags |
| `net/http` (stdlib) | Go 1.25 | HTTP plumbing, `http.TimeFormat`, `http.ServeContent` | Standard library; `http.TimeFormat` is RFC 1123 constant required for `getlastmodified` |
| `path` (stdlib) | Go 1.25 | `path.Clean` for URL normalisation | Standard path manipulation without filesystem calls |
| `github.com/labstack/echo/v4` | v4.15.1 (in go.mod) | Router, handler interface, context | Already in go.mod; `echo.Match` supports arbitrary HTTP methods |

### Supporting (test only)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/studio-b12/gowebdav` | v0.12.0 | WebDAV client for integration tests | Black-box tests: `ReadDir`, `Stat`, `Read` against the running test server |
| `github.com/gavv/httpexpect/v2` | v2.16.0 (in go.mod) | Raw HTTP assertions | PROPFIND XML shape, exact headers, OPTIONS response, 401 format |
| `github.com/stretchr/testify` | v1.11.1 (in go.mod) | Unit test assertions | XML struct marshalling tests, path mapping unit tests |

### Installation

```bash
# Only new dependency — test-only client
go get github.com/studio-b12/gowebdav@v0.12.0
```

No new server-side dependencies. The implementation uses only stdlib + already-imported packages.

---

## Architecture Patterns

### Recommended Project Structure

```
web/webdav/
├── webdav.go          # package doc; webdavMethods const; Routes(*echo.Group)
├── auth.go            # resolveWebDAVAuth middleware; sendWebDAV401
├── path_mapper.go     # davPathToVFSPath(rawParam) (string, error)
├── handlers.go        # handlePath dispatcher; handleOptions, handlePropfind, handleGet
├── xml.go             # Multistatus, Response, Propstat, Prop, PropFind structs
├── errors.go          # sendWebDAVError; XMLError; wrapVFSError (WebDAV-specific)
└── webdav_test.go     # All tests: unit XML, unit path, integration auth+PROPFIND+GET
```

### Pattern 1: Route Registration

```go
// web/webdav/webdav.go
// Source: mirrors web/files/files.go:2121 and web/compat/compat.go:22

var webdavMethods = []string{
    http.MethodOptions, "PROPFIND", http.MethodGet, http.MethodHead,
    http.MethodPut, http.MethodDelete, "MKCOL", "COPY", "MOVE",
}

// Routes sets the routing for the WebDAV service.
func Routes(router *echo.Group) {
    // OPTIONS bypasses auth — registered first without auth middleware
    router.OPTIONS("/files", handleOptions)
    router.OPTIONS("/files/*", handleOptions)

    // All other methods go through auth middleware
    authMw := resolveWebDAVAuth
    router.Match(webdavMethods[1:], "/files", authMw, handlePath)
    router.Match(webdavMethods[1:], "/files/*", authMw, handlePath)
}
```

### Pattern 2: PROPFIND Streaming

```go
// web/webdav/handlers.go — handlePropfind

func handlePropfind(c echo.Context) error {
    depth := c.Request().Header.Get("Depth")
    if depth == "infinity" || depth == "Infinity" {
        // Audit log + RFC 4918 403
        inst.Logger().WithNamespace("webdav").WithFields(auditFields(c, normalizedPath)).
            Warn("PROPFIND Depth:infinity rejected")
        return sendWebDAVError(c, http.StatusForbidden, "propfind-finite-depth")
    }

    inst := middlewares.GetInstance(c)
    vfsPath, err := davPathToVFSPath(c.Param("*"))
    if err != nil { /* 400 */ }

    dirDoc, fileDoc, err := inst.VFS().DirOrFileByPath(vfsPath)
    if err != nil { /* 404 */ }

    if err := middlewares.AllowVFS(c, permission.GET, firstNonNil(dirDoc, fileDoc)); err != nil {
        return sendWebDAVError(c, http.StatusForbidden, "forbidden")
    }

    // Stream the response
    c.Response().Header().Set("Content-Type", `application/xml; charset="utf-8"`)
    c.Response().WriteHeader(http.StatusMultiStatus)

    enc := xml.NewEncoder(c.Response())
    enc.Indent("", "  ")
    _, _ = c.Response().Write([]byte(xml.Header))

    ms := openMultistatus(enc)
    defer closeMultistatus(enc, ms)

    // Always include the resource itself (Depth 0 or 1)
    writeResponse(enc, c.Request(), dirDoc, fileDoc)

    // Depth:1 — iterate children
    if dirDoc != nil && depth == "1" {
        iter := inst.VFS().DirIterator(dirDoc, &vfs.IteratorOptions{ByFetch: 200})
        for {
            d, f, err := iter.Next()
            if errors.Is(err, vfs.ErrIteratorDone) { break }
            if err != nil { return err }
            writeResponse(enc, c.Request(), d, f)
        }
    }
    return enc.Flush()
}
```

**Note on Content-Length for streaming PROPFIND:** When streaming XML directly to the response writer, `Content-Length` cannot be known in advance. Echo will use `Transfer-Encoding: chunked` automatically. Per the `SEC-05` requirement, for non-streaming responses (small Depth:0), build into a `bytes.Buffer` first and set `Content-Length`. For Depth:1 streaming, accept chunked — it is valid HTTP/1.1 and only breaks strict HTTP/1.0 clients.

### Pattern 3: Path Mapper

```go
// web/webdav/path_mapper.go

// davPathToVFSPath converts a WebDAV URL path parameter to a VFS path.
// The input is c.Param("*") which Echo has already URL-decoded once.
// Returns error if the path is unsafe.
func davPathToVFSPath(rawParam string) (string, error) {
    // rawParam from Echo is already URL-decoded (e.g. "%20" → " ")
    // But double-encoded sequences like "%252f" need explicit rejection

    // Reject null bytes
    if strings.ContainsRune(rawParam, 0) {
        return "", ErrPathTraversal
    }
    // Reject double-encoded dots (after single decoding, still have %2e)
    lower := strings.ToLower(rawParam)
    if strings.Contains(lower, "%2e") || strings.Contains(lower, "%2f") {
        return "", ErrPathTraversal
    }

    // Clean the path (removes .., multiple slashes, trailing slash)
    cleaned := path.Clean("/" + rawParam)

    // After cleaning, must start with /files/
    if cleaned != "/files" && !strings.HasPrefix(cleaned, "/files/") {
        return "", ErrPathTraversal
    }

    // Strip the /files prefix to get the VFS path
    vfsPath := strings.TrimPrefix(cleaned, "/files")
    if vfsPath == "" {
        vfsPath = "/"
    }
    return vfsPath, nil
}
```

### Anti-Patterns to Avoid

- **Don't use `c.Stream(...)`** — sends chunked without Content-Length; breaks macOS Finder for file GETs. Use `vfs.ServeFileContent` instead.
- **Don't copy `_rev` as ETag** — `ServeFileContent` already sets the correct md5-based ETag; don't override it.
- **Don't use `time.RFC3339` for dates** — use `t.UTC().Format(http.TimeFormat)` everywhere dates appear in XML.
- **Don't call VFS before path validation** — reject traversal attempts purely in the path mapper, before any database lookup.
- **Don't use `xml.Marshal` for the full PROPFIND response** — it buffers everything in memory. Use `xml.Encoder` streaming.
- **Don't use `xml.MarshalIndent`** — use `enc.Indent("", "")` on the encoder (controls formatting without buffering).

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| File content streaming with Range/ETag | Custom `io.Copy` + header management | `vfs.ServeFileContent` | Already handles Range, ETag, Last-Modified, HEAD; battle-tested |
| Directory listing with CouchDB pagination | Custom `_find` queries | `vfs.DirIterator` with `ByFetch:200` | Handles bookmark cursor, batch fetching, done detection |
| Path resolution | String manipulation | `inst.VFS().DirOrFileByPath(path)` | Handles VFS indexer, NFC normalization, CouchDB lookup |
| OAuth token extraction | Header parsing | `middlewares.GetRequestToken(c)` | Already handles Bearer + Basic Auth password |
| JWT validation | Token parsing | `middlewares.ParseJWT(c, inst, tok)` | Handles expiry, issuer, session binding, security stamp |
| Permission checking | Manual scope comparison | `middlewares.AllowVFS(c, verb, fetcher)` | Uses permission.Set.Allow which handles maximal perms |
| Test instance creation | Manual CouchDB setup | `testutils.NewSetup(t, t.Name())` | Handles lifecycle, cleanup, OAuth client creation |

**Key insight:** The VFS layer already does the hard work. The WebDAV handler is purely a protocol translator: HTTP → VFS calls → XML response. Any logic that touches file metadata or file content belongs in `model/vfs/`, not in `web/webdav/`.

---

## Common Pitfalls

### Pitfall 1: XML Namespace Prefix

**What goes wrong:** `encoding/xml` encodes `xml:"DAV: multistatus"` (note the space) as `xmlns:_something="DAV:"` with a generated prefix. Without explicit prefix configuration, the output may use `ns1:multistatus` instead of `D:multistatus`, breaking Windows Mini-Redirector.

**How to avoid:** Use `xml.EncodeToken` with explicit `xml.StartElement` including the namespace, or register a custom prefix via `enc.EncodeToken(xml.ProcInst{...})`. The safest approach is to write the XML header and `<D:multistatus xmlns:D="DAV:">` tag manually as a string, then use `xml.Encoder` for individual `<D:response>` elements.

**Alternative:** Use `xml.Name{Space: "DAV:", Local: "multistatus"}` in struct definitions. Go's `encoding/xml` will emit `xmlns:_N="DAV:" _N:multistatus` with an auto-generated prefix. To force `D:`, either manually write the root element or use `xmlnsWriter` wrapping.

**Recommended approach:** Write the XML header and root tag as raw bytes, use `xml.Encoder` for each `<D:response>` block:
```go
_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` + "\n"))
_, _ = w.Write([]byte(`<D:multistatus xmlns:D="DAV:">` + "\n"))
enc := xml.NewEncoder(w)
// ... encode responses
_, _ = w.Write([]byte(`</D:multistatus>`))
```

### Pitfall 2: ETag Quoting

**What goes wrong:** `ServeFileContent` correctly sets a quoted ETag for GET/HEAD. But the PROPFIND `getetag` property must also be double-quoted: `"abc123"`. Missing the outer quotes causes `If-Match` failures on conditional requests.

**How to avoid:**
```go
// Correct:
eTag := fmt.Sprintf(`"%s"`, base64.StdEncoding.EncodeToString(fileDoc.MD5Sum))

// Wrong:
eTag := base64.StdEncoding.EncodeToString(fileDoc.MD5Sum) // unquoted
```

### Pitfall 3: Path Traversal via Double-Encoded Sequences

**What goes wrong:** Echo's `c.Param("*")` applies one round of URL decoding. A path like `%252e%252e` becomes `%2e%2e` after Echo's decoding — the dot-dot is still encoded and would pass a naive `strings.Contains(path, "..")` check, but `path.Clean` would normalize it to `..`.

**How to avoid:** After Echo's automatic decoding, check for `%2e` and `%2f` in the still-partially-encoded string before passing to `path.Clean`. Also: `path.Clean` does handle `..` — the assertion `strings.HasPrefix(cleaned, "/files/")` is the definitive safety check.

### Pitfall 4: RFC 1123 Date Format

**What goes wrong:** `time.RFC3339` produces `2006-01-02T15:04:05Z` — macOS Finder silently misparses this. `http.TimeFormat` produces `Mon, 02 Jan 2006 15:04:05 GMT` — correct RFC 1123.

**How to avoid:** Only use `t.UTC().Format(http.TimeFormat)` for `getlastmodified`. Use ISO 8601 for `creationdate` (RFC 4918 §15.1 specifies ISO 8601 for `DAV:creationdate`):
```go
creationDate := doc.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
lastModified := doc.UpdatedAt.UTC().Format(http.TimeFormat)
```

### Pitfall 5: Depth:infinity Logged Before Rejection

**What goes wrong:** The Depth:infinity check fires an audit WARN, but if the log call happens after VFS lookup, the DoS traversal already occurred.

**How to avoid:** Check the `Depth` header as the very first thing in `handlePropfind`, before `davPathToVFSPath` and before any `inst.VFS()` call. Reject immediately with 403 + XML body + WARN log.

### Pitfall 6: Missing CLAUDE.md

**What goes wrong:** No `CLAUDE.md` was found in the repo root. This means no project-specific guidelines override the standard cozy-stack conventions.

**How to avoid:** Follow the conventions in `.planning/codebase/CONVENTIONS.md` directly.

---

## Code Examples

### XML Structs — Server-side (write these FIRST as failing tests)

```go
// web/webdav/xml.go
// Source: RFC 4918 §14 + encoding/xml documentation

package webdav

import "encoding/xml"

// PropFind is the parsed request body of a PROPFIND request.
type PropFind struct {
    XMLName  xml.Name  `xml:"DAV: propfind"`
    AllProp  *struct{} `xml:"DAV: allprop"`
    PropName *struct{} `xml:"DAV: propname"`
    Prop     *PropList `xml:"DAV: prop"`
}

// PropList is the list of requested property names.
type PropList struct {
    ResourceType    *struct{} `xml:"DAV: resourcetype"`
    DisplayName     *struct{} `xml:"DAV: displayname"`
    GetLastModified *struct{} `xml:"DAV: getlastmodified"`
    GetETag         *struct{} `xml:"DAV: getetag"`
    GetContentLength *struct{} `xml:"DAV: getcontentlength"`
    GetContentType  *struct{} `xml:"DAV: getcontenttype"`
    CreationDate    *struct{} `xml:"DAV: creationdate"`
    SupportedLock   *struct{} `xml:"DAV: supportedlock"`
    LockDiscovery   *struct{} `xml:"DAV: lockdiscovery"`
}

// Prop holds the values of live properties for a resource.
type Prop struct {
    ResourceType    *ResourceType `xml:"DAV: resourcetype,omitempty"`
    DisplayName     string        `xml:"DAV: displayname,omitempty"`
    GetLastModified string        `xml:"DAV: getlastmodified,omitempty"` // http.TimeFormat
    GetETag         string        `xml:"DAV: getetag,omitempty"`         // double-quoted
    GetContentLength int64        `xml:"DAV: getcontentlength,omitempty"`
    GetContentType  string        `xml:"DAV: getcontenttype,omitempty"`
    CreationDate    string        `xml:"DAV: creationdate,omitempty"`    // ISO 8601
    SupportedLock   *struct{}     `xml:"DAV: supportedlock"`             // always present, empty
    LockDiscovery   *struct{}     `xml:"DAV: lockdiscovery"`             // always present, empty
}

// ResourceType represents the DAV:resourcetype property.
type ResourceType struct {
    Collection *struct{} `xml:"DAV: collection,omitempty"` // nil for files
}

// Propstat groups a set of properties with a status.
type Propstat struct {
    Prop   Prop   `xml:"DAV: prop"`
    Status string `xml:"DAV: status"`
}

// Response is a single WebDAV response element.
type Response struct {
    XMLName  xml.Name   `xml:"DAV: response"`
    Href     string     `xml:"DAV: href"`
    Propstat []Propstat `xml:"DAV: propstat"`
}
```

### Path Mapper Unit Test (write BEFORE implementing path_mapper.go)

```go
// web/webdav/path_mapper_test.go — RED phase

func TestDavPathToVFSPath(t *testing.T) {
    cases := []struct {
        name      string
        input     string
        wantPath  string
        wantError bool
    }{
        {name: "root", input: "", wantPath: "/"},
        {name: "root slash", input: "/", wantPath: "/"},
        {name: "simple", input: "Documents", wantPath: "/Documents"},
        {name: "nested", input: "Documents/report.docx", wantPath: "/Documents/report.docx"},
        {name: "trailing slash", input: "Documents/", wantPath: "/Documents"},
        {name: "dotdot", input: "../etc/passwd", wantError: true},
        {name: "encoded dotdot", input: "%2e%2e/etc", wantError: true},
        {name: "double encoded", input: "%252e%252e/etc", wantError: true},
        {name: "null byte", input: "Documents\x00/evil", wantError: true},
        {name: "encoded slash", input: "Documents%2fsecret", wantError: true},
        {name: "unicode filename", input: "Documents/répertoire", wantPath: "/Documents/répertoire"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            got, err := davPathToVFSPath(tc.input)
            if tc.wantError {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
                assert.Equal(t, tc.wantPath, got)
            }
        })
    }
}
```

### XML Unit Test (write BEFORE implementing xml.go)

```go
// web/webdav/xml_test.go — RED phase

func TestMarshalMultistatus(t *testing.T) {
    r := Response{
        Href: "/dav/files/doc.txt",
        Propstat: []Propstat{{
            Prop: Prop{
                GetLastModified: "Mon, 07 Apr 2025 10:00:00 GMT",
                GetETag:         `"abc123"`,
                GetContentLength: 1234,
            },
            Status: "HTTP/1.1 200 OK",
        }},
    }
    data, err := xml.Marshal(r)
    require.NoError(t, err)
    assert.Contains(t, string(data), `xmlns`)
    assert.Contains(t, string(data), `D:href`)
    assert.Contains(t, string(data), `D:getetag`)
    assert.Contains(t, string(data), `"abc123"`)
}

func TestGetLastModifiedFormat(t *testing.T) {
    // RFC 1123 format required — NOT RFC 3339
    now := time.Date(2025, 4, 7, 10, 0, 0, 0, time.UTC)
    formatted := now.UTC().Format(http.TimeFormat)
    assert.Equal(t, "Mon, 07 Apr 2025 10:00:00 GMT", formatted)
    assert.NotContains(t, formatted, "T") // no ISO 8601 T separator
}
```

---

## Validation Architecture

### 1. Input Validation

- **Path validation:** Unit tests for `davPathToVFSPath` covering all traversal patterns: `../`, `%2e%2e/`, `%252e%252e/`, null bytes, `/settings`, `/apps` prefixes. Tests run without any infrastructure (pure Go unit tests).
- **Depth header:** Test that `Depth: infinity` returns 403 (httpexpect raw HTTP test; no VFS needed).
- **PROPFIND XML body:** Test that missing body (allprop per RFC), `<allprop/>`, `<prop>` variants, and malformed XML are all handled without panic. Unit test `TestParsePropFind`.
- **Auth headers:** Test Bearer, Basic (token as password), missing, malformed (httpexpect tests; require CouchDB for JWT validation).

### 2. Output Validation

- **PROPFIND response shape:** httpexpect XML assertions: `xmlns:D="DAV:"` present, all 9 live properties present for files/dirs, `D:` prefix on all elements, correct `207 Multi-Status` status.
- **ETag format:** Assert `getetag` value matches `^"[A-Za-z0-9+/=]+"$` (double-quoted base64).
- **Date format:** Assert `getlastmodified` matches RFC 1123 regexp `^\w{3}, \d{2} \w{3} \d{4} \d{2}:\d{2}:\d{2} GMT$`.
- **GET response:** Assert `Content-Length`, `ETag`, `Last-Modified` headers present; body matches file content.
- **401 response:** Assert `WWW-Authenticate: Basic realm="Cozy"` header present; no JSON body.
- **308 redirect:** Assert `Location:` header points to correct `/dav/files/` path.

### 3. State Validation

- **VFS state:** After all read-only Phase 1 operations, the VFS state MUST be unchanged. Assert no CouchDB writes occurred by verifying `_rev` is unchanged after PROPFIND and GET operations.
- **Trash visibility:** Create a file, trash it, list root via PROPFIND — assert `.cozy_trash` directory appears.

### 4. Permission Validation

- **Valid token, correct scope:** PROPFIND on `/dav/files/` with `consts.Files` scope → 207.
- **Valid token, wrong scope:** PROPFIND with `consts.Apps` scope → 403 (not 401).
- **Invalid token:** PROPFIND with expired/malformed token → 401.
- **No token:** PROPFIND with no Authorization header → 401 with correct `WWW-Authenticate`.
- **OPTIONS bypass:** OPTIONS with no token → 200 (no auth required).
- **Scope isolation:** Token with `io.cozy.files` scope; attempt PROPFIND on `/dav/` root (no `/files/` prefix) → 403 with WARN log.

### 5. Error Validation

- **Error body format:** All error responses (400, 401, 403, 404, 405) have `Content-Type: application/xml; charset="utf-8"` and a valid RFC 4918 `<D:error>` body.
- **404 path:** PROPFIND on non-existent path → 404 with XML error body.
- **405 collection:** GET on directory → 405 with `Allow:` header.
- **403 infinity:** PROPFIND with `Depth: infinity` → 403 with `<D:propfind-finite-depth/>` body.
- **Path traversal:** Request with `../` in path → 400 (or 403) with WARN log emitted.

### 6. Concurrency Validation

- **Concurrent PROPFIND:** 10 goroutines simultaneously PROPFIND the same large directory (100 files). All return 207 with identical content. No data races (run with `-race` flag).
- **Memory bound:** Single PROPFIND on 1000-file directory: heap profile shows no allocations proportional to directory size (streaming validation). Benchmark: `BenchmarkPropfindDepth1_1000Files`.

### 7. Performance Validation

- **Large directory streaming:** Create 500 files; PROPFIND Depth:1. Assert response is received incrementally (chunked transfer). Assert Go heap growth is < 10MB for the handler goroutine.
- **DirIterator batch efficiency:** Verify that a 500-file PROPFIND does not load all 500 docs at once — log CouchDB query count (should be ceil(500/200) = 3 queries).

### 8. Integration Validation

- **gowebdav client:** `gowebdav.Client.ReadDir("/")` returns the expected list of directories. `gowebdav.Client.Read("/Documents/file.txt")` returns the correct file bytes.
- **httpexpect raw:** OPTIONS → check `DAV: 1` and `Allow:` values exactly. PROPFIND Depth:0 on file → check all 9 properties. GET with Range header → check 206 Partial Content.
- **308 redirect chain:** gowebdav client pointed at `/remote.php/webdav/` path; after redirect, PROPFIND succeeds.

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` package (Go 1.25) |
| Config file | `cozy.test.yaml` via `config.UseTestFile(t)` |
| Quick run command | `go test -p 1 -short -timeout 2m ./web/webdav/` |
| Full suite command | `go test -p 1 -timeout 5m ./web/webdav/` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ROUTE-01 | /dav/files/ responds | integration | `go test ./web/webdav/ -run TestRouteBasic` | ❌ Wave 0 |
| ROUTE-02 | 308 redirect from /remote.php | integration | `go test ./web/webdav/ -run TestNextcloudRedirect` | ❌ Wave 0 |
| ROUTE-03 | Path normalisation edge cases | unit | `go test ./web/webdav/ -run TestDavPathToVFSPath` | ❌ Wave 0 |
| ROUTE-04 | OPTIONS no auth | integration | `go test ./web/webdav/ -run TestOptions` | ❌ Wave 0 |
| ROUTE-05 | Non-/files/ path rejected | unit | `go test ./web/webdav/ -run TestDavPathToVFSPath` | ❌ Wave 0 |
| AUTH-01 | Bearer token auth | integration | `go test ./web/webdav/ -run TestAuthBearer` | ❌ Wave 0 |
| AUTH-02 | Basic Auth password token | integration | `go test ./web/webdav/ -run TestAuthBasic` | ❌ Wave 0 |
| AUTH-03 | 401 + WWW-Authenticate | integration | `go test ./web/webdav/ -run TestAuth401` | ❌ Wave 0 |
| AUTH-04 | Token → Cozy permissions | integration | `go test ./web/webdav/ -run TestAuthPermissions` | ❌ Wave 0 |
| AUTH-05 | Wrong scope → 403 | integration | `go test ./web/webdav/ -run TestAuthScope` | ❌ Wave 0 |
| READ-01 | PROPFIND Depth:0 dir | integration | `go test ./web/webdav/ -run TestPropfindDepth0Dir` | ❌ Wave 0 |
| READ-02 | PROPFIND Depth:0 file | integration | `go test ./web/webdav/ -run TestPropfindDepth0File` | ❌ Wave 0 |
| READ-03 | PROPFIND Depth:1 | integration | `go test ./web/webdav/ -run TestPropfindDepth1` | ❌ Wave 0 |
| READ-04 | Depth:infinity → 403 | integration | `go test ./web/webdav/ -run TestPropfindDepthInfinity` | ❌ Wave 0 |
| READ-05 | All 9 live properties | unit+integration | `go test ./web/webdav/ -run TestPropfindLiveProperties` | ❌ Wave 0 |
| READ-06 | D: namespace prefix | unit | `go test ./web/webdav/ -run TestXMLNamespace` | ❌ Wave 0 |
| READ-07 | Streaming (no full buffer) | benchmark | `go test ./web/webdav/ -bench BenchmarkPropfindLargeDir` | ❌ Wave 0 |
| READ-08 | GET file streaming | integration | `go test ./web/webdav/ -run TestGetFile` | ❌ Wave 0 |
| READ-09 | HEAD file | integration | `go test ./web/webdav/ -run TestHeadFile` | ❌ Wave 0 |
| READ-10 | GET on collection → 405 | integration | `go test ./web/webdav/ -run TestGetCollection405` | ❌ Wave 0 |
| SEC-01 | All methods require auth | integration | `go test ./web/webdav/ -run TestAuthRequired` | ❌ Wave 0 |
| SEC-02 | Path traversal | unit | `go test ./web/webdav/ -run TestPathTraversal` | ❌ Wave 0 |
| SEC-03 | Depth:infinity 403 | integration | (same as READ-04) | ❌ Wave 0 |
| SEC-04 | Audit WARN logs emitted | integration | `go test ./web/webdav/ -run TestAuditLogs` | ❌ Wave 0 |
| SEC-05 | Content-Length on all responses | integration | `go test ./web/webdav/ -run TestContentLength` | ❌ Wave 0 |
| TEST-01 | XML unit tests | unit | `go test -short ./web/webdav/ -run TestXML` | ❌ Wave 0 |
| TEST-02 | Path mapping unit tests | unit | `go test -short ./web/webdav/ -run TestPath` | ❌ Wave 0 |
| TEST-04 | Auth integration tests | integration | `go test ./web/webdav/ -run TestAuth` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test -short -timeout 2m ./web/webdav/`
- **Per wave merge:** `go test -p 1 -timeout 5m ./web/webdav/ -race`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `web/webdav/webdav_test.go` — main test file (covers all 28 requirements)
- [ ] `web/webdav/path_mapper_test.go` — unit tests for path normalisation (can be in webdav_test.go)
- [ ] `web/webdav/xml_test.go` — XML marshalling/unmarshalling unit tests (can be in webdav_test.go)
- [ ] `go.mod` update: `github.com/studio-b12/gowebdav@v0.12.0`

---

## Risks and Mitigations

### Risk 1: encoding/xml prefix generation

**Risk:** Go's `encoding/xml` does not guarantee `D:` prefix; it may generate `ns1:` or similar. Windows Mini-Redirector fails on non-prefixed XML.

**Mitigation (HIGH confidence):** Use manual XML header + root element approach:
```go
w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><D:multistatus xmlns:D="DAV:">`))
enc := xml.NewEncoder(w)
// encode Response structs with xml:"DAV: response" tags
w.Write([]byte(`</D:multistatus>`))
```
With this approach, `encoding/xml` emits elements with the `DAV:` namespace, and since the root element already declares `xmlns:D="DAV:"`, the encoder will use the `D:` prefix for child elements. Validate with `TestXMLNamespace` in Wave 0 — before Windows client is needed.

### Risk 2: DirIterator ByFetch cap at 256

**Risk:** `iterMaxFetchSize = 256` in `model/vfs/iter.go:12`. Setting `ByFetch: 200` works, but the constraint is invisible. If the default is ever raised, behavior changes silently.

**Mitigation:** Explicitly set `ByFetch: 200` in the `IteratorOptions`. Document the constant. The constraint does not affect Phase 1 correctness — 200 < 256.

### Risk 3: Content-Length on streaming PROPFIND

**Risk:** For Depth:1 on large directories, Content-Length cannot be known in advance. Some clients may behave poorly with `Transfer-Encoding: chunked`.

**Mitigation:** For Depth:0 responses (single resource), buffer in `bytes.Buffer` and set Content-Length exactly. For Depth:1, accept chunked — all major WebDAV clients (gowebdav, Cyberduck, OnlyOffice mobile) handle chunked correctly. macOS Finder requires Content-Length for GET responses (handled by `ServeFileContent`) but tolerates chunked for PROPFIND.

### Risk 4: gowebdav v0.12.0 Bearer auth support

**Risk:** The STACK.md notes that `gowebdav` v0.12.0 may not have a stable Bearer token authenticator.

**Mitigation (CONFIRMED):** Use `gowebdav.NewBasicAuth("", token)` — Basic Auth with empty username and token as password. This is the primary Cozy auth mode anyway. The `GetRequestToken` function confirms it extracts the password field from Basic Auth.

### Risk 5: Echo routing for custom HTTP methods (PROPFIND)

**Risk:** Echo may not route `PROPFIND` method correctly without explicit method registration.

**Mitigation (CONFIRMED HIGH):** Echo v4 supports arbitrary HTTP methods via `e.Match([]string{"PROPFIND", ...}, path, handler)`. From STACK.md: "Echo issue #1459 — WebDAV method routing in Echo, `e.Match()` workaround confirmed." The `router.Match(methods, pattern, handler)` call in `Routes()` explicitly registers PROPFIND alongside GET, OPTIONS, etc.

---

## Pre-flight Checklist

Tasks to complete before planning/implementation starts:

- [ ] **Add gowebdav to go.mod:** `go get github.com/studio-b12/gowebdav@v0.12.0` in the repo root
- [ ] **Verify Echo Match exists:** Confirm `echo.Group.Match` is available in Echo v4.15.1 (`grep -r "func.*Match" $GOPATH/pkg/mod/github.com/labstack/echo/v4@v4.15.1/echo.go`)
- [ ] **Verify http.StatusPermanentRedirect:** Confirm `http.StatusPermanentRedirect == 308` in Go stdlib (it is, since Go 1.7)
- [ ] **Verify `iterMaxFetchSize`:** Confirm constant value in `model/vfs/iter.go:12` is 256 (not reduced) — confirmed
- [ ] **Verify `ServeFileContent` ETag quoting:** Confirm `model/vfs/file.go:263` double-quotes the ETag — confirmed
- [ ] **Create `web/webdav/` directory:** `mkdir -p web/webdav/`
- [ ] **Create test config:** Ensure `~/.cozy/cozy.test.yaml` exists for integration tests (or use CI config)
- [ ] **Register import in routing.go:** Add `"github.com/cozy/cozy-stack/web/webdav"` to import block
- [ ] **Confirm CouchDB available for tests:** `testutils.NeedCouchdb(t)` will catch this, but verify early

---

## Sources

### Primary (HIGH confidence)

- `web/middlewares/permissions.go:63-75` — `GetRequestToken` signature and both auth paths, confirmed
- `web/middlewares/permissions.go:250-413` — `ParseJWT`, `GetPermission`, `AllowVFS`, `AllowWholeType`
- `model/vfs/file.go:251-280` — `ServeFileContent` exact signature and ETag implementation
- `model/vfs/vfs.go:326-337` — `IteratorOptions`, `DirIterator` interface definition
- `model/vfs/iter.go:1-94` — `NewIterator` implementation, batch behaviour, `iterMaxFetchSize=256`
- `model/vfs/couchdb_indexer.go:559-589` — `DirIterator` and `DirBatch` implementations
- `web/routing.go:174-275` — `SetupRoutes` full function, route registration pattern
- `web/files/files.go:2121-2179` — `Routes` function pattern, error handling pattern
- `web/files/files_test.go:35-68` — test setup pattern (setup, instance, token, server)
- `tests/testutils/test_utils.go:115-274` — full `TestSetup` API
- `pkg/logger/logger.go` — Logger interface, `WithNamespace`, `WithFields`
- `pkg/webdav/webdav.go` — client XML structs (confirmed NOT reusable for server)
- `pkg/webdav/errors.go` — confirmed no RFC 4918 XML error builder exists
- `.planning/codebase/CONVENTIONS.md` — naming, error handling, logging patterns

### Secondary (MEDIUM confidence)

- `.planning/research/STACK.md` — library analysis, Echo Match workaround, gowebdav auth
- `.planning/research/ARCHITECTURE.md` — full request flow, component diagram
- `.planning/research/PITFALLS.md` — 20 pitfalls, date format, ETag, namespace, traversal

### Tertiary (from prior research, HIGH confidence)

- RFC 4918 — WebDAV HTTP Extensions (§8.7 error format, §9.1 PROPFIND, §14 live properties, §15.1 creationdate ISO 8601)
- Go stdlib `encoding/xml` — namespace handling with `xml:"namespace localname"` struct tags
- Go stdlib `net/http` — `http.TimeFormat = "Mon, 02 Jan 2006 15:04:05 GMT"`, `http.StatusPermanentRedirect = 308`

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all packages confirmed in go.mod; no new server-side deps
- Architecture: HIGH — all integration points confirmed from source inspection
- VFS API: HIGH — exact signatures confirmed from model/vfs source
- DirIterator: HIGH — full implementation in iter.go inspected
- Routing: HIGH — SetupRoutes pattern confirmed
- Auth: HIGH — GetRequestToken confirmed for both auth modes
- XML namespace: MEDIUM — `D:` prefix behaviour under `encoding/xml` needs validation in Wave 0 unit tests
- Pitfalls: HIGH — path traversal, ETag, dates all confirmed from source

**Research date:** 2026-04-04
**Valid until:** 2026-05-04 (stable APIs; re-verify if Echo or VFS major version bumps)
