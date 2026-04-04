# Codebase Concerns

**Analysis Date:** 2026-04-04

## Tech Debt

**Widespread Use of Deprecated Package-Level Functions:**
- Issue: Three packages expose `Deprecated` shim functions at the package level (as global wrappers over a service singleton). These are still called in ~85 places across the codebase.
- Files:
  - `model/instance/init.go` — `Get()`, `Update()`, `Delete()`, `ListByOrgDomain()`, `CheckPassphrase()` all deprecated
  - `model/settings/init.go` — `PublicName()`, `SettingsDocument()` deprecated
  - `model/emailer/init.go` and `model/cloudery/init.go` — deprecated wrappers
- Impact: Coupling all callers to a global singleton rather than an injected service; prevents testability, obscures dependencies.
- Fix approach: Migrate callers to use the injected `Service` interface; requires passing the service to each caller.

**Magic Link Store Not Migrated to TokenService:**
- Issue: `model/instance/lifecycle/store.go` implements a standalone `Store` for magic link codes with its own memory/Redis implementations, duplicating what `model/token/service.go` already provides. A `TODO` notes this explicitly.
- Files: `model/instance/lifecycle/store.go`, `model/token/service.go`
- Impact: Duplicated TTL management code; inconsistent behaviour between self-hosted (in-memory, loses on reboot) and clustered (Redis) deployments.
- Fix approach: Consolidate into `token.Service` using the `token.MagicLink` namespace.

**Bitwarden Settings Not Covered by SettingsService:**
- Issue: `SettingsService` handles two structured settings documents but the Bitwarden settings (`consts.BitwardenSettingsID`) are not yet managed by it, noted with a `#TODO`.
- Files: `model/settings/service.go`
- Impact: Bitwarden settings remain outside the domain service boundary.
- Fix approach: Add Bitwarden settings lifecycle methods to `SettingsService`.

**Account Routes via Generic Data API (Retro-compat Hack):**
- Issue: `io.cozy.accounts` CRUD routes are overridden inside the generic `/data/*` handler instead of having dedicated routes, deliberately left for retro-compatibility.
- Files: `web/data/accounts.go`
- Impact: Route intent is obfuscated; harder to audit account-specific access control.
- Fix approach: Add dedicated `/accounts/*` routes and deprecate the override.

**Settings Init Refactoring Pending:**
- Issue: A `TODO` in `web/routing.go:247` states "An init refacto will soon be required". The `settings.NewHTTPHandler` is the only settings handler that is directly instantiated in the router with a service argument, inconsistent with other routes.
- Files: `web/routing.go`
- Impact: Architectural inconsistency; likely to cause confusion when adding new settings features.

**VFS MkdirAll Has No Lock:**
- Issue: `vfs.MkdirAll` has no distributed lock, and uses `os.IsExist` as a fallback for race conditions.
- Files: `model/vfs/vfs.go:545`
- Impact: Potential for duplicate directory creation under concurrent requests.
- Fix approach: Wrap `MkdirAll` in the VFS-level lock or document the known race more prominently.

**CouchDB Path-Based String Comparison Bug Risk in VFS Move:**
- Issue: When moving directories, the indexer filters children using string prefix matching on CouchDB `Fullpath`. CouchDB collation can return false positives (e.g. `/Photos/bbb` alongside `/PHOTOS/AAA`), requiring extra filtering.
- Files: `model/vfs/couchdb_indexer.go:391-402`
- Impact: Potential for stale path entries after directory moves if the filter is bypassed.

**Sharing Worker Non-Idempotency:**
- Issue: `share-replicate` and `share-upload` workers are explicitly non-idempotent (`MaxExecCount: 1`). A failure does not retry via the normal worker mechanism but instead appends a new job, which can accumulate retries.
- Files: `worker/share/share.go:43-61`
- Impact: Under fault conditions, sharing queues can grow large; duplicate processing is possible.

**InsertChain Does Not Enforce MaxDepth:**
- Issue: `RevsTree.InsertChain` has a `TODO` stating "ensure the MaxDepth limit is respected" — the method does call `ensureMaxDepth` but only on the first node found in the tree, not the final inserted node.
- Files: `model/sharing/revisions.go:171`
- Impact: Revision trees may exceed the 100-node depth limit over many sync cycles, inflating storage.

