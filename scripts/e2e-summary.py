#!/usr/bin/env python3
"""Persist reports. Missing evidence, failures and skips never become passes."""
import html
import json
import os
import re
from datetime import datetime, timezone
from pathlib import Path

root = Path(__file__).resolve().parent.parent
out = Path(os.environ.get("E2E_ACCEPTANCE_DIR", str(root / ".e2e")))
out.mkdir(parents=True, exist_ok=True)
result = Path(os.environ.get("E2E_RUN_DIR", str(root / ".e2e/current"))) / "results.json"
log = out / "backend.log"
backend = log.read_text(errors="replace") if log.exists() else ""
browser_log_path = out / "browser.log"
browser_log = browser_log_path.read_text(errors="replace") if browser_log_path.exists() else ""
browser_diagnostics = []
browser_log_lines = browser_log.splitlines()
for i, line in enumerate(browser_log_lines):
    if re.search(r"Type error:|Failed to type check|E2E port .*occupied|Error:|error:|Another browser run|missing go.sum entry", line):
        browser_diagnostics.extend(browser_log_lines[max(0, i - 1):i + 1])
preflight_path = out / "preflight.log"
preflight = preflight_path.read_text(errors="replace") if preflight_path.exists() else ""
browser = json.loads(result.read_text()) if result.exists() else None
browser_started = bool(re.search(r"Running \d+ tests? using", browser_log))
incomplete_stages = []
if not browser and browser_started:
    incomplete_stages.append("浏览器已开始执行，但没有完整结果；运行中断或报告未写出，不能计为通过。")
if backend and not re.search(r"^(?:ok|FAIL)\s+.*cmd/regression", backend, re.M):
    incomplete_stages.append("后端已开始执行，但没有套件结束记录；不能计为通过。")
metadata = json.loads(os.environ.get("E2E_PROVIDER_METADATA", "{}"))
backend_mode = os.environ.get("E2E_BACKEND_MODE", "live")
scope = os.environ.get("E2E_SCOPE", "完整真实供应商入口" if backend_mode == "live" else "确定性回归")
cases = []
def collect(suite, prefix=""):
    title = " / ".join(filter(None, [prefix, suite.get("title", "")]))
    for spec in suite.get("specs", []):
        for test in spec.get("tests", []):
            results = test.get("results", [])
            cases.append({"name": title + " / " + spec["title"], "status": test.get("status", "missing"),
                          "duration_ms": sum(r.get("duration", 0) for r in results),
                          "errors": [e.get("message", "") for r in results for e in r.get("errors", [])]})
    for child in suite.get("suites", []): collect(child, title)
if browser:
    for suite in browser.get("suites", []): collect(suite)
backend_cases = [{"status": m[0].lower(), "name": m[1], "seconds": m[2]}
                 for m in re.findall(r"^\s*--- (PASS|FAIL|SKIP): (\S+) \(([\d.]+)s\)", backend, re.M)]
# Go's verbose runner identifies the current test, including subtests. Keep
# assertion output with that test so the final terminal summary is actionable.
active_test = None
diagnostics = {}
for line in backend.splitlines():
    event = re.match(r"^(?:=== RUN|=== CONT)\s+(\S+)", line)
    if event:
        active_test = event[1]
    elif re.match(r"^\s+[^\s]+_test\.go:\d+: .+$", line) and active_test:
        diagnostics.setdefault(active_test, []).append(line.strip())
for case in backend_cases:
    case["errors"] = diagnostics.get(case["name"], [])
top = [c for c in backend_cases if "/" not in c["name"]]
backend_completed = bool(re.search(r"^(?:ok|FAIL)\s+.*cmd/regression", backend, re.M))
failed_assertions = [error for case in backend_cases if case["status"] == "fail" for error in case["errors"]]
build_diagnostics = [line.strip() for line in backend.splitlines() if re.search(
    r"missing go.sum entry|no required module provides package|^[^\s]+\.go:\d+:\d+:|\[setup failed\]|\[build failed\]", line)]
