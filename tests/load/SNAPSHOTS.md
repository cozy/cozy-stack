# Disposable Compose fixtures

Build data once with the API-only [seeder](SEEDING.md), capture a cold baseline,
then restore it into a new isolated deployment for each measured run.
These commands support this harness's five-service Compose topology only.
They are not production backup tools and do not change the portable seeder.

## Prerequisites

- Node 22+, `npm ci`, and a running local Docker engine.
- Docker Compose with `!override` support, used by `compose.fixture.yaml` to
  replace the ordinary Stack ports rather than exposing port 8080 too.
- The service images in `compose.yaml` and the `node:22-alpine` archive helper.
  `fixture-up` resolves all five service images locally before creating resources,
  starts their immutable image IDs, and never builds or pulls missing images.
  To prepare the images explicitly:

```sh
docker compose -f compose.fixture.yaml --profile cozy build cozy-stack
docker compose -f compose.fixture.yaml --profile cozy pull couchdb redis rabbitmq minio
docker pull node:22-alpine
```

Run the examples from `tests/load`. Keep snapshot directories outside the repo.
The ordinary `twake-drive-load` deployment is not a fixture target.

## Build and capture once

```sh
export FIXTURE_PROJECT=load-fixture-seed
export FIXTURE_SNAPSHOT="$HOME/.local/share/cozy-fixtures/baseline-1k"
mkdir -p -m 700 "$(dirname "$FIXTURE_SNAPSHOT")"
make fixture-up

export SEED_ADMIN_URL=http://127.0.0.1:16090
export SEED_STACK_URL=http://127.0.0.1:18090
export SEED_DOMAIN_SUFFIX=localhost:18090
read -rsp 'Disposable fixture admin passphrase: ' SEED_ADMIN_PASSWORD; printf '\n'
export SEED_ADMIN_PASSWORD
make -e seed-scale SEED_FILES=1000 SEED_DRIVES=10

# Verify the generated data before freezing it.
make fixture-snapshot
```

The default disposable Compose admin passphrase is `cozy`. Do not reuse real
production credentials. The builder prints its generated `seed-...` domain;
preserve it and the fixture's entity IDs for your workload configuration.

`FIXTURE_STACK_PORT` and `FIXTURE_ADMIN_PORT` default to loopback ports 18090
and 16090. Set them before `fixture-up` if occupied, and use matching seeder
origins/domain suffix. A snapshot retains its effective connection configuration.
Do not run two deployments on the same published ports simultaneously.

Snapshot requires a new directory and a marked `load-fixture-...` project
created with `fixture-up`. Re-snapshotting a restored deployment is not supported.
It stops the Stack publisher, checks that runtime work is idle, then stops
storage and captures these consistent volumes:

- CouchDB, including the global instance registry and per-instance databases.
- MinIO file data.
- Stack configuration, including admin/vault key material.
- Stack local data.

Redis and RabbitMQ state is deliberately not captured: restore initializes new
empty runtime volumes. Dirty queues or unexpected/shared mounts are refusals,
not a reason to silently drop data. The source remains stopped after capture.

The private bundle includes archive checksums, immutable image references and
the Stack bind configuration needed to restore without the original checkout.
**It contains secrets. Keep directories private, do not commit or share the
bundle, and keep the baseline unchanged.** The commands create private directory
and file permissions; they do not encrypt backups or upload them anywhere.

## Restore, run, discard

```sh
# Snapshot is complete; remove the disposable source if no longer needed.
make fixture-discard

# Target must not already own containers, volumes or networks.
FIXTURE_PROJECT=load-fixture-run1 make fixture-restore

# Mint fresh workload tokens, verify the original IDs/content/drive backlinks,
# configure index/cache warm-up, then run the measured workload.
# Tokens and workload .env updates are intentionally not part of restore.

FIXTURE_PROJECT=load-fixture-run1 make fixture-discard
FIXTURE_PROJECT=load-fixture-run2 make fixture-restore
# The same original baseline is used; changes from run1 are absent.
FIXTURE_PROJECT=load-fixture-run2 make fixture-discard
```

`restore` verifies bundle integrity before writing target data, uses only fresh
owned volumes and pinned service images, and waits for service health.
It does not regenerate instances: domains, UUIDs, file IDs and drive IDs remain
those in the baseline, inside independent storage. Keep the private bundle
available while restored deployments exist: Stack uses its bundled configuration
as a read-only bind mount.

`discard` removes only the named, marked disposable project and its exclusive
volumes/network, including the known anonymous MinIO certificate volume.
Named volumes and the network require their own fixture markers even if no
containers remain after an interrupted discard; standard Compose labels alone
do not establish ownership. Anonymous MinIO certificate storage is identified
separately by its exclusive container attachment, not a required fixture label.
It preserves the snapshot and never runs a global prune, deletes a shared
bucket, or acts on the ordinary load-test project.

The underlying Node entrypoint has the same interface:

```sh
node --import tsx src/scripts/compose-fixture.ts up load-fixture-seed
node --import tsx src/scripts/compose-fixture.ts snapshot load-fixture-seed /private/baseline
node --import tsx src/scripts/compose-fixture.ts restore load-fixture-run1 /private/baseline
node --import tsx src/scripts/compose-fixture.ts discard load-fixture-run1
```

## Failures and limits

Run operations serially. Do not change images/configuration, run external
writers, or start another operator's Compose commands against the source during
capture. A health check is not an application-data verification: archive
integrity/topology/readiness checks do not replace workload checks of counts,
file bytes/checksums and sharing relationships.

A failed capture or restore leaves its partial directory/deployment for
inspection; there is no hidden retry, automatic destructive rollback or resume
manager. Do not treat a client timeout as proof that a server write finished.
Use a new snapshot directory or target name after diagnosing the failure;
use `fixture-discard` only for a safely identifiable owned disposable project.

Keep compatible service images and configuration. This workflow does not promise
cross-version restores, coordination with external systems, recipient-side
sharing, cold OS caches or representative large-file performance. Verify those
separately when they become part of the measured scenario.
