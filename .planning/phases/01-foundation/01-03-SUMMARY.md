---
phase: 01-foundation
plan: 03
subsystem: webdav-security
tags: [webdav, path-traversal, security, green, sec-02, cve-2023-39143]

requires:
  - phase: 01-foundation
    plan: 01
    provides: RED tests for davPathToVFSPath (TestDavPathToVFSPath table + TestDavPathToVFSPath_SentinelError)
  - phase: 01-foundation
    plan: 02
    provides: Compile-only stub for davPathToVFSPath and the ErrPathTraversal sentinel in web/webdav/path_mapper.go
provides:
  - web/webdav/path_mapper.go with the full davPathToVFSPath implementation (traversal-rejecting)
  - containsEncodedTraversal helper (internal)
  - Final ErrPathTraversal sentinel (already stable from Plan 01-02)
affects: [01-05 (auth middleware), 01-06 (route registration), 01-07 (PROPFIND handler), 01-08 (GET handler), all Phase 2/3 VFS-touching handlers]

tech-stack:
  added: []
  patterns:
    - "Security boundary: every WebDAV URL wildcard is normalised and validated by davPathToVFSPath BEFORE any VFS call — never pass a raw Echo :path parameter to model/vfs"
    - "Reject-any-percent policy: since Echo URL-decodes the wildcard once, any residual '%' in the string after that decode is rejected outright. This single check catches %2e, %2f, and double-encoded %25xx variants in one pass without enumerating suffixes"
    - "Anchor-then-clean: prepend /files/ before path.Clean so '..' that escapes the WebDAV URL space fails the /files prefix assertion"

key-files:
  created:
    - .planning/phases/01-foundation/01-03-SUMMARY.md
  modified:
    - web/webdav/path_mapper.go

key-decisions:
  - "Reject any residual percent character (not just %2e / %2f). The plan's reference implementation enumerated %2e and %2f, but that misses the double-encoded case %252e%252e — after Echo's single decode this becomes %25 + 2e, which contains neither %2e nor %2f as substrings. Since Echo has already decoded the wildcard once, ANY surviving '%' is suspicious, so we reject all of them. This is a strict superset of the plan's check and passes every test case including double-encoded."
  - "Anchor input under /files before path.Clean. This reuses the URL-space prefix as a boundary that path.Clean cannot escape — a '..' walk that climbs above /files causes the HasPrefix(\"/files/\") check to fail, which returns ErrPathTraversal. Without anchoring we'd need to re-implement Clean's traversal semantics manually."
  - "Keep containsEncodedTraversal as a named helper even though it's a one-liner. The helper carries the load-bearing doc comment explaining WHY we reject any '%', which is non-obvious and the first thing a future reader will question."
  - "Skip the REFACTOR commit. Task 2 authorises a no-op if the function body is already small and gofmt-clean. After Task 1, path_mapper.go is 65 lines, the public function is 24 lines of code, and gofmt has nothing to say — no refactor warranted, and the plan explicitly says 'otherwise skip the commit entirely'."

patterns-established:
  - "Pattern: Security boundary helpers carry a detailed doc comment naming the CVE / pitfall they defend against (here: CVE-2023-39143 path traversal, PITFALLS.md lines 35-56)"
  - "Pattern: Return a single sentinel error (ErrPathTraversal) for all traversal rejection paths so callers can do one errors.Is check and log/respond uniformly"

requirements-completed: [ROUTE-03, ROUTE-05, SEC-02]

metrics:
  tasks_total: 2
  tasks_completed: 2
  duration: ~2min
  started: 2026-04-05T14:35:18Z
  completed: 2026-04-05T14:36:30Z
---

# Phase 01 Plan 03: Path Mapper GREEN Summary

**Replaced the Plan 01-02 compile-only stub with a real davPathToVFSPath that anchors the WebDAV wildcard under /files, rejects any residual percent escape (including double-encoded %252e), null bytes, and cleaned paths that escape the /files scope — turning all 13 TestDavPathToVFSPath cases plus the sentinel-error test green.**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-04-05T14:35:18Z
- **Completed:** 2026-04-05T14:36:30Z
- **Tasks:** 2 (1 code task + 1 refactor no-op)
- **Files modified:** 1 (`web/webdav/path_mapper.go`)

## Accomplishments

