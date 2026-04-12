---
phase: 03-copy-compliance-and-documentation
plan: "07"
subsystem: webdav/props
tags: [litmus, webdav, proppatch, propfind, dead-properties, strategy-c]
dependency_graph:
  requires: ["03-04"]
  provides: ["litmus-props-clean", "proppatch-strategy-c", "dead-property-store"]
  affects: ["web/webdav/propfind.go", "web/webdav/proppatch.go", "web/webdav/move.go"]
tech_stack:
  added: ["in-memory dead-property store (sync.RWMutex)", "xml.innerxml injection"]
  patterns: ["strategy-c dead properties", "per-namespace prefix tracking in XML serialization"]
key_files:
  created:
    - web/webdav/deadprops.go
    - web/webdav/proppatch.go
    - web/webdav/proppatch_test.go
    - .planning/phases/03-copy-compliance-and-documentation/03-07-INVENTORY.md
  modified:
    - web/webdav/propfind.go
    - web/webdav/propfind_test.go
    - web/webdav/xml.go
    - web/webdav/move.go
    - web/webdav/handlers.go
    - web/webdav/webdav.go
    - scripts/webdav-litmus.sh
decisions:
  - "Strategy C (in-memory dead-property store) chosen over Strategy B (403 per-property) because litmus 0.13 checks per-property status codes explicitly via ne_propset_status — 403 fails the test rather than being accepted"
  - "Namespace prefixes in PROPPATCH XML parsing use a per-element nsMap+nsCounter instead of a fixed 'ns:' prefix to avoid opening/closing tag mismatch for nested elements with different namespaces"
  - "MOVE handler calls deadPropStore.movePropsForPath to follow RFC 4918 §9.9.1 — dead properties must move with the resource"
  - "Dead property XML injection in PROPFIND uses non-self-closing elements with per-element xmlns declarations (compatible with libxml2/neon namespace scoping rules)"
metrics:
  duration: "~143 minutes (including strategy pivot from B to C)"
  completed: "2026-04-12"
  tasks_completed: 2
  files_changed: 9
---

# Phase 03 Plan 07: litmus props Suite — Dead Property Compliance Summary

In-memory dead-property store (Strategy C) achieves litmus props 30/30 PASS on both `/dav/files/` and `/remote.php/webdav/` routes.

## What Was Built

### Strategy Pivot: B → C

The plan anticipated Strategy A (501 tolerated by litmus) or Strategy B (207+403 per-property). First-run results showed:

- litmus 0.13 explicitly calls `ne_propset_status()` to check per-property HTTP status — 403 is a FAIL, not a skip
- Strategy B was implemented (RED `b2121c180`, GREEN `84011cd8a`), but `propmanyns` still failed because its 10-property batch required ALL properties to return 200 OK
- Strategy C was required as a fallback

### Strategy C: In-Memory Dead-Property Store

**`web/webdav/deadprops.go`** (new): keyed by `(domain, vfsPath, namespace, local)`, protected by `sync.RWMutex`. Methods: `set`, `remove`, `get`, `listFor`, `clearForPath`, `movePropsForPath`.

**`web/webdav/proppatch.go`** (new): PROPPATCH handler that parses `<D:propertyupdate>` XML, applies operations to `deadPropStore`, returns 207+200 for each property.

**`web/webdav/xml.go`**: Added `DeadPropsXML []byte \`xml:",innerxml"\`` to `Prop` struct — injects raw XML bytes directly into the marshaled `<D:prop>` element without double-encoding.

**`web/webdav/propfind.go`**: Dead property injection via `buildDeadPropsXML()` + `hrefToVFSPath()`. Added `davNextcloudPrefix` and `hrefPrefixFor()` for correct `<D:href>` values on the Nextcloud route.

**`web/webdav/move.go`**: Added `deadPropStore.movePropsForPath(inst.Domain, srcPath, dstPath)` after the VFS rename — RFC 4918 §9.9.1 requires dead properties to follow the resource on MOVE.

### PROPFIND Hardening

