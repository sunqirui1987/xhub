#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/frontend"
export E2E_UI_PORT="${E2E_UI_PORT:-3100}"
export E2E_GW_PORT="${E2E_GW_PORT:-4100}"
export E2E_UP_PORT="${E2E_UP_PORT:-4110}"
export E2E_GATEWAY="http://127.0.0.1:$E2E_GW_PORT"
export E2E_UPSTREAM="http://127.0.0.1:$E2E_UP_PORT"
export E2E_RUN_DIR="$ROOT/.e2e/current"
export E2E_BUILD_DIR=.next-e2e
# 完整浏览器套件包含 Responses 三轮续接场景，始终装载其本地离线模型夹具。
export E2E_RESPONSES_CONTINUATION=1
export XHUB_GATEWAY_ORIGIN="$E2E_GATEWAY"
export NEXT_PUBLIC_BASE_URL="$E2E_GATEWAY"
LOCK="$ROOT/.e2e/browser.lock"
mkdir -p "$ROOT/.e2e"
if ! mkdir "$LOCK" 2>/dev/null; then
  echo "Another browser run owns $LOCK. If interrupted, confirm it stopped before removing this directory." >&2
  exit 1
fi
cleanup() {
  local exit_code=$?
  # Playwright may terminate its whole server process group before the gateway
  # shell runs its EXIT trap. Recover its private schema once ports are free.
  if python3 - "$E2E_GW_PORT" "$E2E_UP_PORT" <<'PYCLEAN'
import socket, sys
for value in sys.argv[1:]:
    with socket.socket() as probe:
        if probe.connect_ex(("127.0.0.1", int(value))) == 0:
            raise SystemExit(1)
PYCLEAN
  then
    rm -f "$E2E_RUN_DIR/c.yaml"
    if [[ -f "$E2E_RUN_DIR/schema.txt" ]]; then
      local schema
      schema="$(cat "$E2E_RUN_DIR/schema.txt")"
      if [[ "$schema" =~ ^e2e_[0-9]+_[0-9]+$ ]]; then
        docker exec xhub-postgres psql -X -U xhub -d xhub -v ON_ERROR_STOP=1 -q -c "DROP SCHEMA IF EXISTS $schema CASCADE" >"$E2E_RUN_DIR/cleanup.log" 2>&1 || true
      fi
    fi
  fi
  rmdir "$LOCK"
  return "$exit_code"
}
trap cleanup EXIT
mkdir -p "$E2E_RUN_DIR"
rm -f "$E2E_RUN_DIR/results.json" "$E2E_RUN_DIR/junit.xml"
echo "Browser preflight: UI=$E2E_UI_PORT gateway=$E2E_GW_PORT upstream=$E2E_UP_PORT"
python3 - "$E2E_UI_PORT" "$E2E_GW_PORT" "$E2E_UP_PORT" <<'PY'
import socket, sys
ports = [int(value) for value in sys.argv[1:]]
if len(set(ports)) != 3: raise SystemExit("E2E ports must be distinct")
for port in ports:
    with socket.socket() as probe:
        probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try: probe.bind(("127.0.0.1", port))
        except OSError: raise SystemExit(f"E2E port {port} is occupied; set E2E_UI_PORT / E2E_GW_PORT / E2E_UP_PORT")
PY
docker exec xhub-postgres psql -X -U xhub -d xhub -qAt -c 'SELECT 1' >/dev/null
if [[ $# == 0 ]]; then export E2E_FULL_COVERAGE=1; fi
python3 - <<'PYCONFIG'
import json
from pathlib import Path
config = json.loads(Path("tsconfig.json").read_text())
config["include"] = [item for item in config["include"] if not item.startswith(".next")]
config["include"].append(".next-e2e/types/**/*.ts")
config["exclude"] = [*config.get("exclude", []), ".next"]
Path("tsconfig.e2e.json").write_text(json.dumps(config, indent=2) + "\n")
PYCONFIG
echo "Building production console (.next-e2e)..."
npm run build
echo "Building gateway and catalog sweep before the service readiness timeout..."
(cd "$ROOT" && go build -o "$E2E_RUN_DIR/xhub" ./cmd/gateway && go build -o "$E2E_RUN_DIR/livesweep" ./e2e/livesweep)
export E2E_PREBUILT=1
echo "Starting isolated gateway and console, then executing Playwright..."
npx playwright test "$@"