weighted = [json.loads(p.read_text()) for p in sorted((out / "evidence").glob("weighted-*.json"))]
coverage = {}
for name in ("pages", "routes", "chains"):
    p = out / ".e2e-report" / (name + ".txt")
    coverage[name] = p.read_text().splitlines() if p.exists() else []
summary = {
    "run_id": out.name, "completed_at": datetime.now(timezone.utc).isoformat(),
    "revision": os.environ.get("E2E_REVISION", "unknown"),
    "full_browser_suite": os.environ.get("E2E_FULL_COVERAGE") == "1",
    "scope": scope, "backend_mode": backend_mode,
    "provider_config": os.environ.get("E2E_PROVIDER_CONFIG"), "providers": metadata.get("providers", []),
    "configured_weighted_scenarios": metadata.get("weighted_scenarios", []),
    "browser": browser.get("stats") if browser else None,
    "browser_errors": [e.get("message", "") for e in browser.get("errors", [])] if browser else [],
    "browser_diagnostics": browser_diagnostics if not browser else [],
    "browser_exit": int(os.environ.get("E2E_BROWSER_STATUS", "1")),
    "backend_exit": int(os.environ.get("E2E_BACKEND_STATUS", "1")),
    "backend_pass": sum(c["status"] == "pass" for c in top),
    "backend_fail": sum(c["status"] == "fail" for c in top),
    "backend_skip": [c["name"] for c in backend_cases if c["status"] == "skip"],
    "backend_ok": bool(re.search(r"^ok\s+.*cmd/regression", backend, re.M)),
    "backend_completed": backend_completed,
    "backend_diagnostics": list(dict.fromkeys(failed_assertions + build_diagnostics)),
    "preflight_errors": preflight.splitlines()[-20:] if not browser and not browser_log and not backend else [],
    "incomplete_stages": incomplete_stages,
    "incomplete_browser_tail": browser_log_lines[-10:] if not browser and browser_started else [],
    "browser_cases": cases, "backend_cases": backend_cases, "weighted": weighted, "coverage": coverage,
    "unsupported_routes": [line for line in coverage["routes"] if line.startswith("route unsupported")],
}
weighted_missing = {s["name"] for s in metadata.get("weighted_scenarios", [])} - {w["scenario"] for w in weighted}
summary["missing_weighted_evidence"] = sorted(weighted_missing)
def valid_weighted_evidence(w):
    calls = w.get("calls", [])
    expected = w.get("expected", {})
    attempts = w.get("attempts")
    healthy = (attempts is None or
               (w.get("healthy_cycle") is True and len(attempts) == w.get("requests")
                and {a.get("request") for a in attempts} == set(range(1, w.get("requests", 0) + 1))
                and all(not a.get("failure") and 200 <= a.get("status", 0) < 300 for a in attempts)))
    return (w.get("status") == "passed" and bool(expected) and healthy
            and len(calls) == w.get("requests") == sum(expected.values())
            and all(sum(c.get("deployment") == deployment for c in calls) == count
                    for deployment, count in expected.items())
            and all(c.get("call_id") and c.get("cost_usd", 0) > 0 for c in calls))
summary["invalid_weighted_evidence"] = [w["scenario"] for w in weighted if not valid_weighted_evidence(w)]
failed = (summary["browser_exit"] or summary["backend_exit"] or not browser or not cases
          or browser.get("stats", {}).get("unexpected", 0) or summary["browser_errors"]
          or summary["backend_fail"] or not summary["backend_ok"] or not top
          or weighted_missing or summary["invalid_weighted_evidence"] or incomplete_stages)