- **propfind_invalid**: non-well-formed XML body → 400 (was 500 crash)
- **propfind_invalid2**: `xmlns:bar=""` empty namespace binding → 400 (Go's `xml.Decoder` accepts this silently; added explicit check)
- **propfind_d0**: wrong href prefix on Nextcloud route → fixed via `hrefPrefixFor(c)` routing detection

### JWT Token Length Fix

`scripts/webdav-litmus.sh` domain format shortened from `litmus-YYYYMMDD-HHMMSS.localhost:8080` (37 chars) to `lm-{unix_epoch}.localhost:8080` (28 chars) to keep JWT tokens under litmus's 256-char password limit.

## Final litmus Results

```
/dav/files/:          30/30 PASS  (11 warnings — expected: deleted props omitted)
/remote.php/webdav/:  30/30 PASS  (11 warnings — expected)
```

Warnings ("Property N omitted from results with no status") are expected after `propdeletes` removes properties — the resource exists but the deleted properties are absent from the PROPFIND response without a status entry.

## Commits

| Hash | Message |
|------|---------|
| `4401c1ba3` | docs(03-07): inventory litmus props suite first-run results |
| `b2121c180` | test(03-07): RED — reproduce litmus props/propfind_invalid, propfind_invalid2, propfind_d0 |
| `f677ae35d` | fix(03-07): GREEN — address props/propfind_invalid, propfind_invalid2, propfind_d0 |
| `0d398e1c2` | test(03-07): RED — reproduce litmus props/propset, propmanyns (PROPPATCH 405) |
| `84011cd8a` | feat(03-07): GREEN — add minimal PROPPATCH handler (Strategy B: 207 with 403 per-property) |
| `dde5f907a` | feat(03-07): GREEN — Strategy C in-memory dead-property store (props 30/30) |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical Functionality] JWT token exceeded litmus 256-char password limit**
- **Found during:** Task 1 (first litmus run)
- **Issue:** Domain name `litmus-YYYYMMDD-HHMMSS.localhost:8080` generated 260-char tokens
- **Fix:** Shortened domain format to `lm-{epoch}.localhost:8080` in `scripts/webdav-litmus.sh`
- **Files modified:** scripts/webdav-litmus.sh

**2. [Rule 1 - Bug] Strategy B insufficient for litmus 0.13**
- **Found during:** Task 2 (Strategy B GREEN commit)
- **Issue:** litmus 0.13 `ne_propset_status()` rejects 403 per-property as a test FAIL (not a skip). `propmanyns` test requires 200 OK for all 10 properties simultaneously.
- **Fix:** Escalated to Strategy C (in-memory dead-property store with actual persistence per session)
- **Files modified:** web/webdav/deadprops.go (new), web/webdav/proppatch.go (rewrite), web/webdav/xml.go, web/webdav/propfind.go, web/webdav/move.go

**3. [Rule 1 - Bug] parseProppatchOps emitted mismatched XML tags for nested elements**
- **Found during:** Task 2 (litmus final verification run — XML parse error on `/remote.php/webdav/` for propget after propmove)
- **Issue:** When collecting nested XML in a property value, the opening tag used `ns:` prefix (e.g., `<ns1:child xmlns:ns1="...">`) but the closing tag used only the local name (`</child>`). Stored values were malformed XML. Cross-run contamination: first run (`/dav/files/`) stored bad XML at `/litmus/prop2`; second run (`/remote.php/webdav/`) retrieved those values causing XML parse errors.
- **Fix:** Added `nsMap` and `nsCounter` to `parseProppatchOps` to track per-namespace prefixes, plus a `prefixStack` to ensure closing tags use the same prefix as their matching opening tag.
- **Files modified:** web/webdav/proppatch.go

**4. [Rule 2 - Missing Critical Functionality] MOVE did not transfer dead properties**
- **Found during:** Task 2 (litmus propmove test — "No value given for property prop9" after moving /litmus/prop to /litmus/prop2)
- **Issue:** `handleMove` in `move.go` did not call `deadPropStore.movePropsForPath` after renaming the VFS resource
- **Fix:** Added `deadPropStore.movePropsForPath(inst.Domain, srcPath, dstPath)` after the VFS modify call
- **Files modified:** web/webdav/move.go

## Self-Check: PASSED

All key files exist. All commits verified in git log. litmus props 30/30 on both routes confirmed.
