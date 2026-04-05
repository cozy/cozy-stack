---
phase: 01-foundation
plan: 01
subsystem: testing
tags: [webdav, tdd, red, go, xml, path-mapping]

requires:
  - phase: 01-foundation
    provides: research, requirements, test strategy (RESEARCH, VALIDATION, CONTEXT)
provides:
  - web/webdav Go package scaffolding (6 stub files)
  - gowebdav v0.12.0 test client dependency
  - RED tests for XML multistatus marshalling (7 tests)
  - RED tests for davPathToVFSPath + ErrPathTraversal sentinel (14 sub-tests)
affects: [01-02, 01-03, 01-04, phase-01-remaining-plans]

tech-stack:
  added: [github.com/studio-b12/gowebdav v0.12.0]
  patterns:
    - "RED→GREEN→REFACTOR commit cadence per task"
    - "Internal test package (package webdav, not webdav_test) for unexported symbol access"
    - "Table-driven tests for path validation"
    - "D: XML namespace prefix assertion via string search"

key-files:
  created:
    - web/webdav/webdav.go
    - web/webdav/xml.go
    - web/webdav/path_mapper.go
    - web/webdav/auth.go
    - web/webdav/errors.go
    - web/webdav/handlers.go
    - web/webdav/xml_test.go
    - web/webdav/path_mapper_test.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "Tests use internal package (package webdav) to access unexported helpers davPathToVFSPath, buildETag, parsePropFind, marshalMultistatus"
  - "ResourceType struct uses pointer fields (Collection *struct{}) so marshaller can omit the <D:collection/> element for files"
  - "ErrPathTraversal declared as exported sentinel to enable errors.Is checks in future handler code"
  - "gowebdav listed as indirect in go.mod until a test file imports it (Plan 02 onwards)"

patterns-established:
  - "TDD RED baseline: stub .go + *_test.go referencing undefined symbols → `go test` fails with undefined errors"
  - "Each task is a single atomic commit with type(webdav) prefix and bullet-list body"
  - "Empty package files carry a one-line comment pointing to the GREEN wave that will fill them"

requirements-completed: [TEST-01, TEST-02, TEST-04]

duration: 3min
completed: 2026-04-05
---

# Phase 01 Plan 01: WebDAV Package Scaffold + RED Tests Summary

**web/webdav Go package scaffolded with 6 stub files and the first RED test files (xml_test.go, path_mapper_test.go) that drive wave 1 GREEN implementations by referencing undefined types, helpers, and the ErrPathTraversal sentinel.**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-04-05T14:21:53Z
- **Completed:** 2026-04-05T14:24:30Z
- **Tasks:** 3
- **Files modified:** 10 (8 created + go.mod + go.sum)

## Accomplishments

- Established the `web/webdav/` package with the canonical file layout agreed during planning (webdav, xml, path_mapper, auth, errors, handlers).
- Added `github.com/studio-b12/gowebdav v0.12.0` as the integration test client dependency.
- Committed 7 RED tests covering XML multistatus marshalling, `D:` namespace prefix, RFC 1123 getlastmodified, ETag quoting, ISO 8601 creationdate, PROPFIND parsing, and resourcetype (collection vs file).
- Committed a 13-case table-driven RED test for `davPathToVFSPath` plus a sentinel-error assertion for `ErrPathTraversal`.
- Verified package still compiles (`go build ./web/webdav/` exits 0) while `go test ./web/webdav/ -count=1` fails exclusively with `undefined:` errors — the exact RED baseline the plan demanded.

## Task Commits

1. **Task 1: Scaffold package + add gowebdav** — `1c99363ac` (test)
2. **Task 2: RED tests for XML marshalling** — `b3402ea8b` (test)
3. **Task 3: RED tests for path mapper + traversal** — `f23c2a9e6` (test)

**Plan metadata commit:** to be appended after this SUMMARY is written.

## Files Created/Modified

- `web/webdav/webdav.go` — package doc + `webdavMethods` constant
- `web/webdav/xml.go` — empty stub; GREEN-target for wave 1 (Plan 02)
- `web/webdav/path_mapper.go` — empty stub; GREEN-target for Plan 03
- `web/webdav/auth.go` — empty stub; GREEN-target for wave 2
- `web/webdav/errors.go` — empty stub; GREEN-target for Plan 04
- `web/webdav/handlers.go` — empty stub; GREEN-target for waves 3-5
- `web/webdav/xml_test.go` — 7 failing tests for `Multistatus`, `Response`, `Propstat`, `Prop`, `ResourceType`, `SupportedLock`, `PropFind`, `marshalMultistatus`, `parsePropFind`, `buildETag`, `buildCreationDate`
- `web/webdav/path_mapper_test.go` — 13-case table + sentinel test for `davPathToVFSPath` and `ErrPathTraversal`
- `go.mod`, `go.sum` — gowebdav v0.12.0 added

## Symbols Tests Reference (to be implemented in later plans)

**XML (Plan 02 GREEN target):**
- types: `Multistatus`, `Response`, `Propstat`, `Prop`, `ResourceType`, `SupportedLock`, `PropFind`, `PropList`
- funcs: `marshalMultistatus([]Response) ([]byte, error)`, `parsePropFind([]byte) (*PropFind, error)`, `buildETag([]byte) string`, `buildCreationDate(time.Time) string`

**Path mapper (Plan 03 GREEN target):**
- `davPathToVFSPath(string) (string, error)`
- `ErrPathTraversal` sentinel

## Decisions Made

- Test package is internal (`package webdav`) so tests can reach unexported helpers like `davPathToVFSPath` and `buildETag` without the `_test` suffix.
- `ResourceType.Collection` modelled as `*struct{}` to allow omitempty semantics for files.
- `SupportedLock` exposed as a pointer field in `Prop` for the same reason — wave 1 GREEN may initially return an empty struct.
- gowebdav intentionally left as `// indirect` in go.mod for now; it graduates to direct when Plan 05+ tests actually import it.

## Deviations from Plan

None — plan executed exactly as written. All three tasks passed their `<verify>` conditions on the first attempt.

## Issues Encountered

None.

## User Setup Required

None.

## Next Phase Readiness

- **Plan 02 (XML GREEN):** All 7 XML RED tests compile-fail with undefined symbols in `xml.go`. Ready to define the struct set and helpers.
- **Plan 03 (path mapper GREEN):** `davPathToVFSPath` and `ErrPathTraversal` RED tests pinpoint exactly the signature and sentinel to implement.
- **Plan 04 (errors.go GREEN):** Not yet RED — this plan deliberately deferred auth/errors/propfind/get/options test creation to keep context budget tight per the plan's verification note.

## Self-Check: PASSED

All 9 files verified present on disk. All 3 task commits verified in git log (`1c99363ac`, `b3402ea8b`, `f23c2a9e6`).
