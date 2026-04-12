# 03-05 Litmus Basic Suite Inventory

## Run Metadata

- **Date/time:** 2026-04-12 (first run ~17:12, final run ~17:29 UTC+2)
- **cozy-stack version/commit:** Development build, branch `feat/webdav`,
  git HEAD: `86b134cda` (fix: complete pre-existing incomplete work to restore build)
- **litmus version:** litmus 0.13
- **Test suite:** `basic` (16 tests: init, begin, options, put_get, put_get_utf8_segment,
  put_no_parent, mkcol_over_plain, delete, delete_null, delete_fragment, mkcol,
  mkcol_again, delete_coll, mkcol_no_parent, mkcol_with_body, finish)

---

## First Run Results (before fixes)

### Pre-run blocker: server not running

On initial invocation, the cozy-stack server was not running. It was started
via `cozy-stack serve --config ~/.cozy/cozy.yml`. CouchDB was started via Docker.

### First-run failure: init — password must be <256 chars

**Script bug discovered:** The original `scripts/webdav-litmus.sh` used a
timestamp format `litmus-YYYYMMDD-HHMMSS.localhost:8080` (e.g.
`litmus-20260412-171228.localhost:8080`). The domain name was 39 chars long,
causing the generated JWT token to be 260 characters — exceeding litmus's
hard limit of 256 characters in the password field.

**Fix applied (Rule 1 - Bug):** Changed `TIMESTAMP=$(date +%Y%m%d-%H%M%S)`
to `TIMESTAMP=$(date +%s)` and domain pattern to `lm-${TIMESTAMP}.localhost:8080`.
Epoch-based domain (28 chars) produces tokens of ~248 chars, well under 256.

### Second-run failure: MKCOL 405 (stale build cache + missing implementations)

After fixing the token length, `begin` failed: `MKCOL /dav/files/litmus/` returned 405.
Root causes (all pre-existing):

1. **Stale build cache:** The running cozy-stack binary was compiled from a cached
   build that did not include Phase 2/3 handlers (MKCOL, COPY, MOVE). After
   `go clean -cache && go install .`, MKCOL returned 201 correctly.

2. **Incomplete propfind.go:** The working tree had calls to undefined functions
   `buildResponseForFileWithPrefix`, `buildResponseForDirWithPrefix`,
   `streamChildrenWithPrefix`. These were completed (Rule 3 - blocking).

3. **Missing proppatch.go:** `handlers.go` referenced `handleProppatch` but
   no implementation existed. A Strategy-B handler was created (Rule 3 - blocking).

### Third-run failure: put_get_utf8_segment — 403 Forbidden

Route 1 (`/dav/files/`):
- 15/16 passed
- **FAIL:** `put_get_utf8_segment` — PUT of `/dav/files/litmus/res-%e2%82%ac` → 403

Route 2 (`/remote.php/webdav/`):
- 15/16 passed
- **FAIL:** `put_get_utf8_segment` — PUT of `/remote.php/webdav/litmus/res-%e2%82%ac` → 403

**Root cause:** `containsEncodedTraversal()` in `path_mapper.go` rejected ALL
`%` characters. However, Echo does NOT pre-decode the wildcard parameter, so
`%e2%82%ac` (the euro sign €) reached the handler undecoded. The function
incorrectly rejected valid UTF-8 percent-encoded filenames.

**TDD fix applied:**
- RED test in `path_mapper_test.go`: three new cases for percent-encoded UTF-8
  (`%e2%82%ac`, `%41`, `%c3%a9t%c3%a9`) plus `%00` rejection
- GREEN fix in `path_mapper.go`:
  - `containsEncodedTraversal` now rejects only `%2e`, `%2f`, `%00` (case-insensitive)
  - `davPathToVFSPath` calls `url.PathUnescape` after the traversal check
  - Post-decode re-check for double-encoded traversal (`%252e` → `%2e`)
  - Post-decode null byte check

---

## Final Run Results

**Run date/time:** 2026-04-12 ~17:29 UTC+2

### Route 1: /dav/files/

```
<- summary for `basic': of 16 tests run: 16 passed, 0 failed. 100.0%
-> 1 warning was issued.
```

| # | Test | Result |
|---|------|--------|
| 0 | init | PASS |
| 1 | begin | PASS |
| 2 | options | PASS (warning: no Class 2 — expected) |
| 3 | put_get | PASS |
| 4 | put_get_utf8_segment | PASS |
| 5 | put_no_parent | PASS |
| 6 | mkcol_over_plain | PASS |
| 7 | delete | PASS |
| 8 | delete_null | PASS |
| 9 | delete_fragment | PASS |
| 10 | mkcol | PASS |
| 11 | mkcol_again | PASS |
| 12 | delete_coll | PASS |
| 13 | mkcol_no_parent | PASS |
| 14 | mkcol_with_body | PASS |
| 15 | finish | PASS |

### Route 2: /remote.php/webdav/

```
<- summary for `basic': of 16 tests run: 16 passed, 0 failed. 100.0%
-> 1 warning was issued.
```

All 16 tests pass (same results as Route 1).

### Warning: Class 2 compliance

Both routes emit "WARNING: server does not claim Class 2 compliance". This is
**expected and intentional** — the server advertises `DAV: 1` only (no LOCK
support). Class 2 requires LOCK/UNLOCK, which are deferred to a future phase.

---

## Fixes Summary

| Fix | Type | Files | Commits |
|-----|------|-------|---------|
| Script domain name too long → JWT > 256 chars | Rule 1 - Bug | `scripts/webdav-litmus.sh` | `74d1b5b70` (in propfind changes) |
| containsEncodedTraversal rejected valid UTF-8 % sequences | Rule 1 - Bug (litmus gap) | `path_mapper.go`, `path_mapper_test.go` | RED: `74d1b5b70`, GREEN: `f14603ef0` |
| Stale build cache + incomplete handlers blocking build | Rule 3 - Blocking | `propfind.go`, `handlers.go`, `webdav.go`, `copy.go`, `copy_test.go`, `proppatch.go` | `86b134cda` |

---

## Final State: BOTH ROUTES CLEAN — 16/16

`LITMUS_TESTS="basic" scripts/webdav-litmus.sh` exits 0 with 16 passed, 0 failed
on both `/dav/files/` and `/remote.php/webdav/`.

The `options` warning (Class 2 not claimed) is expected for a Class 1 server.
