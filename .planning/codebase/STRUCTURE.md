# Codebase Structure

**Analysis Date:** 2026-04-04

## Directory Layout

```
cozy-stack/
├── main.go                  # Binary entry point — calls cmd.RootCmd.Execute()
├── go.mod / go.sum          # Go module definition
├── cozy.example.yaml        # Reference configuration file
├── Makefile                 # Build, test, and tooling targets
│
├── cmd/                     # CLI commands (Cobra)
│   ├── root.go              # RootCmd, shared flags
│   ├── serve.go             # `cozy-stack serve` — starts stack + HTTP servers
│   ├── instances.go         # Instance management commands
│   ├── apps.go              # App installation commands
│   ├── jobs.go              # Job management commands
│   ├── files.go             # VFS commands
│   └── browser/             # Browser helper for dev
│
├── web/                     # HTTP layer — handlers, routing, middleware
│   ├── server.go            # ListenAndServe, Servers struct
│   ├── routing.go           # SetupRoutes, SetupAdminRoutes, SetupAppsHandler
│   ├── middlewares/         # Echo middlewares (auth, instance, CORS, CSP, etc.)
│   ├── auth/                # Login, OAuth, passphrase, 2FA handlers
│   ├── files/               # VFS HTTP frontend
│   ├── data/                # Generic CouchDB CRUD API
│   ├── apps/                # Webapp and konnector install/serve handlers
│   ├── accounts/            # OAuth account handlers
│   ├── ai/                  # AI/RAG handlers
│   ├── bitwarden/           # Bitwarden-compatible vault API
│   ├── contacts/            # Contacts handlers
│   ├── instances/           # Admin instance management API
│   ├── intents/             # Intent system handlers
│   ├── jobs/                # Job HTTP API
│   ├── move/                # Instance move/export handlers
│   ├── notes/               # Collaborative notes handlers
│   ├── notifications/       # Notification handlers
│   ├── oauth/               # Admin OAuth client management
│   ├── office/              # OnlyOffice integration handlers
│   ├── oidc/                # OpenID Connect handlers
│   ├── permissions/         # Permission management API
│   ├── public/              # Unauthenticated public routes
│   ├── realtime/            # WebSocket realtime events
│   ├── registry/            # App registry proxy
│   ├── remote/              # Remote doctype + Nextcloud/WebDAV proxy
│   ├── settings/            # User and instance settings API
│   ├── sharings/            # File/data sharing handlers
│   ├── shortcuts/           # Shortcut handlers
│   ├── status/              # Health check endpoint
│   ├── version/             # Version endpoint
│   ├── wellknown/           # .well-known endpoints
│   ├── errors/              # Global HTTP error handler
│   ├── statik/              # Embedded asset renderer
│   └── tools/               # Admin tooling endpoints
│
├── model/                   # Business logic and domain models
│   ├── account/             # OAuth connector accounts
│   ├── app/                 # Webapp and konnector install/management
│   ├── bi/                  # Budget Insight integration
│   ├── bitwarden/           # Bitwarden vault model
│   ├── cloudery/            # Cloudery (instance manager) integration
│   ├── contact/             # Contact model
│   ├── feature/             # Feature flags
│   ├── instance/            # Instance model + lifecycle (create/destroy/patch)
│   │   └── lifecycle/       # Instance CRUD operations
│   ├── intent/              # App intent model
│   ├── job/                 # Job broker, scheduler, triggers, worker registry
│   ├── move/                # Instance export/import/move
│   ├── nextcloud/           # Nextcloud WebDAV integration model
│   ├── note/                # Collaborative notes model (ProseMirror)
│   ├── notification/        # Push/email notifications
│   ├── oauth/               # OAuth client and token model
│   ├── office/              # OnlyOffice document model
│   ├── oidc/                # OIDC binding/provider model
│   ├── permission/          # Permission rules and sets
│   ├── rag/                 # RAG (AI retrieval) model
│   ├── remote/              # Remote doctype definitions
│   ├── session/             # User session model
│   ├── settings/            # Settings service (instance + user settings)
│   ├── sharing/             # Cozy-to-Cozy sharing model and replicator
│   ├── stack/               # Stack bootstrapper (Start function, Services struct)
│   ├── token/               # Token service
│   └── vfs/                 # Virtual filesystem abstraction
│       ├── vfs.go           # VFS interface and helpers
│       ├── file.go          # FileDoc model
│       ├── directory.go     # DirDoc model
│       ├── vfsafero/        # Local filesystem backend (uses afero)
│       └── vfsswift/        # OpenStack Swift backend
│
├── worker/                  # Asynchronous job workers
│   ├── antivirus/           # ClamAV virus scan worker
│   ├── archive/             # File archive/zip worker
│   ├── exec/                # Konnector and service execution workers
│   ├── log/                 # Log ingestion worker
│   ├── mails/               # Email sending worker
│   ├── migrations/          # Data migration worker
│   ├── moves/               # Instance move workers
│   ├── notes/               # Notes export worker
│   ├── oauth/               # OAuth client cleanup worker
│   ├── push/                # Mobile push notification worker
│   ├── rag/                 # RAG indexing worker
│   ├── share/               # Sharing replication worker
│   ├── sms/                 # SMS worker
│   ├── thumbnail/           # Image thumbnail generation worker
│   └── trash/               # Trash emptying worker
│
├── pkg/                     # Infrastructure packages (no business logic)
│   ├── appfs/               # Application filesystem abstraction
│   ├── assets/              # Static assets (dynamic + statik embedded)
│   ├── avatar/              # Avatar generation
│   ├── cache/               # Redis/in-memory cache client
│   ├── clamav/              # ClamAV client
│   ├── config/              # Build-mode flags (dev/prod)
│   │   └── config/          # Full config struct (viper-based)
│   ├── consts/              # App slugs, setting IDs, doctype names
│   ├── couchdb/             # CouchDB HTTP client + Doc interface
│   ├── emailer/             # Email service interface + implementation
│   ├── filetype/            # File MIME type detection
│   ├── i18n/                # Internationalization (gettext .po files)
│   ├── jsonapi/             # JSON:API response helpers and error types
│   ├── keyring/             # OS keyring integration
│   ├── limits/              # Rate limiting
│   ├── lock/                # Redis/in-memory distributed locks
│   ├── logger/              # Logrus-based structured logger
│   ├── mail/                # Mail composition helpers
│   ├── manager/             # Manager (hosted operator) API client
│   ├── metadata/            # Document metadata helpers
│   ├── metrics/             # Prometheus metrics
│   ├── pdf/                 # PDF generation helpers
│   ├── prefixer/            # Prefixer interface (instance identity for CouchDB)
│   ├── previewfs/           # File preview filesystem
│   ├── rabbitmq/            # RabbitMQ service interface
│   ├── realtime/            # Pub/sub realtime event hub
│   ├── registry/            # App registry client
│   ├── safehttp/            # HTTP client with safety restrictions
│   ├── shortcut/            # Shortcut file helpers
│   ├── statik/              # Embedded static asset server
│   ├── tlsclient/           # TLS client configuration
│   ├── utils/               # General utilities (shutdown, paths, etc.)
│   └── webdav/              # WebDAV HTTP client library
│
├── client/                  # Go client library for the cozy-stack API
│   ├── client.go            # HTTP client wrapper
│   ├── auth/                # OAuth auth helpers
│   ├── request/             # Low-level HTTP request builder
│   ├── apps.go              # Apps API client
│   ├── files.go             # Files API client
│   └── instances.go         # Instances admin API client
│
├── assets/                  # Static assets (templates, locales, images, etc.)
│   ├── locales/             # .po translation files
│   ├── templates/           # HTML templates (login, emails, etc.)
│   ├── styles/              # CSS
│   ├── scripts/             # JS
│   └── mails/               # Email HTML templates
│
├── tests/                   # Integration and system tests
│   ├── fixtures/            # Test fixture data
│   ├── testutils/           # Shared test helpers
│   └── system/              # System-level Ruby-based test suite
│
├── docs/                    # Documentation
│   ├── cli/                 # Generated CLI docs
│   └── archives/            # Archived design docs
│
└── scripts/                 # Packaging and Docker scripts
    ├── docker/
    └── packaging/
```

