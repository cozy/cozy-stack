# Coding Conventions

**Analysis Date:** 2026-04-04

## Naming Patterns

**Files:**
- Lowercase snake_case: `couchdb_indexer.go`, `rate_limiting.go`
- Test files: `<subject>_test.go`, co-located with source
- Mock files: `<subject>_mock.go`, co-located with the interface they mock (e.g., `service_mock.go`, `broker_mock.go`)
- Sub-packages using compound lowercase names: `vfsswift`, `vfsafero`

**Functions and Methods:**
- Exported functions: PascalCase — `CreateDoc`, `UseTestFile`, `GetTestInstance`
- Unexported functions: camelCase — `makeTestDoc`, `wrapVfsError`, `sanitizeSetupName`
- HTTP handler functions: PascalCase verb or noun — `Create`, `SharedDrivesCreationHandler`
- Route registration: always named `Routes(router *echo.Group)` or `<Qualifier>Routes`

**Variables:**
- Exported package-level errors: `ErrNotFound`, `ErrSkipDir`, `ErrImpossibleMerge`
- Unexported package-level errors: `errFailFast`, `errRevokeSharing`
- Exported package-level vars: PascalCase — `MaxString`, `ForbiddenFilenameChars`

**Types:**
- Exported struct types: PascalCase — `Instance`, `Document`, `Client`
- Interfaces end with a descriptive noun, not `-er` suffix by default — `Fs`, `Doc`, `Indexer`, `Emailer`
- Mock types are named `Mock` and live in the same package as the interface — `type Mock struct { mock.Mock }`

**Constants:**
- Exported constants: PascalCase — `TrashDirName`, `MaxDepth`, `PBKDF2_SHA256`
- The project uses a `//lint:ignore ST1003` directive when ALL_CAPS is intentional (e.g., `PBKDF2_SHA256`)
- CouchDB doctypes: reverse-DNS notation as string consts in `pkg/consts/` — `"io.cozy.files"`, `"io.cozy.notes.documents"`

## Code Style

**Formatting:**
- `gofmt` enforced via golangci-lint formatter configuration (`.golangci.yaml`)
- golangci-lint v2.11.4 (version pinned in `.golangci-lint-version`)

**Linting:**
Tools enabled via `.golangci.yaml` (default linters disabled, explicit opt-in):
- `bidichk` — detect dangerous Unicode bidirectional control characters
- `errname` — enforce error type naming conventions
- `forbidigo` — `fmt.Printf` is forbidden (use `fmt.Fprintf(os.Stdout, ...)` instead)
- `gocritic` — general code quality (disabled checks: `appendAssign`, `ifElseChain`, `argOrder`)
- `govet` — standard vet checks
- `misspell` — catch common spelling mistakes in comments
- `nolintlint` — every `//nolint` directive must name the specific linter and include an explanation
- `unconvert` — remove unnecessary type conversions
- `unused` — detect unused code
- `whitespace` — trailing whitespace

## Import Organization

**Order (standard Go gofmt grouping):**
1. Standard library packages (e.g., `"bytes"`, `"encoding/json"`, `"net/http"`)
2. Third-party packages (e.g., `"github.com/labstack/echo/v4"`, `"github.com/stretchr/testify/assert"`)
3. Internal packages (e.g., `"github.com/cozy/cozy-stack/model/instance"`, `"github.com/cozy/cozy-stack/pkg/config/config"`)

Blank import lines separate groups. Blank imports (side-effect only) are placed last in the import block with a comment, e.g.:
```go
import (
    _ "github.com/cozy/cozy-stack/web/statik"
    _ "github.com/cozy/cozy-stack/worker/thumbnail"
)
```

**Path Aliases:**
- Used when two packages share a name: `build "github.com/cozy/cozy-stack/pkg/config"`, `webRealtime "github.com/cozy/cozy-stack/web/realtime"`, `modelsharing "github.com/cozy/cozy-stack/model/sharing"`

## Error Handling

**Patterns:**
- Sentinel errors declared as package-level vars using `errors.New`: `var ErrNotFound = errors.New("...")`
- Exported errors are PascalCase `Err`-prefixed; unexported are camelCase `err`-prefixed
- Error wrapping uses `fmt.Errorf("...: %w", err)` — 182 instances across `model/` alone
- `errors.Is` and `errors.As` used for inspection throughout
- HTTP handler errors returned directly as `error` to Echo; the central `ErrorHandler` in `web/errors/errors.go` maps them to JSON-API or HTML responses
- CouchDB errors are typed as `*couchdb.Error` and converted to `*jsonapi.Error` in the error handler
- VFS errors are wrapped via `wrapVfsError(err)` in `web/files/files.go`

**Error Handler:**
`web/errors/errors.go` contains `ErrorHandler` (JSON-API routes) and `HTMLErrorHandler` (browser routes). Both are registered on the Echo instance via `ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler`.

## Logging

**Framework:** `github.com/sirupsen/logrus` wrapped by the internal `pkg/logger` package

**Logger interface** (`pkg/logger/logger.go`):
```go
type Logger interface {
    Debugf(format string, args ...interface{})
    Infof(format string, args ...interface{})
    Warnf(format string, args ...interface{})
    Errorf(format string, args ...interface{})
    WithField(fn string, fv interface{}) Logger
    WithFields(fields Fields) Logger
    WithDomain(s string) Logger
}
```

**Patterns:**
- Instance-scoped logging: `inst.Logger().WithNamespace("files").Warnf("...")`
- Package-level logging: `logger.WithNamespace("http").Errorf("...")`
- Namespaces group logs by subsystem — `"files"`, `"http"`, `"couchdb"`
- Debug logs only emitted in dev release: `if build.IsDevRelease() { ... log.Errorf(...) }`

## Comments

**When to Comment:**
- Every exported type, function, and constant has a doc comment starting with the identifier name
- Package-level doc comment at top of main file: `// Package vfs is for ...`
- Inline comments for non-obvious logic, especially around CouchDB quirks and concurrency
- `XXX` prefix for known issues/workarounds that need attention but aren't immediate bugs
- `TODO` prefix for planned work items

**Example doc comment style:**
```go
// RTEvent published a realtime event for a couchDB change
func RTEvent(db prefixer.Prefixer, verb string, doc, oldDoc Doc) {
```

## Function Design

**Size:** Handlers in `web/` can be long (200+ lines) covering full HTTP lifecycle; model functions tend to be shorter
**Parameters:** Echo handlers follow `func Name(c echo.Context) error` signature
**Return Values:** Consistently returns `error` as final return value; multiple returns use named results sparingly

## Module Design

**Exports:** Each package exports a focused public API; unexported types are used freely for internal state
**Interface placement:** Interfaces are declared in the package that uses them, not the package that implements them (e.g., `Emailer` in `pkg/emailer`, `Broker` in `model/job`)
**Mock placement:** `*_mock.go` files live alongside the interface they mock in the same package; never in test files

## Type Usage

- `interface{}` still predominates in older code (442 occurrences in `model/`); `any` is used in newer code (51 occurrences)
- JSON struct tags always include `omitempty` for optional fields
- CouchDB document fields use `json:"_id,omitempty"` and `json:"_rev,omitempty"` consistently

---

*Convention analysis: 2026-04-04*
