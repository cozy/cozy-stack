# Build a reusable dataset fixture

Generate an instance once through existing Stack APIs, then reuse a consistent
snapshot for isolated test runs. Generation is deliberately sequential: it is
fixture preparation, not an upload-throughput benchmark.

The Node builder uses the existing instance admin API, dedicated file-upload API
and the harness's `createSharedDrive()`. It does not write CouchDB documents or S3
objects directly. No new Go package, admin endpoint or Stack configuration flag is
needed. Generic data APIs can be added for suitable document types later; files
and sharings must keep their dedicated creation APIs.

## Organization fixtures

For configurable user counts with optional per-user files and shared drives, see
[ORGANIZATIONS.md](ORGANIZATIONS.md). Organization mode defaults to empty users;
the single-instance defaults below are unchanged.

## Build once

Use Node 22 or newer and run `npm ci` in `tests/load`. The target must be a
**disposable test deployment**, with nested subdomains and private admin access.
Do not generate fixtures on production or reuse the normal load-test instance.

```sh
export SEED_ADMIN_URL=https://admin.example.test:6443
export SEED_STACK_URL=https://stack.load.example.test:8443
export SEED_DOMAIN_SUFFIX=load.example.test:8443
read -rsp 'Admin passphrase: ' SEED_ADMIN_PASSWORD; echo
export SEED_ADMIN_PASSWORD

make seed-scale SEED_FILES=1000 SEED_DRIVES=10
# Seed instance: seed-<32-hex-id>.load.example.test:8443
```

`SEED_ADMIN_URL` and `SEED_STACK_URL` are explicit HTTP(S) connection origins,
without credentials, paths, queries or fragments. Public requests connect to
`SEED_STACK_URL` while sending the generated instance domain as the exact `Host`
header, including its public port. This also supports loopback/tunnel endpoints
without adding every generated instance to DNS. HTTPS certificates must validate
for the connection hostname; TLS verification stays enabled. Private CAs can use
`NODE_EXTRA_CA_CERTS`.

The CLI prints its disposable domain before the first API request. It refuses an
existing instance. Optional `SEED_ADMIN_USERNAME` defaults to `admin`. An explicit
`SEED_DOMAIN` can select a fresh `seed-<32 lowercase hex>.<DNS suffix>[:port]` domain
for Make; otherwise `SEED_DOMAIN_SUFFIX` is required. The hyphenated label requires
`subdomains: nested`, not flat mode. Existing admin access rules apply; keep
credentials in environment variables, never in URLs or command-line arguments.

Equivalent direct commands:

```sh
node --import tsx src/scripts/seed-scale-data.ts seed
node --import tsx src/scripts/seed-scale-data.ts seed "$SEED_DOMAIN"
node --import tsx src/scripts/seed-scale-data.ts cleanup "$SEED_DOMAIN"
```

Files contain 1 KiB of deterministic data. Shared drives have distinct empty
roots, created as owner drives without invited recipients. Defaults are 100 files
and zero drives. Bounds are 0..1,000,000 files and 0..100,000 drives, with at least
one requested object. Those bounds are not a performance guarantee. The builder
never changes the harness's normal `.env` or prints access tokens.

## Save and restore separately

Snapshot management depends on the deployment and is **not implemented by this
builder**. Keep it in separate infrastructure scripts rather than adding another
Stack seeding API or a generic backup framework here.

The first workflow should use an isolated cold baseline:

1. Finish generation, verify the dataset and wait for background work to settle.
2. Stop that disposable Stack and its storage services cleanly. Capture consistent
   CouchDB state, including global instance metadata, actual file storage, and
   required configuration/key material. Keep credentials and archives private.
3. Pin compatible Stack/storage versions. Define which Redis/RabbitMQ state must
   be retained, or prove queues are idle and start those runtime services empty.
4. For each test run, restore the same immutable baseline into **new isolated
   storage**. Preserve its instance domains/IDs inside that isolated environment;
   do not restore over an active or shared deployment.
5. Read back fixture counts, file contents/checksums and shared-drive relationships.
   Mint fresh API tokens, define index/cache warm-up, then run the measured tests.
6. Discard only that restored environment and its owned storage. Keep the baseline
   unchanged for the next run.

A local prototype verified this flow with CouchDB, MinIO and fresh Redis/RabbitMQ
services, including two independent restores and removal of a test-added file on
reset. That proves the small-fixture mechanism, not large-dataset performance or
support for every deployment. Measure capture/restore/start-up cost on the target
fixture. Scenarios involving external systems may require a wider snapshot scope.

## Failures and explicit cleanup

There are no automatic retries, resume, cleanup or recovery jobs. A failure can
leave a partial instance. `SEED_REQUEST_TIMEOUT_MS` is a complete-response deadline
for **each request**, defaulting to 1,800,000 milliseconds (30 minutes); it is not a
deadline for the whole build. A timeout, Ctrl-C or disconnect does not prove an
accepted server operation has finished. Establish that writes stopped before
cleanup, especially after a lost instance-create/upload response.

```sh
# Replace the placeholder with the printed domain; run only after writes stopped.
export SEED_DOMAIN='seed-<32-hex-id>.load.example.test:8443'
make seed-scale-cleanup
```

Cleanup uses the existing `DELETE /instances/:domain`, accepts an already-absent
instance, and confirms absence through the existing admin API. It does not need
`SEED_STACK_URL` or valid generation counts. Only explicit disposable `seed-...`
domains are accepted. For a restored test run, discard the entire owned environment
instead; never delete a shared bucket or someone else's volumes.
