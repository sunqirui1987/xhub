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
export XHUB_GATEWAY_ORIGIN="$E2E_GATEWAY"
export NEXT_PUBLIC_BASE_URL="$E2E_GATEWAY"
LOCK="$ROOT/.e2e/browser.lock"
mkdir -p "$ROOT/.e2e"
if ! mkdir "$LOCK" 2>/dev/null; then
  echo "Another browser run owns $LOCK. If interrupted, confirm it stopped before removing this directory." >&2
  exit 1
fi
trap 'rmdir "$LOCK"' EXIT
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
npm run build
npx playwright test "$@"
