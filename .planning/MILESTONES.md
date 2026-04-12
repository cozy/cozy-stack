# Milestones

## v1.1 WebDAV RFC 4918 Class 1 (Shipped: 2026-04-12)

**Phases:** 3 — Foundation, Write Operations, Copy/Compliance/Docs
**Plans:** 24 (9 + 5 + 10), all executed
**Timeline:** 2026-04-04 → 2026-04-12 (8 days)
**Git range:** `273602d13` → `fa1cf6778` (feat/webdav branch)
**Code:** ~2311 LOC production + ~2760 LOC tests in `web/webdav/`
**Documentation:** `docs/webdav.md` (587 lines, 27 curl examples)

### Delivered

A WebDAV server integrated into cozy-stack that exposes each user's `/files/` tree as a mountable network filesystem. RFC 4918 Class 1 strict compliance, served on two URL prefixes (`/dav/files/` native + `/remote.php/webdav/` Nextcloud-compat), authentication via Basic (token-as-password) or OAuth Bearer, streaming PUT with conditional If-Match/If-None-Match, soft-delete via cozy trash, recursive COPY/MOVE with Overwrite semantics, minimal in-memory PROPPATCH (dead property storage), and first-class Cozy Note support in COPY (delegates to note.CopyFile when Mime matches).

### Key accomplishments

1. **10 WebDAV methods implemented and wired on both routes** (OPTIONS, PROPFIND, PROPPATCH, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE) with identical handlers shared by both route prefixes.
2. **External compliance validated by litmus 63/63 on both routes** — basic 16/16, copymove 13/13, props 30/30, http 4/4; locks auto-skipped (Class 1 only).
3. **Multi-tier test coverage** — 50+ unit tests, 6 E2E gowebdav sub-tests (real WebDAV client library), litmus orchestration script (`make test-litmus`), and retrospective manual validation against OnlyOffice Documents Android v9.1.0 (pre-regression APK).
4. **Strict TDD discipline enforced** — every handler in Phase 3 landed via separate RED (failing test) and GREEN (implementation) commits per TEST-07.
5. **Security invariants baked in from Phase 1** — path traversal prevention, auth isolation, Depth:infinity guard, audit logging on write verbs, Content-Length policy, ETag consistency.
6. **User-facing documentation complete** — 7 sections, method table, client config guides (rclone, curl, OnlyOffice mobile, iOS Files), compatibility notes, troubleshooting, compliance testing procedure.

### Scope reductions (documented in v1.1-REQUIREMENTS.md)

- iOS/iPadOS Files app manual validation deferred to v1.2 (best-effort; covered transitively by litmus).
- CI integration of litmus deferred post-v1 (manual `make test-litmus` only).
- OnlyOffice mobile manual validation deferred pending upstream fix v9.3.2+ (server-side validated against v9.1.0 APK; traced in `03-MANUAL-VALIDATION-OO-MOBILE.md`).

### Known tech debt carried forward

- **FOLLOWUP-01** — pre-existing race in `pkg/config` / `model/stack` / `model/job` test harness (not WebDAV code). Reproducible on master without any Phase 1 change. Ships with documented caveat; strongly recommended to address as first task of v1.2.

### Audit

Full audit report: `.planning/milestones/v1.1-MILESTONE-AUDIT.md` — status **passed**, 53/53 requirements satisfied with 3-source agreement (REQUIREMENTS traceability + VERIFICATION tables + SUMMARY frontmatter), 8/8 integration wiring paths traced with zero orphans, Nyquist COMPLIANT on all phases after audit-phase corrections.

---