**Simulated Instance in revokeTrashed:**
- Issue: `revokeTrashed` in `model/sharing/revoke_trashed.go:28` constructs a synthetic `instance.Instance` with only `Prefix` and `Domain` set. This is described as "a hack".
- Files: `model/sharing/revoke_trashed.go`
- Impact: Fragile; if `Revoke` or `RevokeRecipientBySelf` ever read additional instance fields, this will silently use zero values.
- Fix approach: Accept a proper `prefixer.Prefixer` + load the instance from the store, or pass a full instance through the call stack.

## Known Bugs

**Non-Atomic Instance Reset / CouchDB Eventual Consistency:**
- Symptoms: After `Reset()`, databases are deleted and immediately recreated. There is a small window where a recreated DB can silently fail.
- Files: `model/instance/lifecycle/reset.go:36-39`
- Trigger: High-load multi-node CouchDB clusters.
- Workaround: A `time.Sleep(2 * time.Second)` is inserted, but this is not guaranteed to be sufficient.

**CouchDB Index Race on App Creation:**
- Symptoms: Querying a permission just after app creation can return empty results in a CouchDB cluster because the index has not yet propagated.
- Files: `model/permission/permissions.go:262-268`
- Trigger: Fast app install + immediate permission lookup on a CouchDB cluster.
- Workaround: A `time.Sleep(1 * time.Second)` + one retry is in place. Not fully reliable.

**Sharing Upload: Fake Revision Injection:**
- Symptoms: In some conflict resolution paths a fake revision must be injected into a newly created file's revision tree.
- Files: `model/sharing/upload.go:689`
- Trigger: Conflicting concurrent uploads across Cozy instances.

**Notes getListFromCache Uses Redis KEYS Command:**
- Symptoms: Calling `getListFromCache` in `model/note/note.go` executes a Redis `KEYS` scan over `"note:<domain>:*"`, which blocks Redis during the scan.
- Files: `model/note/note.go:669`, `pkg/cache/impl_redis.go:63`
- Trigger: Large number of cached notes on a busy instance.
- Workaround: None currently.

## Security Considerations

**http.DefaultClient Used for Internal RAG API Calls:**
- Risk: `model/rag/index.go` and `model/rag/chat.go` use `http.DefaultClient` (no timeout, no SSRF protection) to communicate with the RAG server.
- Files: `model/rag/index.go:122,138,277,371`, `model/rag/chat.go:463`
- Current mitigation: RAG server URL is admin-configured; not user-controlled.
- Recommendations: Use a dedicated HTTP client with an explicit timeout; document that the RAG server address must not be exposed to user input.

**http.DefaultClient Used for Huawei Push and OAuth Token Endpoints:**
- Risk: `model/notification/huawei/client.go:149` uses `http.Get` (no timeout). `model/account/type.go:402` uses `http.PostForm` (no timeout) for OAuth token exchanges.
- Files: `model/notification/huawei/client.go`, `model/account/type.go`
- Current mitigation: URLs are sourced from account configuration, not direct user input.
- Recommendations: Replace with a timeout-scoped HTTP client.

**Share Code CSRF Reliance on Secrecy:**
- Risk: A comment in `web/sharings/sharings.go:844` explicitly notes "we don't have an anti-CSRF system, we rely on shareCode being secret."
- Files: `web/sharings/sharings.go`
- Current mitigation: Share codes are long random tokens.
- Recommendations: Document this design decision formally; consider adding CSRF tokens to sensitive share-code-authenticated forms.

**Realtime: Unauthenticated Subscriptions for Synthetic Doctypes:**
- Risk: `io.cozy.sharings.initial_sync` and `io.cozy.auth.confirmations` realtime events are explicitly exempted from permission checks.
- Files: `web/realtime/realtime.go:212-216`
- Current mitigation: These are low-sensitivity events; actual data is not included.
- Recommendations: Audit periodically as new synthetic doctypes are added.

**Token Validation Does Not Delete Used Tokens:**
- Risk: `model/token/service.go:78` notes with a TODO that validated tokens are not consumed; they remain valid until TTL expiry.
- Files: `model/token/service.go`
- Impact: Replay attacks are possible within the token's lifetime.
- Recommendations: Delete the token on first successful validation.

