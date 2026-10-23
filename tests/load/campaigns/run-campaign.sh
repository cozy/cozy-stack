#!/usr/bin/env bash
# Runs the campaign defined in campaigns/<name>.sh and keeps everything about
# the run in data/results/campaigns/<run id>-<name>/: run.log, stats.log,
# environment.txt, steps.tsv, and summary.md.
#
# Usage: make campaign CAMPAIGN=quick
set -uo pipefail

HARNESS_DIR=$(cd "$(dirname "$0")/.." && pwd)
CAMPAIGN=${1:?usage: run-campaign.sh <campaign>}
DEFINITION="$HARNESS_DIR/campaigns/$CAMPAIGN.sh"

if [ ! -f "$DEFINITION" ] || [ "$CAMPAIGN" = run-campaign ]; then
  echo "Unknown campaign '$CAMPAIGN'. Available: $(cd "$HARNESS_DIR/campaigns" && ls *.sh | grep -v '^run-campaign' | sed 's/\.sh$//' | tr '\n' ' ')" >&2
  exit 2
fi

cd "$HARNESS_DIR"
RUN_ID=$(date -u +%Y%m%dT%H%M%SZ)
RUN_DIR="data/results/campaigns/$RUN_ID-$CAMPAIGN"
mkdir -p "$RUN_DIR"
: > "$RUN_DIR/steps.tsv"
exec > >(tee -a "$RUN_DIR/run.log") 2>&1

now() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

# CPU and memory limits as Docker applies them, after the stack is up.
effective_limits() {
  local service container values entries=()
  for service in cozy-stack couchdb; do
    container=$(docker compose --profile cozy ps -q "$service" 2>/dev/null)
    [ -n "$container" ] || continue
    values=$(docker inspect -f '{{.HostConfig.NanoCpus}} {{.HostConfig.Memory}}' "$container")
    entries+=("$(echo "$values" | awk -v s="$service" '{
      cpus = $1 == 0 ? "unlimited" : $1 / 1e9
      mem = $2 == 0 ? "unlimited" : sprintf("%.1fGiB", $2 / 1073741824)
      printf "%s cpus=%s memory=%s", s, cpus, mem
    }')")
  done
  local IFS=';'
  echo "${entries[*]:-stack not running}"
}

# Container CPU and memory every 10s: shows whether k6 or the stack
# saturated first on a single host.
record_resources() {
  while :; do
    echo "--- $(now) load $(cut -d' ' -f1-3 /proc/loadavg 2>/dev/null || uptime)"
    docker stats --no-stream --format '{{.Name}} cpu={{.CPUPerc}} mem={{.MemUsage}}'
    sleep 10
  done
}

record_resources > "$RUN_DIR/stats.log" 2>&1 &
STATS_PID=$!
trap 'kill "$STATS_PID" 2>/dev/null' EXIT

{
  echo "campaign=$CAMPAIGN"
  echo "run_id=$RUN_ID"
  echo "topology=${LOAD_TOPOLOGY:-single-host}"
  echo "commit=$(git rev-parse --short HEAD 2>/dev/null) $(git log -1 --format=%s 2>/dev/null)"
  echo "host=$(hostname) cpus=$(getconf _NPROCESSORS_ONLN 2>/dev/null)"
  echo "configured_limits=COZY_STACK_CPUS=${COZY_STACK_CPUS:-} COZY_STACK_MEMORY=${COZY_STACK_MEMORY:-} COUCHDB_CPUS=${COUCHDB_CPUS:-} COUCHDB_MEMORY=${COUCHDB_MEMORY:-}"
} > "$RUN_DIR/environment.txt"

# Steps keep going after a failure; the definition decides what depends on
# what. Each step is recorded in steps.tsv for the summary.
run_step() {
  local name=$1 kind=$2 id=$3 started status
  shift 3
  started=$(now)
  echo "=== $started START $name"
  make "$@"
  status=$?
  echo "=== $(now) END $name exit=$status"
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$name" "$kind" "$id" "$started" "$(now)" "$status" >> "$RUN_DIR/steps.tsv"
  return "$status"
}

upload_capacity() {
  local name=$1
  shift
  run_step "$name" upload-capacity "$RUN_ID-$name" cozy-capacity CAPACITY_RUN_ID="$RUN_ID-$name" "$@"
}

shared_drive_prepare() {
  run_step shared-drive-prepare prepare '' cozy-shared-drive-prepare "$@"
}

shared_drive_ramp() {
  local name=$1
  shift
  run_step "$name" shared-drive-ramp "$RUN_ID-$name" cozy-shared-drive-ramp EXECUTION_ID="$RUN_ID-$name" "$@"
}

org_drive_fanout() {
  local name=$1
  shift
  run_step "$name" org-drive-fanout "$RUN_ID-$name" org-drive-fanout EXECUTION_ID="$RUN_ID-$name" "$@"
}

echo "Campaign $CAMPAIGN, run $RUN_ID, directory $RUN_DIR"
# shellcheck source=/dev/null
source "$DEFINITION"
campaign

echo "limits=$(effective_limits)" >> "$RUN_DIR/environment.txt"
./node_modules/.bin/tsx src/scripts/campaign-summary.ts "$RUN_DIR"
echo "Campaign directory: $RUN_DIR"