- `davPathToVFSPath` rejects every traversal variant in the test table: literal `..`, `%2e%2e`, `%2E%2E`, double-encoded `%252e%252e`, null byte `\x00`, encoded slash `%2f`, and system-directory climbs like `../settings`
- Valid inputs (including Unicode `Documents/répertoire`) normalise correctly: empty → `/`, `/` → `/`, `Documents/` → `/Documents`
- Single sentinel error `ErrPathTraversal` for all rejection paths — callers use `errors.Is`
- Security boundary now in place for every Phase 1 / 2 / 3 handler that touches the VFS

## Task Commits

1. **Task 1: Implement davPathToVFSPath (GREEN)** — `af5b6f177` (feat)
2. **Task 2: REFACTOR** — no-op, no commit (function body already 24 lines, `gofmt -l` empty, helper already extracted as part of Task 1)

## Files Created/Modified

- `web/webdav/path_mapper.go` — Replaced the compile-only stub from Plan 01-02 with the full implementation plus the `containsEncodedTraversal` helper. `ErrPathTraversal` sentinel unchanged (it was already final in Plan 01-02).

## Final Public / Internal API (`web/webdav/path_mapper.go`)

```go
// Exported
var ErrPathTraversal = errors.New("webdav: path traversal rejected")

// Unexported (package-internal — tests use the internal test package)
func davPathToVFSPath(rawParam string) (string, error)
func containsEncodedTraversal(s string) bool
```

### Algorithm

```
1. if rawParam contains '\x00'                    -> ErrPathTraversal
2. if rawParam contains '%' (any case)            -> ErrPathTraversal
3. cleaned := path.Clean("/files/" + rawParam)
4. if cleaned != "/files" && !strings.HasPrefix(cleaned, "/files/") -> ErrPathTraversal
5. vfsPath := strings.TrimPrefix(cleaned, "/files"); if "" -> "/"
6. return vfsPath, nil
```

## Verification

```
$ go test ./web/webdav/ -run TestDavPathToVFSPath -count=1 -v
=== RUN   TestDavPathToVFSPath
    --- PASS: TestDavPathToVFSPath/root_empty
    --- PASS: TestDavPathToVFSPath/root_slash
    --- PASS: TestDavPathToVFSPath/simple
    --- PASS: TestDavPathToVFSPath/nested
    --- PASS: TestDavPathToVFSPath/trailing_slash
    --- PASS: TestDavPathToVFSPath/unicode
    --- PASS: TestDavPathToVFSPath/dotdot_literal
    --- PASS: TestDavPathToVFSPath/encoded_dotdot_lowercase
    --- PASS: TestDavPathToVFSPath/encoded_dotdot_uppercase
    --- PASS: TestDavPathToVFSPath/double_encoded
    --- PASS: TestDavPathToVFSPath/null_byte
    --- PASS: TestDavPathToVFSPath/encoded_slash
    --- PASS: TestDavPathToVFSPath/settings_prefix_rejected
--- PASS: TestDavPathToVFSPath (0.00s)
--- PASS: TestDavPathToVFSPath_SentinelError (0.00s)
PASS
ok  	github.com/cozy/cozy-stack/web/webdav	0.005s
```

Full-package run (`go test ./web/webdav/ -count=1`) also passes — no regressions in the xml_test.go suite from Plan 01-02. `gofmt -l web/webdav/path_mapper.go` produces no output. `go vet ./web/webdav/` is clean.

### Acceptance criteria (Task 1)

- [x] `go test ./web/webdav/ -run TestDavPathToVFSPath -count=1` exits 0
- [x] `grep -q 'ErrPathTraversal' web/webdav/path_mapper.go`
- [x] `grep -q 'path.Clean' web/webdav/path_mapper.go`
- [x] git log shows a commit matching `feat(.*): davPathToVFSPath.*GREEN` — `af5b6f177`

### Acceptance criteria (Task 2)

- [x] Tests still pass
- [x] `gofmt -l web/webdav/path_mapper.go` produces no output
- [x] No refactor commit needed (function already compact and clean — plan explicitly permits skipping the commit)

## Decisions Made

See `key-decisions` in frontmatter. The load-bearing one is the **reject-any-percent** policy: the plan's reference implementation only checked `%2e` and `%2f` as substrings, which does not catch the double-encoded case because after Echo's single decode `%252e%252e` becomes `%25 + 2e + %25 + 2e` — neither `%2e` nor `%2f` appears as a substring there. The first test run confirmed this (the `double_encoded` case failed). Switching to "reject any `%`" is a strict superset that handles every variant in one check, with a doc comment explaining the invariant for future readers.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] Broadened encoded-traversal check from `%2e`/`%2f` substrings to any `%`**

