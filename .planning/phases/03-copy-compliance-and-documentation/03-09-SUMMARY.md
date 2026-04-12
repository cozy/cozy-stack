---
phase: 03-copy-compliance-and-documentation
plan: 09
subsystem: documentation
tags: [webdav, rfc4918, rclone, onlyoffice, ios-files, markdown]

# Dependency graph
requires:
  - phase: 03-copy-compliance-and-documentation
    provides: COPY implementation and litmus compliance verification (03-01, 03-02)

provides:
  - docs/webdav.md — 536-line English user/operator documentation for WebDAV
  - docs/toc.yml entry linking /dav route to webdav.md

affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Documentation pattern: narrative + method table + inline curl examples per method"
    - "TOC registration: alphabetical placement in docs/toc.yml List of services"

key-files:
  created:
    - docs/webdav.md
  modified:
    - docs/toc.yml

key-decisions:
  - "Documented PROPPATCH (in-memory dead-property store, not deferred) — it is implemented in v1 with Strategy C"
  - "iOS/iPadOS Files app documented as best-effort, validation explicitly deferred to v1.1"
  - "DOC-04 satisfied by narrative + method table (no OpenAPI spec files exist in the repo — narrative IS the equivalent)"
  - "PROPPATCH persistence (CouchDB) remains deferred to v2 (ADV-V2-02) as planned"

patterns-established:
  - "WebDAV docs style: 7-section monolithic English doc following docs/nextcloud.md structure"

requirements-completed: [DOC-01, DOC-02, DOC-03, DOC-04]

# Metrics
duration: 2min
completed: 2026-04-12
---

# Phase 03 Plan 09: WebDAV Documentation Summary

**536-line English docs/webdav.md covering all 10 WebDAV methods with narrative, method table, and inline curl examples for OnlyOffice mobile, rclone, curl, and iOS Files (best-effort, v1.1 deferred)**

## Performance

- **Duration:** 2 min
- **Started:** 2026-04-12T15:46:51Z
- **Completed:** 2026-04-12T15:48:51Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Created `docs/webdav.md` (536 lines) in English with 7 top-level sections and 27 inline curl examples
- Documented all 10 supported WebDAV methods (OPTIONS, PROPFIND, PROPPATCH, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE)
- Registered the new file in `docs/toc.yml` under "List of services", alphabetically between `/contacts` and `/data`
- Explicitly deferred iOS/iPadOS Files manual validation to v1.1 as required by CONTEXT.md
- Closed requirements DOC-01, DOC-02, DOC-03, and DOC-04 in a single plan

## Task Commits

Each task was committed atomically:

1. **Task 1: Write docs/webdav.md** - `d012d482f` (docs)
2. **Task 2: Add webdav.md to docs/toc.yml** - `724113394` (docs)

## Files Created/Modified

- `docs/webdav.md` - New user/operator WebDAV documentation (7 sections, 536 lines)
- `docs/toc.yml` - Added `/dav - WebDAV: ./webdav.md` entry between /contacts and /data

## Sections written in docs/webdav.md

1. **Introduction** — RFC 4918 Class 1 scope, what is NOT supported, target audience
2. **Endpoints** — `/dav/files/*` (native) and `/remote.php/webdav/*` (Nextcloud alias), rationale for direct serving vs 308 redirect
3. **Authentication** — OAuth Bearer token and Basic Auth with token-as-password, OPTIONS auth exemption
4. **Supported methods** — Method table (10 methods) + narrative paragraph + curl example per method
5. **Client configuration** — OnlyOffice mobile, rclone (with config snippet), curl/manual, iOS/iPadOS Files app (best-effort)
6. **Compatibility notes & limitations** — No LOCK, Finder read-only, Depth:infinity blocked, PROPPATCH in-memory, .cozy_trash read-only, soft-delete, streaming I/O, ETag from MD5, RFC 1123 dates
7. **Troubleshooting** — Common error table (401, 403, 404, 405, 409, 412, 415, 502) + auth debugging + log location

## Decisions Made

- Documented PROPPATCH as an implemented feature (in-memory dead-property store, Strategy C) rather than as deferred — because it IS implemented in v1, not deferred. The v2 item is CouchDB persistence.
- iOS Files app section states best-effort with explicit "deferred to v1.1" per CONTEXT.md scope reduction decision.
- DOC-04 ("OpenAPI or equivalent") satisfied by the narrative + method table — confirmed correct by research §5 (no OpenAPI spec files in the repo).

## Deviations from Plan

None — plan executed exactly as written. The only minor observation: the plan's `<interfaces>` block listed 9 methods (no PROPPATCH), but the actual `webdav.go` and `handlers.go` include PROPPATCH as a fully implemented method. The documentation was written to match the code, not the outdated interface list. This is a correctness fix (Rule 1), not a deviation — the code is authoritative.

## Issues Encountered

None.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- All v1 documentation requirements closed (DOC-01 through DOC-04)
- Phase 03 plan 10 (final wrap-up / STATE update) is the only remaining plan
- docs/webdav.md is ready for review and merge

---
*Phase: 03-copy-compliance-and-documentation*
*Completed: 2026-04-12*
