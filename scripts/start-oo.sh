#!/bin/bash
#
# Start OnlyOffice Document Server for local development
#
# This script:
# 1. Stops and removes any existing container named OO_CONTAINER
# 2. Collects all instance hostnames from cozy-stack
# 3. Starts OnlyOffice with proper host mappings so it can reach cozy-stack
# 4. Waits for the first-init healthcheck to complete
# 5. Deterministically rewrites local.json (on the HOST) to establish the
#    cozy-stack <-> OnlyOffice JWT contract, then restarts the DS supervisord
#    processes and waits for the second healthcheck
#
# Environment variables (all optional):
#   OO_CONTAINER  Container name           (default: onlyoffice-ds)
#   OO_PORT       Published host port      (default: 8000)
#   OO_SECRET     JWT secret               (default: empty — JWT outbox disabled)
#                 Must equal cozy-stack's onlyoffice_outbox_secret /
#                 onlyoffice_inbox_secret in your office config block.
#
# Usage examples:
#
#   # Single context server (JWT OFF, cozy-stack secrets also empty):
#   ./scripts/start-oo.sh
#
#   # Context server with JWT secret:
#   OO_SECRET=changeme-shared-secret ./scripts/start-oo.sh
#
#   # Override server on a second port/container (same JWT secret):
#   OO_CONTAINER=oo-override OO_PORT=8001 OO_SECRET=changeme-shared-secret \
#     ./scripts/start-oo.sh
#
# For the full JWT contract and the env-var / SSRF traps, see:
#   docs/onlyoffice-dev-server-jwt-setup.md

set -e

OO_CONTAINER="${OO_CONTAINER:-onlyoffice-ds}"
OO_PORT="${OO_PORT:-8000}"
OO_SECRET="${OO_SECRET:-}"

# ---------------------------------------------------------------------------
# wait_healthy PORT
#   Polls the /healthcheck endpoint until it returns "true" or times out.
#   On timeout it prints a warning with a docker-logs hint (does NOT exit 0).
# ---------------------------------------------------------------------------
wait_healthy() {
  local port="$1"
  echo "Waiting for OnlyOffice to be ready on port $port..."
  for i in $(seq 1 60); do
    if curl -s "http://localhost:$port/healthcheck" 2>/dev/null | grep -q "true"; then
      echo "OnlyOffice is ready at http://localhost:$port"
      return 0
    fi
    sleep 2
    printf "."
  done
  echo ""
  echo "Warning: OnlyOffice did not become ready in time. Check logs with:"
  echo "  docker logs $OO_CONTAINER"
  return 1
}

# ---------------------------------------------------------------------------
# Stop / remove any existing container with this name
# ---------------------------------------------------------------------------
echo "Stopping existing OnlyOffice container '$OO_CONTAINER'..."
docker stop "$OO_CONTAINER" 2>/dev/null || true
docker rm "$OO_CONTAINER" 2>/dev/null || true

# ---------------------------------------------------------------------------
# Collect instance hostnames
# ---------------------------------------------------------------------------
echo "Collecting instance hostnames..."
HOSTS=""
for domain in $(go run . instances ls 2>/dev/null | awk '{print $1}' | sed 's/:[0-9]*$//'); do
  HOSTS="$HOSTS --add-host=$domain:host-gateway"
done

if [ -z "$HOSTS" ]; then
  echo "Warning: No instances found. Adding default cozy.localhost"
  HOSTS="--add-host=cozy.localhost:host-gateway"
fi

# ---------------------------------------------------------------------------
# Start the container
# The -e JWT_ENABLED / ALLOW_* vars are a best-effort hint for first-init
# only; they are silently ignored if local.json already exists.  The
# authoritative configuration is written below via local.json rewrite.
# ---------------------------------------------------------------------------
echo "Starting OnlyOffice Document Server '$OO_CONTAINER' on port $OO_PORT..."
# shellcheck disable=SC2086
docker run -d --name "$OO_CONTAINER" \
  -p "$OO_PORT:80" \
  $HOSTS \
  onlyoffice/documentserver:latest

