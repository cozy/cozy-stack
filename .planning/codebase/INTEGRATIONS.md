# External Integrations

**Analysis Date:** 2026-04-04

## APIs & External Services

**Push Notifications:**
- Firebase Cloud Messaging (FCM) - Android push notifications
  - SDK/Client: `firebase.google.com/go/v4` (messaging package)
  - Auth: `notifications.fcm_credentials_file` config key → path to Google service account JSON
  - Implementation: `worker/push/push.go`
- Apple Push Notification Service (APNS/2) - iOS push notifications
  - SDK/Client: `github.com/sideshow/apns2`
  - Auth: P12 certificate (`notifications.ios_certificate_key_path`, `notifications.ios_certificate_password`) or token-based (`ios_key_id`, `ios_team_id`)
  - Implementation: `worker/push/push.go`
- Huawei Push Kit - Huawei Android device push notifications
  - Client: custom HTTP client in `model/notification/huawei/`
  - Auth: token endpoint configured via `notifications.contexts.<name>.huawei_get_token`
  - Implementation: `worker/push/push.go`

**AI / RAG:**
- RAG Server (Retrieval-Augmented Generation) - AI indexing and querying
  - Protocol: HTTP REST
  - Auth: API key via `rag.<context>.api_key` config key
  - URL: `rag.<context>.url` config key (default: `http://localhost:8000`)
  - Workers: `worker/rag/` (index and query workers)
  - Model: `model/rag/`

**Document Collaboration:**
- OnlyOffice Document Server - collaborative editing of office documents
  - Protocol: HTTP + webhook callbacks
  - Auth: shared inbox/outbox secrets (`office.<context>.onlyoffice_inbox_secret`, `office.<context>.onlyoffice_outbox_secret`)
  - URL: `office.<context>.onlyoffice_url` config key
  - Implementation: `model/office/`, `web/office/`

**App Registry:**
- Cozy App Registry (`https://apps-registry.cozycloud.cc/`) - app and konnector catalog
  - Protocol: HTTP REST
  - Auth: None (public API)
  - Configurable: `registries.default` in config (supports multiple registry URLs per context)
  - Implementation: `pkg/registry/`

**Banking Integration (Budget Insight / BI):**
- BI API - bank account aggregation webhooks (incoming events trigger konnectors)
  - Protocol: incoming HTTP webhooks
  - Auth: HMAC-based webhook signature verification
  - Implementation: `model/bi/webhook.go`, `model/bi/api.go`

**Move Wizard:**
- Cozy Move service (`https://move.cozycloud.cc/`) - instance relocation between hosters
  - Protocol: HTTP REST
  - Configurable: `move.url` config key
  - Implementation: `web/move/`, `model/move/`

**GeoIP Lookups:**
- MaxMind GeoLite2 database - IP-to-city geolocation for login history
  - Client: `github.com/oschwald/maxminddb-golang`
  - Auth: Local database file (path via `geodb` config key)
  - Implementation: `model/session/login_history.go`

**Password Breach Check:**
- HaveIBeenPwned API (`https://api.pwnedpasswords.com`) - leaked password checks
  - Protocol: HTTP (k-Anonymity model - only first 5 hash chars sent)
  - Referenced in CSP allowlist: `web/routing.go`

**Analytics (CSP-whitelisted):**
- Matomo Analytics (`https://matomo.cozycloud.cc`) - usage analytics for hosted apps
  - Referenced in `web/routing.go` CSP allowlist (script-src and img-src)

**Error Tracking (CSP-whitelisted):**
- Cozy Errors service (`https://errors.cozycloud.cc`) - frontend error reporting
  - Referenced in `web/routing.go` CSP allowlist (script-src)

**Maps (CSP-whitelisted):**
- OpenStreetMap tile servers (`https://*.tile.openstreetmap.org`, `https://*.tile.osm.org`) - map tiles for address display
  - Referenced in `web/routing.go` CSP img-src allowlist

**Flagship App Integrity:**
- Google Play Integrity API - Android app integrity attestation
  - Implementation: `model/oauth/android_play_integrity.go`
  - Config: `flagship.play_integrity_decryption_keys`, `flagship.play_integrity_verification_keys`
- Apple App Attest - iOS app integrity
  - Implementation: `model/oauth/ios.go`
  - Config: `flagship.apple_app_ids`

