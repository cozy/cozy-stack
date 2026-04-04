# Testing Patterns

**Analysis Date:** 2026-04-04

## Test Framework

**Runner:**
- Go's built-in `testing` package (Go 1.25+)
- No external test runner — all tests use `go test`

**Assertion Library:**
- `github.com/stretchr/testify v1.11.1`
- Both `assert` (non-fatal) and `require` (fatal) are used
- `require` is used for setup/preconditions; `assert` for individual checks within a test

**HTTP Testing:**
- `github.com/gavv/httpexpect/v2 v2.16.0` — fluent HTTP client for integration tests
- `net/http/httptest` — for spinning up test servers

**Mocking:**
- `github.com/stretchr/testify/mock` — for mock objects on service interfaces

**Containers (infrastructure):**
- `github.com/testcontainers/testcontainers-go v0.40.0` — used for RabbitMQ and ClamAV in `tests/testutils/`

**Run Commands:**
```bash
go test -p 1 -timeout 5m ./...      # Full test suite (CI — requires CouchDB + Redis)
go test -p 1 -timeout 2m -short ./... # Unit tests only (no external services)
make unit-tests                      # Alias for go test -p 1 -timeout 2m -short ./...
make system-tests                    # Ruby-based system tests (scripts/system-test.sh)
```

## Test File Organization

**Location:**
- Co-located: test files sit next to source files (`pkg/couchdb/couchdb_test.go` alongside `pkg/couchdb/couchdb.go`)
- Shared test helpers: `tests/testutils/test_utils.go`, `tests/testutils/rabbitmq_utils.go`, `tests/testutils/clamav_utils.go`
- System (end-to-end) tests: Ruby scripts in `tests/system/tests/*.rb`

**Naming:**
- Test files: `<subject>_test.go`
- Test functions: `func TestSubjectName(t *testing.T)`
- Sub-test names: PascalCase strings passed to `t.Run` — `"CreateDoc"`, `"SET/GET/EXPIRE"`, `"AcceptsPermissionsField"`

**Package naming:**
- Most test files use the same package as the source (`package files`, `package couchdb`)
- External/black-box tests use `<pkg>_test` suffix (`package sharings_test`, `package settings_test`, `package apps_test`)
- Both styles coexist in the project; external-package tests are used for packages that benefit from black-box coverage

**Structure:**
```
pkg/couchdb/
├── couchdb.go
├── couchdb_test.go      # internal package test
├── errors.go
├── errors_test.go
└── ...

model/settings/
├── service.go
├── service_mock.go      # mock alongside interface
├── service_test.go
└── storage_mock.go

tests/
├── testutils/
│   ├── test_utils.go         # TestSetup, NeedCouchdb, CreateTestClient
│   ├── rabbitmq_utils.go     # StartRabbitMQ, RabbitFixture
│   └── clamav_utils.go       # ClamAV container helpers
└── system/
    └── tests/
        ├── sharing_several_members.rb
        ├── webdav_nextcloud.rb
        └── ...
```

## Test Structure

**Top-level suite pattern:**
```go
func TestFiles(t *testing.T) {
    if testing.Short() {
        t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
    }

    config.UseTestFile(t)
    testutils.NeedCouchdb(t)
    setup := testutils.NewSetup(t, t.Name())
    inst := setup.GetTestInstance()
    _, token := setup.GetTestClient(consts.Files)

    ts := setup.GetTestServer("/files", Routes)
    t.Cleanup(ts.Close)

    t.Run("SubTestName", func(t *testing.T) {
        e := testutils.CreateTestClient(t, ts.URL)
        // ...
    })
}
```

**Integration test with `--short` skip:**
All integration tests requiring CouchDB or a running instance guard with:
```go
if testing.Short() {
    t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
}
```

Unit tests that work in isolation do not include this guard.

**`t.Cleanup` for resource teardown:**
- Used consistently instead of `defer` for test-scoped cleanup
- Server teardown: `t.Cleanup(ts.Close)`
- Instance teardown: registered automatically inside `setup.GetTestInstance()`
- Permission cleanup: `t.Cleanup(func() { _ = permission.DestroyWebapp(instance, slug) })`

**`config.UseTestFile(t)`:**
Reads a `cozy.test.yaml` config from `$HOME/.cozy/` or falls back to defaults. Required in any test that touches config, CouchDB, or the stack.

**`testutils.NeedCouchdb(t)`:**
Calls `t.Fatal` if CouchDB is unreachable. Used at the top of integration tests instead of `testing.Short()` skip.

## Mocking

**Framework:** `github.com/stretchr/testify/mock`

**Pattern for mock files (`*_mock.go`):**
```go
// Mock implementation of [Emailer].
type Mock struct {
    mock.Mock
}

// NewMock instantiates a new [Mock].
func NewMock(t *testing.T) *Mock {
    m := new(Mock)
    m.Test(t)
    t.Cleanup(func() { m.AssertExpectations(t) })
    return m
}

// SendEmail mock method.
func (m *Mock) SendEmail(inst *instance.Instance, cmd *TransactionalEmailCmd) error {
    return m.Called(inst, cmd).Error(0)
}
```

Mock files live in the same package as the interface they implement (not in `_test.go` files), making them available to both tests and production code where needed.

