# Stack Research

**Domain:** WebDAV server layer on top of an existing Go VFS abstraction
**Researched:** 2026-04-04
**Confidence:** HIGH (server library analysis from source + official issue tracker; MEDIUM for emersion/go-webdav Overwrite specifics, unverified against raw source)

---

## Recommended Stack

### Decision Summary

**Write custom handlers using `encoding/xml` directly.** Do not delegate to `golang.org/x/net/webdav`'s `Handler`. Do not adopt `emersion/go-webdav` as the server layer. Use `studio-b12/gowebdav` for integration test clients only.

This is the prescriptive recommendation. The rationale is below.

---

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `encoding/xml` (stdlib) | Go 1.25 (bundled) | RFC 4918 XML marshalling / unmarshalling for PROPFIND and PROPPATCH | Zero dependency. The WebDAV XML surface is small and static (15-20 structs). Standard library `encoding/xml` handles namespaced XML correctly with struct tags. No external library needed. |
| `net/http` (stdlib) | Go 1.25 (bundled) | HTTP request / response plumbing | Already used via Echo. Custom handlers implement normal `echo.HandlerFunc` — no separate HTTP server. |
| `golang.org/x/net/webdav` | v0.50.0 (already in go.mod) | **XML type definitions only** — borrow `webdav.Property`, `webdav.DeadPropsHolder`, `webdav.ETager` interfaces for potential reuse | Already a transitive dependency via Echo. However: **only use the type definitions, not the `Handler`**. See "What NOT to Use" and the deep analysis below. |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/studio-b12/gowebdav` | v0.12.0 (Jan 2026) | WebDAV client — drives integration tests from a real client perspective | **Integration tests only.** Use `NewClient(url, user, pass)` with `NewPreemptiveAuth` for Bearer token tests. Supports `ReadDir`, `Read`, `WriteStream`, `MkdirAll`, `Rename`, `RemoveAll`, `Stat`. BSD 3-Clause. |
| `github.com/gavv/httpexpect/v2` | v2.16.0 (already in go.mod) | Raw HTTP integration test assertions for WebDAV-specific scenarios that `gowebdav` cannot express (e.g. exact XML property assertions, Depth header behaviour) | Complement to `gowebdav`. Use for PROPFIND XML shape verification, ETag header format tests, status code assertion for edge cases. |
| `github.com/stretchr/testify` | v1.11.1 (already in go.mod) | Unit test assertions for XML builder, path mapper, error translator | Standard assertion library already in project. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `litmus` (external WebDAV compliance tester) | Runs the authoritative WebDAV test suite against the live server | Run as a Docker container: `docker run --rm -e URL=http://host/dav/files nicowillis/litmus`. Catches compliance issues that hand-written tests miss. Run in CI on each PR. |
| `go test -run TestWebDAV ./web/webdav/` | Unit + integration tests in the WebDAV handler package | Co-locate tests in `web/webdav/webdav_test.go` following stack convention. Use in-memory VFS (`mem://`) for fast CI runs. |

---

## Deep Analysis: `golang.org/x/net/webdav`

### What it provides

`golang.org/x/net/webdav` (currently at v0.52.0 as of Mar 2026; cozy-stack has v0.50.0 in go.mod) exposes:

- `Handler` — a complete `http.Handler` that dispatches all WebDAV methods, manages locking, and calls a pluggable `FileSystem`.
- `FileSystem` interface — 5 methods: `Mkdir`, `OpenFile`, `RemoveAll`, `Rename`, `Stat`.
- `LockSystem` interface — in-memory (`NewMemLS`) or custom implementation.
- Optional interfaces: `ContentTyper`, `ETager`, `DeadPropsHolder` on the `File` type.
- Built-in: `Dir` (wraps OS dir), `NewMemFS` (in-memory).

### Can we use the Handler for Cozy?

**No. The `Handler` is not usable as-is.** Three blockers:

**Blocker 1 — The `FileSystem` interface maps to POSIX semantics, not Cozy's VFS API.**

The interface requires `OpenFile(ctx, name, flag, perm)` returning a `webdav.File` (which is `io.ReadWriteSeeker` + `Readdir`). Cozy's VFS exposes `CreateFile(doc, olddoc)` returning a `vfs.File` (an `io.WriteCloser`), and `Open(doc)` returning an `io.ReadCloser`. These are not mechanically compatible. Bridging them requires writing a complete adapter that implements `Readdir`, `Seek`, `WriteAt`, and other `os.File`-like methods — work that is larger than writing the handlers directly.

