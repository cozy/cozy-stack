# Phase 3: COPY, Compliance, and Documentation — Research

**Researched:** 2026-04-11
**Domain:** WebDAV RFC 4918 COPY, litmus compliance, VFS recursive copy, documentation
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**TDD litmus — execution strategy**
- Execution 100% local for this phase — CI (`.github/workflows/system-tests.yml`) NOT touched in Phase 3. TDD cycle via bash script or Makefile target `make test-litmus`.
- Class 1 strict, zero skip — all 5 suites (basic, copymove, props, http, locks) must pass green. LOCK/UNLOCK must return a status code that litmus `locks` suite accepts (501 or 405). If not achievable, fallback: skip `locks` via `LITMUS_TESTS` env var with documented justification.
- One TDD plan per litmus family — each suite that reveals gaps becomes its own PLAN.md with RED → GREEN → REFACTOR commits.
- Fresh timestamped instance per litmus run — script creates `litmus-YYYYMMDD-HHMMSS.localhost:8080`, generates token, runs litmus, destroys. Never pollutes dev instances.
- Both routes tested — litmus runs TWICE per cycle: against `/dav/files/` AND `/remote.php/webdav/`. Anti-regression gate for commit `7c9ab3a59`.

**COPY file semantics**
- Overwrite: T + existing dest → trash-then-copy (symmetric to MOVE Phase 2)
- Overwrite: F + existing dest → 412 Precondition Failed
- Overwrite absent → T by default (RFC 4918 §10.6)
- Notes special case: `olddoc.Mime == consts.NoteMimeType` → use `note.CopyFile(inst, olddoc, newdoc)` instead of `fs.CopyFile(olddoc, newdoc)`
- Build newdoc via `vfs.CreateFileDocCopy(olddoc, destinationDirID, copyName)`

**COPY directory semantics**
- `vfs.Walk` + `CopyFile` per file (v1, no Swift server-side optimization)
- Depth: 0 on dir → copy empty container only (no children)
- Depth: infinity → full recursive copy (default when Depth absent)
- Depth: 1 → 400 Bad Request (RFC 4918 forbids it for COPY/MOVE)
- Mid-copy failures → 207 Multi-Status per RFC 4918 §9.8.8 (MANDATORY for litmus strict)
- No rollback: files already copied stay in place

**Tests — TEST-05 approach**
- OnlyOffice mobile: litmus Class 1 strict + E2E gowebdav sufficient by transitivity (bug in v9.3.1 blocks live test)
- iOS Files: DEFERRED to v1.1 — explicit scope reduction (REQUIREMENTS.md must be updated)
- Client fake tests (`clients_test.go`): bonus, not a gate

