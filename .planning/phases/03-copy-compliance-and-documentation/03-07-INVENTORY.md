# Phase 03 Plan 07: Litmus Props Suite — First-Run Inventory

**Date:** 2026-04-12
**Stack commit:** 60c13ede0
**Litmus version:** 0.13

---

## Pre-Run Fix: litmus script domain name length

Before the first real run, a blocking issue was found in `scripts/webdav-litmus.sh`:
the `TIMESTAMP=$(date +%Y%m%d-%H%M%S)` pattern produced domain names like
`litmus-20260412-171207.localhost:8080` (37 chars), which caused the generated JWT token
to be 260 characters — exceeding litmus's hard limit of 256 chars for the password field.
Litmus rejected the token with "password must be <256 chars", aborting test 0 (init).

**Fix applied (Rule 3 — blocking issue):** Changed timestamp format to Unix epoch
(`date +%s`) and shortened domain prefix to `lm-`, yielding `lm-1776006772.localhost:8080`
(28 chars) and token length of 248 chars.

---

## First-Run Results

### Route 1: `/dav/files/`

| Test # | Test Name | Result |
|--------|-----------|--------|
| 0 | init | PASS |
| 1 | begin | PASS |
| 2 | propfind_invalid | **FAIL** |
| 3 | propfind_invalid2 | **FAIL** |
| 4 | propfind_d0 | PASS |
| 5 | propinit | PASS |
| 6 | propset | **FAIL** |
| 7 | propget | SKIPPED (depends on propset) |
| 8 | propextended | PASS |
| 9–22 | propmove, propdeletes, propreplace, propnullns, prophighunicode, propremoveset, propsetremove, propvalnspace, propget×8 | SKIPPED (chain depends on propset) |
| 24 | propwformed | PASS |
| 25 | propinit | PASS |
| 26 | propmanyns | **FAIL** |
| 27 | propget | **FAIL** (depends on propmanyns) |
| 28 | propcleanup | PASS |
| 29 | finish | PASS |

**Summary: 9 passed, 5 failed, 16 skipped (64.3% of run tests)**

### Route 2: `/remote.php/webdav/`

| Test # | Test Name | Result |
|--------|-----------|--------|
| 0 | init | PASS |
| 1 | begin | PASS |
| 2 | propfind_invalid | **FAIL** |
| 3 | propfind_invalid2 | **FAIL** |
| 4 | propfind_d0 | **FAIL** (WARNING: response href for wrong resource + No responses returned) |
| 5 | propinit | PASS |
| 6 | propset | **FAIL** |
| 7–22 | (chain) | SKIPPED |
| 24 | propwformed | PASS |
| 25 | propinit | PASS |
| 26 | propmanyns | **FAIL** |
| 27 | propget | **FAIL** |
| 28 | propcleanup | PASS |
| 29 | finish | PASS |

**Summary: 8 passed, 6 failed, 16 skipped (57.1% of run tests)**

---

## Failure Analysis

### Group A: PROPPATCH failures (propset, propmanyns)

**Error:** `PROPPATCH on '/dav/files/litmus/prop': 405 Method Not Allowed`

PROPPATCH is NOT in `webdavMethods` and thus falls through to Echo's default 405 handler.
All subsequent prop* tests that depend on propset/propmanyns are SKIPPED (not FAILED) because
litmus tracks dependency chains.

**Determination:** These are pure PROPPATCH-routing failures. litmus counts these as FAILED
(not skipped/expected behavior). Strategy A (just returning 501) is NOT enough — litmus
treats 405 as a hard failure. Strategy B (207 with 403 per-property) is required.

### Group B: PROPFIND invalid-XML failures (propfind_invalid, propfind_invalid2)

**Error:** `PROPFIND with non-well-formed XML request body got 207 response not 400`
**Error:** `PROPFIND with invalid namespace declaration in body got 207 response not 400`

The current `handlePropfind` does not parse the request body at all — it uses a fixed
`allprop` response regardless of what the client sends in the body. This means malformed
XML bodies are silently ignored and a 207 is returned. RFC 4918 §9.1 requires 400 Bad Request
for unparseable request bodies.

**Fix:** Parse the PROPFIND body (if present and non-empty). If the XML fails to parse,
return 400 Bad Request. Empty bodies remain valid (treated as allprop).

### Group C: propfind_d0 on /remote.php/webdav/ (WARNING + FAIL)

**Error:** `WARNING: response href for wrong resource` + `No responses returned`

