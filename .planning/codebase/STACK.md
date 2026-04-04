# Technology Stack

**Analysis Date:** 2026-04-04

## Languages

**Primary:**
- Go 1.25+ - All server-side logic, API handlers, background workers, data models
- YAML - Configuration (`cozy.example.yaml`, GitHub Actions workflows)

**Secondary:**
- JavaScript (ESM) - Frontend browser scripts in `assets/scripts/` (login, OAuth, 2FA UI)
- Ruby - System integration tests in `tests/system/`
- CSS - Frontend styles in `assets/styles/`
- HTML/Go templates - Email templates in `assets/mails/`, web templates in `assets/templates/`

## Runtime

**Environment:**
- Go runtime 1.25.x – 1.26.x (matrix-tested in CI: min 1.25.x, max 1.26.x)
- Node.js - Dev tooling only (linting, formatting, asset optimization); not required at runtime

**Package Manager:**
- Go modules (`go.mod` / `go.sum`) - present and locked
- npm (`scripts/package.json`, `scripts/package-lock.json`) - dev tooling only

## Frameworks

**Core:**
- `github.com/labstack/echo/v4` v4.15.1 - HTTP server, routing, middleware for all web routes
- `github.com/spf13/cobra` v1.9.1 - CLI command framework (`cmd/`)
- `github.com/spf13/viper` v1.19.0 - Configuration file loading (YAML, env vars, flags)

**Testing:**
- `github.com/stretchr/testify` v1.11.1 - Unit test assertions throughout Go code
- `github.com/gavv/httpexpect/v2` v2.16.0 - HTTP-level integration test assertions
- `github.com/testcontainers/testcontainers-go` v0.40.0 - Spins up Docker containers (RabbitMQ, ClamAV) in tests
- `github.com/docker/docker` v28.5.2 - Docker client used by testcontainers
- Ruby Minitest + Testcontainers gem - System-level integration tests (`tests/system/`)

**Build/Dev:**
- GNU Make (`Makefile`) - Primary build orchestration
- `scripts/build.sh` - Cross-platform binary build script (linux/amd64, linux/arm, linux/arm64, freebsd/amd64)
- golangci-lint v2.11.4 - Go linting (`scripts/golangci-lint`)
- ESLint 9.10.0 - JavaScript linting (`scripts/eslint.config.js`)
- Prettier 3.3.3 - JavaScript/CSS formatting
- svgo 3.3.3 - SVG optimization for icon assets
- `statik` (internal: `pkg/statik`) - Embeds `assets/` directory into the Go binary at build time
- `scripts/build.sh assets` produces `web/statik/statik.go`

## Key Dependencies

**Critical:**
- `github.com/redis/go-redis/v9` v9.17.3 - Session storage, job queue, distributed locking, caching, rate limiting (9 logical databases)
- `github.com/cozy/prosemirror-go` v0.5.3 - ProseMirror document model for collaborative notes
- `github.com/golang-jwt/jwt/v5` v5.2.2 - JWT generation/validation for auth tokens
- `golang.org/x/oauth2` v0.35.0 - OAuth2 client flows for external account integrations
- `github.com/ncw/swift/v2` v2.0.3 - OpenStack Swift object storage client (alternative VFS backend)
- `github.com/spf13/afero` v1.11.0 - Filesystem abstraction layer used in VFS