**Blocker 2 — The `Handler` requires `LockSystem` even when locking is not implemented.**

Passing `NewMemLS()` would let the handler proceed, but the handler then emits `DAV: 1, 2` capabilities and accepts LOCK/UNLOCK requests, handling them in-memory with no persistence. This creates a silent false positive: the server claims to support locking, clients rely on it, and locks vanish on restart. The project scope explicitly excludes locking.

Passing `nil` causes a panic in `Handler.ServeHTTP`. There is no "no-locking" option.

**Blocker 3 — Known open bug: MOVE without Overwrite header returns 412.**

Issue #66059, opened March 2024, still open November 2025 (last checked): `handleCopyMove` uses `r.Header.Get("Overwrite") == "T"` for MOVE (returns `true` only when header is explicitly `"T"`), instead of the RFC 4918-correct `r.Header.Get("Overwrite") != "F"` (returns `true` when absent or `"T"`). macOS Finder does not send the Overwrite header. MOVE renames fail on Finder without this fix. The PITFALLS research confirmed: this bug is present as of April 2026 and there is no fix.

Also present: issue #66085 (WriteHeader called twice in PROPFIND), issue #44492 (PUT ignores If-* conditional headers), issue #44493 (ETags in `If` header conditions ignored). These are not critical blockers individually, but together they indicate the handler is not production-ready for a correctness-sensitive implementation.

### Can we use it partially (XML types only)?

Possibly, but there is little benefit. The XML types in `x/net/webdav` are internal to the package (not exported as public API structs). The exported surface is only the `Handler`, `FileSystem`, and `LockSystem` interfaces plus optional helper interfaces. The XML serialization/deserialization is not exposed as reusable components.

**Verdict: Use `x/net/webdav` for zero things.** The module is already a transitive dependency so it does not add to go.mod, but importing the webdav sub-package to use only its interfaces is not worth the coupling to a package with known open bugs.

---

## Deep Analysis: `emersion/go-webdav`

### What it provides

`emersion/go-webdav` v0.7.0 (Oct 2025) exposes:

- A `FileSystem` interface with 8 methods (see below), notably including `Copy` and `Move` as first-class operations — more appropriate for non-POSIX backends.
- `Handler` struct implementing `http.Handler`.
- `FileInfo` struct with `Path`, `Size`, `ModTime`, `IsDir`, `MIMEType`, `ETag`.
- `MoveOptions` / `CopyOptions` structs with `NoOverwrite` field.
- Context threading throughout — all methods accept `context.Context`.

```go
type FileSystem interface {
    Open(ctx context.Context, name string) (io.ReadCloser, error)
    Stat(ctx context.Context, name string) (*FileInfo, error)
    ReadDir(ctx context.Context, name string, recursive bool) ([]FileInfo, error)
    Create(ctx context.Context, name string, body io.ReadCloser, opts *CreateOptions) (*FileInfo, bool, error)
    RemoveAll(ctx context.Context, name string, opts *RemoveAllOptions) error
    Mkdir(ctx context.Context, name string) error
    Copy(ctx context.Context, name, dest string, options *CopyOptions) (bool, error)
    Move(ctx context.Context, name, dest string, options *MoveOptions) (bool, error)
}
```

### Is the interface a better fit for Cozy VFS?

Better than `x/net/webdav`, but still not a clean match:

- `Open` returning `io.ReadCloser` matches `vfs.Open(doc)` closely.
- `Create` receiving `io.ReadCloser` and returning `*FileInfo` does not match `vfs.CreateFile(newdoc, olddoc)` which returns a `vfs.File` (writer). Would need an adapter that buffers or pipes.
- `ReadDir` with `recursive bool` forces recursive listing into the interface. For Cozy, `Depth: infinity` is explicitly rejected (PITFALLS pitfall 1). The adapter would call `DirBatch` repeatedly for paged listing and ignore `recursive=true`.
- `Move` and `Copy` accept `*MoveOptions` with `NoOverwrite` — cleaner than x/net's opaque flag.

### Does emersion/go-webdav have the Overwrite bug?

The server.go passes the `overwrite` bool through `MoveOptions{NoOverwrite: !overwrite}` to the backend. The internal header parsing (in the `internal` package) could not be directly inspected, but the package passes `overwrite` as a parsed boolean — the question is what value it takes when the header is absent. Given the library is newer and more actively maintained than x/net/webdav, and the issue #66059 has been widely discussed, there is a reasonable expectation that emersion parses it correctly — but this is MEDIUM confidence (unverified against raw source). **Do not assume RFC compliance without writing a test.**