summary["status"] = "failed" if failed else "passed"
(out / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
def cell(value): return str(value).replace("|", "\\|").replace("\n", " ")
lines = ["# XHub E2E 回归报告", "", "结果：**" + summary["status"].upper() + "**", "",
         "运行：" + out.name + "；完成时间：" + summary["completed_at"],
         "代码版本：" + summary["revision"] + "（包含当前工作区修改）", "",
         "验证范围：" + scope, "",
         "浏览器范围：" + ("完整套件" if summary["full_browser_suite"] else "筛选运行或尚未完成"), "",
         "## 三项独立验收", "",
         "- 页面与用户操作：浏览器逐页、实际点击、重新读取保存结果及错误分支。",
         "- 接口接线：catalog 巡检仅证明状态与协议标头，不能代替业务断言。",
         ("- 业务正确性：真实网关、PostgreSQL、Redis；真实供应商响应与计费，及模拟供应商的确定性故障注入。"
          if backend_mode == "live" else "- 业务正确性：真实网关、PostgreSQL、Redis及本地供应商；本次未启用真实供应商调用，不能验收其可用性及付费任务。"), "",
         "## 结果统计", "", "| 执行层 | 通过 | 失败 | 跳过 | 退出码 |", "| --- | ---: | ---: | ---: | ---: |"]
stats = summary["browser"] or {}
lines += [f"| 浏览器 | {stats.get('expected', 0)} | {stats.get('unexpected', '结果缺失')} | {stats.get('skipped', 0)} | {summary['browser_exit']} |",
          f"| 后端顶层用例 | {summary['backend_pass']} | {summary['backend_fail']} | {sum(c['status']=='skip' for c in top)} | {summary['backend_exit']} |", "",
          "后端顶层统计不重复计算子测试，子测试逐项列在下方。跳过不计为通过。", "",
          "## 真实供应商", "", "| 供应商 | 模型 | 协议 |", "| --- | --- | --- |"]
for p in summary["providers"]:
    lines.append("| " + " | ".join(cell(v) for v in [p["id"], ", ".join(p["models"]), p["protocol"]]) + " |")
lines += ["", "## 权重分流与计费证据", "",
          "平滑加权轮询按完整约分权重周期核对数量；7:3 的 10 次调用应精确为 7/3。要求候选部署健康、无粘滞复用和缓存命中。失败不自动重试整套测试。",
          "归属从实际成功响应的网关 affinity 记录读取，再以 call_id 对上账单；不根据返回模型名字猜测部署。", ""]
if not weighted: lines.append("本次没有权重测试证据，不能据此认定真实分流通过。")
for w in weighted:
    if w.get("failure_reason"):
        lines += ["失败原因：" + w["failure_reason"], ""]
    lines += ["### " + w["scenario"], "", "状态：" + w["status"] + "；完成调用：" + str(len(w["calls"])) + "/" + str(w["requests"]), "",
              "| 部署 | 预期请求 | 实际请求 | 费用 USD |", "| --- | ---: | ---: | ---: |"]
    for d in next(s["deployments"] for s in metadata["weighted_scenarios"] if s["name"] == w["scenario"]):
        i = d["id"]
        lines.append(f"| {cell(i)} | {w.get('expected', {}).get(i, '未完成')} | {sum(c['deployment']==i for c in w['calls'])} | {sum(c['cost_usd'] for c in w['calls'] if c['deployment']==i):.9f} |")
    lines += ["", "逐次 call_id、token 和费用见 evidence/ 下的 JSON；汇总见 summary.json。", ""]
lines += ["", "## 浏览器用例", "", "| 用例 | 状态 | 耗时 ms |", "| --- | --- | ---: |"]
for c in cases: lines.append(f"| {cell(c['name'])} | {c['status']} | {c['duration_ms']} |")
for c in cases:
    if c["errors"]: lines += ["", "失败：" + c["name"], "", re.sub(r"\x1b\[[0-9;]*m", "", "\n".join(c["errors"]))]
lines += ["", "## 后端用例（含子测试）", "", "| 用例 | 状态 | 耗时 s |", "| --- | --- | ---: |"]
for error in summary["browser_errors"]:
    lines += ["", "浏览器启动或全局检查失败：", "", re.sub(r"\x1b\[[0-9;]*m", "", error)]
if summary["browser_diagnostics"]:
    lines += ["", "浏览器构建或启动日志：", "", *summary["browser_diagnostics"]]
for c in backend_cases: lines.append(f"| {cell(c['name'])} | {c['status']} | {c['seconds']} |")
if summary["backend_fail"] or (not summary["backend_ok"] and backend):
    lines += ["", "## 后端失败诊断", "", "完整上下文见 backend.log；以下保留测试断言输出。", ""]
    lines += [re.sub(r"\x1b\[[0-9;]*m", "", line.strip()) for line in summary["backend_diagnostics"]]
if summary["preflight_errors"]:
    lines += ["", "## 启动前检查失败", "", *summary["preflight_errors"]]
if incomplete_stages:
    lines += ["", "## 执行未完成", "", *incomplete_stages, "", *summary["incomplete_browser_tail"]]
lines += ["", "## 跳过与范围限制", ""]
for name in summary["backend_skip"]: lines.append("- " + name)
lines += ["", "具体跳过原因见 backend.log。媒体真实生成与双协议对照需要额外配置，未配置不宣称通过。业务覆盖以文档矩阵与断言为准，不能有限穷举任意输入和故障组合。", "",
          "## 页面 / 接口巡检证据", "", *coverage["pages"], *coverage["routes"], "",
          "## 文件", "", "- [浏览器交互报告、截图和 trace](playwright-report/index.html)",
          "- [配置检查与入口日志](preflight.log)", "- [浏览器日志](browser.log)", "- [后端日志与跳过原因](backend.log)",
          "- [完整机器可读结果](summary.json)", "- [JUnit](browser/junit.xml)", ""]
report = "\n".join(lines)
(out / "report.md").write_text(report)
(out / "report.html").write_text('<!doctype html><meta charset="utf-8"><title>XHub E2E report</title><style>body{max-width:1200px;margin:40px auto;padding:20px;font:15px monospace}pre{white-space:pre-wrap;overflow-wrap:anywhere}</style><h1>XHub E2E '+summary["status"].upper()+'</h1><p><a href="playwright-report/index.html">Playwright 交互报告</a> · <a href="summary.json">JSON</a> · <a href="report.md">Markdown</a></p><pre>'+html.escape(report)+'</pre>')
print(f"E2E {summary['status'].upper()}: browser {stats.get('expected', 0)} passed / {stats.get('unexpected', 'missing')} failed; backend {summary['backend_pass']} passed / {summary['backend_fail']} failed; skipped subtests={len(summary['backend_skip'])}")
def short_error(error):
    clean = re.sub(r"\x1b\[[0-9;]*m", "", error)
    parts = [line.strip() for line in clean.splitlines() if line.strip()]
    return "; ".join(parts[:3])[:600]
if failed:
    print("Failure details:")
    for error in incomplete_stages:
        print("  [incomplete] " + error)
    for case in cases:
        if case["status"] not in ("expected", "skipped"):
            print("  [browser] " + case["name"])
            for error in case["errors"][:1]:
                print("    " + short_error(error))
    for error in summary["browser_errors"]:
        print("  [browser startup] " + short_error(error))
    for case in backend_cases:
        if case["status"] == "fail":
            print("  [backend] " + case["name"])
            for error in case["errors"][-1:]:
                print("    " + short_error(error))
    if not browser or not cases:
        print("  [browser] Test results missing; inspect browser.log / preflight.log")
        for error in summary["browser_diagnostics"][:8]:
            print("    " + short_error(error))
    if not summary["backend_ok"] and not summary["backend_fail"]:
        print("  [backend] Suite did not finish successfully; inspect backend.log")
        for error in summary["backend_diagnostics"][:8]:
            print("    " + short_error(error))
    for error in summary["preflight_errors"]:
        print("  [preflight] " + short_error(error))
    for scenario in summary["missing_weighted_evidence"]:
        print("  [weighted] Evidence missing: " + scenario)
    for scenario in summary["invalid_weighted_evidence"]:
        print("  [weighted] Evidence failed validation: " + scenario)
        for w in weighted:
            if w["scenario"] == scenario and w.get("failure_reason"):
                print("    " + short_error(w["failure_reason"]))
print("Report HTML: " + str(out / "report.html"))
print("Report Markdown: " + str(out / "report.md"))
print("Results JSON: " + str(out / "summary.json"))
print("Logs: " + str(out / "browser.log") + " ; " + str(log))
raise SystemExit(1 if failed else 0)