- **Found during:** Task 1 (first `go test -run TestDavPathToVFSPath` run)
- **Issue:** The plan's reference implementation checked `strings.Contains(lower, "%2e") || strings.Contains(lower, "%2f")`. This catches single-encoded `%2e%2e` and `%2E%2E` correctly, but misses the double-encoded case `%252e%252e/etc`: after Echo decodes the URL once the string becomes literally `%2e%2e/etc` (which would be caught NEXT time), but the *test* passes the raw pre-decode string `%252e%252e/etc` directly to the function. The substring `%2e` does not appear in `%252e` (the `%2` is followed by `5`, not `e`), so the plan's check lets it through. Test output: `TestDavPathToVFSPath/double_encoded ... An error is expected but got nil.`
- **Fix:** Reject any residual `%` character via `strings.ContainsRune(s, '%')`. Since Echo decodes the WebDAV wildcard once before we see it, ANY remaining `%` is either a double encoding or a deliberate attempt to smuggle a dot/slash past `path.Clean`. This is a strict superset of the plan's enumeration check and is simpler to reason about.
- **Files modified:** `web/webdav/path_mapper.go` (containsEncodedTraversal body + doc comment)
- **Verification:** Re-ran the full table — all 13 cases plus sentinel test now pass.
- **Committed in:** `af5b6f177` (part of Task 1 commit — the fix was applied before commit so history stays clean)

---

**Total deviations:** 1 auto-fixed (1 bug in the plan's reference implementation).
**Impact on plan:** Strict improvement. The plan's own test table included the double-encoded case, so the reference implementation was internally inconsistent — the fix brings the code into line with the plan's own acceptance criteria. No scope change, no new behaviour beyond what the tests already required.

## Issues Encountered

None beyond the deviation above. The plan was otherwise followed exactly.

## User Setup Required

None.

## Handoff to Downstream Plans

The path mapper is now the security boundary for every future WebDAV handler. Downstream consumers:

- **Plan 01-05 (auth middleware):** Runs before path mapping. Passes raw `c.Param("*")` through after credential check. No changes needed there — the mapper handles all validation.
- **Plan 01-06 (route registration):** Registers the `/dav/files/*` route. The handler should call `davPathToVFSPath(c.Param("*"))` as its very first step, return 400 Bad Request (or 404, depending on policy — to be decided in 01-06) on `ErrPathTraversal`, and log the original raw input for intrusion detection.
- **Plan 01-07 (PROPFIND):** Uses the returned VFS path as the argument to `vfs.DirOrFileByPath(fs, vfsPath)`. Never logs the raw input to the client response.
- **Plan 01-08 (GET):** Same as PROPFIND — the VFS path from the mapper is safe to hand to `vfs.ServeFileContent`.
- **Phase 2/3 handlers (MKCOL, PUT, MOVE, COPY, DELETE):** All call the mapper first. For two-path operations (MOVE, COPY) the Destination header must also be mapped through this same function.

**Invariant for all callers:** never pass `c.Param("*")` to `model/vfs` directly — always route through `davPathToVFSPath` and check the error.

## Next Phase Readiness

- Plan 01-03 complete. Ready for Plan 01-04 (next in Phase 01 sequence — see ROADMAP.md for the wave map).
- No blockers introduced. The security boundary is in place for all VFS-touching work that follows.

---
*Phase: 01-foundation*
*Completed: 2026-04-05*

## Self-Check: PASSED

- `web/webdav/path_mapper.go` present and contains `davPathToVFSPath`, `ErrPathTraversal`, `containsEncodedTraversal`, `path.Clean` — verified.
- Commit `af5b6f177` present in `git log` with subject `feat(01-03): davPathToVFSPath with traversal prevention — GREEN` — verified.
- `go test ./web/webdav/ -run TestDavPathToVFSPath -count=1` exits 0 (13/13 cases + sentinel test) — verified.
- `go test ./web/webdav/ -count=1` full package exits 0 (no regression in Plan 01-02 XML tests) — verified.
- `gofmt -l web/webdav/path_mapper.go` empty — verified.
- `go vet ./web/webdav/` clean — verified.
