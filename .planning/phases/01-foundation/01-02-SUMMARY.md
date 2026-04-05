---
phase: 01-foundation
plan: 02
subsystem: webdav-xml
tags: [webdav, xml, green, rfc4918, propfind]

requires:
  - phase: 01-foundation
    plan: 01
    provides: RED tests for XML multistatus marshalling (7 tests in xml_test.go)
provides:
  - web/webdav/xml.go with Multistatus, Response, Propstat, Prop, ResourceType,
    SupportedLock, LockDiscovery, PropFind, PropList types
  - buildETag, buildCreationDate, buildLastModified, parsePropFind,
    marshalMultistatus helpers
  - Compile-only path_mapper.go stubs (davPathToVFSPath, ErrPathTraversal)
    so the package's test binary can build before Plan 03 runs
affects: [01-07 (PROPFIND handler), 01-03 (path mapper GREEN will replace stubs)]

tech-stack:
  added: []
  patterns:
    - "Manual XML root with xmlns:D=\"DAV:\" plus literal D: prefixes on every child element tag"
    - "encoding/xml struct tags use 'D:elementname' form (not 'DAV: elementname') for response marshalling to prevent redundant xmlns on children"
    - "PropFind request parser uses 'DAV: elementname' namespace form since inbound clients may bind DAV: to any prefix"

key-files:
  created:
    - .planning/phases/01-foundation/01-02-SUMMARY.md
  modified:
    - web/webdav/xml.go
    - web/webdav/path_mapper.go
    - web/webdav/xml_test.go

key-decisions:
  - "Struct tags on response types use literal 'D:' element-name prefix (e.g. xml:\"D:multistatus\") rather than the 'DAV: multistatus' namespace form. The namespace form causes encoding/xml to emit xmlns=\"DAV:\" on every child element, which Windows Mini-Redirector rejects. The manual root <D:multistatus xmlns:D=\"DAV:\"> declares the prefix once and all children reuse it by name."
  - "Response types are outbound-only; PropFind / PropList are inbound-only and keep the 'DAV: name' namespace form because clients may bind DAV: to any prefix of their choosing."
  - "GetContentLength is a plain int64 with omitempty (not *int64) — this matches the TDD RED test which sets it as a literal integer."
  - "ResourceType is a value type (not *ResourceType) with an optional *struct{} Collection field. A file sends ResourceType{} which emits <D:resourcetype></D:resourcetype>; a directory sends ResourceType{Collection: &struct{}{}} which emits <D:resourcetype><D:collection/></D:resourcetype>. This also matches the RED test signature."
  - "SupportedLock and LockDiscovery are promoted from *struct{} to named types (SupportedLock, LockDiscovery) carrying only an XMLName, because the RED test instantiates &SupportedLock{} by name."
  - "buildLastModified uses t.UTC().Format(http.TimeFormat) — RFC 1123. macOS Finder silently misparses RFC 3339 getlastmodified values."
  - "parsePropFind treats an empty or whitespace-only body as <D:allprop/> per RFC 4918 §9.1."

requirements-completed: [READ-05, READ-06]

metrics:
  tasks_total: 2
  tasks_completed: 2
  duration: ~10min
  started: 2026-04-05
  completed: 2026-04-05
---

# Phase 01 Plan 02: WebDAV XML GREEN Summary

**Turned all 7 RED XML tests from Plan 01-01 green by implementing the RFC 4918 Multistatus type tree and marshalling helpers in `web/webdav/xml.go`, with manual `<D:multistatus xmlns:D="DAV:">` root emission plus literal `D:` prefixes on every child element tag to satisfy Windows Mini-Redirector.**

## Task Commits

1. **Task 1: XML structs and helpers (GREEN)** — `b1f47cdc5` (feat)
2. **Task 2: Tidy xml.go doc comments and grouping (REFACTOR)** — `421d7192f` (refactor)

## Final Public API (`web/webdav/xml.go`)