**Infrastructure:**
- `github.com/sirupsen/logrus` v1.9.3 - Structured logging throughout (wrapped in `pkg/logger`)
- `github.com/robfig/cron/v3` v3.0.1 - Cron-style job scheduling for triggers
- `github.com/rabbitmq/amqp091-go` v1.9.0 - RabbitMQ AMQP client for event messaging (`pkg/rabbitmq`)
- `github.com/prometheus/client_golang` v1.18.0 - Prometheus metrics exposition (`pkg/metrics`)
- `github.com/pquerna/otp` v1.4.0 - TOTP generation/validation for 2FA
- `golang.org/x/crypto` v0.48.0 - Bcrypt, NaCl, and other cryptographic primitives
- `github.com/ugorji/go/codec` v1.2.12 - MessagePack/CBOR codec (used by Echo)
- `github.com/yuin/goldmark` v1.7.4 - Markdown rendering
- `github.com/hashicorp/golang-lru/v2` v2.0.7 - In-memory LRU caches
- `github.com/gorilla/websocket` v1.5.1 - WebSocket support for realtime push
- `github.com/oschwald/maxminddb-golang` v1.13.1 - MaxMind GeoIP database lookups (login history geolocation)
- `firebase.google.com/go/v4` v4.14.1 - Firebase Cloud Messaging (Android push notifications)
- `github.com/sideshow/apns2` v0.25.0 - Apple Push Notification Service (iOS push notifications)
- `github.com/h2non/filetype` v1.1.3 - MIME/file type detection by magic bytes
- `github.com/dhowden/tag` v0.0.0-20240413230847-dc579f508b6b - Audio file metadata extraction
- `github.com/cozy/goexif2` v1.3.1 - EXIF metadata extraction from images
- `github.com/andybalholm/brotli` v1.1.0 - Brotli compression for static assets
- `github.com/Masterminds/semver/v3` v3.2.1 - Semantic version parsing (app registry)
- `github.com/gofrs/uuid/v5` v5.3.0 - UUID generation
- `github.com/cozy/gomail` v0.0.0-20170313100128-1395d9a6a6c0 - SMTP email sending
- `github.com/cozy/httpcache` v0.0.0-20210224123405-3f334f841945 - HTTP response caching
- `github.com/ohler55/ojg` v1.20.3 - Fast JSON parsing

## Configuration

**Environment:**
- Primary config: YAML file searched in `.`, `.cozy/`, `$HOME/.cozy`, `$HOME/.config/cozy`, `/etc/cozy/` (filename: `cozy.yaml` or `cozy.yml`)
- Template syntax within YAML using Go `text/template` - env vars accessed via `{{ .Env.VAR_NAME }}`
- All config keys can also be set via CLI flags or environment variables (Viper)
- Reference: `cozy.example.yaml` at project root

**Key config required:**
- `couchdb.url` - CouchDB connection URL (default: `http://localhost:5984/`)
- `fs.url` - VFS storage backend URL (`file://...` or `swift://...`)
- `redis.addrs` - Redis cluster/sentinel addresses (optional but needed for multi-instance deployments)
- `mail.host` / `mail.port` / `mail.username` / `mail.password` - SMTP credentials
- `notifications.fcm_credentials_file` - Path to Firebase credentials JSON for push
- `notifications.ios_certificate_key_path` - P12 cert path for APNS push
- `vault.credentials_encryptor_key` / `vault.credentials_decryptor_key` - NaCl keypair for credential encryption
- `rabbitmq.url` - AMQP connection URL

**Build:**
- `go.mod` / `go.sum` - Go module dependencies
- `scripts/build.sh` - Build script; injects `Version`, `BuildTime`, `BuildMode` via `-ldflags`
- `pkg/config/build.go` - Declares `Version`, `BuildTime`, `BuildMode` variables populated at build time
- Asset embedding: `make assets` runs `statik` to produce `web/statik/statik.go` (committed)

## Platform Requirements

**Development:**
- Go 1.25+
- CouchDB 3.2.3+ (tested against 3.2.3 and 3.3.3)
- Redis (optional for local single-instance dev, required for multi-stack)
- ImageMagick (`convert` binary) - thumbnail generation worker
- Ghostscript (`gs` binary) - PDF thumbnail generation worker
- Node.js + npm - asset tooling only (`make assets`, `make lint`, `make pretty`)

**Production:**
- Linux (amd64, arm, arm64) or FreeBSD (amd64) - pre-built binaries published on each release
- CouchDB 3.2.3+ (external service)
- Redis (external service, required for HA/multi-instance)
- OpenStack Swift or local filesystem for VFS
- SMTP server for email delivery
- Optional: RabbitMQ, ClamAV daemon, OnlyOffice Document Server, MaxMind GeoIP database

**Deployment packaging:**
- Docker images (`cozycloud/cozy-stack`, `cozycloud/cozy-app-dev`) published to Docker Hub on release
- Debian/Ubuntu `.deb` packages for Debian 10/11/12, Ubuntu 22.04/24.04 - published to Cozy APT repo via Jenkins

---

*Stack analysis: 2026-04-04*
