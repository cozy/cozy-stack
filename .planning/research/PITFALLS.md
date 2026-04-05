# Pitfalls Research

**Domain:** WebDAV server over a non-filesystem backend (CouchDB metadata + Swift/local blob storage)
**Researched:** 2026-04-04
**Confidence:** HIGH (security/correctness/interop from official RFC + real CVEs + codebase audit), MEDIUM (client quirks from community reports), LOW (OnlyOffice mobile internals)

---

## Critical Pitfalls

### Pitfall 1 [SECURITY]: Infinite PROPFIND Depth DoS

**Category:** Security / Performance

**What goes wrong:**
A client sends `Depth: infinity` to PROPFIND. The server recursively traverses the entire VFS tree — every directory, every file — builds a 207 Multi-Status XML response in memory, and sends it all at once. On a Cozy instance with tens of thousands of files this consumes hundreds of megabytes and can take minutes. The request also holds a connection open the entire time. Multiple concurrent requests kill the stack process.

**Why it happens:**
RFC 4918 originally defined `Depth: infinity` as a valid value. Many implementations just pass it through to their recursive listing logic, not realising the VFS call underneath may touch thousands of CouchDB documents.

**How to avoid:**
- Reject `Depth: infinity` with `403 Forbidden` and an `Allow: 0, 1` note in the body. Apache does this by default (requires explicit `DavDepthInfinity On` to enable). This is explicitly called out in the Cozy PROJECT.md constraints.
- Log and alert on any request with `Depth: infinity` even after rejection — it signals misconfigured clients or probing.
- For `Depth: 1`, cap total items returned at a configurable limit (e.g., 1000) and return `507 Insufficient Storage` or a truncated response with a `DAV-warning` header rather than silently truncating.

**Warning signs:**
- Memory growth proportional to directory depth during PROPFIND.
- Go heap profiles showing large `encoding/xml` allocations from PROPFIND handler goroutines.
- Any test or integration suite that passes an empty `Depth` header and expects recursive results.

**Phase to address:** Phase 1 (PROPFIND implementation) — block `Depth: infinity` before any directory traversal code is written, not as a late hardening pass.

---

### Pitfall 2 [SECURITY]: Path Traversal via URL Encoding

**Category:** Security

**What goes wrong:**
A URL like `/dav/files/..%2F..%2Fetc%2Fpasswd` or `/dav/files/%2e%2e/secret` bypasses naive string prefix checks but decodes to a path outside `/files/`. In CVE-2023-39143, PaperCut's WebDAV servlet traversed outside its intended directory because paths were sanitised for forward slashes but not backslashes — and their servlet container (Jetty) allowed backslashes through. The CVSS score was 8.4.

**Why it happens:**
Developers clean the decoded path but forget that `net/http` (and Echo) may pass a partially-decoded or doubly-encoded URL. The `.BasePath + path` concatenation pattern in the existing `pkg/webdav` client is a reference — the server must not replicate it naively.

**How to avoid (Go-specific):**
- Use `path.Clean(u.Path)` after URL-decoding, and assert the result has the expected `/dav/files` prefix before passing to the VFS.
- Never construct CouchDB `fullpath` strings by concatenating URL path segments directly. Always pass through `vfs.FileByPath` or equivalent VFS function — it owns the namespace.
- Add a table-driven test covering: `../`, `%2e%2e/`, `%2e%2e%2f`, `%252e%252e`, and null bytes `%00`.
- Echo's `c.Param("*")` already decodes `%XX` sequences; double-encoding (`%252f`) must be explicitly rejected or normalised.

**Warning signs:**
- Handler code doing `strings.HasPrefix(rawPath, "/dav/files")` before URL decoding.
- Any path construction that concatenates user input with a separator directly.

**Phase to address:** Phase 1 — path normalisation must be the first thing the routing layer does, before any handler dispatches.

---

### Pitfall 3 [SECURITY]: Basic Auth Over Non-HTTPS (iOS Hard Requirement)

**Category:** Security / Interop

**What goes wrong:**
iOS enforces ATS (App Transport Security): connections to non-`localhost` servers without HTTPS are blocked at the OS level. The native iOS Files app and OnlyOffice mobile will silently fail to connect if the Cozy instance does not serve TLS. The risk here is not just security — it is a complete client failure that is opaque to the user.

Additionally, Basic Auth credentials travel in plaintext over HTTP. If a developer or test environment uses HTTP, app-specific passwords are exposed in every request.

**Why it happens:**
Developers test on `localhost` (which iOS and macOS exempt from ATS), so HTTP works during development. Staging and production reveal the failure without a clear error message.