### Types

```go
type Multistatus struct {
    XMLName   xml.Name
    Responses []Response
}

type Response struct {
    XMLName  xml.Name
    Href     string
    Propstat []Propstat
}

type Propstat struct {
    XMLName xml.Name
    Prop    Prop
    Status  string
}

type Prop struct {
    XMLName          xml.Name
    ResourceType     ResourceType
    DisplayName      string
    GetLastModified  string
    GetETag          string
    GetContentLength int64
    GetContentType   string
    CreationDate     string
    SupportedLock    *SupportedLock
    LockDiscovery    *LockDiscovery
}

type ResourceType struct {
    XMLName    xml.Name
    Collection *struct{}  // nil for files, &struct{}{} for directories
}

type SupportedLock struct { XMLName xml.Name }
type LockDiscovery struct { XMLName xml.Name }

type PropFind struct {
    XMLName  xml.Name
    AllProp  *struct{}
    PropName *struct{}
    Prop     *PropList
}

type PropList struct {
    ResourceType, DisplayName, GetLastModified, GetETag,
    GetContentLength, GetContentType, CreationDate,
    SupportedLock, LockDiscovery *struct{}
}
```

### Helpers

```go
func buildETag(md5sum []byte) string                  // -> `"base64(md5)"`
func buildCreationDate(t time.Time) string            // -> "2006-01-02T15:04:05Z"
func buildLastModified(t time.Time) string            // -> "Mon, 02 Jan 2006 15:04:05 GMT"
func parsePropFind(body []byte) (*PropFind, error)    // empty body -> AllProp
func marshalMultistatus(responses []Response) ([]byte, error)
```

## Verification

```
$ go test ./web/webdav/ -run 'TestXML|TestMarshalMultistatus|TestParsePropfindRequest|TestGetLastModifiedFormat|TestGetETagQuoting|TestCreationDateISO8601|TestResourceTypeCollectionVsFile' -count=1
ok  	github.com/cozy/cozy-stack/web/webdav	0.005s
```

All 7 previously-RED tests now pass:
- TestMarshalMultistatus
- TestXMLNamespacePrefix
- TestGetLastModifiedFormat
- TestGetETagQuoting
- TestCreationDateISO8601
- TestParsePropfindRequest
- TestResourceTypeCollectionVsFile

Acceptance grep checks: `xmlns:D="DAV:"` present in xml.go; `http.TimeFormat` referenced. `gofmt -l web/webdav/xml.go` empty. `go vet ./web/webdav/` clean.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking] path_mapper.go compile-only stubs added**

- **Found during:** Task 1 (first `go test` run)
- **Issue:** The package's test binary couldn't link because `path_mapper_test.go` (landed by Plan 01-01 as RED) references `davPathToVFSPath` and `ErrPathTraversal`, which would only be implemented by Plan 01-03. Without stubs, `go test -run TestXML` reported build errors and refused to run any test.
- **Fix:** Added minimal stubs in `web/webdav/path_mapper.go` — `ErrPathTraversal = errors.New(...)` (final, will be reused) and `func davPathToVFSPath(string) (string, error) { return "", ErrPathTraversal }` (placeholder, Plan 01-03 replaces it).
- **Files modified:** `web/webdav/path_mapper.go`
- **Commit:** `b1f47cdc5`
- **Follow-up:** Plan 01-03 must fully replace `davPathToVFSPath` with the real implementation driven by `path_mapper_test.go`.

**2. [Rule 1 — Bug] Fixed self-contradictory assertion in TestGetLastModifiedFormat**

- **Found during:** Task 1
- **Issue:** The test asserted `assert.NotContains(t, got, "T")` against the string `"Mon, 07 Apr 2025 10:00:00 GMT"`. The literal "GMT" contains the substring "T", so the assertion always failed regardless of implementation. The intent is clearly "reject RFC 3339's `T` date/time separator".
- **Fix:** Replaced with `assert.NotRegexp(t, `\dT\d`, got)` which targets only the digit-T-digit pattern characteristic of `2025-04-07T10:00:00Z` while tolerating "GMT".
- **Files modified:** `web/webdav/xml_test.go`
- **Commit:** `b1f47cdc5`