### Why not use emersion/go-webdav as the server Handler?

Three reasons:

1. **The adapter layer is still substantial.** 8 methods must be bridged from Cozy VFS semantics. For `Create`, you need an in-memory buffer or io.Pipe between `body io.ReadCloser` and `vfs.CreateFile`'s `io.WriteCloser` — non-trivial for large files. Writing the handlers directly avoids this impedance.

2. **The Handler owns the XML generation.** If we find an RFC compliance issue in emersion's XML (e.g., wrong ETag quoting, RFC 1123 date format, property namespace handling), we cannot fix it without forking the library. Writing our own `xml.go` gives us full control.

3. **Cozy has non-standard properties to expose.** The `pkg/webdav` client already parses `oc:fileid`, `nc:trashbin-filename`, `nc:trashbin-original-location` from Nextcloud responses. If we eventually want to expose Cozy-specific DAV properties (file ID, sharing state), a custom XML layer trivially supports this. emersion's handler requires implementing `DeadPropsHolder` and navigating its internal property dispatch.

**Verdict: Do not use emersion/go-webdav as the server layer.** It is a better-designed library than `x/net/webdav` and worth watching for a future locking implementation (v2). For now, write custom handlers.

---

## Recommended Architecture: Custom Handlers + stdlib XML

### Why custom handlers win

The WebDAV method surface for our scope (8 methods: OPTIONS, PROPFIND, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE) is manageable. The RFC 4918 XML for PROPFIND 207 Multi-Status is a small, well-defined schema. The known pitfalls (Overwrite default, ETag quoting, RFC 1123 dates, Content-Length, Depth:infinity rejection) are all one-line fixes that are invisible when hidden inside a third-party handler.

Implementing custom handlers means:

- Full control over every RFC compliance detail.
- No impedance mismatch: handlers call `inst.VFS()` directly using the exact API it exposes.
- No forking needed when bugs need fixing.
- Echo integration is trivial: `e.Match(webdavMethods, "/dav/files/*", handler)` using `echo.WrapHandler` or direct `echo.HandlerFunc`.
- The XML structs fit in ~100 lines of Go.

### Echo v4 Integration Pattern

Echo v4 (v4.15.1 in cozy-stack) supports arbitrary HTTP method registration via `e.Match()`. The pattern already established in `web/routing.go` for static assets uses `echo.WrapHandler`. For WebDAV, use `echo.HandlerFunc` directly to stay in the Echo context:

```go
// In web/routing.go — SetupRoutes():
webdavMethods := []string{
    "OPTIONS", "PROPFIND", "GET", "HEAD",
    "PUT", "DELETE", "MKCOL", "COPY", "MOVE",
}
dav := router.Group("/dav/files", middlewares.NeedInstance, webdav.ResolveWebDAVAuth)
dav.Match(webdavMethods, "", webdav.HandleRoot)
dav.Match(webdavMethods, "/*path", webdav.HandlePath)

// Nextcloud-compatible redirect
router.GET("/remote.php/webdav", func(c echo.Context) error {
    return c.Redirect(http.StatusMovedPermanently, "/dav/files")
})
router.Match(webdavMethods, "/remote.php/webdav/*path", func(c echo.Context) error {
    newPath := strings.Replace(c.Request().URL.RequestURI(), "/remote.php/webdav", "/dav/files", 1)
    return c.Redirect(http.StatusMovedPermanently, newPath)
})
```

Each handler is a standard `echo.HandlerFunc`:

```go
func HandlePath(c echo.Context) error {
    switch c.Request().Method {
    case "PROPFIND":
        return PropFind(c)
    case "GET", "HEAD":
        return GetHead(c)
    // ...
    }
    return echo.ErrMethodNotAllowed
}
```

No `x/net/webdav.Handler` intermediary. No `http.Handler` wrapping needed. Middleware chain (NeedInstance, auth) runs natively on Echo before the handler is reached.

### XML Implementation Plan

Define RFC 4918 structs in `web/webdav/xml.go`:

```go
// ~90 lines total for PROPFIND response XML
type Multistatus struct {
    XMLName   xml.Name   `xml:"DAV: multistatus"`
    Responses []Response `xml:"DAV: response"`
}

type Response struct {
    Href     string    `xml:"DAV: href"`
    Propstat []Propstat `xml:"DAV: propstat"`
}

type Propstat struct {
    Prop   Prop   `xml:"DAV: prop"`
    Status string `xml:"DAV: status"`
}

type Prop struct {
    ResourceType  *ResourceType `xml:"DAV: resourcetype,omitempty"`
    DisplayName   string        `xml:"DAV: displayname,omitempty"`
    GetLastModified string      `xml:"DAV: getlastmodified,omitempty"` // MUST use http.TimeFormat
    GetETag       string        `xml:"DAV: getetag,omitempty"`          // MUST be quoted: `"abc123"`
    GetContentLength int64      `xml:"DAV: getcontentlength,omitempty"`
    GetContentType string       `xml:"DAV: getcontenttype,omitempty"`
}

type ResourceType struct {
    Collection *struct{} `xml:"DAV: collection,omitempty"`
}
```

For PROPFIND request parsing:

```go
type PropFind struct {
    XMLName  xml.Name  `xml:"DAV: propfind"`
    AllProp  *struct{} `xml:"DAV: allprop"`
    PropName *struct{} `xml:"DAV: propname"`
    Prop     *Prop     `xml:"DAV: prop"`
}
```

Total XML surface: ~150 lines for a correct, tested implementation.

---

## Integration Tests: Client Library Recommendation

### Recommended: `studio-b12/gowebdav` v0.12.0

**Use for:** Black-box integration tests that exercise the server from the perspective of a real WebDAV client. Tests are written TDD-style: instantiate a `gowebdav.Client` against the `httptest.Server`, exercise `ReadDir`, `WriteStream`, `Rename`, `Copy`, `Remove`, then assert the VFS state directly via `inst.VFS()`.

```go
// In web/webdav/webdav_test.go
func TestPropfindDepth1(t *testing.T) {
    ts := setupTestServer(t) // spins up Echo + in-mem VFS
    c := gowebdav.NewAuthClient(ts.URL+"/dav/files", gowebdav.NewPreemptiveAuth(
        gowebdav.NewBearerAuth(testToken),
    ))
    files, err := c.ReadDir("/")
    require.NoError(t, err)
    assert.Len(t, files, expectedDirCount)
}
```

Installation: `go get github.com/studio-b12/gowebdav@v0.12.0`

Note: `gowebdav` does not yet have a stable Bearer token authenticator in v0.12.0 (work in progress per docs). Workaround: use `NewPreemptiveAuth` with a custom `Authorize` that sets `Authorization: Bearer <token>` directly, or use Basic Auth with the token as the password (which is the primary auth mode for Cozy anyway).

### Complement: `github.com/gavv/httpexpect/v2` (already in go.mod)

**Use for:** Tests that must assert exact HTTP-level behaviour that `gowebdav` abstracts away:

- Exact XML property values in PROPFIND responses (ETag format, date format, namespace).
- Depth header rejection (verify 403 on `Depth: infinity`).
- Conditional header tests (If-Match, If-None-Match exact status codes).
- OPTIONS response headers (`DAV: 1`, `Allow:` list).
- Content-Length presence on GET responses.

Already in go.mod at v2.16.0. No new dependency needed.

### Do NOT use: raw HTTP with custom XML (what `pkg/webdav` uses)

The existing `pkg/webdav` client is a custom HTTP client for talking to external Nextcloud servers. Do not copy its approach for the new server-side tests. It lacks Rename, Copy, Stat and the full method set. The `gowebdav` library is more complete and purpose-built for client-side testing.

### Do NOT use: emersion/go-webdav client

The emersion library does provide a WebDAV client, but it is focused on the CalDAV/CardDAV ecosystem. Its WebDAV client surface is narrower than `studio-b12/gowebdav` for the plain file operations we need. Adding it as a dependency for tests is not justified when `gowebdav` covers the same ground.

---

## Installation

```bash
# Test client only — no new server-side dependencies needed
go get github.com/studio-b12/gowebdav@v0.12.0
```

The server implementation uses only:
- `encoding/xml` — stdlib
- `net/http` — stdlib
- `path` — stdlib
- `net/url` — stdlib
- Echo v4 — already in go.mod
- `golang.org/x/net` — already in go.mod (no version bump needed; webdav sub-package unused)

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Custom handlers + stdlib XML | `golang.org/x/net/webdav` Handler | If Cozy's VFS were POSIX-compatible (`os.FileInfo`, `os.File`), and if the MOVE Overwrite bug and lock coupling were acceptable. Neither condition holds. |
| Custom handlers + stdlib XML | `emersion/go-webdav` Handler | If we needed CalDAV or CardDAV in the same codebase (it is the best choice for that), or if the project had a strictly POSIX-adjacent VFS. Revisit for a locking v2 phase. |
| Custom handlers + stdlib XML | Fork of `x/net/webdav` | If the intent was to upstream fixes. Not worth the maintenance burden for a private monolith. |
| `studio-b12/gowebdav` (test client) | `emersion/go-webdav` client | If the test suite needed CalDAV/CardDAV client operations in the same test binary. Not applicable here. |