**Cloudery (Twake hosting platform):**
- Internal API for managed hosting operations
  - Protocol: HTTP REST with bearer token
  - Auth: `clouderies.<context>.api.token` config key
  - URL: `clouderies.<context>.api.url` config key
  - Implementation: `model/cloudery/`

**Common Settings Service:**
- Internal service for context-specific instance settings
  - Protocol: HTTP REST with bearer token
  - Auth: `common_settings.<context>.token` config key
  - URL: `common_settings.<context>.url` config key

**Konnectors (data connectors):**
- External processes executed via shell command
  - Runtime: Node.js (default: `scripts/konnector-node-run.sh`)
  - Alternatives: rkt container, nsjail sandbox
  - Config: `konnectors.cmd` config key

## Data Storage

**Databases:**
- CouchDB 3.2.3+ - Primary document database for all application data
  - Connection: `couchdb.url` config key (default: `http://localhost:5984/`)
  - Client: custom HTTP client in `pkg/couchdb/` (no third-party ORM; direct HTTP to CouchDB REST API)
  - TLS: configurable with `root_ca`, `client_cert`, `client_key`, `pinned_key`
  - Multi-cluster: `couchdb.clusters` config key (supports multiple CouchDB clusters with per-cluster instance creation routing)

**File Storage (VFS - Virtual File System):**
- Local filesystem - default for development
  - URL scheme: `file://localhost/path/to/storage`
  - Implementation: `model/vfs/vfsafero/` (using `spf13/afero`)
- OpenStack Swift - recommended for production
  - URL scheme: `swift://openstack/?UserName=...&Password=...&ProjectName=...`
  - Client: `github.com/ncw/swift/v2`
  - TLS: configurable with `fs.root_ca`, `fs.client_cert`, `fs.client_key`
  - Implementation: `model/vfs/vfsswift/`
- In-memory filesystem - testing only
  - URL scheme: `mem://`

**Caching:**
- Redis - distributed caching, session storage, distributed locks, rate limiting
  - Connection: `redis.addrs` config key (supports standalone, cluster, sentinel)
  - Auth: `redis.password` config key
  - Logical databases: jobs (0), cache (1), lock (2), sessions (3), downloads (4), konnectors (5), realtime (6), log (7), rate_limiting (8)
  - Client: `github.com/redis/go-redis/v9`
  - Optional: if not configured, in-process fallback implementations are used

## Authentication & Identity

**Auth Provider:**
- Custom (self-hosted)
  - Implementation: passphrase-based login with PBKDF2, stored in CouchDB
  - Sessions: stored in Redis (or in-memory if Redis not configured)
  - 2FA: TOTP (`github.com/pquerna/otp`), magic links, email OTP
  - OAuth2 Provider: built-in OAuth2 authorization server for third-party apps (`web/auth/`, `model/oauth/`)
  - Magic link authentication: `web/auth/magic_link.go`

**OIDC (OpenID Connect):**
- Generic OIDC provider support - federated identity for enterprise contexts
  - Implementation: `model/oidc/provider/`, `web/oidc/`
  - Config: `authentication.<context>` config key
- FranceConnect - French government identity provider (specific integration)
  - Implementation: `model/oidc/provider/provider.go` (`FranceConnectProvider` kind)
  - Config: separate FranceConnect context config

**Credential Vault:**
- NaCl asymmetric encryption for storing third-party account credentials (konnector tokens, etc.)
  - Keys: `vault.credentials_encryptor_key` / `vault.credentials_decryptor_key` config keys
  - Implementation: `pkg/keyring/`

**Bitwarden-compatible Password Manager:**
- Bitwarden-compatible REST API for password manager features
  - Implementation: `model/bitwarden/`, `web/bitwarden/`

## Monitoring & Observability

**Metrics:**
- Prometheus - metrics exposition endpoint
  - Client: `github.com/prometheus/client_golang`
  - Implementation: `pkg/metrics/`

**Logs:**
- Logrus structured logging (`github.com/sirupsen/logrus`), wrapped in `pkg/logger`
- Log level configurable: `log.level` (debug, info, warning, panic, fatal)
- Syslog output: `log.syslog: true` config key
- Redis log drain: log entries can be streamed to Redis (`redis.databases.log`)