## Performance Bottlenecks

**Redis KEYS Scan in Note Cache:**
- Problem: `getListFromCache` in `model/note/note.go` calls `cache.Keys(prefix)` which maps to a blocking `KEYS` command in `pkg/cache/impl_redis.go:63`.
- Files: `pkg/cache/impl_redis.go:63`, `model/note/note.go:669`
- Cause: The cache interface exposes a `Keys(prefix)` method backed by Redis `KEYS`, which is O(N) over all keys and blocks other Redis operations.
- Improvement path: Replace with Redis `SCAN` for cursor-based iteration, or maintain a separate Redis Set per instance to track note cache keys.

**Import BulkUpdate Sleeps 5 Minutes Per Retry:**
- Problem: When CouchDB is overloaded during import, the retry loop sleeps 5 minutes and retries up to 12 times (max 60 minutes total stall per batch).
- Files: `model/move/importer.go:236`
- Cause: Empirical back-off to avoid CouchDB overload; no backpressure mechanism.
- Improvement path: Use exponential back-off with jitter; surface overload errors to the user.

**Hard-Coded AllDocs Limit of 1000 for findAccountWithSameConnectionID:**
- Problem: `web/accounts/oauth.go:193` fetches up to 1000 accounts to scan in memory for a `connection_id` match.
- Files: `web/accounts/oauth.go`
- Cause: Accounts doctype has no CouchDB index on `connection_id`.
- Improvement path: Add a Mango index on `oauth.query.connection_id` and use a filtered query.

**Sharing Reupload Hard Limit of 100:**
- Problem: `model/sharing/reupload.go:23` fetches at most 100 sharings. Instances with more than 100 active sharings silently fall back to the existing retry mechanism, losing the quota-increase notification.
- Files: `model/sharing/reupload.go`
- Cause: Intentional to avoid overloading the instance, but the hard limit has no pagination fallback.
- Improvement path: Implement cursor-based pagination or emit a job per sharing.

**context.TODO() Throughout Redis Cache Layer:**
- Problem: All Redis operations in `pkg/cache/impl_redis.go` use `context.TODO()`, bypassing request-scoped deadlines and cancellations.
- Files: `pkg/cache/impl_redis.go:35,48,63,70,75,80,115`
- Cause: Cache interface does not propagate context.
- Improvement path: Thread `context.Context` through the cache interface methods.

## Fragile Areas

**Sharing Subsystem:**
- Files: `model/sharing/` (12+ files, most files 500–1500 lines)
- Why fragile: The sharing replication logic has multiple `XXX` guards acknowledging impossible-but-handled states, fake revision injection, simulated instances, non-idempotent workers, and a TODO on revision depth enforcement. Race condition handling is scattered across `upload.go`, `indexer.go`, and `revisions.go`.
- Safe modification: Always run the full sharings test suite (`web/sharings/sharings_test.go`, `web/sharings/drives_test.go`, `web/sharings/move_test.go`) after any change.
- Test coverage: Good integration coverage via test files but the race condition paths are inherently hard to reproduce.

**VFS CouchDB Indexer Directory Move:**
- Files: `model/vfs/couchdb_indexer.go`
- Why fragile: Path-based child detection uses string prefix matching that must compensate for CouchDB's case-sensitive collation ordering. Multiple `XXX` comments acknowledge this is not correct in theory but handled defensively.
- Safe modification: Always test with directory names that differ only in case (`Photos` vs `PHOTOS`).

**aferoVFS Lock Ordering in CopyFile:**
- Files: `model/vfs/vfsafero/impl.go:253-291`
- Why fragile: The `defer` order to release the VFS lock before calling `newfile.Close()` (which re-acquires the lock) is explicitly documented and must be preserved. Any refactoring of the defer stack could introduce a deadlock.

**Job Broker During Import:**
- Files: `model/job/broker.go:249-256`
- Why fragile: During an import the `io.cozy.jobs` database is deleted and recreated. The broker handles this by re-creating the job when `IsNotFoundError` is returned from `Update()`. Adding more error handling around `Update()` could silently swallow real errors.

## Scaling Limits

