#!/usr/bin/env python3
"""把 go test -json 收成实时的一行结果。

用途：真实供应商回归跑的时候，终端立刻看到每个用例的 RUN / PASS / FAIL / SKIP，
不用等整包结束。网关调试日志仍留在原始 JSON 里，不刷屏。
参数：live 日志路径、JSONL 路径、报告路径。标准输入是 go test -json。
返回：包通过时进程退出码 0，失败或没有包结果时为 1。
调用：scripts/regression-live-log.sh。密钥样子的字符串会打成 sk-***。
"""
import json
import os
import queue
import re
import sys
import threading
import time

secret = re.compile(r"sk-[A-Za-z0-9_\-]{8,}")


def emit(live, text):
    """把一行同时写到终端和实时日志。参数 live 是已打开的日志；text 是这一行。无返回值。"""
    text = secret.sub("sk-***", text)
    print(text, flush=True)
    live.write(text + "\n")
    return time.time()


def interesting(blob):
    """留下断言和跳过原因，丢掉网关自己的调试行。参数 blob 是该用例的输出原文。返回行列表。"""
    rows = []
    for line in blob.splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("2026/") or stripped.startswith(("debug ", "trace ", "info ")):
            continue
        rows.append(secret.sub("sk-***", stripped))
    return rows[-12:]


def heartbeat_seconds():
    """返回静默心跳间隔。参数无。返回秒数。调用方是 main。测试可把 XHUB_REGRESSION_LOG_HEARTBEAT 设得很短。"""
    raw = os.environ.get("XHUB_REGRESSION_LOG_HEARTBEAT", "20")
    try:
        value = float(raw)
    except ValueError:
        return 20.0
    return value if value > 0 else 20.0


def main():
    """读完标准输入并写出实时日志和摘要。参数来自命令行。返回进程退出码。

    读标准输入放在后台线程里。主线程每秒看一次队列，所以供应商调用长时间不打日志时，
    仍能按心跳间隔写出当前用例，不必等下一条 go test 事件。
    """
    live_path, json_path, report_path = sys.argv[1:]
    live = open(live_path, "a", buffering=1)
    raw = open(json_path, "w", buffering=1)
    counts = {"pass": 0, "fail": 0, "skip": 0}
    top = {"pass": 0, "fail": 0, "skip": 0}
    fails = []
    current = ""
    started = {}
    last_print = time.time()
    package = None
    outputs = {}
    interval = heartbeat_seconds()
    incoming = queue.Queue()

    def read_stdin():
        """把标准输入逐行放进队列，结束后放 None。参数无。返回无。只由后台线程调用。"""
        for incoming_line in sys.stdin:
            incoming.put(incoming_line)
        incoming.put(None)

    threading.Thread(target=read_stdin, daemon=True).start()

    while True:
        try:
            line = incoming.get(timeout=1)
        except queue.Empty:
            if current and time.time() - last_print >= interval:
                waited = int(time.time() - started.get(current, time.time()))
                last_print = emit(live, f"{time.strftime('%H:%M:%S')} .... 仍在执行 {current}，已 {waited}s")
            continue
        if line is None:
            break
        # 落盘前去掉密钥样子的字符串。原始事件仍保留断言，但不保留 sk- 凭据。
        line = secret.sub("sk-***", line)
        raw.write(line)
        raw.flush()
        stripped = line.strip()
        if not stripped:
            continue
        try:
            event = json.loads(stripped)
        except json.JSONDecodeError:
            if "sk-" not in stripped:
                last_print = emit(live, stripped)
            continue
        action = event.get("Action")
        name = event.get("Test") or ""
        now = time.strftime("%H:%M:%S")
        if action == "output" and name:
            outputs.setdefault(name, []).append(event.get("Output") or "")
        if action == "run" and name:
            current = name
            started[name] = time.time()
            if "/" not in name:
                last_print = emit(live, f"{now} RUN  {name}")
        if action in ("pass", "fail", "skip") and name:
            counts[action] += 1
            if "/" not in name:
                top[action] += 1
            elapsed = event.get("Elapsed")
            extra = f" ({elapsed:.2f}s)" if isinstance(elapsed, (int, float)) else ""
            if action == "fail":
                rows = interesting("".join(outputs.get(name, [])))
                fails.append((name, rows))
                last_print = emit(live, f"{now} FAIL {name}{extra}")
                for row in rows:
                    last_print = emit(live, "       " + row)
            elif action == "skip":
                reason = ""
                for row in interesting("".join(outputs.get(name, []))):
                    if ".go:" in row:
                        reason = row
                last_print = emit(live, f"{now} SKIP {name}{extra} {reason}".rstrip())
            else:
                last_print = emit(live, f"{now} PASS {name}{extra}")
            if name == current:
                current = ""
        if action in ("pass", "fail") and not name:
            package = (action, event.get("Elapsed"))
        if current and time.time() - last_print >= interval:
            waited = int(time.time() - started.get(current, time.time()))
            last_print = emit(live, f"{now} .... 仍在执行 {current}，已 {waited}s")

    status = "未完成"
    if package:
        status = "PASS" if package[0] == "pass" else "FAIL"
        emit(live, f"包结果 {status}，耗时 {package[1]}s")
    emit(
        live,
        "顶层 pass={p} fail={f} skip={s}；全部结果项 pass={ap} fail={af} skip={as_}".format(
            p=top["pass"], f=top["fail"], s=top["skip"],
            ap=counts["pass"], af=counts["fail"], as_=counts["skip"],
        ),
    )
    lines = [
        "# 真实供应商回归实时结果",
        "",
        f"包结果: {status}",
        "",
        f"顶层: 通过 {top['pass']}，失败 {top['fail']}，跳过 {top['skip']}",
        f"含子测试: 通过 {counts['pass']}，失败 {counts['fail']}，跳过 {counts['skip']}",
        "",
        "失败明细:",
        "",
    ]
    if not fails:
        lines.append("无。")
    else:
        for name, rows in fails:
            lines.append(f"## {name}")
            lines.append("")
            lines.extend(rows or ["（无断言摘录）"])
            lines.append("")
    open(report_path, "w").write("\n".join(lines) + "\n")
    return 0 if package and package[0] == "pass" else 1


if __name__ == "__main__":
    sys.exit(main())
