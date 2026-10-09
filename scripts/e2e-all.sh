#!/usr/bin/env bash
# Full acceptance: browser clicks, real suppliers and deterministic fault cases.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [[ "${E2E_LIVE:-0}" != 1 ]]; then
  export E2E_ACCEPTANCE_DIR="$ROOT/.e2e/runs/$(date -u +%Y%m%dT%H%M%SZ)-$$"
  mkdir -p "$E2E_ACCEPTANCE_DIR"
  ln -sfn "$E2E_ACCEPTANCE_DIR" "$ROOT/.e2e/latest"
  preflight_status=0
  python3 "$ROOT/scripts/with-live-vendors.py" bash "$0" "$@" 2>&1 | tee "$E2E_ACCEPTANCE_DIR/preflight.log" || preflight_status=$?
  if [[ ! -f "$E2E_ACCEPTANCE_DIR/summary.json" ]]; then
    export E2E_RUN_DIR="$E2E_ACCEPTANCE_DIR/browser"
    export E2E_BROWSER_STATUS=1 E2E_BACKEND_STATUS=1
    python3 "$ROOT/scripts/e2e-summary.py" || true
  fi
  exit "$preflight_status"
fi
mkdir -p "$ROOT/.e2e"
LOCK="$ROOT/.e2e/acceptance.lock"
if ! mkdir "$LOCK" 2>/dev/null; then
  echo "Another full regression is running ($LOCK)." >&2
  exit 1
fi
REDIS_NAME="xhub-e2e-redis-$$"
export E2E_ACCEPTANCE_DIR="${E2E_ACCEPTANCE_DIR:-$ROOT/.e2e/runs/$(date -u +%Y%m%dT%H%M%SZ)-$$}"
export E2E_EVIDENCE_DIR="$E2E_ACCEPTANCE_DIR/evidence"
export E2E_RUN_DIR="$ROOT/.e2e/current"
export E2E_REVISION="$(git -C "$ROOT" rev-parse HEAD)"
export E2E_BROWSER_STATUS=1 E2E_BACKEND_STATUS=1
export E2E_FULL_COVERAGE=0
if [[ $# == 0 ]]; then export E2E_FULL_COVERAGE=1; fi
mkdir -p "$E2E_ACCEPTANCE_DIR/browser" "$E2E_EVIDENCE_DIR" "$E2E_RUN_DIR"
ln -sfn "$E2E_ACCEPTANCE_DIR" "$ROOT/.e2e/latest"
rm -f "$E2E_RUN_DIR/results.json" "$E2E_RUN_DIR/junit.xml"
cleanup() {
  local exit_code=$?
  if [[ "${REPORT_WRITTEN:-0}" != 1 ]]; then
    python3 "$ROOT/scripts/e2e-summary.py" || true
  fi
  docker rm -f "$REDIS_NAME" >/dev/null 2>&1 || true
  rmdir "$LOCK"
  return "$exit_code"
}
trap cleanup EXIT
echo "Run directory: $E2E_ACCEPTANCE_DIR"
echo "[1/4] Checking environment and starting isolated Redis; suppliers: $E2E_LIVE_VENDORS"
# Dedicated Redis with no persistent volume and a random local host port.
docker run --rm -d --name "$REDIS_NAME" -p 127.0.0.1::6379 redis:7-alpine >/dev/null
REDIS_PORT="$(docker port "$REDIS_NAME" 6379/tcp | cut -d: -f2)"
export XHUB_REGRESSION_REDIS_URL="redis://127.0.0.1:$REDIS_PORT/0"
for attempt in {1..30}; do
  if docker exec "$REDIS_NAME" redis-cli ping >/dev/null 2>&1; then break; fi
  sleep 0.2
done
status=0
# A heartbeat also covers compilation and long external requests with no output.
run_stage() {
  local label=$1 logfile=$2
  shift 2
  (set -o pipefail; "$@" 2>&1 | tee "$logfile") &
  local stage_pid=$!
  (while kill -0 "$stage_pid" 2>/dev/null; do
    sleep 30
    kill -0 "$stage_pid" 2>/dev/null && echo "[$(date +%H:%M:%S)] $label still running; log: $logfile"
  done) &
  local heartbeat_pid=$! stage_status=0
  wait "$stage_pid" || stage_status=$?
  kill "$heartbeat_pid" 2>/dev/null || true
  wait "$heartbeat_pid" 2>/dev/null || true
  return "$stage_status"
}
echo "[2/4] Building console and running browser clicks (live output)"
if run_stage Browser "$E2E_ACCEPTANCE_DIR/browser.log" bash "$ROOT/scripts/e2e.sh" "$@"; then
  browser_status=0
else
  browser_status=1
  status=1
fi
for artifact in results.json junit.xml; do
  if [[ -f "$E2E_RUN_DIR/$artifact" ]]; then cp "$E2E_RUN_DIR/$artifact" "$E2E_ACCEPTANCE_DIR/browser/"; fi
done
for artifact in playwright-report test-results .e2e-report; do
  if [[ -d "$ROOT/frontend/$artifact" ]]; then cp -R "$ROOT/frontend/$artifact" "$E2E_ACCEPTANCE_DIR/"; fi
done
export E2E_RUN_DIR="$E2E_ACCEPTANCE_DIR/browser"
echo "[3/4] Running backend business regression and real suppliers"
if run_stage Backend "$E2E_ACCEPTANCE_DIR/backend.log" bash "$ROOT/scripts/regression.sh" --live -v; then
  backend_status=0
else
  backend_status=1
  status=1
fi
echo "[4/4] Summarizing results: browser=$browser_status backend=$backend_status"
export E2E_BROWSER_STATUS="$browser_status" E2E_BACKEND_STATUS="$backend_status"
python3 "$ROOT/scripts/e2e-summary.py" || status=1
REPORT_WRITTEN=1
echo "Regression finished: exit=$status; report: $E2E_ACCEPTANCE_DIR/report.html"
exit "$status"