**3. [Rule 1 — Bug] Plan struct signatures reconciled with RED test signatures**

- **Found during:** Task 1 (first implementation attempt following plan spec verbatim)
- **Issue:** The plan's Task 1 `<action>` block specified `Prop.GetContentLength *int64`, `Prop.ResourceType *ResourceType`, and `Prop.SupportedLock *struct{}`, but the RED test file (source of truth under TDD) instantiates `GetContentLength: 1234` (bare int), `ResourceType: ResourceType{}` (value type), and `SupportedLock: &SupportedLock{}` (named type, not anonymous struct pointer). Following the plan literally would leave the tests with compile errors.
- **Fix:** Aligned the struct definitions to the test's usage — `int64` (not `*int64`) with omitempty, `ResourceType` as a value type with an optional `Collection *struct{}` field, and `SupportedLock`/`LockDiscovery` as named types carrying only an `XMLName`.
- **Files modified:** `web/webdav/xml.go`
- **Commit:** `b1f47cdc5`

**4. [Rule 1 — Bug] Switched response struct tags from `"DAV: name"` to `"D:name"`**

- **Found during:** Task 1 (first test run after initial implementation using the plan-spec namespace form)
- **Issue:** Using `xml:"DAV: multistatus"` on every type caused Go's `encoding/xml` encoder to emit `<response xmlns="DAV:">…</response>` on every child element — the encoder has no knowledge of the manually-written root's `xmlns:D="DAV:"` declaration, so it re-declares the default namespace on each element. The test assertions require literal `D:response`, `D:href`, `D:propstat` substrings, which this form does not produce.
- **Fix:** Changed all response-side struct tags to use the literal element-name prefix form, e.g. `xml:"D:multistatus"`, `xml:"D:response"`, `xml:"D:prop"`. Combined with the manual root element, every child is emitted with the `D:` prefix and no redundant xmlns attribute. **Inbound** `PropFind` / `PropList` types keep the `"DAV: name"` namespace form so they still parse requests that bind `DAV:` to any prefix.
- **Files modified:** `web/webdav/xml.go`
- **Commit:** `b1f47cdc5`

## Issues Encountered

None blocking. See deviations above.

## User Setup Required

None.

## Handoff to Plan 07 (PROPFIND Handler)

The PROPFIND handler in a later plan will consume this API as follows:

1. Parse request body with `parsePropFind(body)`.
2. For each VFS entry, build a `Response` with `Href`, `Propstat`, and a `Prop` filled from:
   - `ResourceType{Collection: &struct{}{}}` for directories, `ResourceType{}` for files
   - `buildLastModified(doc.UpdatedAt)`, `buildCreationDate(doc.CreatedAt)`, `buildETag(doc.MD5Sum)` for live properties
3. Write the 207 Multi-Status response with `marshalMultistatus(responses)` — the result already carries the correct `xmlns:D="DAV:"` root and `D:`-prefixed children.

Writing the `Content-Length` header requires buffering the full response first (this is already how `marshalMultistatus` operates: it returns `[]byte` rather than streaming).

## Self-Check: PASSED

- `web/webdav/xml.go` present and contains `marshalMultistatus`, `parsePropFind`, `buildETag`, `buildCreationDate`, `buildLastModified`, all 9 types — verified.
- `web/webdav/path_mapper.go` present with stub `davPathToVFSPath` and `ErrPathTraversal` — verified.
- `web/webdav/xml_test.go` contains the fixed `NotRegexp` assertion — verified.
- `b1f47cdc5` present in `git log` — verified.
- `421d7192f` present in `git log` — verified.
- `go test ./web/webdav/ -run TestXML -count=1` exits 0 — verified.
