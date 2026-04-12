#!/usr/bin/env bash
#
# scripts/webdav-litmus.sh — orchestrate litmus WebDAV compliance tests against a running cozy-stack.
#
# Usage:
#   scripts/webdav-litmus.sh                    # run all suites against both routes
#   LITMUS_TESTS="basic copymove" scripts/webdav-litmus.sh  # run subset of suites
#   scripts/webdav-litmus.sh --dry-run          # exercise lifecycle only, skip real litmus call
#
# Requires:
#   - cozy-stack running on localhost:8080 (invoke `cozy-stack serve` separately)
#   - /usr/bin/litmus installed (apt install litmus on Debian/Ubuntu)
#   - cozy-stack binary on PATH
#
# Exit codes:
#   0 — both routes passed (zero failed tests)
#   1 — one or both routes had failed tests
#   2 — script setup failure (missing binary, stack not running, etc.)
set -euo pipefail

DRY_RUN=0
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
fi

# --- Preflight checks ---
command -v cozy-stack >/dev/null 2>&1 || { echo "ERROR: cozy-stack not on PATH" >&2; exit 2; }
if [[ $DRY_RUN -eq 0 ]]; then
  command -v litmus >/dev/null 2>&1 || { echo "ERROR: litmus not installed (apt install litmus)" >&2; exit 2; }
fi

# Verify stack reachable (skip in dry-run).
if [[ $DRY_RUN -eq 0 ]]; then
  if ! curl -sf "http://localhost:8080/version" >/dev/null 2>&1; then
    echo "ERROR: cozy-stack not reachable at localhost:8080 (run 'cozy-stack serve' first)" >&2
    exit 2
  fi
fi

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
DOMAIN="litmus-${TIMESTAMP}.localhost:8080"

cleanup() {
  local rc=$?
  echo ""
  echo "--- Cleanup: destroying instance ${DOMAIN} ---"
  cozy-stack instances rm "${DOMAIN}" --force 2>/dev/null || true
  exit $rc
}
trap cleanup EXIT INT TERM

echo "--- Creating instance ${DOMAIN} ---"
if [[ $DRY_RUN -eq 0 ]]; then
  cozy-stack instances add "${DOMAIN}" --passphrase cozy --email "litmus@cozy.localhost"
else
  echo "(dry-run: skipping instances add)"
fi

echo "--- Generating CLI token ---"
if [[ $DRY_RUN -eq 0 ]]; then
  TOKEN=$(cozy-stack instances token-cli "${DOMAIN}" "io.cozy.files")
else
  TOKEN="dryrun-fake-token"
fi
if [[ -z "${TOKEN}" ]]; then
  echo "ERROR: failed to obtain token" >&2
  exit 2
fi

TESTS="${LITMUS_TESTS:-basic copymove props http locks}"

# --- Run litmus against both routes ---
FAILURES=0
run_suite() {
  local route="$1"
  local url="http://${DOMAIN}${route}"
  echo ""
  echo "=========================================="
  echo "  Litmus run: TESTS='${TESTS}' against ${url}"
  echo "=========================================="
  if [[ $DRY_RUN -eq 1 ]]; then
    echo "(dry-run: skipping litmus invocation)"
    return 0
  fi
  if TESTS="${TESTS}" litmus "${url}" "" "${TOKEN}"; then
    echo "PASS: ${url}"
  else
    echo "FAIL: ${url}"
    FAILURES=$((FAILURES + 1))
  fi
}

run_suite "/dav/files/"
run_suite "/remote.php/webdav/"

echo ""
echo "=========================================="
if [[ $FAILURES -eq 0 ]]; then
  echo "  Litmus RESULT: PASS (both routes clean)"
  echo "=========================================="
  exit 0
else
  echo "  Litmus RESULT: FAIL (${FAILURES} route(s) had failed tests)"
  echo "=========================================="
  exit 1
fi