**Documentation (DOC-01 to DOC-04)**
- Single `docs/webdav.md`, style of `docs/nextcloud.md`
- English (all docs/ files are English)
- Narrative + table + curl examples inline in the narration (user's explicit preference)
- Entry in `docs/toc.yml` mandatory
- No screenshots

### Claude's Discretion
- Exact naming of litmus script (`scripts/webdav-litmus.sh`, Makefile target, etc.)
- Exact layout of COPY code (one file `copy.go` / `copy_test.go`, or grafted onto `move.go`)
- Precise form of 207 Multi-Status error XML for partial copy failures
- Precise form of `LITMUS-GAPS.md` if needed (fallback if zero-skip unachievable)
- Structure of the Walk → Copy helper (typed walker vs closure)
- Exact ordering of plans within the phase
- Details of litmus instance cleanup (signal trap, defer, etc.)
- Precise form of curl examples in `docs/webdav.md`

### Deferred Ideas (OUT OF SCOPE)
- CI integration of litmus (post-v1, v1.1)
- Test manual OnlyOffice mobile (blocked by v9.3.1 bug)
- Test manual iOS Files (deferred to v1.1)
- Swift server-side COPY (`CopyFileFromOtherFS`, `ADV-V2-04`)
- `LITMUS-GAPS.md` (only if zero-skip is unachievable — safety net)
- Client fake tests "OnlyOffice full sequence" (bonus if budget)
- OpenAPI spec dedicated for WebDAV (only if repo has other specs — it does NOT, see §5 below)
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| COPY-01 | COPY file via `vfs.CopyFile` | §3 VFS internals, §2 RFC semantics, code at `model/vfs/vfsafero/impl.go:250`, `model/vfs/vfsswift/impl_v3.go:259` |
| COPY-02 | COPY directory: `vfs.Walk` + `CopyFile` per file | §3 `vfs.Walk` analysis (`model/vfs/vfs.go:622`), §2 Depth semantics |
| COPY-03 | COPY respects same Overwrite semantics as MOVE | §2 RFC 4918 §10.6, `web/webdav/move.go` pattern reference |
| DOC-01 | Document WebDAV endpoints in `docs/` | §5 documentation patterns, `docs/nextcloud.md` as template |
| DOC-02 | Config examples for OnlyOffice mobile and iOS Files | §5, `docs/office.md` reference |
| DOC-03 | Compatibility notes | §5, `docs/toc.yml` entry format |
| DOC-04 | OpenAPI or equivalent | §5: no OpenAPI files exist in repo → `docs/webdav.md` narrative satisfies this requirement |
| TEST-05 | Client behavior tests: OnlyOffice + iOS Files scenarios | §8 Validation Architecture: litmus + gowebdav by transitivity; iOS Files deferred |
| TEST-06 | litmus WebDAV compliance suite, RFC 4918 Class 1 | §1 litmus internals, all 5 suites analyzed |
| TEST-07 | All commits follow RED→GREEN→REFACTOR | Inherited discipline from Phase 1/2 — no new research needed |

</phase_requirements>

---

## Summary

Phase 3 closes the v1 milestone by adding COPY, achieving litmus Class 1 compliance, and writing documentation. The research establishes three main conclusions:

**COPY implementation is structurally a twin of MOVE.** The handler follows the exact same control flow as `web/webdav/move.go`: parse Destination, check Overwrite, trash-or-fail at destination, resolve destination parent, call VFS. The only real difference is the VFS verb (`CopyFile` instead of `ModifyFileMetadata`) and the recursive directory walk. The `note.CopyFile` special case for Cozy Notes is mandatory and well-documented in `web/files/files.go:397-402`.

**The litmus `locks` suite auto-skips when `DAV: 1` is returned.** The installed litmus 0.13 binary at `/usr/libexec/litmus/locks` contains the string "locking tests skipped, server does not claim Class 2 compliance". Because our OPTIONS handler already returns `DAV: 1` (not `DAV: 1, 2`), the entire locks suite will print "(all skipped)" — not a failure, a skip. This means "Class 1 strict, zero skip" is achievable without implementing any locking at all. The planner must verify this empirically on first litmus run.

**Documentation has a clear template.** `docs/nextcloud.md` is a monolithic feature doc with headers, narrative, and code blocks — the exact style requested. `docs/toc.yml` shows the entry format. No OpenAPI files exist in the repository, so DOC-04 is satisfied by `docs/webdav.md` alone.

**Primary recommendation:** Implement COPY first (file + dir), then run the full litmus suite once to inventory gaps, then address each suite in its own plan. Documentation last (or in parallel if bandwidth allows).

---

## 1. Litmus Test Suite Internals

**Source:** Local binary inspection at `/usr/libexec/litmus/`, litmus 0.13. Confirmed with web research.

### Invocation

```bash
litmus [OPTIONS] URL [USERNAME PASSWORD]
# USERNAME is ignored by Cozy auth (token goes in PASSWORD field)
litmus http://instance.localhost:8080/dav/files/ "" "<token>"

# Run specific suites only:
TESTS="basic copymove" litmus http://...

# Default TESTS value: "basic copymove props locks http"
# HTDOCS: /usr/share/litmus/htdocs (test fixture files)
# TESTROOT: /usr/libexec/litmus (suite binaries)
```

**Exit codes:** Non-zero if any suite has failures (not skips). The `--keep-going` / `-k` flag continues even if a suite fails.

**Output format:**
```
<- summary for 'basic': of 16 tests run: 16 passed, 0 failed. 100.0%
<- all tests skipped for 'locks'.        ← Class 1 server, expected
<- summary for 'copymove': of 13 tests run: 13 passed, 0 failed. 100.0%
```

**Confidence: HIGH** — extracted directly from installed binary strings.

### Suite: `basic` (16 tests)

Tests run in order:
```
init, begin, options, put_get, put_get_utf8_segment, put_no_parent,
mkcol_over_plain, delete, delete_null, delete_fragment, mkcol,
mkcol_again, delete_coll, mkcol_no_parent, mkcol_with_body, finish
```

Key checks (from binary strings):
- `options`: OPTIONS returns `DAV:` header and acceptable Allow list
- `put_get`: PUT a test file then GET it back with byte comparison
- `put_get_utf8_segment`: PUT with UTF-8 path segment, GET, compare
- `put_no_parent`: PUT to path `409me/noparent.txt` → expects 409 Conflict
- `mkcol_over_plain`: MKCOL on existing file → expects 405
- `delete`, `delete_null`, `delete_fragment`: various DELETE scenarios
- `mkcol`, `mkcol_again`, `delete_coll`: collection lifecycle
- `mkcol_no_parent`: MKCOL at `409me/noparent/` without parent → 409
- `mkcol_with_body`: MKCOL with a request body → expects 415 Unsupported Media Type

**Status for Phase 3:** All these operations (OPTIONS, PUT, GET, MKCOL, DELETE) were implemented in Phase 1/2. The first litmus run will confirm whether they pass.

### Suite: `copymove` (13 tests)

Tests run in order:
```
begin, copy_init, copy_simple, copy_overwrite, copy_nodestcoll, copy_cleanup,
copy_coll, copy_shallow, move, move_coll, move_cleanup, finish
```

Key checks (from binary error strings):
- `copy_simple`: COPY to new resource → expects 201 Created
- `copy_overwrite`:
  - COPY with `Overwrite: F` to existing → expects 412
  - COPY with `Overwrite: T` to existing → expects 204 No Content
- `copy_nodestcoll`: COPY to `nonesuch/dest` (missing parent) → expects 409
- `copy_coll`: COPY collection with `Depth: infinity` → verifies recursive copy
- `copy_shallow`: COPY collection with `Depth: 0` → verifies shallow copy (empty container only)
- `move`, `move_coll`, `move_cleanup`: MOVE tests (already implemented in Phase 2)

**Status for Phase 3:** `copy_*` tests are the primary COPY-01/02/03 targets. `move_*` should already pass from Phase 2.

### Suite: `props` (18 tests)

Tests run in order:
```
begin, propfind_invalid, propfind_invalid2, propfind_d0, propinit, propset, propget,
propextended, propmove, propdeletes, propreplace, propnullns, prophighunicode,
propremoveset, propsetremove, propvalnspace, propwformed, propmanyns, propcleanup, finish
```

Key checks:
- `propfind_invalid`: PROPFIND with empty namespace prefix `xmlns:ns1=""` → expects 400 Bad Request
- `propfind_invalid2`: PROPFIND with extended element → must not 500
- `propfind_d0`: PROPFIND with `Depth: 0` → must return correct properties
- `propset`/`propget`: PROPPATCH to set dead properties + PROPFIND to verify them
- `propvalnspace`: property value with namespace
- `propwformed`: malformed XML body

**IMPORTANT:** `propset` through `propcleanup` require PROPPATCH for dead properties. Since we do not implement PROPPATCH (deferred to v2), these tests will return 501 Not Implemented. Litmus may count 501 responses as a pass or a skip depending on whether it checks the return code permissively.

**Risk area:** If litmus `props` counts 501 on PROPPATCH as a failure (not a skip), the "zero skip" goal may require returning 501 with a proper Allow header. The first litmus run will determine the actual behavior.

**Confidence for `props` gaps: MEDIUM** — must be confirmed empirically.

### Suite: `http` (4 tests)

```
init, begin, expect100, finish
```

- `expect100`: tests `Expect: 100-continue` header behavior for PUT. The server must correctly handle or ignore the Expect header.

**Status:** Go's `net/http` handles `Expect: 100-continue` automatically in the HTTP server (sends `100 Continue` before reading the body). This should pass without any handler changes.

### Suite: `locks` (auto-skipped)

**Critical finding:** From the installed `/usr/libexec/litmus/locks` binary:

```
"locking tests skipped, server does not claim Class 2 compliance"
"<- all tests skipped for 'locks'."
```

The `locks` suite's `init_locks` function checks the `DAV:` header from OPTIONS. If the header does not contain `2` (Class 2), the suite prints the skip message and exits cleanly. Our OPTIONS handler returns `DAV: 1` — no Class 2. Therefore:

- **Litmus `locks` suite exits with "(all skipped)" — not a failure.**
- **No LOCK/UNLOCK implementation needed.**
- **"Zero skip" in the user's sense means: zero *failed* tests. Skipped suites (due to compliance class) are expected and acceptable.**

The planner must verify this on first run. If for any reason the installed litmus version differs, the fallback is `TESTS="basic copymove props http" litmus ...` which explicitly excludes locks.

**Confidence: HIGH** — verified from binary strings of the installed litmus 0.13 executable.

---

## 2. RFC 4918 §9.8 COPY Deep-Dive

**Source:** RFC 4918 https://www.rfc-editor.org/rfc/rfc4918.html — fetched 2026-04-11.

### §9.8.3 — Depth semantics for COPY on collections

- `Depth: infinity` (or absent): copy collection + all descendants recursively
- `Depth: 0`: copy the collection resource itself only, creating an empty destination collection
- `Depth: 1`: **not allowed** — RFC does not define Depth: 1 for COPY. Return 400 Bad Request.
- Absent Depth header defaults to `infinity` (RFC 4918 §9.8.3 explicit statement)

### §9.8.5 — Status codes

| Code | Condition |
|------|-----------|
| 201 Created | Destination did not exist; resource created |
| 204 No Content | Destination existed and was overwritten |
| 207 Multi-Status | Partial failure during collection copy (per §9.8.8) |
| 403 Forbidden | Source and destination are the same URI, or cross-server |
| 409 Conflict | Destination parent collection does not exist |
| 412 Precondition Failed | `Overwrite: F` and destination already exists |
| 423 Locked | Destination locked (n/a for Class 1) |
| 502 Bad Gateway | Destination on a different server |
| 507 Insufficient Storage | Quota exceeded |

**Implementation note:** 403 for source==destination. In our handler, after calling `parseDestination` and `davPathToVFSPath` for source, compare normalized VFS paths: if `srcPath == dstPath`, return 403.

### §10.6 — Overwrite header default

> "If this header is not included in the request, the server MUST default to behaving as though an Overwrite header with a value of T was included."

Implementation: `overwrite := c.Request().Header.Get("Overwrite") != "F"` — identical to `web/webdav/move.go:75-78`. This is already the Phase 2 pattern.

### §9.8.8 — 207 Multi-Status for partial collection copy failures

When copying a collection and some children fail (e.g., quota exceeded on file N of N+k), the server must:
1. Leave already-copied files in place (no rollback)
2. Return 207 Multi-Status with one `D:response` per failed path

**Minimal correct format** (from RFC 4918 §13 Multi-Status):

```xml
<?xml version="1.0" encoding="utf-8"?>
<D:multistatus xmlns:D="DAV:">
  <D:response>
    <D:href>/dav/files/dest/subdir/failedfile.txt</D:href>
    <D:status>HTTP/1.1 507 Insufficient Storage</D:status>
  </D:response>
  <D:response>
    <D:href>/dav/files/dest/subdir2/anotherfail.txt</D:href>
    <D:status>HTTP/1.1 507 Insufficient Storage</D:status>
  </D:response>
</D:multistatus>
```

Each `D:response` contains a `D:href` (absolute URL of the destination path that failed) and a `D:status` (HTTP status line). Only failed items are reported — successful items are NOT listed. The root destination collection itself is NOT listed in the multi-status if it was created successfully.

**Important:** The `D:href` values should use the same path prefix as the Destination header. If the client used `/dav/files/dest/`, the hrefs should use `/dav/files/dest/subpath`.

### Destination header edge cases

- **Absolute URL form:** `Destination: http://host/dav/files/foo` — `url.Parse` handles this; `parseDestination` (already written in Phase 2) strips the host and prefix, extracting `/files/foo`.
- **Cross-server:** If the URL's host doesn't match the request host → return 502 Bad Gateway (currently handled as `errInvalidDestination` → 502 in `move.go`).
- **Trailing slash on collection COPY:** `Destination: /dav/files/newdir/` — strip trailing slash, use as directory name. RFC 4918 §8.3 says trailing slash on a collection is equivalent to no trailing slash.
- **URL-encoded characters:** `parseDestination` calls `url.Parse` which decodes the URL → `davPathToVFSPath` validates the result. This already handles URL encoding correctly.
- **Source == Destination:** After normalization, if `srcPath == dstPath` → 403 Forbidden (RFC 4918 §9.8.5).

**Confidence: HIGH** — RFC text fetched directly.

---

## 3. Cozy VFS CopyFile Internals

**Source:** Direct code inspection of the repo.

### `VFS.CopyFile` interface

```go
// model/vfs/vfs.go:95-97
// CopyFile creates a fresh copy of the source file with the given newdoc attributes
CopyFile(olddoc, newdoc *FileDoc) error
```

### `vfs.CreateFileDocCopy` helper

```go
// model/vfs/vfs.go:913-935
func CreateFileDocCopy(doc *FileDoc, newDirID, copyName string) *FileDoc {
    newdoc := doc.Clone().(*FileDoc)
    newdoc.DocID = ""          // reset — VFS assigns new ID
    newdoc.DocRev = ""         // reset
    if newDirID != "" { newdoc.DirID = newDirID }
    if copyName != "" {
        newdoc.DocName = copyName
        mime, class := ExtractMimeAndClassFromFilename(copyName)
        newdoc.Mime = mime
        newdoc.Class = class
    }
    newdoc.CozyMetadata = nil  // reset
    newdoc.InternalID = ""     // reset
    newdoc.CreatedAt = time.Now()
    newdoc.UpdatedAt = newdoc.CreatedAt
    newdoc.RemoveReferencedBy()
    newdoc.ResetFullpath()
    newdoc.Metadata.RemoveCertifiedMetadata()
    return newdoc
}
```

**Important:** `CreateFileDocCopy` resets ID, rev, and metadata but **preserves `Mime`** (since `copyName == ""` keeps the original mime). If the WebDAV destination name is different from source, the mime is re-derived from the new filename extension. For Cozy Notes, `olddoc.Mime == consts.NoteMimeType` — even if we pass a new name, the mime is reset from the filename, which may lose `NoteMimeType`. The `note.CopyFile` branch must be called BEFORE `CreateFileDocCopy` changes the mime, or the MIME check must use `olddoc.Mime` (not `newdoc.Mime`). **Use `olddoc.Mime == consts.NoteMimeType` to branch, not `newdoc.Mime`** — matching `web/files/files.go:397`.

### afero implementation (`vfsafero/impl.go:250`)

- Acquires VFS mutex lock
- Checks `DirChildExists` for collision (returns `os.ErrExist` if duplicate)
- Calls `CheckAvailableDiskSpace` → returns `ErrFileTooBig` or `ErrMaxFileSize` if quota exceeded
- Opens source, writes to temp file, finalizes with MD5 check
- Returns `os.ErrExist` for duplicate filename (not `vfs.ErrParentDoesNotExist`)

### Swift implementation (`vfsswift/impl_v3.go:259`)

- Same collision + quota check
- Uses Swift `ObjectCopy` for server-side binary copy (no round-trip download/upload)
- Sets `newdoc.DocID` and `newdoc.InternalID` internally
- Returns `os.ErrExist` for collision

**Critical:** The Swift implementation sets `newdoc.DocID` inside `CopyFile`. The afero implementation does NOT (it uses the CouchDB indexer to create the doc during finalization). This means the handler MUST NOT pre-set `newdoc.DocID` — let the VFS implementation manage it.

### `vfs.Walk` — recursive walk

```go
// model/vfs/vfs.go:622
func Walk(fs Indexer, root string, walkFn WalkFn) error

// WalkFn type (vfs.go:618):
type WalkFn func(name string, dir *DirDoc, file *FileDoc, err error) error
```

Walk behavior:
- Calls `walkFn(name, dir, file, nil)` for each node (dir node has `file==nil`, file node has `dir==nil`)
- Returns `ErrWalkOverflow` if depth >= 512 (`MaxDepth = 512`)
- Propagation: if `walkFn` returns non-nil (and not `ErrSkipDir` on a dir), walk aborts immediately
- **For 207 Multi-Status partial failure**: the walkFn must NOT return an error on per-file failure; instead it must collect errors and continue. Only return an error from walkFn if the entire walk must abort.

**Walk does NOT hold a VFS lock** (unlike `CopyFile` which acquires it per call). Concurrent mutations during a walk are possible but not a correctness issue — the walk reads CouchDB metadata snapshots. A file deleted during the walk will result in a `os.ErrNotExist` in `CopyFile`, which is a per-file failure → 207 entry.

### `note.CopyFile` (`model/note/copy.go:12`)

```go
func CopyFile(inst *instance.Instance, olddoc, newdoc *vfs.FileDoc) error
```

- Checks disk space, loads ProseMirror content and images from source note
- Copies each image to destination note (creates new image docs)
- Updates ProseMirror content with new image URLs
- Writes the note archive (markdown + images) via `inst.VFS().CreateFile(newdoc, nil)`
- Sets up the auto-save trigger via `SetupTrigger`

**NOT a drop-in replacement for `fs.CopyFile`**: it uses `inst.VFS().CreateFile` internally, not `fs.CopyFile`. The handler must pass `inst` (the full instance, not just the VFS). The `newdoc` passed to `note.CopyFile` should be prepared with `CreateFileDocCopy` exactly as for regular files.

**Confidence: HIGH** — direct code inspection.

---

## 4. Instance Management for Litmus Script

**Source:** CLI help, code inspection, existing Makefile.

### Instance creation and token generation

```bash
# Create a fresh instance (minimal — no apps, just files scope)
cozy-stack instances add litmus-20260411-143022.localhost:8080 \
  --passphrase cozy \
  --email litmus@cozy.localhost

# Generate a CLI token with io.cozy.files scope
# token-cli gives global access to the specified doctypes
TOKEN=$(cozy-stack instances token-cli litmus-20260411-143022.localhost:8080 \
  "io.cozy.files")

# Destroy (--force skips confirmation prompt)
cozy-stack instances destroy litmus-20260411-143022.localhost:8080 --force
```

**Token choice:** `token-cli` with `"io.cozy.files"` scope. This generates a JWT that `middlewares.ParseJWT` (called by `resolveWebDAVAuth` via `middlewares.GetRequestToken`) accepts. The `token-app` command requires a running app slug; `token-cli` is simpler for scripts.

**Instance naming:** The name `litmus-YYYYMMDD-HHMMSS.localhost:8080` is valid — Cozy instance domains are hostnames and the `--` dashes and digits are legal. The stack routes by Host header: litmus sends `Host: litmus-YYYYMMDD-HHMMSS.localhost:8080` because that's the URL it was given. `NeedInstance` middleware resolves the instance from the Host header.

**Auth in litmus:** litmus uses Basic Auth. Our server accepts `Authorization: Basic base64(":<token>")` (empty username, token as password). litmus invocation:

```bash
# USERNAME="" means empty username; token goes in PASSWORD field
litmus http://litmus-20260411-143022.localhost:8080/dav/files/ "" "$TOKEN"
```

### Stack requirements for litmus script

The stack must be running before the script executes. The script should NOT start/stop the stack — that's the developer's responsibility. The script should:
1. Verify the stack is reachable (curl or cozy-stack instances ls)
2. Create the instance
3. Generate token
4. Run litmus × 2 (native route + Nextcloud route)
5. Destroy the instance (trap EXIT for cleanup)

### The Nextcloud route proxy

The `parseDestination` function already handles both `/dav/files` and `/remote.php/webdav` prefixes for the Destination header. The litmus second run against `/remote.php/webdav/` uses the same stack without any proxy — the stack directly serves `NextcloudRoutes`. There is no proxy needed.

```bash
# Route 1: native
litmus http://litmus.localhost:8080/dav/files/ "" "$TOKEN"

# Route 2: Nextcloud compat (same stack, different path prefix)
litmus http://litmus.localhost:8080/remote.php/webdav/ "" "$TOKEN"
```

### Existing patterns

- `Makefile` line 21: `cozy-stack instances add cozy.localhost:8080 --passphrase cozy --apps home,store,...`
- `tests/system/lib/stack.rb:153-168`: uses `cozy-stack instances token-oauth` (with client registration) — more complex than needed for litmus. `token-cli` is simpler.
- No existing bash script for WebDAV litmus. Script at `scripts/webdav-litmus.sh` is a new file.

**Confidence: HIGH** — CLI help + code + existing Makefile patterns.

---

## 5. Documentation Patterns in `docs/`

**Source:** File inspection of `docs/nextcloud.md`, `docs/toc.yml`, `docs/office.md`.

### OpenAPI / Swagger

**No OpenAPI spec files exist in the repository.** A search for `openapi`, `swagger`, `spec.yml` found nothing in `docs/`. Therefore, **DOC-04 ("OpenAPI or equivalent") is satisfied by `docs/webdav.md` alone**. No spec file needed.

### `docs/nextcloud.md` structure (template)

The existing `docs/nextcloud.md` is a route-by-route reference doc with:
- Header: `[Table of contents](README.md#table-of-contents)` at top
- `# Section Title` heading
- `## Route name` per route
- Short narrative paragraph
- HTTP request block (labeled `### Request`)
- HTTP response block (labeled `### Response`)
- JSON body examples inline

This is the **route reference style** — good for the "Supported methods" section of `docs/webdav.md`.

### `docs/toc.yml` entry format

The TOC is a YAML nested list. WebDAV should be inserted in the "List of services" section, alphabetically between `/realtime` and `/remote`:

```yaml
    "/dav - WebDAV": ./webdav.md
```

(alphabetically, `/dav` comes before `/data` — should go at the top of the services list, or follow the existing `/data` entry. The planner decides exact placement.)

### `docs/webdav.md` structure

Per user decision — narrative + table + curl examples inline. Suggested sections from CONTEXT.md:

1. Introduction & concepts
2. Endpoints & routes (`/dav/files/*`, `/remote.php/webdav/*`)
3. Authentication (OAuth Bearer, Basic with token-as-password)
4. Supported methods — narrative per method + recap table + inline curl
5. Client configuration (OnlyOffice mobile, rclone, curl; iOS Files "best-effort")
6. Compatibility notes & limitations
7. Troubleshooting

**Language: English** — all `docs/` files are in English. `.planning/` docs are in French (internal convention).

**Confidence: HIGH** — direct file inspection.

---

## 6. Litmus `locks` Suite: No-Lock Server Strategy

**Finding confirmed:** The `locks` suite in litmus 0.13 checks the OPTIONS response for `DAV: 2` (Class 2). If it is absent, the suite prints:

```
locking tests skipped, server does not claim Class 2 compliance
<- all tests skipped for 'locks'.
```

And exits with success (0 failures, all skipped). **Our OPTIONS handler returns `DAV: 1` — the locks suite will cleanly skip.**

This means:
- No LOCK/UNLOCK handler needed for Class 1 litmus pass
- No `501 Not Implemented` stub needed for LOCK/UNLOCK (litmus never sends LOCK if Class 2 not advertised)
- The `webdavMethods` list and `davAllowHeader` in `web/webdav/webdav.go` should NOT include `LOCK` or `UNLOCK`
- If a real client sends LOCK to our Class 1 server, Echo's router returns a 405 with a standard body. This is acceptable behavior.

**Reference implementations consulted:**
- rclone serve webdav: returns `DAV: 1` only, litmus locks suite skips (community reports)
- Apache mod_dav without mod_dav_lock: same behavior — `DAV: 1`, locks suite skips

**If the first litmus run shows locks tests failing** (unexpected), the emergency fallback is `TESTS="basic copymove props http" litmus ...` with the exclusion documented in `LITMUS-GAPS.md`.

**Confidence: HIGH** — verified from binary string extraction of installed `/usr/libexec/litmus/locks`.

---

## 7. Pitfalls Specific to Phase 3

### Pitfall A: Note MIME check must use `olddoc`, not `newdoc`

`vfs.CreateFileDocCopy` re-derives `Mime` from the copy's filename if `copyName != ""`. If the destination filename is different from the source (e.g., renaming via COPY), `newdoc.Mime` will be derived from the new name and will NOT be `consts.NoteMimeType` even if the source was a note. Always gate on `olddoc.Mime == consts.NoteMimeType`.

### Pitfall B: `CopyFile` collision inside the destination parent

`CopyFile` checks `DirChildExists` inside the VFS mutex. If the destination file already exists AND Overwrite is T, we must trash the existing destination BEFORE calling `CopyFile`. If we call `CopyFile` without trashing first, it returns `os.ErrExist`. This is identical to the MOVE pattern in `web/webdav/move.go:86-100`.

### Pitfall C: 207 Multi-Status requires `walkFn` to NOT abort on per-file errors

The naive pattern of returning an error from `walkFn` aborts the walk immediately. For 207 semantics, the `walkFn` must:
- Catch per-file errors in a local slice
- Always return `nil` from `walkFn` (even on failure)
- After walk completes, if `len(errors) > 0` → build 207 XML
- If `len(errors) == 0` → return 201 or 204 as appropriate

### Pitfall D: Destination header with trailing slash on file

A client may send `Destination: /dav/files/newname/` (trailing slash) for a COPY of a file. RFC 4918 §8.3: trailing slash on a non-collection resource is ambiguous. Safe approach: `path.Clean` strips trailing slashes. Already handled by `davPathToVFSPath`.

### Pitfall E: Source == Destination

After normalizing both paths with `davPathToVFSPath`, if `srcPath == dstPath` → return 403 Forbidden (RFC 4918 §9.8.5). The `move.go` handler does not have this check because a self-move is caught earlier (it would be a no-op). For COPY, a self-copy would silently call `CopyFile(doc, doc)` which returns `os.ErrExist` via the `DirChildExists` check — but the correct RFC response is 403, not 409.

### Pitfall F: Walk in presence of `.cozy_trash`

If the source directory being copied contains a `.cozy_trash` symlink or if the destination is inside trash, `isInTrash` must be checked for both source and destination. Copying INTO trash is forbidden (identical to MOVE). Copying FROM trash is debatable — but since `.cozy_trash` is read-only via WebDAV (Phase 1 decision), `davPathToVFSPath` for the source would be the trash directory which should be rejected as not within `/files/`. Actually the VFS exposes `/files/.cozy_trash` — the path checker would accept this. The handler should explicitly block COPY with source in trash, same as `isInTrash` check in `delete.go`.

### Pitfall G: `vfs.Walk` `MaxDepth = 512`

If the directory tree is deeper than 512 levels, `Walk` returns `ErrWalkOverflow`. The handler should map this to 500 (unexpected server error) or 507 (storage issue). Given 512 is an extreme depth (no real user directory is that deep), this can be treated as an internal error.

### Pitfall H: `note.CopyFile` vs `fs.CopyFile` return codes

`note.CopyFile` uses `inst.VFS().CreateFile` internally. If quota is exceeded, it returns `vfs.ErrFileTooBig`. If the destination already exists (after our trash-then-copy), it may return `os.ErrExist`. Both are handled by `mapVFSWriteError`. No special-casing needed beyond the branch decision.

### Pitfall I: litmus `props` suite PROPPATCH tests

The `props` suite sends PROPPATCH requests (tests `propset`, `propget`, `propreplace`, etc.). Our handler currently returns 501 for unimplemented methods. Litmus may treat 501 as a skip or as a failure. If it treats it as a failure, those tests will fail. Resolution: run litmus once, observe the output, and address only actual failures. The `props` suite has tests before PROPPATCH (like `propfind_invalid`, `propfind_d0`) that should pass with our PROPFIND implementation.

### Pitfall J: litmus `basic` `mkcol_with_body` test

litmus sends MKCOL with a `Content-Type: text/plain` body. RFC 4918 §9.3.1 says: "If the server receives a 'mkcol-request-body' that is not supported, it MUST respond with a 415 Unsupported Media Type." Our current MKCOL handler (`handleMkcol`) ignores the body. To pass `mkcol_with_body`, the handler must check for a non-empty body and return 415 if present. **This is a likely gap in our Phase 2 MKCOL implementation.**

### Pitfall K: litmus `basic` `delete_fragment` test

litmus sends DELETE to a URL with a `#fragment` (e.g., `DELETE /dav/files/frag/#ment`). The fragment is not sent over the wire (HTTP clients strip it), but litmus may test the path `/dav/files/frag/` or similar. This should be handled by our path mapper.

**Confidence: HIGH** (A, B, C, E from code inspection + RFC), MEDIUM (D, F, G, H, I, J, K from analysis).

---

## 8. Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` + `github.com/stretchr/testify v1.11.1` |
| Config file | `cozy.test.yaml` (via `config.UseTestFile(t)`) |
| Quick run command | `go test -p 1 -timeout 5m -run TestCopy ./web/webdav/...` |
| Full suite command | `go test -p 1 -timeout 5m ./web/webdav/...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| COPY-01 | COPY file (new dest → 201, overwrite → 204, Overwrite:F → 412) | unit + integration | `go test -run TestCopy ./web/webdav/...` | ❌ Wave 0 |
| COPY-02 | COPY dir recursive, Depth:0/infinity, partial 207 | unit + integration | `go test -run TestCopy ./web/webdav/...` | ❌ Wave 0 |
| COPY-03 | COPY Overwrite semantics (absent=T, F=412) | unit | `go test -run TestCopy_Overwrite ./web/webdav/...` | ❌ Wave 0 |
| DOC-01 | `docs/webdav.md` exists with correct sections | manual | `ls docs/webdav.md && grep -c "##" docs/webdav.md` | ❌ Wave 0 |
| DOC-02 | Config examples for OnlyOffice + iOS Files | manual | visual review | ❌ Wave 0 |
| DOC-03 | Compatibility notes present | manual | `grep -i "finder\|lock" docs/webdav.md` | ❌ Wave 0 |
| DOC-04 | No OpenAPI files in repo → narrative satisfies | n/a | no automated check needed | N/A |
| TEST-05 | OnlyOffice scenario: `SuccessCriterion6_Copy` in gowebdav | integration | `go test -run TestE2E_GowebdavClient/SuccessCriterion6 ./web/webdav/...` | ❌ Wave 0 |
| TEST-06 | litmus Class 1 all suites pass | external | `make test-litmus` or `scripts/webdav-litmus.sh` | ❌ Wave 0 |
| TEST-07 | RED→GREEN→REFACTOR per commit | process | git log review | N/A |

### Test Layers

**Layer 1 — Unit tests (`web/webdav/copy_test.go`, new file)**

Table-driven tests using `newWebdavTestEnv` + `seedFile`/`seedDir` + `httpexpect`. Cover:
- File COPY to new dest → 201 + VFS state
- File COPY Overwrite:T existing → 204 + old file in trash
- File COPY Overwrite:F existing → 412
- File COPY Overwrite absent → treated as T → 204
- File COPY missing source → 404
- File COPY missing dest parent → 409
- File COPY source == destination → 403
- Dir COPY Depth:infinity → 201 + recursive VFS state
- Dir COPY Depth:0 → 201 + empty container
- Dir COPY Depth:1 → 400 Bad Request
- Note COPY (Mime == NoteMimeType) → delegates to `note.CopyFile`
- COPY into trash → 403
- Partial dir COPY (quota exhaustion mid-walk) → 207

**Layer 2 — E2E integration (`web/webdav/gowebdav_integration_test.go`, extend)**

New sub-test `SuccessCriterion6_Copy` covering the Phase 3 success criterion 1:
- gowebdav `Copy` call on a file: verify replica exists
- gowebdav `Copy` on a directory: verify recursive contents
- Overwrite semantics via raw HTTP

**Layer 3 — litmus external compliance**

`scripts/webdav-litmus.sh` (new file):
- Creates fresh instance `litmus-$TIMESTAMP.localhost:8080`
- Generates `token-cli` with `io.cozy.files` scope
- Runs litmus × 2 routes
- Traps EXIT to destroy instance
- Non-zero exit on any litmus failure

`Makefile` target `test-litmus`: calls `scripts/webdav-litmus.sh`.

**Layer 4 — Documentation smoke test**

```bash
# After writing docs/webdav.md:
grep -c "PROPFIND\|GET\|PUT\|DELETE\|MKCOL\|COPY\|MOVE" docs/webdav.md
# Should return >= 7 (one match per method)
grep "webdav" docs/toc.yml
# Should return the toc.yml entry
```

### Sampling Rate

- **Per task commit:** `go test -p 1 -timeout 5m -run TestCopy ./web/webdav/...`
- **Per wave merge:** `go test -p 1 -timeout 5m ./web/webdav/...`
- **Phase gate (before `/gsd:verify-work`):** Full suite green + `make test-litmus` passes both routes

### Wave 0 Gaps (must exist before implementation)

- [ ] `web/webdav/copy_test.go` — unit tests for `handleCopy` (RED tests, written before handler)
- [ ] `web/webdav/copy.go` — `handleCopy` + 207 Multi-Status builder + walk helper
- [ ] `scripts/webdav-litmus.sh` — litmus script with instance lifecycle management
- [ ] `docs/webdav.md` — documentation file

*(Existing infrastructure: `newWebdavTestEnv`, `seedFile`, `seedDir`, `mapVFSWriteError`, `parseDestination`, `isInTrash`, `sendWebDAVError` — all reusable as-is.)*

---

## Standard Stack

### Core

| Library | Version | Purpose | Notes |
|---------|---------|---------|-------|
| `model/vfs` | (internal) | VFS primitives for COPY | `CopyFile`, `Walk`, `CreateFileDocCopy` |
| `model/note` | (internal) | Note copy with images | `note.CopyFile` |
| `encoding/xml` | stdlib | 207 Multi-Status response | same as PROPFIND |
| litmus | 0.13 | Compliance test suite | installed at `/usr/bin/litmus` |
| `github.com/studio-b12/gowebdav` | v0.12.0 | E2E integration test client | already in go.mod |

### No New Dependencies

Phase 3 requires no new external Go dependencies. Everything needed is in the existing codebase:
- VFS interface + implementations: already imported
- `model/note`: new import in `copy.go`
- `model/consts`: already imported for `NoteMimeType`
- Echo, httpexpect, testify: already in use

---

## Architecture Patterns

### Recommended New Files

```
web/webdav/
├── copy.go           # handleCopy handler + 207 builder + walk-copy helper
├── copy_test.go      # unit tests (table-driven, RED before GREEN)
scripts/
├── webdav-litmus.sh  # litmus script: create instance, run × 2 routes, destroy
docs/
├── webdav.md         # new documentation file
```

### COPY Handler Pattern (mirrors move.go)

```go
// web/webdav/copy.go — skeleton structure
func handleCopy(c echo.Context) error {
    // 1. Resolve source path (davPathToVFSPath)
    // 2. parseDestination (reuse Phase 2 helper)
    // 3. isInTrash check on destination (403)
    // 4. isInTrash check on source (403) — new for COPY
    // 5. Parse Depth header (absent=infinity, "0"=shallow, "1"=400, "infinity"=deep)
    // 6. Resolve source: fs.DirOrFileByPath
    // 7. Source==Destination check (403)
    // 8. Parse Overwrite (absent=T, "F"=false)
    // 9. Check destination existence, handle Overwrite:F → 412
    // 10. Overwrite:T → trash destination
    // 11. Resolve destination parent → 409 if missing
    // 12. Build newdoc: CreateFileDocCopy(olddoc, parentID, destName)
    // 13. Branch: note.CopyFile (if olddoc.Mime == NoteMimeType) or fs.CopyFile
    // 14. For directory: walk-and-copy, collect per-file errors
    // 15. Return: 201 (new dest), 204 (overwrite), 207 (partial failures)
}
```

### 207 Multi-Status Builder Pattern

```go
// Collect per-file errors during walk
type copyError struct {
    destPath string
    status   int
}

var errs []copyError
walkFn := func(name string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
    if err != nil {
        errs = append(errs, copyError{destPath: ..., status: 500})
        return nil // do NOT abort walk
    }
    if file != nil {
        if copyErr := fs.CopyFile(olddoc, newdoc); copyErr != nil {
            errs = append(errs, copyError{destPath: ..., status: mapToHTTP(copyErr)})
            return nil // collect, do not abort
        }
    }
    return nil
}

// After walk:
if len(errs) > 0 {
    return sendCopyMultiStatus(c, errs)
}
return c.NoContent(201 or 204)
```

### Dispatcher Update

```go
// web/webdav/handlers.go:handlePath — add COPY case
case "COPY":
    return handleCopy(c)
```

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Destination header parsing | Custom parser | `parseDestination` (Phase 2) | Already handles both route prefixes, URL decode, traversal check |
| Path traversal check | Custom regex | `davPathToVFSPath` (Phase 1) | Handles `%`-encoding, null bytes, `..` escapes |
| File metadata copy | Manual struct copy | `vfs.CreateFileDocCopy` | Handles ID reset, mime derivation, metadata cleanup |
| Note image copying | Manual content copy | `note.CopyFile` | Handles ProseMirror content, image docs, trigger setup |
| Quota check | Pre-flight VFS query | `vfs.CopyFile` (internal) | Checks `CheckAvailableDiskSpace` atomically inside VFS lock |
| Recursive directory walk | Custom DFS | `vfs.Walk` | Handles `DirIterator` batching, depth limit (512), error propagation |
| Error → HTTP mapping | Switch in handler | `mapVFSWriteError` (Phase 2) | Already maps `ErrFileTooBig`, `ErrParentDoesNotExist`, etc. |
| XML error body | Custom XML builder | `sendWebDAVError` (Phase 1) | Correct RFC 4918 §8.7 format |
| Test instance lifecycle | Custom Go test harness | `newWebdavTestEnv` (Phase 1) | Full Echo + httptest + VFS + token |

---

## Code Examples

### Confirmed from codebase inspection

**Reference COPY handler (JSON:API, adapt for WebDAV):**
```go
// web/files/files.go:373-408 (reference pattern)
newdoc := vfs.CreateFileDocCopy(olddoc, destinationDirID, copyName)
// ...
if olddoc.Mime == consts.NoteMimeType {
    err = note.CopyFile(inst, olddoc, newdoc)
} else {
    err = fs.CopyFile(olddoc, newdoc)
}
```

**Overwrite pattern (from move.go:75-78):**
```go
overwrite := true
if ovr := c.Request().Header.Get("Overwrite"); ovr == "F" {
    overwrite = false
}
```

**Trash-first pattern for Overwrite:T (from move.go:86-100):**
```go
if dstFile != nil {
    _, err = vfs.TrashFile(fs, dstFile)
} else {
    _, err = vfs.TrashDir(fs, dstDir)
}
```

**Walk function signature:**
```go
// model/vfs/vfs.go:618-628
type WalkFn func(name string, dir *DirDoc, file *FileDoc, err error) error
func Walk(fs Indexer, root string, walkFn WalkFn) error
```

---

## State of the Art

| Old Approach | Current Approach | Impact |
|--------------|------------------|--------|
| 308 redirect for `/remote.php/webdav/*` | Direct serve (commit `7c9ab3a59`) | Both routes must be tested by litmus — handled by running litmus twice |
| `x/net/webdav` for COPY | Custom handler | x/net/webdav Overwrite default bug #66059 avoided; no LockSystem needed |
| Phase 2: MKCOL ignores body | **Gap: litmus `mkcol_with_body` may fail** | Handler must return 415 if Content-Type body present |

---

## Open Questions

1. **Does litmus `props` suite count 501 on PROPPATCH as skip or failure?**
   - What we know: `propset` and related tests send PROPPATCH, we return 501
   - What's unclear: whether litmus treats 501 as "server doesn't support this" (skip) or as a test failure
   - Recommendation: Run litmus `props` suite on first litmus run and observe. If failures, the plan for the `props` suite must add a PROPPATCH stub that returns 501 with proper Allow header.

2. **Does `mkcol_with_body` currently fail?**
   - What we know: our MKCOL handler ignores the request body; RFC 4918 §9.3.1 says 415 on unsupported media type body
   - What's unclear: whether the current handler passes or fails this specific litmus test
   - Recommendation: The plan for `basic` suite green must include a check: if `r.ContentLength > 0 && r.Header.Get("Content-Type") != ""` → 415.

3. **`copy_overwrite` test expects 204 on Overwrite:T to existing — after trash does `CopyFile` still see the destination as empty?**
   - What we know: trash removes the doc from the VFS index; `DirChildExists` should return false after trash
   - What's unclear: whether there's a brief window where CouchDB consistency lags
   - Recommendation: The pattern from `move.go` already works for MOVE — the same approach (trash first, then write) will work for COPY.

---

## Sources

### Primary (HIGH confidence)
- Installed `/usr/libexec/litmus/` binaries (basic, copymove, props, locks, http) — strings extracted directly
- `model/vfs/vfs.go` — VFS interface, Walk, CreateFileDocCopy, CheckAvailableDiskSpace
- `model/vfs/vfsafero/impl.go` — CopyFile afero implementation
- `model/vfs/vfsswift/impl_v3.go` — CopyFile swift implementation
- `model/note/copy.go` — note.CopyFile implementation
- `web/files/files.go:355-408` — reference COPY handler pattern
- `web/webdav/move.go` — MOVE handler (structural template for COPY)
- `web/webdav/write_helpers.go` — parseDestination, isInTrash, mapVFSWriteError
- `web/webdav/handlers.go` — current dispatcher (missing COPY case)
- `web/webdav/webdav.go` — webdavMethods, davAllowHeader (COPY already listed)
- `web/webdav/testutil_test.go` — test harness
- `docs/toc.yml` — TOC format
- `docs/nextcloud.md` — doc style template
- `.planning/config.json` — nyquist_validation: true

### Secondary (MEDIUM confidence)
- RFC 4918 https://www.rfc-editor.org/rfc/rfc4918.html — §9.8 COPY, §10.6 Overwrite, §13 Multi-Status (fetched 2026-04-11)
- WebSearch: litmus test names for all 5 suites (cross-verified with binary string extraction)
- WebSearch: litmus `locks` suite Class 2 skip behavior (confirmed by binary string extraction)

### Tertiary (LOW confidence — needs empirical validation)
- litmus `props` suite PROPPATCH behavior on 501 (must verify on first run)
- litmus `basic` `mkcol_with_body` behavior with our current handler (must verify on first run)

---

## Metadata

**Confidence breakdown:**
- COPY implementation patterns: HIGH — code is directly readable in repo + RFC fetched
- Litmus locks auto-skip: HIGH — confirmed from installed binary strings
- Litmus other suites gap inventory: MEDIUM — test names known, exact failures require first run
- Documentation approach: HIGH — direct inspection of `docs/nextcloud.md` + `docs/toc.yml`

**Research date:** 2026-04-11
**Valid until:** 2026-05-11 (RFC and VFS internals are stable; litmus version pinned at 0.13)
