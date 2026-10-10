#!/usr/bin/env bash
# 真实供应商全量回归。终端按用例实时打印 RUN / PASS / FAIL / SKIP。
#
# 密钥只从环境读取，不写入仓库、YAML 或这条脚本。用法见
# docs/development/regression-live.md。
#
# 用法：
#   export FENNO_AI_API_KEY=... QINIU_API_KEY=...
#   bash scripts/regression-live-log.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -z "${FENNO_AI_API_KEY:-}" || -z "${QINIU_API_KEY:-}" ]]; then
  echo "缺少 FENNO_AI_API_KEY 或 QINIU_API_KEY。只把密钥放在环境里，不要写进文件。" >&2
  exit 1
fi

FENNO_AI_API_BASE="${FENNO_AI_API_BASE:-https://api.fenno.ai}"
QINIU_API_BASE="${QINIU_API_BASE:-https://api.modelink.ai}"
FENNO_LIVE_MODEL="${FENNO_LIVE_MODEL:-gpt-5.5}"
QINIU_LIVE_MODEL="${QINIU_LIVE_MODEL:-claude-4.5-haiku}"
FENNO_AI_API_BASE="${FENNO_AI_API_BASE%/}"
QINIU_API_BASE="${QINIU_API_BASE%/}"

RUN_DIR="${XHUB_REGRESSION_RUN_DIR:-.e2e/runs/regression-live-$(date -u +%Y%m%dT%H%M%SZ)}"
mkdir -p "$RUN_DIR"
LIVE_LOG="$RUN_DIR/live.log"
JSONL="$RUN_DIR/test.jsonl"
STDERR="$RUN_DIR/test.stderr"
YAML="$RUN_DIR/providers.yaml"
REPORT="$RUN_DIR/report.md"
: > "$LIVE_LOG"

say() {
  printf '%s\n' "$*" | tee -a "$LIVE_LOG"
}

say "运行目录: $RUN_DIR"
say "实时日志: $LIVE_LOG"
say "另开一个终端可执行: tail -f $ROOT/$LIVE_LOG"
say "Fenno base=${FENNO_AI_API_BASE} model=${FENNO_LIVE_MODEL} key_len=${#FENNO_AI_API_KEY}"
say "Qiniu base=${QINIU_API_BASE} model=${QINIU_LIVE_MODEL} key_len=${#QINIU_API_KEY}"

# 确认所选模型在供应商目录里。这一步只读 /v1/models，不发起付费对话。
python3 - "$FENNO_AI_API_BASE" "$FENNO_LIVE_MODEL" "$QINIU_API_BASE" "$QINIU_LIVE_MODEL" << 'PY' | tee -a "$LIVE_LOG"
import json, os, sys, urllib.request, urllib.error
pairs = [
    ("Fenno", sys.argv[1], sys.argv[2], os.environ["FENNO_AI_API_KEY"]),
    ("Qiniu", sys.argv[3], sys.argv[4], os.environ["QINIU_API_KEY"]),
]
for label, base, model, key in pairs:
    url = base.rstrip("/") + "/v1/models"
    req = urllib.request.Request(url, headers={"Authorization": "Bearer " + key})
    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            data = json.load(response)
    except urllib.error.HTTPError as exc:
        sys.exit(label + " 模型目录请求失败 HTTP " + str(exc.code))
    ids = [row.get("id") for row in data.get("data", []) if isinstance(row, dict)]
    if model not in ids:
        sys.exit(label + " 目录中没有模型 " + model + "。用 FENNO_LIVE_MODEL / QINIU_LIVE_MODEL 换成目录里的名字。")
    print(label + " 目录包含 " + model + "，共 " + str(len(ids)) + " 个模型", flush=True)
PY

if ! docker exec xhub-postgres psql -U xhub -d postgres -qAt -c 'select 1' >/dev/null 2>&1; then
  say "PostgreSQL 不可达。先执行: docker compose up -d postgres"
  exit 1
fi

