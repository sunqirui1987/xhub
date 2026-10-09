#!/usr/bin/env bash
# 模型配置专项：真实控制台/网关/PostgreSQL/Redis，本地供应商，不触发付费调用。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p "$ROOT/.e2e"
# 与完整入口共享锁，防止同一工作区的报告和共享浏览器产物相互覆盖。
LOCK="$ROOT/.e2e/acceptance.lock"
if ! mkdir "$LOCK" 2>/dev/null; then
  echo "Another full regression is running ($LOCK)." >&2
  exit 1
fi
export E2E_ACCEPTANCE_DIR="$ROOT/.e2e/runs/$(date -u +%Y%m%dT%H%M%SZ)-models-$$"
export E2E_REVISION="$(git rev-parse HEAD)"
export E2E_BACKEND_MODE=deterministic E2E_FULL_COVERAGE=0
export E2E_SCOPE='模型完整目录、Kling/Vidu 保存编辑、端点加载异常、供应商切换、护栏、密钥生命周期、权重配置；后端完整确定性业务回归'
export E2E_BROWSER_STATUS=1 E2E_BACKEND_STATUS=1
unset E2E_LIVE E2E_LIVE_VENDORS E2E_PROVIDER_METADATA XHUB_REGRESSION_LIVE
mkdir -p "$E2E_ACCEPTANCE_DIR/browser" "$ROOT/.e2e"
REDIS_NAME="xhub-model-e2e-redis-$$"
report_written=0
# cleanup 保留入口退出码，补写中断报告并清理本次 Redis 与锁；由 EXIT 调用。
cleanup() {
  local code=$?
  if [[ "$report_written" == 0 ]]; then
    E2E_RUN_DIR="$E2E_ACCEPTANCE_DIR/browser" python3 "$ROOT/scripts/e2e-summary.py" || true
  fi
  docker rm -f "$REDIS_NAME" >/dev/null 2>&1 || true
  rmdir "$LOCK" || true
  return "$code"
}
trap cleanup EXIT
# run_stage 执行 label 对应命令并将实时输出写入 log；返回命令/tee 的失败码。
# 调用方以 if 接收退出码，后台心跳在阶段结束后终止，不改变测试结果。
run_stage() {
  local label=$1 log=$2
  shift 2
  (set -o pipefail; "$@" 2>&1 | tee "$log") &
  local stage_pid=$! stage_status=0
  (timer_pid=''
  # TERM 必须同时停止定时子进程，否则 sleep 仍持有日志管道，入口结束后迟迟没有 EOF。
  trap 'if [[ -n "$timer_pid" ]]; then kill "$timer_pid" 2>/dev/null || true; wait "$timer_pid" 2>/dev/null || true; fi; exit 0' TERM INT
  while kill -0 "$stage_pid" 2>/dev/null; do
    sleep 30 &
    timer_pid=$!
    wait "$timer_pid" || true
    timer_pid=''
    kill -0 "$stage_pid" 2>/dev/null && echo "[$(date +%H:%M:%S)] $label still running; log: $log"
  done) &
  local heartbeat_pid=$!
  wait "$stage_pid" || stage_status=$?
  kill "$heartbeat_pid" 2>/dev/null || true
  wait "$heartbeat_pid" 2>/dev/null || true
  return "$stage_status"
}
echo "Report directory: $E2E_ACCEPTANCE_DIR"
echo '[1/4] Starting isolated Redis (no paid provider calls)'
docker run --rm -d --name "$REDIS_NAME" -p 127.0.0.1::6379 redis:7-alpine >/dev/null
REDIS_PORT="$(docker port "$REDIS_NAME" 6379/tcp | cut -d: -f2)"
export XHUB_REGRESSION_REDIS_URL="redis://127.0.0.1:$REDIS_PORT/0"
ready=0
for attempt in {1..30}; do
  if docker exec "$REDIS_NAME" redis-cli ping >/dev/null 2>&1; then ready=1; break; fi
  sleep 0.2
done
[[ "$ready" == 1 ]] || { echo 'Redis readiness failed' >&2; exit 1; }
echo '[2/4] Building console and exercising browser flows'
if run_stage Browser "$E2E_ACCEPTANCE_DIR/browser.log" bash scripts/e2e.sh \
  '(^|/)model-discovery.spec.ts$' '(^|/)model-endpoints.spec.ts$' '(^|/)wizards.spec.ts$' '(^|/)writes.spec.ts$' \
  '(^|/)weighted-routing.spec.ts$' '(^|/)xgo-guardrails.spec.ts$'; then
  export E2E_BROWSER_STATUS=0
else
  export E2E_BROWSER_STATUS=$?
fi
# 只有本次确实启动浏览器后才复制结果，避免把上一轮报告当成当前证据。
if rg -q '^Running [0-9]+ tests? using' "$E2E_ACCEPTANCE_DIR/browser.log"; then
  for artifact in results.json junit.xml; do
    if [[ -f "$ROOT/.e2e/current/$artifact" ]]; then cp "$ROOT/.e2e/current/$artifact" "$E2E_ACCEPTANCE_DIR/browser/"; fi
  done
  for artifact in playwright-report test-results .e2e-report; do
    if [[ -d "$ROOT/frontend/$artifact" ]]; then cp -R "$ROOT/frontend/$artifact" "$E2E_ACCEPTANCE_DIR/"; fi
  done
fi
echo '[3/4] Running complete deterministic backend regression'
if run_stage Backend "$E2E_ACCEPTANCE_DIR/backend.log" bash scripts/regression.sh -v; then
  export E2E_BACKEND_STATUS=0
else
  export E2E_BACKEND_STATUS=$?
fi
echo '[4/4] Writing JSON, Markdown and HTML report'
export E2E_RUN_DIR="$E2E_ACCEPTANCE_DIR/browser"
status=0
if [[ "$E2E_BROWSER_STATUS" != 0 || "$E2E_BACKEND_STATUS" != 0 ]]; then status=1; fi
python3 scripts/e2e-summary.py || status=1
report_written=1
echo "Regression finished: exit=$status; report: $E2E_ACCEPTANCE_DIR/report.html"
exit "$status"