**Mock usage in tests:**
```go
brokerMock := job.NewBrokerMock(t)
brokerMock.On("PushJob", &inst, mock.MatchedBy(func(req *job.JobRequest) bool {
    assert.Equal(t, "sendmail", req.WorkerType)
    return true
})).Return(nil, nil).Once()
```

**Known mock files:**
- `pkg/emailer/service_mock.go` — `Emailer` interface
- `model/cloudery/service_mock.go` — cloudery service
- `model/settings/service_mock.go` — settings `Service`
- `model/settings/storage_mock.go` — settings `Storage`
- `model/token/service_mock.go` — token service
- `model/job/broker_mock.go` — job `Broker`
- `model/instance/service_mock.go` — instance service

**What to Mock:**
- External services (job broker, emailer, cloudery API)
- Interfaces that require running infrastructure (databases, queues)

**What NOT to Mock:**
- CouchDB (integration tests use a real CouchDB — `testutils.NeedCouchdb(t)`)
- The VFS (tests use a real in-memory `afero` or local-file VFS via `t.TempDir()`)
- Echo HTTP handling (tests spin up a real `httptest.Server`)

## Fixtures and Factories

**Test setup via `testutils.TestSetup`:**
```go
setup := testutils.NewSetup(t, t.Name())
inst := setup.GetTestInstance()              // creates real CouchDB instance
client, token := setup.GetTestClient(scopes) // creates OAuth client + JWT
ts := setup.GetTestServer("/prefix", Routes) // starts httptest.Server
e := testutils.CreateTestClient(t, ts.URL)   // creates httpexpect client
```

`TestSetup` registers all cleanup via `t.Cleanup` — no manual teardown needed.

**Swift storage for VFS tests:**
```go
setup.SetupSwiftTest() // starts in-memory Swift server
```

**Feature flags in tests:**
```go
testutils.WithFlag(t, inst, "flag-name", true) // sets flag, restores on cleanup
```

**Manager config in tests:**
```go
testutils.WithManager(t, inst, testutils.ManagerConfig{URL: "http://..."})
```

**Testdata directories:**
- `pkg/config/config/testdata/full_config.yaml` — full config fixture for config parsing tests
- `tests/testutils/testdata/` — TLS certificates for RabbitMQ TLS tests
- `pkg/avatar/testdata/` — image fixtures for avatar tests
- `pkg/rabbitmq/testdata/` — RabbitMQ fixtures

**`testutils.TODO` helper:**
```go
testutils.TODO(t, "2025-01-01", "Expected this to be fixed by now")
// Fails the test after the given date — enforces deadline-driven TODO resolution
```

**`testutils.WaitForOrFail`:**
```go
testutils.WaitForOrFail(t, 5*time.Second, func() bool {
    // poll condition
})
```
Used for async assertions (e.g., realtime events, job processing).

## HTTP Integration Test Pattern

Integration tests for HTTP handlers use `httpexpect` for fluent assertions:

```go
e := testutils.CreateTestClient(t, ts.URL)

obj := e.POST("/files/"+dirID).
    WithQuery("Name", "myfile.txt").
    WithQuery("Type", "file").
    WithHeader("Authorization", "Bearer "+token).
    WithHeader("Content-Type", "text/plain").
    WithBytes([]byte("content")).
    Expect().Status(201).
    JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
    Object().Path("$.data.id").String().NotEmpty().Raw()
```

The `--debug` flag enables verbose request/response printing:
```bash
go test ./web/files --debug
```

## Coverage

**Configuration:** `tests/codecov.yml`
- Target range: 50–80%
- Excluded: `web/statik` (generated code)

**View Coverage:**
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

No enforced per-package minimum; Codecov is used for tracking trends on CI.

## Test Types

**Unit Tests (no infrastructure):**
- Scope: pure Go logic — string manipulation, data structures, algorithm correctness
- Examples: `pkg/utils/utils_test.go`, `model/sharing/revisions_test.go`, `pkg/avatar/service_test.go`
- Can run with `--short` flag or independently

**Integration Tests (require CouchDB + Redis):**
- Scope: HTTP handler round-trips, database operations, instance lifecycle
- Guard: `if testing.Short() { t.Skip(...) }` or `testutils.NeedCouchdb(t)`
- Examples: `web/files/files_test.go`, `web/notes/notes_test.go`, `web/sharings/sharings_test.go`
- These form the majority of the test suite (143 test files total)

**Container-based Tests (require Docker):**
- Scope: RabbitMQ broker, ClamAV antivirus
- Container lifecycle managed by `testcontainers-go` via helpers in `tests/testutils/`
- Examples: `pkg/rabbitmq/rabbitmq_test.go`, `web/files/antivirus_test.go`

**System Tests (Ruby, require full stack):**
- Scope: end-to-end multi-instance scenarios (sharing, export/import, WebDAV)
- Location: `tests/system/tests/*.rb`
- Run via: `make system-tests` (CI: `.github/workflows/system-tests.yml`)
- Notable: `tests/system/tests/webdav_nextcloud.rb` — WebDAV integration with Nextcloud

## CI Matrix

Defined in `.github/workflows/go-tests.yml`:
- Ubuntu 22.04
- Go 1.25.x (minimum) and 1.26.x (maximum)
- CouchDB 3.2.3 (minimum) and 3.3.3 (maximum)
- Redis (service container on port 6379)
- Additional system dependencies: ghostscript, ImageMagick

---

*Testing analysis: 2026-04-04*