DB_NAME="${XHUB_REGRESSION_DB_NAME:-xhub_regression_live}"
if ! docker exec xhub-postgres psql -U xhub -d postgres -qAt -c \
  "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
  docker exec xhub-postgres psql -U xhub -d postgres -v ON_ERROR_STOP=1 -c \
    "CREATE DATABASE ${DB_NAME} OWNER xhub;"
fi
say "数据库: ${DB_NAME} @ 127.0.0.1:5433（每条用例仍使用自己的 schema，结束时删除）"

REDIS_NAME="${XHUB_REGRESSION_REDIS_NAME:-xhub-regression-live-log}"
if ! docker inspect "$REDIS_NAME" >/dev/null 2>&1; then
  docker run --rm -d --name "$REDIS_NAME" -p 127.0.0.1::6379 redis:7-alpine >/dev/null
fi
REDIS_PORT="$(docker port "$REDIS_NAME" 6379/tcp | awk -F: 'NR==1 {print $NF}')"
docker exec "$REDIS_NAME" redis-cli FLUSHALL >/dev/null
say "Redis: redis://127.0.0.1:${REDIS_PORT}/0 （容器 ${REDIS_NAME}，已清空）"

# 只写非密钥元数据。with-live-vendors.py 按 key_env 从环境注入测试变量。
cat > "$YAML" << EOF
version: 1
providers:
  - id: FENNO
    enabled: true
    credential_name: fenno-live
    key_env: FENNO_AI_API_KEY
    base: ${FENNO_AI_API_BASE}
    protocol: openai
    models: [${FENNO_LIVE_MODEL}]
  - id: QINIU
    enabled: true
    credential_name: qiniu-live
    key_env: QINIU_API_KEY
    base: ${QINIU_API_BASE}
    protocol: openai
    models: [${QINIU_LIVE_MODEL}]
weighted_scenarios:
  - name: live-equal
    model_name: ${QINIU_LIVE_MODEL}
    requests: 2
    deployments:
      - {id: fenno-eq, provider: FENNO, model: ${FENNO_LIVE_MODEL}, weight: 1}
      - {id: qiniu-eq, provider: QINIU, model: ${QINIU_LIVE_MODEL}, weight: 1}
  - name: live-zero
    model_name: ${QINIU_LIVE_MODEL}
    requests: 2
    deployments:
      - {id: qiniu-pos, provider: QINIU, model: ${QINIU_LIVE_MODEL}, weight: 1}
      - {id: fenno-zero, provider: FENNO, model: ${FENNO_LIVE_MODEL}, weight: 0}
EOF

export XHUB_REGRESSION_STRICT=1
export XHUB_REGRESSION_TIMEOUT="${XHUB_REGRESSION_TIMEOUT:-3600s}"
export XHUB_TEST_DATABASE_URL="postgres://xhub:xhub_dev_password@127.0.0.1:5433/${DB_NAME}?sslmode=disable"
export XHUB_REGRESSION_REDIS_URL="redis://127.0.0.1:${REDIS_PORT}/0"
export E2E_PROVIDER_CONFIG="$ROOT/$YAML"
export E2E_EVIDENCE_DIR="$ROOT/$RUN_DIR/evidence"
mkdir -p "$E2E_EVIDENCE_DIR"
say "开始 go test ./cmd/regression 。超时 ${XHUB_REGRESSION_TIMEOUT}。付费调用期间每 20 秒报一次仍在执行的用例。"

set +e
python3 scripts/with-live-vendors.py \
  go test ./cmd/regression/ -count=1 -timeout="${XHUB_REGRESSION_TIMEOUT}" -json \
  2> "$STDERR" | python3 -u scripts/regression_live_follow.py "$LIVE_LOG" "$JSONL" "$REPORT"
CODE=${PIPESTATUS[0]}
FOLLOW_CODE=${PIPESTATUS[1]}
set -e
if [[ "$FOLLOW_CODE" -ne 0 && "$CODE" -eq 0 ]]; then
  CODE=$FOLLOW_CODE
fi
say "退出码: $CODE"
say "实时日志: $LIVE_LOG"
say "结果摘要: $REPORT"
say "原始事件: $JSONL"
if [[ -s "$STDERR" ]]; then
  say "标准错误: $STDERR"
fi
exit "$CODE"