The PROPFIND handler always uses `davFilesPrefix = "/dav/files"` for building `<D:href>`
values regardless of which route (dav or Nextcloud) handled the request. When a PROPFIND
to `/remote.php/webdav/litmus/` returns hrefs prefixed with `/dav/files/litmus/`, litmus
cannot find the expected resource URL and counts it as a failure.

**Fix:** The PROPFIND handler needs to detect the incoming route prefix and use it for
href construction. The `echo.Context` has the full request URI — use that to determine
the correct href prefix.

---

## Strategy Decision: **Strategy B**

**Rationale:**
- PROPPATCH tests (propset, propmanyns) are hard FAIL (not skip) because litmus tries PROPPATCH
  and gets 405, not 501.
- A 501 response to PROPPATCH would likely also fail (litmus treats 501 as failure for
  PROPPATCH since Class 1 compliance does not require PROPPATCH to be 501-able — see
  RFC 4918 §8.1 which does not list PROPPATCH as optional for Class 1).
- Strategy B (207 Multi-Status with 403 per-property rejection) is the minimal implementation
  that makes litmus accept our "server rejects dead properties" stance cleanly.

**Implementation plan:**
1. Fix `propfind.go`: parse and validate XML body; return 400 on malformed XML
2. Fix `propfind.go`: use request URI prefix for href construction (fixes propfind_d0 on remote.php)
3. Add PROPPATCH to `webdavMethods` and `davAllowHeader` in `webdav.go`
4. Add `case "PROPPATCH": return handleProppatch(c)` to `handlePath` in `handlers.go`
5. Create `proppatch.go` with minimal 207+403 handler

**Non-PROPPATCH failures to fix separately:**
- propfind_invalid + propfind_invalid2: parse PROPFIND body for XML validity
- propfind_d0 (remote.php only): use dynamic href prefix instead of hardcoded `/dav/files`

---

## Strategy Pivot: **B → C**

During execution, Strategy B was implemented and reduced failures, but litmus 0.13 rejected
the 403-per-property response via `ne_propset_status()` — it expects **successful** PROPPATCH
writes before a subsequent PROPGET can read them back. Strategy C was adopted: a minimal
in-memory dead-property store (`deadprops.go`) that accepts PROPPATCH writes and returns
them on PROPFIND.

**Strategy C implementation:**
- `deadprops.go` — package-level `deadPropStore` with `setFor`, `listFor`, `removeFor`, `movePropsForPath` methods, protected by RWMutex
- `proppatch.go` — parses PROPPATCH body, applies sets/removes, returns 207 with 200 OK per-property (ns-prefix-aware XML builder)
- `propfind.go` — merges dead properties into the response via `buildDeadPropsXML`
- `move.go:132` — calls `deadPropStore.movePropsForPath` so dead properties follow resources (RFC 4918 §9.9.1)

Properties are **in-memory only**; lost on server restart. CouchDB-backed persistence is
documented as a v2 requirement.

---

## Final Run: PASS (30/30 both routes)

**Date:** 2026-04-12
**Stack commits:** b2121c180, f677ae35d, 0d398e1c2, 84011cd8a, dde5f907a, b49515ab9
**Command:** `LITMUS_TESTS=props scripts/webdav-litmus.sh`

### Route 1: `/dav/files/`

**Summary: 30 passed, 0 failed, 0 skipped (100%)**

All tests in the props suite pass: init, begin, propfind_invalid, propfind_invalid2,
propfind_d0, propinit, propset, propget, propextended, propmove, propdeletes, propreplace,
propnullns, prophighunicode, propremoveset, propsetremove, propvalnspace, propwformed,
propinit, propmanyns, propget variants, propcleanup, finish.

Warnings (cosmetic, do not cause FAIL): "Property N omitted from results with no status" —
emitted on some propget iterations. Litmus counts these as PASS regardless.

### Route 2: `/remote.php/webdav/`

**Summary: 30 passed, 0 failed, 0 skipped (100%)**

Identical result to Route 1, with the same cosmetic warnings on propget. The `propfind_d0`
href-prefix fix (route-aware href construction) is the differentiator vs first run.

### First-run → Final-run delta

| Issue | First-run | Final-run |
|-------|-----------|-----------|
| PROPPATCH returns 405 | ❌ FAIL | ✓ PASS (Strategy C in-memory store) |
| PROPFIND invalid XML returns 207 not 400 | ❌ FAIL | ✓ PASS (body parsing added) |
| propfind_d0 href wrong on /remote.php/ | ❌ FAIL (Route 2) | ✓ PASS (route-aware prefix) |
| propmanyns | ❌ FAIL | ✓ PASS (ns-prefix-aware PROPPATCH parser) |

**Verdict:** Phase 3 Plan 07 closed. TEST-06 props portion satisfied.
