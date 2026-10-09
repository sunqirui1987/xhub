#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIR="$ROOT/.e2e/current"
mkdir -p "$DIR"
UP_PORT="${E2E_UP_PORT:-4110}"
GW_PORT="${E2E_GW_PORT:-4100}"
MASTER="${E2E_MASTER_KEY:-sk-e2e-master}"
SCHEMA="e2e_$$_$RANDOM"
UP_PID=""
GW_PID=""
SCHEMA_CREATED=0
sql() { docker exec xhub-postgres psql -X -U xhub -d xhub -v ON_ERROR_STOP=1 -q -c "$1"; }
cleanup() {
  trap - EXIT INT TERM
  [[ -z "$GW_PID" ]] || { kill "$GW_PID" 2>/dev/null || true; wait "$GW_PID" 2>/dev/null || true; }
  [[ -z "$UP_PID" ]] || { kill "$UP_PID" 2>/dev/null || true; wait "$UP_PID" 2>/dev/null || true; }
  rm -f "$DIR/c.yaml"
  if [[ "$SCHEMA_CREATED" == 1 ]]; then sql "DROP SCHEMA IF EXISTS $SCHEMA CASCADE" >"$DIR/cleanup.log" 2>&1 || true; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
sql "CREATE SCHEMA $SCHEMA"
SCHEMA_CREATED=1
printf '%s\n' "$SCHEMA" > "$DIR/schema.txt"
E2E_UP_PORT="$UP_PORT" E2E_MASTER_KEY="$MASTER" E2E_SCHEMA="$SCHEMA" E2E_RUN_DIR="$DIR" python3 "$ROOT/e2e/config.py"
if [[ "${E2E_PREBUILT:-0}" != 1 ]]; then
  (cd "$ROOT" && go build -o "$DIR/xhub" ./cmd/gateway && go build -o "$DIR/livesweep" ./e2e/livesweep)
fi
python3 "$ROOT/e2e/fake_upstream.py" --port "$UP_PORT" >"$DIR/up.log" 2>&1 &
UP_PID=$!
XHUB_PRICE_FEED_URL="http://127.0.0.1:$UP_PORT/v1/market/models" XHUB_PUBLIC_ORIGIN="http://127.0.0.1:$GW_PORT" "$DIR/xhub" -config "$DIR/c.yaml" -addr "127.0.0.1:$GW_PORT" >"$DIR/gw.log" 2>&1 &
GW_PID=$!
wait "$GW_PID"
