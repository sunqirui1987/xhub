#!/usr/bin/env bash
# Start fake upstream + XHub gateway for Playwright (foreground).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIR="$ROOT/.e2e"
mkdir -p "$DIR"
UP_PORT="${E2E_UP_PORT:-4010}"
GW_PORT="${E2E_GW_PORT:-4000}"
MASTER="${E2E_MASTER_KEY:-sk-e2e-master}"

cleanup() {
  if [[ -f "$DIR/gw.pid" ]]; then kill "$(cat "$DIR/gw.pid")" 2>/dev/null || true; fi
  if [[ -f "$DIR/up.pid" ]]; then kill "$(cat "$DIR/up.pid")" 2>/dev/null || true; fi
}
trap cleanup EXIT

python3 "$ROOT/e2e/fake_upstream.py" --port "$UP_PORT" >"$DIR/up.log" 2>&1 &
echo $! > "$DIR/up.pid"

cat > "$DIR/c.yaml" <<YAML
model_list:
  - model_name: gpt-4o-mini
    litellm_params:
      model: openai/gpt-4o-mini
      api_key: sk-fake
      api_base: http://127.0.0.1:${UP_PORT}
router_settings:
  routing_strategy: simple-shuffle
  num_retries: 2
  timeout: 15
general_settings:
  master_key: ${MASTER}
  database_url: sqlite://${DIR}/e2e.db
YAML

(cd "$ROOT" && go build -o "$DIR/xhub" ./cmd/gateway)
rm -f "$DIR/e2e.db" "$DIR/e2e.db-shm" "$DIR/e2e.db-wal"

"$DIR/xhub" -config "$DIR/c.yaml" -addr ":$GW_PORT" >"$DIR/gw.log" 2>&1 &
echo $! > "$DIR/gw.pid"

ok=0
for _ in $(seq 1 40); do
  if curl -sf "http://127.0.0.1:${GW_PORT}/health/liveliness" >/dev/null; then
    ok=1
    break
  fi
  sleep 0.25
done
if [[ "$ok" != 1 ]]; then
  echo "gateway failed to listen on :$GW_PORT" >&2
  cat "$DIR/gw.log" >&2 || true
  exit 1
fi
wait "$(cat "$DIR/gw.pid")"