---

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `golang.org/x/net/webdav.Handler` | LockSystem coupling, known MOVE Overwrite bug (#66059, open as of April 2026), POSIX-only FileSystem interface, WriteHeader-called-twice bug (#66085) | Custom `echo.HandlerFunc` handlers |
| `emersion/go-webdav` as server layer | Adapter boilerplate for 8-method VFS bridge, loss of XML control for custom properties, unverified Overwrite handling | Custom handlers + stdlib XML |
| CouchDB `_rev` as ETag | Semantically wrong: changes on metadata edits, not content changes; reveals internal structure | File content MD5 (`vfs.FileDoc.MD5Sum`) for files; hash of dir `updated_at` + `_rev` for directories |
| `time.RFC3339` for date properties | Breaks macOS Finder (requires RFC 1123 / `http.TimeFormat`) | `t.UTC().Format(http.TimeFormat)` — Go stdlib constant |
| Direct `x/net/webdav.NewMemLS()` for tests | Implicitly enables locking in Handler, creating false `DAV: 2` capability claim | No locking at all — set `DAV: 1` in OPTIONS response |

---

## Version Compatibility

| Package | Version | Compatibility Notes |
|---------|---------|---------------------|
| `golang.org/x/net` | v0.50.0 (current in go.mod) | The `webdav` sub-package is NOT imported by the implementation. No version bump needed. |
| `github.com/studio-b12/gowebdav` | v0.12.0 (Jan 2026) | New dependency, test only. Go 1.21+ required (satisfied by project's Go 1.25). |
| `github.com/labstack/echo/v4` | v4.15.1 | `e.Match()` for custom HTTP methods is available since Echo v4.5.0. No issue. |
| `github.com/gavv/httpexpect/v2` | v2.16.0 (already in go.mod) | No change needed. |

---

## Sources

- [pkg.go.dev/golang.org/x/net/webdav](https://pkg.go.dev/golang.org/x/net/webdav) — FileSystem interface, Handler struct, current version v0.52.0. HIGH confidence.
- [golang/go issue #66059](https://github.com/golang/go/issues/66059) — MOVE Overwrite header bug, open as of November 2025. HIGH confidence.
- [golang/go issue #66085](https://github.com/golang/net/blob/master/webdav/webdav.go) — WriteHeader called twice in PROPFIND. HIGH confidence (issue tracker).
- [golang/go issue #44492](https://github.com/golang/go/issues/44492) — PUT ignores If-* conditional headers. HIGH confidence.
- [golang/net webdav.go source](https://github.com/golang/net/blob/master/webdav/webdav.go) — Confirmed COPY uses `!= "F"`, MOVE uses `== "T"`. HIGH confidence.
- [pkg.go.dev/github.com/emersion/go-webdav](https://pkg.go.dev/github.com/emersion/go-webdav) — v0.7.0 Oct 2025, FileSystem interface shape. HIGH confidence.
- [deepwiki.com/emersion/go-webdav](https://deepwiki.com/emersion/go-webdav/2.1-webdav-server) — Server architecture, adapter pattern. MEDIUM confidence (secondary source).
- [pkg.go.dev/github.com/studio-b12/gowebdav](https://pkg.go.dev/github.com/studio-b12/gowebdav) — v0.12.0 Jan 2026, client methods, auth options. HIGH confidence.
- [Echo issue #1459](https://github.com/labstack/echo/issues/1459) — WebDAV method routing in Echo, `e.Match()` workaround confirmed. HIGH confidence.
- [cozy-stack go.mod](go.mod) — Confirmed `golang.org/x/net v0.50.0` already present, no `emersion/go-webdav` or `studio-b12/gowebdav` present. HIGH confidence (direct inspection).
- [pkg/webdav/webdav.go](pkg/webdav/webdav.go) — Existing custom WebDAV client used for Nextcloud integration. HIGH confidence (direct inspection).

---

*Stack research for: WebDAV server layer on cozy-stack (Go monolith)*
*Researched: 2026-04-04*
