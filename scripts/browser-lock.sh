#!/usr/bin/env bash
# acquire_browser_lock 为 e2e.sh 串行获取共享浏览器资源。
# 参数是锁目录；返回 0 表示调用方拥有锁，1 表示配置错误、目录错误或等待超时。
# E2E_BROWSER_LOCK_TIMEOUT 是最大等待秒数，默认 1800，0 表示立即失败；不删除其他运行的锁。
# 调用方必须在完成服务、schema 和报告清理后释放目录，等待期间不修改共享产物。
acquire_browser_lock() {
  local browser_lock_path=$1 browser_lock_timeout=${E2E_BROWSER_LOCK_TIMEOUT:-1800}
  local browser_lock_started=$SECONDS browser_lock_notified=0
  if [[ ! "$browser_lock_timeout" =~ ^[0-9]+$ || ${#browser_lock_timeout} -gt 6 ]]; then
    echo 'E2E_BROWSER_LOCK_TIMEOUT must be a non-negative integer (seconds, at most 6 digits).' >&2
    return 1
  fi
  browser_lock_timeout=$((10#$browser_lock_timeout))
  while ! mkdir "$browser_lock_path" 2>/dev/null; do
    # mkdir 失败后持有者可能恰好释放目录，此时继续竞争，不能把正常释放误判为路径错误。
    if [[ ( -e "$browser_lock_path" && ! -d "$browser_lock_path" ) || ! -w "$(dirname "$browser_lock_path")" ]]; then
      echo "Cannot acquire browser lock: $browser_lock_path" >&2
      return 1
    fi
    if (( SECONDS - browser_lock_started >= browser_lock_timeout )); then
      echo "Browser lock wait timed out after ${browser_lock_timeout}s: $browser_lock_path. The owner lock was preserved." >&2
      return 1
    fi
    if [[ "$browser_lock_notified" == 0 ]]; then
      echo "Another browser run owns $browser_lock_path; waiting up to ${browser_lock_timeout}s before preflight."
      browser_lock_notified=1
    fi
    sleep 1
  done
  if [[ "$browser_lock_notified" == 1 ]]; then
    echo "Browser lock acquired after $((SECONDS - browser_lock_started))s; continuing preflight."
  fi
}
