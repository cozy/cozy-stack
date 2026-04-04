# Architecture

**Analysis Date:** 2026-04-04

## Pattern Overview

**Overall:** Multi-tenant monolith with layered architecture

The stack is a single Go binary ("cozy-stack") that serves all HTTP endpoints, runs background workers, and manages per-user instances. It is explicitly designed to be a single deployable unit rather than microservices. Each user ("instance") gets their own CouchDB prefix and VFS prefix. Multiple implementations can be swapped behind interfaces (e.g., Redis vs. in-memory broker, Swift vs. local filesystem).

**Key Characteristics:**
- One binary, two HTTP servers (main + admin), one job system
- Per-instance database isolation via CouchDB prefix (`pkg/prefixer`)
- Interface-driven: CouchDB layer, VFS, job broker, realtime hub, and services are all interface-backed
- JSON:API response format throughout the API layer
- Subdomain routing: app subdomains (`app.instance.cozy.cloud`) are proxied through the main router

## Layers

**CLI Entry (`cmd/`):**
- Purpose: Parse flags, load config, start the stack
- Location: `cmd/`
- Contains: Cobra commands — `serve`, `instances`, `apps`, `jobs`, `files`, etc.
- Depends on: `model/stack`, `pkg/config`, `web`
- Used by: `main.go` via `cmd.RootCmd.Execute()`

**HTTP Handlers (`web/`):**
- Purpose: Parse HTTP requests, call model layer, serialize JSON:API responses
- Location: `web/`
- Contains: One sub-package per domain area (files, auth, data, sharings, etc.)
- Depends on: `model/*`, `pkg/jsonapi`, `pkg/couchdb`, `web/middlewares`
- Used by: `web/routing.go` which registers all route groups on Echo

**Middleware (`web/middlewares/`):**
- Purpose: Cross-cutting request concerns — instance resolution, auth/permission checks, CORS, CSP, session loading
- Location: `web/middlewares/`
- Key files: `instance.go` (NeedInstance, GetInstance), `permissions.go` (AllowWholeType, AllowTypeAndID), `session.go` (LoadSession)
- Depends on: `model/instance`, `model/permission`, `model/oauth`, `model/session`
- Used by: `web/routing.go` (applies middleware chains to route groups)

**Model (`model/`):**
- Purpose: Business logic, domain entities, orchestration between pkg services
- Location: `model/`
- Contains: Sub-packages per domain: `instance`, `app`, `vfs`, `job`, `sharing`, `permission`, `session`, `oauth`, `note`, `nextcloud`, etc.
- Depends on: `pkg/couchdb`, `pkg/prefixer`, `pkg/realtime`, `pkg/config`
- Used by: `web/` handlers and `worker/` workers

**Workers (`worker/`):**
- Purpose: Asynchronous background processing (konnectors, thumbnails, emails, sharing, etc.)
- Location: `worker/`
- Contains: One sub-package per worker type — each calls `job.AddWorker(...)` in their `init()` function
- Depends on: `model/*`, `pkg/couchdb`
- Used by: `model/job` broker which dispatches queued jobs

**Infrastructure (`pkg/`):**
- Purpose: Low-level infrastructure primitives reused by model and web layers
- Location: `pkg/`
- Contains: `couchdb` (DB client + Doc interface), `prefixer` (instance identity), `realtime` (pub/sub hub), `config/config` (viper-based config), `jsonapi` (response helpers), `lock`, `cache`, `limits`, `logger`, `webdav` (WebDAV client), and more
- Depends on: External libraries only (Redis, CouchDB HTTP API, Swift, etc.)
- Used by: `model/*` and `web/*`

**Stack Bootstrapper (`model/stack/`):**
- Purpose: Initialize all services, connect to CouchDB + Swift, start the job system
- Location: `model/stack/main.go`
- Returns: `(utils.Shutdowner, *Services, error)` — the Services struct carries injectable service interfaces (Emailer, Settings, RabbitMQ)

## Data Flow

**Authenticated API Request (e.g., file upload):**

1. HTTP request arrives at the main Echo server (`web/server.go`)
2. `firstRouting` resolves subdomain: if it matches an app slug, route to app handler; otherwise fall through to the API router
3. Middleware chain runs: `NeedInstance` loads the `Instance` from CouchDB by domain → `LoadSession` reads the session cookie → `CheckPermissions` validates the OAuth token or session permission scope
4. Handler in `web/files/files.go` extracts the instance from echo context via `middlewares.GetInstance(c)`
5. Handler calls model functions: `vfs.CreateFile(inst.VFS(), ...)` which writes metadata to CouchDB and binary content to Swift or local disk
6. CouchDB wrapper fires a realtime event via `realtime.GetHub().Publish(...)` after each document write
7. Connected WebSocket clients (subscribed via `/realtime`) receive the event pushed from the hub
8. Handler returns JSON:API response

**Background Job Execution:**

1. An HTTP handler (or trigger) calls `job.System().PushJob(inst, &JobRequest{...})`
2. The broker (Redis or in-memory) queues the job
3. A worker goroutine picks up the job, calls the registered `WorkerFunc`
4. The worker imports the relevant model packages and performs the operation
5. Job state transitions (Queued → Running → Done/Errored) are persisted in CouchDB