**Sharing Count Scalability:**
- Current capacity: `AskReupload` processes at most 100 sharings per quota-increase event.
- Limit: Instances with >100 sharings do not receive reupload notifications.
- Scaling path: Paginate the sharings query and emit individual jobs.

**CouchDB Index Creation Concurrency:**
- Current capacity: Concurrent index creation requests use a 100ms sleep + retry loop (`pkg/couchdb/views.go:283-286`).
- Limit: Race conditions on CouchDB cluster node index propagation are not fully eliminated.
- Scaling path: Use CouchDB's `/_index` endpoint's built-in conflict handling and rely on application-level idempotent retries.

**Realtime Hub (In-Memory Mode):**
- Current capacity: The in-memory realtime hub (`pkg/realtime/mem_realtime.go`) is not shared across multiple stack processes.
- Limit: Multi-process deployments without Redis lose cross-process realtime events.
- Scaling path: Always configure Redis for production deployments (already the documented path).

## Dependencies at Risk

**`github.com/mssola/user_agent` v0.6.0:**
- Risk: This library has very low maintenance activity and its last release is old. It is used for user-agent parsing in session tracking and middleware.
- Files: `model/session/login_history.go`, `model/instance/auth.go`, `web/settings/context.go`, `web/settings/settings.go`, `web/middlewares/user_agent.go`, `web/auth/oauth.go`, `web/auth/deprecated_app_list.go`
- Impact: No security fixes if new UA string formats introduce vulnerabilities or panics.
- Migration plan: Consider replacing with `github.com/ua-parser/uap-go` or inlining a minimal parser.

**`github.com/sirupsen/logrus` v1.9.3:**
- Risk: Logrus is in maintenance mode; the Go ecosystem has largely moved to `slog` (stdlib since Go 1.21) or `zerolog`/`zap`. The project already uses Go 1.25 (`go.mod:3`).
- Files: `pkg/logger/logger.go`, `pkg/logger/syslog.go`, `web/server.go`, `pkg/realtime/log_hook.go`
- Impact: Low immediate risk; technical debt as the ecosystem diverges.
- Migration plan: Migrate logger package to `slog` and replace logrus.Entry references.

**`github.com/prometheus/client_golang` v1.18.0:**
- Risk: v1.18.0 is behind the current stable (v1.20+). Minor API changes in newer versions may affect metric registration patterns.
- Impact: Low; no known security issues.
- Migration plan: Routine upgrade.

## Missing Critical Features

**AI Streaming Response Not Implemented:**
- Problem: `web/ai/ai.go:47` has a `TODO: handle streaming response` — the current `/v1/chat/completions` endpoint buffers the full AI response before returning it.
- Blocks: Real-time streaming of AI completions to the client, which is a standard UX expectation for chat interfaces.

**RAG Metadata Not Updated on File Move/Rename:**
- Problem: `model/rag/index.go:171` notes a TODO: when a file is moved or renamed, the metadata in the vector database is not patched — only re-indexation on content change is handled.
- Files: `model/rag/index.go`
- Impact: Vector DB metadata (`name`, `path`) becomes stale after file moves, potentially degrading RAG retrieval quality.

## Test Coverage Gaps

**Sharing Non-Idempotent Workers:**
- What's not tested: The retry-via-new-job path for `share-replicate` and `share-upload` under failure.
- Files: `worker/share/share.go`
- Risk: Silent accumulation of duplicate retry jobs under fault conditions could go undetected.
- Priority: Medium

**Token Replay After Validation:**
- What's not tested: Behaviour when the same magic link or email-update token is used twice within its TTL window.
- Files: `model/token/service.go`
- Risk: Token replay is possible and not caught by tests.
- Priority: High

**MkdirAll Race Condition:**
- What's not tested: Concurrent `MkdirAll` calls creating the same path.
- Files: `model/vfs/vfs.go:545`
- Risk: Duplicate directory creation under concurrent load is handled defensively but not validated.
- Priority: Medium

**findAccountWithSameConnectionID With >1000 Accounts:**
- What's not tested: Behaviour when an instance has more than 1000 accounts.
- Files: `web/accounts/oauth.go:193`
- Risk: The function silently returns "not found" for the correct account if it sits beyond the 1000-record page boundary.
- Priority: Low

---

*Concerns audit: 2026-04-04*