**How to avoid:**
- Document in the WebDAV integration test plan: "iOS client tests MUST use an HTTPS origin, even if self-signed via `mkcert`."
- For Basic Auth, document in the project that app-specific passwords are only safe under TLS. Enforce at the server level: if the request is non-TLS AND not `localhost`, reject Basic Auth with `426 Upgrade Required`.
- In the OPTIONS response, only advertise Basic Auth schemes when the connection is TLS (inspect Echo's `c.IsTLS()` or the `X-Forwarded-Proto` header set by the Cozy router).

**Warning signs:**
- Integration tests run exclusively against `http://localhost` where iOS behaviour is not reproduced.
- `WWW-Authenticate: Basic` header returned on plaintext responses in staging logs.

**Phase to address:** Phase 1 (auth layer) — this must be decided before the first login test is written.

---

### Pitfall 4 [SECURITY]: OPTIONS Method Leaks Server Capabilities Without Auth

**Category:** Security

**What goes wrong:**
RFC 4918 requires that `OPTIONS *` (or `OPTIONS /dav/files`) return the `DAV:` and `Allow:` headers. Many implementations return this without authentication. This is intentional per spec — but in practice it leaks the server fingerprint (WebDAV class level, allowed methods), enables targeted probing, and in the historic IIS 6.0 vulnerability (CVE-2009-1535), a malformed OPTIONS request bypassed authentication entirely.

**Why it happens:**
Developers see that clients probe with unauthenticated OPTIONS at connection time and exempt it, not realising the security surface.

**How to avoid:**
- Allow unauthenticated OPTIONS for `/dav/files` only if the response is minimal: `DAV: 1`, `Allow: OPTIONS, PROPFIND, GET, PUT, DELETE, MKCOL, COPY, MOVE, HEAD`. Do NOT include resource-specific properties or directory listing in an OPTIONS response.
- All other methods must require authentication. In Echo middleware ordering: OPTIONS handler is excepted from the auth middleware, but the auth middleware runs for all others.
- Do not include `Server:` header revealing version or framework. The existing stack should already suppress this.

**Warning signs:**
- OPTIONS handler calling any VFS function (it should not touch the backend at all).
- OPTIONS returning `Lock-Token:` or similar session information.

**Phase to address:** Phase 1 (OPTIONS handler + middleware chain).

---

### Pitfall 5 [CORRECTNESS]: ETag Semantics — Weak vs. Strong, Quotes Required

**Category:** Correctness

**What goes wrong:**
RFC 4918 §8.8 requires strong ETags for WebDAV conditional operations. The CouchDB `_rev` field looks like an ETag but is not: it is not stable across COPY (the copy gets a new revision), is not content-addressed, and changes on metadata edits even if the file content is identical.

A second common mistake: ETags in HTTP headers must be quoted (`"abc123"`), not bare strings. Many implementations return the raw revision string without quotes. Clients either ignore it or fail `If-Match` checks silently.

A third variant: if the ETag is generated from the file content hash but the server returns the old ETag before the VFS finishes writing (i.e. the metadata is committed before the blob is stored in Swift), clients may cache a stale ETag for a file that is mid-upload.

**Why it happens:**
- CouchDB `_rev` is a natural candidate but has the wrong semantics.
- The existing `pkg/webdav` client already reads `ETag` without quoting normalisation — this creates a precedent the server side should not repeat.

**How to avoid:**
- Derive ETags from the file content MD5 or SHA (the VFS already stores this as `md5sum`). This is stable across metadata changes.
- Always return ETags as double-quoted strings: `"\"" + md5sum + "\""`.
- For directories, derive the ETag from a hash of the directory's `updated_at` timestamp + its `rev`. Directories have no content hash in the VFS.
- Do not expose CouchDB `_rev` as an ETag — it reveals internal document structure and changes on every metadata write.
- Write an explicit test: PUT a file, GET it, check ETag is quoted, overwrite with same content, check ETag is identical.

**Warning signs:**
- ETag values in responses without surrounding double-quotes.
- ETag changing after a metadata-only update (e.g., rename) when file content is unchanged.
- 412 Precondition Failed on a GET or HEAD where If-None-Match is sent with a previously returned ETag.

**Phase to address:** Phase 1 (GET/PUT/PROPFIND) — establish ETag strategy from the start.

---

### Pitfall 6 [CORRECTNESS]: If-Match / If-None-Match Handling Incorrectly Implemented or Ignored

**Category:** Correctness

**What goes wrong:**
RFC 4918 §8.8 and RFC 7232 define exactly when conditional headers should be checked and what responses to return:
- `If-Match: *` on PUT to a non-existent resource must return `412`.
- `If-None-Match: *` on PUT to an existing resource must return `412`.
- `If-Match: "etag"` where the current ETag doesn't match must return `412`.

Nextcloud bug #14428 and #37605 show that even mature WebDAV servers get this wrong — returning 412 when the ETag *does* match, or ignoring the header entirely. A server that ignores `If-None-Match: *` allows a client to silently overwrite existing files thinking it is creating new ones.

**Why it happens:**
Conditional headers are often treated as optional/advisory and the ETag comparison logic is implemented separately from the PUT handler, so they get out of sync.

**How to avoid:**
- Implement conditional header evaluation as a single middleware that runs before any mutating handler. It reads the current resource state from the VFS (one CouchDB lookup), evaluates all conditions, and short-circuits with 412 before any write happens.
- Test every precondition combination: `If-Match: *` (resource exists / not exists), `If-None-Match: *` (same), `If-Match: "correct-etag"`, `If-Match: "wrong-etag"`.

**Warning signs:**
- PUT handler that doesn't check for `If-Match` / `If-None-Match` headers before calling `vfs.CreateFile`.
- 200 OK (or 204) returned when `If-None-Match: *` is sent for an existing resource.

**Phase to address:** Phase 2 (PUT handler) — must be implemented alongside PUT, not deferred.

---

### Pitfall 7 [CORRECTNESS]: Date Format — RFC 1123 Required, Not RFC 3339

**Category:** Correctness

**What goes wrong:**
The `getlastmodified` DAV property and the `Last-Modified` HTTP header MUST use the RFC 1123 / RFC 7231 date format (`Mon, 02 Jan 2006 15:04:05 GMT`), not Go's default `time.RFC3339` (`2006-01-02T15:04:05Z07:00`). Some clients tolerate ISO 8601, but macOS Finder — per the sabre/dav Finder compatibility page — treats every `getlastmodified` value as UTC and misparses non-RFC1123 dates silently, resulting in incorrect modification times shown in Finder.

**Why it happens:**
Go's `time.Time.Format` defaults and CouchDB stores timestamps as RFC 3339. Developers copy the timestamp directly into the XML property.

**How to avoid (Go-specific):**
```go
// Correct: RFC 1123 with explicit GMT suffix
t.UTC().Format(http.TimeFormat)       // "Mon, 02 Jan 2006 15:04:05 GMT"

// Wrong: RFC 3339
t.Format(time.RFC3339)                 // "2006-01-02T15:04:05Z"
```
`http.TimeFormat` in the Go standard library is exactly RFC 1123 / RFC 7231 with a hard-coded `GMT` zone.

- Write a test asserting the `getlastmodified` property in every PROPFIND response matches the regexp `^\w{3}, \d{2} \w{3} \d{4} \d{2}:\d{2}:\d{2} GMT$`.

**Warning signs:**
- Any XML property builder using `time.RFC3339` or `time.RFC3339Nano`.
- Dates in PROPFIND responses ending in `Z` instead of `GMT`.

**Phase to address:** Phase 1 (PROPFIND XML response builder).

---

### Pitfall 8 [CORRECTNESS]: MOVE Without Overwrite Header Defaults to T

**Category:** Correctness / Interop

**What goes wrong:**
RFC 4918 §9.9.2: "If the Overwrite header is not included in a COPY or MOVE request, then the resource MUST treat the request as if it has an Overwrite header of value T." The macOS WebDAV client (Finder) does NOT include the `Overwrite` header during MOVE operations. If the server treats the missing header as `Overwrite: F`, renaming a file to overwrite an existing file fails silently.

This is a documented open bug in `golang.org/x/net/webdav` (issue #66059, opened March 2024, still open as of April 2026) with the exact check `r.Header.Get("Overwrite") == "T"` instead of the correct `r.Header.Get("Overwrite") != "F"`.

**Why it happens:**
The natural coding pattern is to check for an affirmative `"T"` value. The RFC requires treating absence as `"T"`.

**How to avoid:**
```go
overwrite := r.Header.Get("Overwrite")
allowOverwrite := overwrite != "F"  // "" and "T" both mean allow
```
- Add a test: MOVE a file to a destination that already exists, without an `Overwrite` header. The server MUST overwrite and return 204.

**Warning signs:**
- 412 Precondition Failed returned when Overwrite header is absent and destination exists.
- Finder renames appearing to fail without error on the client.

**Phase to address:** Phase 2 (MOVE handler) — the correct default must be baked in from the start.

---

### Pitfall 9 [INTEROP]: macOS Finder Requires Locking — Without It, Read-Only

**Category:** Interop / Client

**What goes wrong:**
macOS Finder's WebDAV implementation checks the server's `DAV:` header level. If the server only advertises `DAV: 1` (Class 1, no locking), Finder mounts the share in read-only mode. The user sees files but cannot create, edit, or delete anything, with no error message explaining why.

The Cozy project explicitly scopes out locking (LOCK/UNLOCK) for v1. This means Finder is read-only by design. This is acceptable, but must be documented and communicated to users.

**Why it happens:**
Developers implement all other operations, test with iOS Files or Cyberduck, and declare the implementation complete — without testing Finder, which has a unique locking requirement.

**How to avoid:**
- In the OPTIONS response, return `DAV: 1` only (no class 2). Do not claim class 2 without implementing locking.
- Add Finder to the "known limitations" section of the documentation.
- Consider: if Finder read-write is a future requirement, plan a locking v2 phase. A minimal in-memory lock store (no persistence) is enough for Finder because Finder's locks are short-lived per-operation.
- Finder also leaves `.DS_Store` and `._*` (resource fork) files everywhere. The DELETE handler should accept requests to delete these files without error, even if the VFS ignores them.

**Warning signs:**
- `DAV: 1, 2` in the OPTIONS response without LOCK/UNLOCK handlers implemented.
- Tests passing with Cyberduck or iOS Files but untested with macOS Finder.

**Phase to address:** Phase 1 (OPTIONS response) — lock the DAV level at class 1 from the start; document the Finder limitation explicitly.

---

### Pitfall 10 [INTEROP]: iOS Files App Requires HTTPS and Content-Length on Every Response

**Category:** Interop / Security

**What goes wrong:**
Two independent iOS requirements:
1. iOS ATS: connections to non-localhost hosts without TLS are blocked. The native Files app will fail to connect to an HTTP Cozy WebDAV endpoint with no user-readable error.
2. Finder and iOS HTTP/1.1 clients: `Content-Length` must be present on file GET responses. Without it, "really strange results" occur (per sabre/dav Finder compatibility docs). Echo can send chunked responses without `Content-Length` when the response size is not known.

**Why it happens:**
In Go/Echo, if you write to `c.Response()` without calling `c.Response().Header().Set("Content-Length", ...)` first, and the response is larger than the buffer, Echo uses `Transfer-Encoding: chunked`. This is correct HTTP/1.1 but breaks some WebDAV clients.

**How to avoid:**
- For file GET responses, always set `Content-Length` from the VFS metadata (`vfs.FileDoc.ByteSize`) before writing the body — even if the VFS returns an `io.Reader` for streaming.
- In test suites, always verify `Content-Length` is present and matches the actual body length for GET responses.
- For PROPFIND and other XML responses, the response is built in a `bytes.Buffer` before writing; set `Content-Length` from `buf.Len()` before `c.Response().WriteHeader(207)`.

**Warning signs:**
- Go `http.ResponseWriter` not having `Content-Length` set before the first `Write()` call.
- Echo handlers that call `c.Stream(200, mime, reader)` — this sends chunked without Content-Length.

**Phase to address:** Phase 1 (PROPFIND), Phase 2 (GET) — Content-Length policy must be in the handler template from the start.

---

### Pitfall 11 [INTEROP]: Trailing Slash Normalisation — MKCOL and Collection URLs

**Category:** Correctness / Interop

**What goes wrong:**
RFC 4918 recommends collection URLs have a trailing slash. Clients do not always send one. Nginx WebDAV returns `409 Conflict` for MKCOL without a trailing slash. The existing `pkg/webdav` client's `fixSlashes()` function adds trailing slashes before every request — the server-side must handle both forms.

The concrete failure mode: a client sends `MKCOL /dav/files/Documents` (no trailing slash). The server checks if `/dav/files/Documents/` exists (with trailing slash). If the normalisation is not done consistently, you can create a directory and then fail to PROPFIND it.

**Why it happens:**
Go's `path.Clean` removes trailing slashes. URL routing frameworks normalise paths. The VFS uses `fullpath` without trailing slash. These three representations can diverge.

**How to avoid:**
- Establish a single canonical rule: WebDAV collection URLs are internally represented *without* trailing slashes (matching the VFS `fullpath` convention). All incoming URLs are normalised by stripping trailing slashes before dispatching.
- Exception: `PROPFIND /dav/files/` (root) must work — handle the root path specially.
- Write a test matrix: `MKCOL /dav/files/foo`, `MKCOL /dav/files/foo/`, `PROPFIND /dav/files/foo`, `PROPFIND /dav/files/foo/` — all must be equivalent.

**Warning signs:**
- Separate code paths for paths with and without trailing slashes.
- Echo routes registered for `/dav/files/:path` (no trailing slash) but not `/dav/files/:path/`.

**Phase to address:** Phase 1 (routing setup).

---

### Pitfall 12 [PERFORMANCE]: PROPFIND on Large Directories — Blocking CouchDB Fetch + Full XML in Memory

**Category:** Performance

**What goes wrong:**
A PROPFIND `Depth: 1` on a directory with 10,000 files currently requires:
1. A CouchDB `_find` query to list all children (one round trip, returns all 10,000 documents).
2. Building a 207 Multi-Status XML response in memory, one `<response>` element per file.
3. Writing the complete response body to the client.

At step 1, the CouchDB query uses the `by_parent_id` index (if it exists) and returns all results at once. For 10,000 files at ~500 bytes per doc, that is 5 MB of CouchDB JSON, decoded into Go structs, then re-encoded as XML. Total memory per PROPFIND: ~15–20 MB. Ten concurrent PROPFIND requests on large directories: ~150–200 MB just for this handler.

**Why it happens:**
The VFS `DirByID` / `ListDir` functions return `[]vfs.DirOrFileDoc` slices. The WebDAV handler builds the XML response from this slice. No streaming, no pagination.

**How to avoid:**
- Cap `Depth: 1` PROPFIND at a configurable page size (default 1000). Return the page with a `DAV-max-items` header and a cursor for the next page (non-standard but tolerated by most clients that care).
- If the VFS provides a cursor-based listing API, use it; if not, add one. The `vfs.DirIterator` interface (if it exists) would be ideal.
- Stream the XML response using `xml.Encoder` writing directly to the response writer rather than building a `bytes.Buffer` first. This requires setting `Transfer-Encoding: chunked` (acceptable for PROPFIND since Content-Length is hard to know in advance for streaming XML).
- Add a Go benchmark test: `BenchmarkPropfindDepth1_1000Files` — track memory allocations per call.

**Warning signs:**
- Any code that calls `append(items, ...)` inside a PROPFIND handler loop without a size bound.
- XML built into a `strings.Builder` or `bytes.Buffer` from a slice of unbounded size.

**Phase to address:** Phase 1 (PROPFIND) — establish the memory model before integration tests with large datasets.

---

### Pitfall 13 [PERFORMANCE]: PUT Does Not Stream — Full File Buffered in Memory

**Category:** Performance

**What goes wrong:**
A common mistake in Go WebDAV handlers: the PUT body (`r.Body`) is read entirely into a `[]byte` or `bytes.Buffer` before being passed to the VFS `CreateFile`. For a 500 MB video upload, this consumes 500 MB of Go heap.

The VFS `CreateFile` function takes an `io.Reader`. The key is to pass `r.Body` directly as the reader — but the VFS may need the `Content-Length` upfront (for pre-allocating Swift segments or CouchDB metadata). If the client sends `Transfer-Encoding: chunked` without a `Content-Length`, the handler must detect this and either reject the request with `411 Length Required` or buffer to determine the size.

**Why it happens:**
The `ContentLength` field on `http.Request` is `-1` when chunked transfer encoding is used. Developers check `r.ContentLength > 0` and fall back to `io.ReadAll` for the `-1` case, inadvertently buffering.

**How to avoid:**
- Pass `r.Body` directly to `vfs.CreateFile` when `r.ContentLength >= 0`. The VFS can stream the body to Swift in chunks.
- When `r.ContentLength == -1` (chunked), write the body to a temporary file on disk to determine size, then pass the temp file reader and its size to `vfs.CreateFile`. This is how ownCloud handles it.
- Set `MaxBytesReader` on `r.Body` proportional to the user's quota before any read.
- Write a test: upload a file larger than the test process's soft memory limit and verify heap usage does not spike.

**Warning signs:**
- `io.ReadAll(r.Body)` anywhere in a PUT handler.
- `ioutil.ReadAll(r.Body)` (deprecated but still found in older code).

**Phase to address:** Phase 2 (PUT handler) — streaming must be designed in, not retrofitted.

---

### Pitfall 14 [INTEGRATION]: VFS MkdirAll Race Condition Under Concurrent WebDAV Clients

**Category:** Integration / Concurrency

**What goes wrong:**
`vfs.MkdirAll` (see `model/vfs/vfs.go:545`, flagged in CONCERNS.md) has no distributed lock. If two WebDAV clients simultaneously issue `MKCOL /dav/files/Photos/2024/` (e.g. two desktop sync clients), `MkdirAll` can create duplicate CouchDB documents for the same path.

**Why it happens:**
The VFS uses `os.IsExist` as a race fallback. Under concurrent requests, the check-then-create window is large enough for duplication. The CONCERNS.md audit explicitly flags this as a known gap.

**How to avoid:**
- For `MKCOL` in the WebDAV handler: use the CouchDB document conflict (409 from CouchDB) as the canonical "already exists" signal. If the VFS returns a conflict error on directory creation, treat it as `405 Method Not Allowed` (RFC 4918: "collection already exists").
- Do not rely on `vfs.MkdirAll` for WebDAV `MKCOL`. Issue a direct `vfs.Mkdir` (single directory creation) and let the RFC 4918 rule "intermediate parents must already exist → 409 Conflict" stand. This avoids the race and is spec-compliant.
- Write a concurrent test: 10 goroutines simultaneously `MKCOL /dav/files/race-test/` — only one should get 201, the rest 405.

**Warning signs:**
- WebDAV MKCOL handler calling `vfs.MkdirAll` rather than `vfs.Mkdir`.
- Absence of a race test for concurrent MKCOL.

**Phase to address:** Phase 2 (MKCOL handler).

---

### Pitfall 15 [INTEGRATION]: COPY Is a Full Download-and-Upload Over Swift

**Category:** Integration / Performance

**What goes wrong:**
The WebDAV COPY method copies a file to a new path. For a VFS backed by Swift object storage, there is no server-side copy operation available through the VFS abstraction. The naive implementation:
1. Downloads the file from Swift (streaming through Go heap).
2. Creates a new VFS document.
3. Uploads the file back to Swift.

For a 1 GB file this is 1 GB of network transfer through the Go process, not a server-side copy.

The `golang.org/x/net/webdav` issue #38974 explicitly documents this: the library uses `io.Copy` between source and destination, which on remote backends does a full round-trip.

**Why it happens:**
The VFS `FileSystem` interface does not expose a `CopyFile` method that can be optimised. The WebDAV handler has no choice but to fall back to read-then-write.

**How to avoid:**
- Check whether the Swift client library (`ncw/swift`) supports server-side copy (`X-Copy-From` header). It does — use it when both source and destination are in the same Swift container.
- If source and destination are in different containers (shouldn't happen in Cozy's single-tenant VFS), fall back to streaming copy.
- Add a server-side copy path to the VFS interface and implement it in the Swift backend before implementing COPY in the WebDAV handler. This is a VFS enhancement, not just a WebDAV handler concern.
- Document COPY performance limitations in v1 if Swift server-side copy is not implemented.

**Warning signs:**
- WebDAV COPY handler using `io.Copy` between a `vfs.File` reader and a `vfs.File` writer.
- No `CopyFile` optimisation in the VFS interface.

**Phase to address:** Phase 3 (COPY/MOVE) — assess Swift server-side copy feasibility before committing to the COPY handler design.

---

### Pitfall 16 [INTEGRATION]: VFS Move with CouchDB String-Prefix Path Matching

**Category:** Integration

**What goes wrong:**
`model/vfs/couchdb_indexer.go:391-402` (flagged in CONCERNS.md): directory moves use CouchDB fullpath string prefix matching. CouchDB collation returns false positives for paths that differ only in case (`/Photos/` vs `/PHOTOS/`). After a WebDAV MOVE of a directory, some children may not have their `fullpath` updated if the post-move filter misidentifies them.

**Why it happens:**
WebDAV MOVE of a collection requires updating `fullpath` for all descendant documents. The indexer does this via a CouchDB query that finds all docs with `fullpath` starting with the old path. The case-sensitivity bug is inherent to CouchDB's collation.

**How to avoid:**
- Do not implement directory MOVE yourself — use the existing `vfs.MoveDir` function exclusively, even if it is slow for large directories.
- Write a test: create `/PHOTOS/` and `/photos/` (if the VFS allows it), move `/PHOTOS/` to `/archive/`, verify `/photos/` children are unaffected.
- After a MOVE, issue a PROPFIND on the destination and the source to verify the old path returns 404 and the new path returns the correct children. Include this in the integration test suite.

**Warning signs:**
- Any WebDAV MOVE handler that manually updates `fullpath` fields instead of delegating entirely to the VFS.

**Phase to address:** Phase 3 (MOVE handler) — rely on VFS, add regression tests.

---

### Pitfall 17 [TESTING]: Brittle XML Comparison in PROPFIND Tests

**Category:** Testing

**What goes wrong:**
Testing a 207 Multi-Status XML response by comparing the raw string output is fragile. XML attribute order, namespace prefix choices (`d:` vs `DAV:`), whitespace, and element ordering can all vary without changing semantics. A test that does `assert.Equal(t, expectedXML, actualXML)` breaks whenever the XML renderer changes indentation or reorders attributes, creating false failures.

**Why it happens:**
The XML response body is the most visible output of a PROPFIND handler, so it is the natural thing to compare in tests.

**How to avoid:**
- Parse the 207 response into a struct (using `encoding/xml` with the same structs used by the `pkg/webdav` client — reuse them) and assert on the struct fields.
- Test specific semantic properties: "response contains an element with href `/dav/files/Documents/` and `resourcetype: collection`" rather than the whole response string.
- Use the `litmus` WebDAV compliance test suite (Go version: `net/webdav/litmus_test_server.go`) as an integration-level protocol compliance check, separate from unit tests.
- For XML namespace testing, assert that the `DAV:` namespace is correctly declared and that all required properties (`getlastmodified`, `getcontentlength`, `resourcetype`, `getetag`) are present.

**Warning signs:**
- Test code containing multi-line raw XML strings that are compared with `==` or `assert.Equal`.
- Test failures that appear after reformatting the XML encoder output.

**Phase to address:** Phase 1 — establish the XML testing pattern before writing the first PROPFIND test.

---

### Pitfall 18 [TESTING]: Locking Out the VFS in Unit Tests Hides Integration Bugs

**Category:** Testing

**What goes wrong:**
TDD discipline requires fast unit tests, which pushes teams toward mocking the VFS. A mock that always returns success for `CreateFile` will not reveal:
- The CouchDB conflict behaviour on concurrent writes.
- The Swift upload failure path.
- The `vfs.MkdirAll` race condition.
- The path prefix matching bug on MOVE.

The mock is effectively testing that the handler wires up correctly, not that the full stack behaves correctly.

**Why it happens:**
VFS integration tests are slow (require a running CouchDB + Swift), so they are deferred to CI. By the time the integration failure is discovered, the handler code is committed and patching it is expensive.

**How to avoid:**
- Use the in-memory VFS (`vfsafero`) for integration tests — it runs without CouchDB/Swift and exercises most VFS code paths. The existing test infrastructure already supports this.
- Separate test tiers:
  1. Unit tests (handler logic, XML serialisation, header parsing) — pure Go, no I/O.
  2. VFS integration tests (handler + afero VFS) — run in the same `go test` invocation.
  3. Protocol compliance tests (litmus or manual curl scripts against a test server) — run in CI.
- Never mock VFS in tests that are testing the full WebDAV handler. Only mock external dependencies (e.g., the blob upload endpoint).

**Warning signs:**
- `vfs_mock.go` or `mock_filesystem.go` files in the WebDAV handler test directory.
- Test suite that has 100% unit test coverage but zero integration test coverage.

**Phase to address:** All phases — the test architecture must be defined before the first handler is written (TDD strict).

---

### Pitfall 19 [INTEROP]: OnlyOffice Mobile Hardcodes `/remote.php/webdav` Path

**Category:** Interop / Client

**What goes wrong:**
OnlyOffice mobile (and many other Nextcloud-compatible clients) hardcode `/remote.php/webdav` as the WebDAV base path. The PROJECT.md notes a redirect from `/remote.php/webdav` to `/dav/files`. If this redirect is implemented as a `302 Found` (temporary redirect), some clients preserve the `POST` → `GET` downgrade (per HTTP spec) and WebDAV methods other than GET get downgraded silently.

**Why it happens:**
`302` is the default redirect in many frameworks. A `301 Permanent` redirect preserves the method for non-GET methods only in HTTP/1.1-compliant clients, but some clients still downgrade.

**How to avoid:**
- Use `308 Permanent Redirect` (not 301, not 302) for the `/remote.php/webdav` redirect. RFC 7538: 308 preserves the HTTP method and body on redirect, explicitly designed for this use case.
- Alternatively, mount the handler at BOTH paths (`/dav/files` and `/remote.php/webdav`) rather than redirecting — this avoids the redirect entirely.
- Test: send `PROPFIND /remote.php/webdav/` and verify the response is equivalent to `PROPFIND /dav/files/`.

**Warning signs:**
- `c.Redirect(http.StatusMovedPermanently, ...)` or `c.Redirect(http.StatusFound, ...)` in the compatibility route.
- Tests that only verify the redirect status code but not the redirected request method.

**Phase to address:** Phase 1 (routing setup).

---

### Pitfall 20 [CORRECTNESS]: XML Namespace Handling in PROPFIND Request Body

**Category:** Correctness

**What goes wrong:**
Clients send a PROPFIND request body with an XML namespace declaration. A common pattern:
```xml
<D:propfind xmlns:D="DAV:"><D:allprop/></D:propfind>
```
The prefix `D:` is arbitrary — RFC 4918 allows any prefix. Servers that do naive string matching on `"DAV:propfind"` (without namespace-aware parsing) will fail when a client uses a different prefix (e.g. `<dav:propfind xmlns:dav="DAV:">`).

A second issue: `allprop` means "return all properties", including dead properties (custom properties stored by clients). If the server does not support dead properties, it must return at least the live properties (`getlastmodified`, `getcontentlength`, `getetag`, `resourcetype`) and not return a 415 or 400.

**Why it happens:**
`encoding/xml` in Go handles namespace-aware parsing correctly if used with `xml.Name{Space: "DAV:", Local: "propfind"}`. But if the XML is parsed with simple `xml:"propfind"` struct tags, it does namespace-unaware matching.

**How to avoid:**
- Use `xml.Name{Space: "DAV:", Local: "..."}` struct tags in all WebDAV XML type definitions.
- Test with clients that use `xmlns:dav="DAV:"` prefix (non-standard `D:` prefix).
- For `allprop`: return all live properties. Log but silently ignore requests for dead properties in v1.

**Warning signs:**
- XML struct tags using `xml:"propfind"` without a namespace.
- Test XML bodies always using the same namespace prefix.

**Phase to address:** Phase 1 (PROPFIND request parsing).

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Using CouchDB `_rev` as ETag | No extra computation | Wrong semantics; changes on metadata edits; leaks internal structure | Never |
| Returning XML as a raw string from `fmt.Sprintf` | Fast to write | Namespace bugs; escaping bugs; untestable | Never |
| Mocking VFS in all tests | Fast unit tests | Hides integration bugs; see Pitfall 18 | Only for pure handler logic tests |
| `io.ReadAll(r.Body)` in PUT | Simple code | OOM for large files; see Pitfall 13 | Never |
| Reject PROPFIND with `allprop` as unimplemented | Avoids complexity | Breaks Finder, Cyberduck, and most sync clients | Never — must handle `allprop` |
| Implementing COPY as read-then-write | Simple VFS interface | 1 GB file = 1 GB RAM + double Swift I/O | Acceptable for v1 if documented and Swift server-side copy is planned for v2 |
| Serving DAV:1 and DAV:2 without locking | Clients try write ops | macOS Finder mounts read-only silently | `DAV:1` only is correct for v1 |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| CouchDB VFS | Calling `vfs.MkdirAll` for MKCOL | Call `vfs.Mkdir`; let RFC 4918 §9.3 require parents to exist |
| Swift blob storage | Streaming PUT via `io.ReadAll` | Pass `r.Body` directly to Swift upload with known `ContentLength` |
| Swift COPY | Read-then-write for file copy | Use Swift server-side copy (`X-Copy-From`) when source and dest are in same container |
| CouchDB directory MOVE | Manual `fullpath` update | Delegate entirely to `vfs.MoveDir`; never touch `fullpath` directly |
| CouchDB ETags | Expose `_rev` as ETag | Use MD5/SHA from VFS metadata, always double-quoted |
| Echo routing | Registering WebDAV methods as HTTP routes | Register custom HTTP methods with Echo's `router.Add()` or use a sub-handler with a manual method dispatch |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Unbounded PROPFIND Depth:1 | Memory spikes on large dirs | Cap at 1000 items, add pagination | Directories with >1000 files |
| `io.ReadAll` on PUT body | OOM on large uploads | Stream `r.Body` to Swift directly | Files larger than available heap (~500 MB) |
| COPY via full round-trip | High latency, double bandwidth | Swift server-side copy | Any file >10 MB |
| Full XML in `bytes.Buffer` for PROPFIND | Memory proportional to dir size | Stream `xml.Encoder` to response writer | Directories with >500 files |
| Unbounded `Depth: infinity` | CPU + memory DoS | Reject with 403 | Any directory with >100 files |
| CouchDB `_find` without index | PROPFIND Depth:1 slow | Ensure `by_parent_id` index exists; create if absent | Directories with >100 docs |

---

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| `Depth: infinity` not rejected | DoS — server goes OOM on large trees | Reject with 403 before any VFS call |
| Basic Auth over HTTP (non-localhost) | App-specific password exposed in plaintext | Enforce TLS at server middleware; reject non-TLS Basic Auth with 426 |
| OPTIONS handler touches VFS | Information disclosure + attack surface expansion | OPTIONS must return static headers only, no VFS calls |
| URL path not normalised before VFS dispatch | Path traversal (CVE-2023-39143 pattern) | `path.Clean` + prefix assert before every VFS call |
| Exposing CouchDB `_rev` as ETag | Internal document structure revealed | Use content hash (MD5/SHA) from VFS metadata |
| Serving `/files/` and app data under same WebDAV root | Data leakage — settings, connectors accessible | Enforce VFS root = `/files/` in the routing layer; never expose other doctypes |

---

## "Looks Done But Isn't" Checklist

- [ ] **PROPFIND:** Returns correct 207 with all required live properties — verify `getlastmodified` format is RFC 1123, not ISO 8601
- [ ] **PROPFIND:** Rejects `Depth: infinity` with 403 before any VFS call
- [ ] **PUT:** Streams body to Swift without buffering — verify with a file > 100 MB in integration test
- [ ] **PUT:** Handles `If-Match` and `If-None-Match` conditional headers
- [ ] **MOVE:** Treats missing `Overwrite` header as `Overwrite: T`
- [ ] **MKCOL:** Returns 409 when parent does not exist (not 500, not 404)
- [ ] **MKCOL:** Returns 405 when collection already exists (not 409)
- [ ] **OPTIONS:** Returns `DAV: 1` (not `DAV: 1, 2`), no locking methods in `Allow:`
- [ ] **OPTIONS:** Does not require authentication
- [ ] **ETags:** Always double-quoted, derived from content hash not `_rev`
- [ ] **Redirect:** `/remote.php/webdav` redirects with 308 (not 301 or 302) to preserve method
- [ ] **Auth:** Basic Auth rejected with 426 on non-TLS non-localhost connections
- [ ] **Finder:** `.DS_Store` and `._*` DELETE requests return 204 (not 404 or 500)
- [ ] **iOS Files:** Every GET response has `Content-Length` header set

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| ETags using `_rev` instead of content hash | HIGH | Re-derive ETags from VFS md5sum; clients will re-validate all cached files; breaking change |
| `Depth: infinity` not blocked (DoS discovered in prod) | MEDIUM | Add reject middleware with one deployment; restart after OOM |
| PUT buffering OOM in production | MEDIUM | Emergency memory cap via `MaxBytesReader`; streaming refactor in next sprint |
| MOVE Overwrite default bug (Finder renames broken) | LOW | One-line fix: `!= "F"` instead of `== "T"`; deploy hotfix |
| XML namespace parsing broken | MEDIUM | Fix struct tags, deploy; clients may need to retry failed requests |
| CouchDB `_rev` exposed in ETag (security) | MEDIUM | Rotate to content-hash ETag; document cache invalidation for clients |

---

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Infinite PROPFIND DoS | Phase 1 (PROPFIND) | Test: `Depth: infinity` returns 403 |
| Path traversal | Phase 1 (routing) | Table-driven test with encoded paths |
| Basic Auth over HTTP | Phase 1 (auth middleware) | Test: HTTP non-localhost returns 426 |
| OPTIONS auth bypass | Phase 1 (middleware chain) | Test: unauthenticated OPTIONS returns headers only |
| ETag semantics | Phase 1 (PROPFIND/GET) | Test: ETag quoted, stable on metadata change, changes on content change |
| If-Match / If-None-Match | Phase 2 (PUT) | Conditional request test matrix |
| Date format RFC 1123 | Phase 1 (PROPFIND XML builder) | Regexp test on `getlastmodified` value |
| MOVE Overwrite default | Phase 2/3 (MOVE) | Test: MOVE without header overwrites destination |
| Finder read-only (locking) | Phase 1 (OPTIONS) | Document limitation; test `DAV: 1` returned |
| iOS HTTPS requirement | Phase 1 (auth) | Note in test plan; integration test with HTTPS origin |
| Trailing slash normalisation | Phase 1 (routing) | Test matrix: slash/no-slash variants of MKCOL and PROPFIND |
| Large PROPFIND memory | Phase 1 (PROPFIND) | Benchmark: 1000-file dir, measure allocations |
| PUT streaming | Phase 2 (PUT) | Integration test: upload >100 MB, check heap |
| VFS MkdirAll race | Phase 2 (MKCOL) | Concurrent MKCOL test |
| COPY full round-trip | Phase 3 (COPY) | Assess Swift server-side copy; document v1 limitation |
| VFS MOVE path matching | Phase 3 (MOVE) | Post-MOVE PROPFIND regression test |
| Brittle XML tests | All phases | Establish struct-based assertion pattern before Phase 1 |
| VFS mock test isolation | All phases | Require afero VFS in handler integration tests |
| OnlyOffice `/remote.php/webdav` | Phase 1 (routing) | Test: PROPFIND via `/remote.php/webdav/` succeeds |
| XML namespace parsing | Phase 1 (PROPFIND) | Test: request body with non-`D:` prefix is parsed correctly |

---

## Sources

- [RFC 4918 — HTTP Extensions for WebDAV (IETF)](https://datatracker.ietf.org/doc/html/rfc4918) — authoritative specification
- [CVE-2023-39143 — PaperCut WebDAV path traversal (Horizon3.ai)](https://horizon3.ai/attack-research/disclosures/writeup-for-cve-2023-39143-papercut-webdav-vulnerability/) — real-world path traversal via backslash + WebDAV
- [sabre/dav Finder Compatibility Guide](https://sabre.io/dav/clients/finder/) — macOS Finder locking, Content-Length, character encoding quirks
- [golang/go issue #66059 — x/net/webdav MOVE missing Overwrite header defaults](https://github.com/golang/go/issues/66059) — open bug March 2024
- [golang/go issue #23871 — x/net/webdav Windows 7 Explorer interop failure](https://github.com/golang/go/issues/23871)
- [golang/go issue #38974 — x/net/webdav COPY inefficiency on remote filesystems](https://github.com/golang/go/issues/38974)
- [OnlyOffice community: iOS app 9.2 breaks WebDAV/Nextcloud](https://community.onlyoffice.com/t/ios-app-9-2-breaks-webdav-nextcloud-support/17415) — auth credential mismatch bug
- [Nextcloud issue #14428 — If-None-Match PreconditionFailed bug](https://github.com/nextcloud/server/issues/14428)
- [litmus — WebDAV server compliance test suite](http://www.webdav.org/neon/litmus/)
- [nginx trac #1966 — MKCOL refuses creation without trailing slash](https://trac.nginx.org/nginx/ticket/1966)
- [Apache mod_dav — DavDepthInfinity directive](https://httpd.apache.org/docs/2.4/mod/mod_dav.html)
- [ownCloud issue #29618 — 6 GB Swift/S3 upload via WebDAV](https://github.com/owncloud/core/issues/29618)
- [Cozy codebase CONCERNS.md — VFS MkdirAll race, couchdb_indexer path matching](/.planning/codebase/CONCERNS.md)
- [iOS ATS — HTTPS required for non-localhost WebDAV](https://forums.zotero.org/discussion/116306/ios-restrictions-must-use-https-to-connect-a-non-local-webdav-server)

---
*Pitfalls research for: WebDAV server over CouchDB/Swift VFS (cozy-stack)*
*Researched: 2026-04-04*