**Process Inspection:**
- `github.com/google/gops` v0.3.29 - runtime diagnostics (goroutine dumps, etc.) accessible on admin port

**Antivirus:**
- ClamAV - file scanning on upload
  - Protocol: TCP socket (clamd daemon)
  - Config: `contexts.<name>.antivirus.address`, `contexts.<name>.antivirus.timeout`
  - Implementation: `pkg/clamav/`, `worker/antivirus/`

## CI/CD & Deployment

**Hosting:**
- Docker Hub (`cozycloud/cozy-stack`, `cozycloud/cozy-app-dev`) - container images
- GitHub Releases - binary releases (linux/amd64, linux/arm, linux/arm64, freebsd/amd64)
- APT repository (`ci.cozycloud.cc`) - Debian/Ubuntu packages published via Jenkins trigger

**CI Pipeline:**
- GitHub Actions (`.github/workflows/`)
  - `go-tests.yml` - unit + integration tests (matrix: Go 1.25.x & 1.26.x, CouchDB 3.2.3 & 3.3.3, Redis service container)
  - `go-lint.yml` - golangci-lint
  - `js-lint.yml` - ESLint for frontend scripts
  - `assets.yml` - asset build check
  - `cli.yml` - CLI docs generation check
  - `release.yml` - on semver tag: creates GitHub release, builds multi-arch binaries, Docker images, .deb packages
  - `system-tests.yml` - Ruby Minitest system tests (uses Testcontainers for NextCloud, etc.)
- Codecov (`.github/codecov.yml`) - test coverage reporting

## Environment Configuration

**Required env vars (via YAML template or shell env):**
- `COZY_COUCHDB_URL` - CouchDB URL with credentials (used in CI)
- `COZY_MAIL_USERNAME` / `COZY_MAIL_PASSWORD` - SMTP credentials
- `COZY_BETA_MAIL_USERNAME` / `COZY_BETA_MAIL_PASSWORD` - SMTP credentials for beta context
- `COZY_BETA_SMS_TOKEN` - SMS provider token for beta context
- `OS_USERNAME` / `OS_PASSWORD` / `OS_PROJECT_NAME` / `OS_USER_DOMAIN_NAME` - OpenStack Swift credentials

**Secrets location:**
- Config file at `/etc/cozy/cozy.yaml` (production) or local path (development)
- Admin passphrase hash in `cozy-admin-passphrase` file (same dir as config)
- Firebase credentials in a JSON file (path configured via `notifications.fcm_credentials_file`)
- iOS APNS certificate as P12 file (path configured via `notifications.ios_certificate_key_path`)
- Vault keypair files (paths configured via `vault.credentials_encryptor_key` / `vault.credentials_decryptor_key`)
- GitHub Actions secrets: `DOCKERHUB_USERNAME`, `DOCKERHUB_SECRET`, `JENKINS_AUTH`, `JENKINS_REPO_PUBLISH_JOB`, `JENKINS_REPO_PUBLISH_TOKEN`

## Webhooks & Callbacks

**Incoming:**
- `/webhooks/bi` - Budget Insight (BI) banking events (`model/bi/webhook.go`)
- `/webhooks/shared-notes` - OnlyOffice document save callbacks (`web/office/`)
- Cozy-to-Cozy sharing replication endpoints (`web/sharings/`)

**Outgoing:**
- Konnector execution results trigger job completion events via the realtime stack
- RabbitMQ publishes events (e.g., `password.changed`) to configured exchanges for downstream consumers
  - Config: `rabbitmq.exchanges` in YAML config
  - Implementation: `pkg/rabbitmq/publisher.go`

## WebDAV Integration (feat/webdav branch)

- WebDAV client library - connects to external WebDAV servers (e.g., Nextcloud)
  - Implementation: `pkg/webdav/webdav.go` (custom HTTP client, no third-party WebDAV library)
  - Operations: MKCOL, DELETE, GET, PUT, MOVE, COPY, PROPFIND
  - Used by: `model/nextcloud/` for Cozy-to-Nextcloud file sync
  - System tests: `tests/system/tests/webdav_nextcloud.rb` (runs Nextcloud in Docker via Testcontainers)

---

*Integration audit: 2026-04-04*