# ---------------------------------------------------------------------------
# First healthcheck — DS must complete first-init before local.json exists
# ---------------------------------------------------------------------------
wait_healthy "$OO_PORT"

# ---------------------------------------------------------------------------
# JWT / SSRF config — deterministic local.json rewrite ON THE HOST
#
# The onlyoffice/documentserver:latest (9.3.x) image ships with NO python3.
# We read the file out of the container, transform it on the host with
# python3, then write it back.  OO_SECRET is passed via the environment
# (not interpolated into the Python source) to avoid quoting/injection bugs.
# ---------------------------------------------------------------------------
if [ -z "$OO_SECRET" ]; then
  echo ""
  echo "WARNING: OO_SECRET empty — JWT outbox disabled."
  echo "  Saves only work if cozy-stack also has an empty onlyoffice_outbox_secret."
  echo "  To enable JWT, re-run with: OO_SECRET=<your-secret> ./scripts/start-oo.sh"
  echo ""
fi

echo "Reading local.json from container '$OO_CONTAINER'..."
CURRENT_JSON=$(docker exec "$OO_CONTAINER" cat /etc/onlyoffice/documentserver/local.json)

echo "Applying JWT/SSRF contract settings on host..."
# OO_SECRET is passed via the environment (not interpolated into Python source)
# to avoid quoting and injection issues.
# OO_LOCAL_JSON carries the file contents; both are exported to the subprocess.
PATCHED_JSON=$(OO_SECRET="$OO_SECRET" OO_LOCAL_JSON="$CURRENT_JSON" python3 <<'PY'
import sys, json, os

secret = os.environ.get("OO_SECRET", "")
outbox_enabled = bool(secret)

raw = os.environ.get("OO_LOCAL_JSON", "")
cfg = json.loads(raw)

# Ensure path services.CoAuthoring.token exists
svc = cfg.setdefault("services", {})
ca = svc.setdefault("CoAuthoring", {})
token = ca.setdefault("token", {})

# token.enable sub-object
enable = token.setdefault("enable", {})
req = enable.setdefault("request", {})
req["inbox"] = False      # cozy-stack uses its own JSON shape; DS inbox validation rejects it
req["outbox"] = outbox_enabled
enable["browser"] = False  # same reason as inbox

# secret sub-object (always write, even when empty string)
sec = ca.setdefault("secret", {})
for bucket in ("inbox", "outbox", "session"):
    sec.setdefault(bucket, {})["string"] = secret

# request-filtering-agent: allow private IPs so DS can reach cozy via docker gateway
# (e.g. 172.17.0.1); allowMetaIPAddress stays false for safety.
# NOTE: this MUST live under services.CoAuthoring (not top-level) — OnlyOffice
# reads services.CoAuthoring.request-filtering-agent; a top-level key is ignored
# and the SSRF filter keeps blocking the docker gateway → "download failed".
ca["request-filtering-agent"] = {
    "allowPrivateIPAddress": True,
    "allowMetaIPAddress": False,
}

print(json.dumps(cfg, indent=2))
PY
)

echo "Writing patched local.json back into container '$OO_CONTAINER'..."
printf '%s' "$PATCHED_JSON" | docker exec -i -u root "$OO_CONTAINER" \
  sh -c 'cat > /etc/onlyoffice/documentserver/local.json && chown ds:ds /etc/onlyoffice/documentserver/local.json'

# ---------------------------------------------------------------------------
# Restart Document Server processes to pick up the new config
# ---------------------------------------------------------------------------
echo "Restarting OnlyOffice services inside '$OO_CONTAINER'..."
docker exec "$OO_CONTAINER" supervisorctl restart all

# ---------------------------------------------------------------------------
# Second healthcheck — config is now active
# ---------------------------------------------------------------------------
wait_healthy "$OO_PORT"

if [ -n "$OO_SECRET" ]; then
  echo "Mode: JWT outbox ENABLED (outbox secret set)."
else
  echo "Mode: JWT fully OFF (no outbox secret)."
fi
echo "Container: $OO_CONTAINER"
echo "Done."