**State Management:**
- Per-instance state is stored in CouchDB (prefixed per instance)
- Sessions stored in Redis (or in-memory) via `pkg/cache`
- Distributed locks via Redis or in-memory (`pkg/lock`)
- Realtime events distributed via Redis pub/sub or in-memory hub (`pkg/realtime`)

## Key Abstractions

**`couchdb.Doc` interface:**
- Purpose: Any document that can be stored in CouchDB
- Examples: `model/instance/instance.go` (Instance), `model/vfs/file.go` (FileDoc), `model/job/broker.go` (Job)
- Pattern: Structs implement `ID()`, `Rev()`, `DocType()`, `Clone()`, `SetID()`, `SetRev()`

**`prefixer.Prefixer` interface:**
- Purpose: Identify an instance by its DB cluster, prefix, and domain — passed to all CouchDB operations to scope queries
- Examples: `model/instance/instance.go` (Instance implements Prefixer), `pkg/prefixer/prefixer.go`
- Pattern: `inst.DBPrefix()` returns the CouchDB prefix for this user's documents

**`vfs.VFS` interface:**
- Purpose: Abstract filesystem operations (files + directories) over two backends
- Examples: `model/vfs/vfsafero/` (local/Afero), `model/vfs/vfsswift/` (OpenStack Swift)
- Pattern: Obtained from `inst.VFS()`, provides CreateFile, GetFileDocByPath, Walk, etc.

**`job.Broker` interface:**
- Purpose: Abstract job queue over Redis vs. in-memory
- Examples: `model/job/redis_broker.go`, `model/job/mem_broker.go`
- Pattern: `job.System().PushJob(db, request)` — the global system singleton dispatches

**`jsonapi.Object` interface:**
- Purpose: Serialize domain objects into JSON:API format
- Examples: Most model types implement `ID()`, `DocType()`, `Relationships()`, `Included()`, `Links()`
- Pattern: Handlers call `jsonapi.Data(c, statusCode, obj, included)` or `jsonapi.DataList(...)`

**`stack.Services` struct:**
- Purpose: Dependency injection container for services that need to be wired at startup
- Location: `model/stack/main.go`
- Pattern: Created by `stack.Start()`, passed down to route registrations that need it (e.g., `settings.NewHTTPHandler(services.Settings, ...)`)

## Entry Points

**`main.go`:**
- Location: `main.go`
- Triggers: Binary execution
- Responsibilities: Calls `cmd.RootCmd.Execute()`

**`cmd/serve.go` (`serveCmd`):**
- Location: `cmd/serve.go`
- Triggers: `cozy-stack serve`
- Responsibilities: Load config, call `stack.Start()` (initializes CouchDB, Swift, job system), then `web.ListenAndServe(services)` which starts the main + admin HTTP servers

**`web/routing.go` (`SetupRoutes`):**
- Location: `web/routing.go`
- Triggers: Called by `web.ListenAndServe`
- Responsibilities: Register all API route groups with their middleware chains; separate admin routes in `SetupAdminRoutes`

**`web/routing.go` (`firstRouting`):**
- Location: `web/routing.go`
- Triggers: Every incoming HTTP request
- Responsibilities: Subdomain split — if host is `slug.domain`, serve the app; otherwise fall through to API router or OIDC login domain

**Worker `init()` functions:**
- Location: Each `worker/*/` package
- Triggers: Go `init()` called when the package is imported (via `_ "github.com/cozy/cozy-stack/worker/..."` imports in `model/job/workers_list.go`)
- Responsibilities: Register `WorkerConfig` via `job.AddWorker(...)`

## Error Handling

**Strategy:** Errors bubble up through return values; HTTP errors are formatted as JSON:API at the edge.

**Patterns:**
- Model functions return `(result, error)` — callers check and propagate
- HTTP handlers return `error` to Echo; the global `errors.ErrorHandler` in `web/errors/errors.go` converts to JSON:API error objects
- CouchDB errors (`*couchdb.Error`) and `os.IsNotExist` / `os.IsExist` are specifically recognized and mapped to appropriate HTTP status codes
- Custom `*jsonapi.Error` type carries HTTP status, title, and detail fields
- Panic recovery middleware (`middlewares.RecoverWithConfig`) is installed in production; disabled in dev mode to expose stack traces

## Cross-Cutting Concerns

**Logging:** `pkg/logger` wraps logrus; used as `logger.WithDomain(domain).WithNamespace("ns").Infof(...)` throughout; namespaced log entries make filtering easy.

**Validation:** No single validation layer — input validation is done inline in handlers using echo binding, and in model functions before CouchDB writes.

**Authentication:** Two paths — (1) session cookie for browser-based access, resolved by `LoadSession` middleware; (2) Bearer token (OAuth JWT) for API clients, resolved by `AllowTypeAndID` / `AllowWholeType` in `web/middlewares/permissions.go`. Admin endpoints use HTTP Basic Auth against a secret file.

**Multi-tenancy:** Enforced via `prefixer.Prefixer` — every CouchDB operation takes a prefixer that scopes all queries to the correct instance namespace.

---

*Architecture analysis: 2026-04-04*