## Directory Purposes

**`cmd/`:**
- Purpose: Cobra CLI commands for operating the stack
- Contains: One file per command group; flags are bound to viper keys
- Key files: `cmd/serve.go` (main server entrypoint), `cmd/root.go` (RootCmd, config loading), `cmd/instances.go` (admin instance management)

**`web/`:**
- Purpose: HTTP request parsing and response serialization
- Contains: One sub-package per API domain; each exposes a `Routes(*echo.Group)` function
- Key files: `web/routing.go` (central route registration), `web/server.go` (server lifecycle), `web/middlewares/permissions.go` (auth enforcement)

**`model/`:**
- Purpose: All business logic — no HTTP concerns here
- Contains: Domain structs, service functions, CouchDB interactions, external service calls
- Key files: `model/instance/instance.go` (core Instance struct), `model/vfs/vfs.go` (VFS interface), `model/job/broker.go` (Broker interface), `model/stack/main.go` (startup)

**`worker/`:**
- Purpose: Background processing workers registered with the job system
- Contains: Each package has an `init()` that calls `job.AddWorker(&job.WorkerConfig{...})`
- Pattern: Workers are registered automatically when imported; imports are collected in `model/job/workers_list.go`

**`pkg/`:**
- Purpose: Infrastructure libraries with no domain business logic; safe to import from both model and web
- Contains: Database clients, utilities, protocol clients, cross-cutting concerns
- Key files: `pkg/couchdb/couchdb.go` (Doc interface, DB operations), `pkg/prefixer/prefixer.go` (Prefixer interface), `pkg/realtime/realtime.go` (Hub interface)

## Key File Locations

**Entry Points:**
- `main.go`: Binary entry — calls `cmd.RootCmd.Execute()`
- `cmd/serve.go`: `cozy-stack serve` command — starts everything
- `web/server.go`: `ListenAndServe` — creates Echo instances, wires routes
- `web/routing.go`: `SetupRoutes` — registers all API route groups; `SetupAdminRoutes` — admin routes

**Configuration:**
- `pkg/config/config/config.go`: Full config struct definition and viper loading
- `cozy.example.yaml`: Reference config with all available keys documented
- `cmd/serve.go`: CLI flags wired to viper keys via `viper.BindPFlag`

**Core Model:**
- `model/instance/instance.go`: `Instance` struct — central tenant entity
- `model/instance/lifecycle/`: CRUD operations for instances
- `model/stack/main.go`: `Start()` — initializes all services, returns `Services` struct
- `model/job/broker.go`: `Broker` and `Scheduler` interfaces
- `model/vfs/vfs.go`: `VFS` interface and filesystem constants

**CouchDB:**
- `pkg/couchdb/couchdb.go`: `Doc` interface, `JSONDoc`, `RTEvent`, CRUD helpers
- `pkg/prefixer/prefixer.go`: `Prefixer` interface

**Middleware:**
- `web/middlewares/permissions.go`: `AllowWholeType`, `AllowTypeAndID`, `GetPermission`
- `web/middlewares/instance.go`: `NeedInstance`, `GetInstance`, `CheckInstanceBlocked`
- `web/middlewares/session.go`: `LoadSession`

**WebDAV / Nextcloud (feat/webdav branch):**
- `pkg/webdav/webdav.go`: WebDAV HTTP client (Mkcol, Delete, Move, Put, Get, Propfind)
- `pkg/webdav/errors.go`: WebDAV client error types
- `model/nextcloud/nextcloud.go`: Nextcloud integration model using the WebDAV client
- `web/remote/nextcloud.go`: HTTP handlers for the Nextcloud proxy API

**Testing:**
- `tests/testutils/`: Shared Go test utilities
- `tests/system/`: Ruby-based system test suite

## Naming Conventions

**Files:**
- Snake case: `my_feature.go`, `my_feature_test.go`
- Tests co-located with source: `broker.go` + `broker_test.go` in same directory
- Mock files suffixed `_mock.go`: `broker_mock.go`, `service_mock.go`

**Directories:**
- Lowercase single-word or concatenated: `vfsafero`, `vfsswift`, `jsonapi`, `couchdb`
- Sub-packages mirror parent names where needed: `pkg/config/config/`

**Go packages:**
- Package name matches directory name
- Shared test helpers grouped under `testutils`

**Types:**
- Structs: `PascalCase` — `FileDoc`, `DirDoc`, `Instance`, `JobRequest`
- Interfaces: Descriptive nouns — `Broker`, `Scheduler`, `VFS`, `Hub`, `Prefixer`, `Doc`
- Errors: `Err` prefix — `ErrNotFound`, `ErrInvalidAuth`, `ErrForbidden`
- Constants for doctypes: `consts.Files`, `consts.Apps` (namespaced under `pkg/consts`)

## Where to Add New Code

**New API endpoint:**
- Create or extend a handler file in `web/<domain>/`
- Expose a `Routes(g *echo.Group)` function
- Register the group in `web/routing.go` `SetupRoutes` (or `SetupAdminRoutes` for admin)

**New business logic:**
- Create a new package under `model/<domain>/` or extend an existing one
- Use `pkg/couchdb` for persistence, `pkg/prefixer.Prefixer` as the first argument to scoped DB calls
- Never import `web/` from `model/`

**New background worker:**
- Create `worker/<name>/` package
- Register with `job.AddWorker(&job.WorkerConfig{WorkerType: "name", ...})` in an `init()` function
- Add a blank import `_ "github.com/cozy/cozy-stack/worker/<name>"` in `model/job/workers_list.go`

**New infrastructure package:**
- Create under `pkg/<name>/`
- Should have no imports from `model/` or `web/`

**New CLI command:**
- Add file to `cmd/` following the cobra command pattern
- Register with `RootCmd.AddCommand(...)` in the file's `init()`

**New doctype constant:**
- Add to `pkg/consts/doctype.go`

## Special Directories

**`assets/`:**
- Purpose: Static files embedded into the binary at build time via `statik`
- Generated: Partially (statik embeds them); source files are committed
- Committed: Yes

**`web/statik/`:**
- Purpose: Go package generated by `//go:generate statik -f -src=../assets` in `web/routing.go`
- Generated: Yes (from `assets/`)
- Committed: Only the generated file is committed; regenerate with `go generate ./web/`

**`.planning/`:**
- Purpose: GSD planning documents for this repository
- Generated: No
- Committed: Yes (planning artifacts)

**`tests/system/`:**
- Purpose: End-to-end system tests written in Ruby
- Committed: Yes

---

*Structure analysis: 2026-04-04*
