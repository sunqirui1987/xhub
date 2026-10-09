#!/usr/bin/env python3
"""审计真实 Go 覆盖率，分别保留后台 regression 与单元测试的覆盖证据。"""
import argparse
import json
import subprocess
from pathlib import Path

MODULE = "github.com/sunqirui1987/xhub/"


def read_profile(path):
    """读取覆盖块；参数为 Go profile 路径，返回块映射；CLI 调用，坏格式直接失败，不改源文件。"""
    rows = {}
    if path is None:
        return rows
    lines = Path(path).read_text().splitlines()
    if not lines or lines[0] not in {"mode: set", "mode: count", "mode: atomic"}:
        raise ValueError(f"invalid coverage mode: {path}")
    for line in lines[1:]:
        location, statements, hits = line.rsplit(" ", 2)
        filename, span = location.rsplit(":", 1)
        if filename.startswith(MODULE + "internal/"):
            key = (filename, span)
            value = (int(statements), int(hits) > 0)
            if key in rows and rows[key][0] != value[0]:
                raise ValueError(f"inconsistent statement count: {location}")
            rows[key] = (value[0], value[1] or rows.get(key, (0, False))[1])
    return rows


def summarize(regression, unit, packages):
    """按模块汇总两层覆盖；参数为覆盖块和生产包名，返回完整矩阵；报告调用，无副作用。"""
    result = {package: {"statements": 0, "regression": 0, "combined": 0, "uncovered_blocks": []}
              for package in packages}
    for (filename, span) in sorted(regression.keys() | unit.keys()):
        r = regression.get((filename, span))
        u = unit.get((filename, span))
        if r and u and r[0] != u[0]:
            raise ValueError(f"profiles refer to different source revisions: {filename}:{span}")
        count = (r or u)[0]
        row = result.setdefault(filename.rsplit("/", 1)[0],
                                {"statements": 0, "regression": 0, "combined": 0, "uncovered_blocks": []})
        row["statements"] += count
        row["regression"] += count if r and r[1] else 0
        row["combined"] += count if (r and r[1]) or (u and u[1]) else 0
        if not r or not r[1]:
            row["uncovered_blocks"].append({"file": filename.removeprefix(MODULE), "span": span,
                                             "statements": count, "unit_covered": bool(u and u[1])})
    return result


def main():
    """生成覆盖审计；参数由 CLI 指定 profile 和目录，返回进程状态；本地验证调用，仅写报告。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--regression", required=True)
    parser.add_argument("--unit")
    parser.add_argument("--out", default=".e2e/internal-coverage")
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    packages = subprocess.check_output(["go", "list", "./internal/..."], cwd=root, text=True).splitlines()
    r, u = read_profile(args.regression), read_profile(args.unit)
    if not r:
        raise ValueError("regression profile contains no internal evidence; run the instrumented suite successfully first")
    rows = summarize(r, u, packages)
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    totals = {key: sum(row[key] for row in rows.values()) for key in ("statements", "regression", "combined")}
    report = {"profiles": {"regression": args.regression, "unit": args.unit}, "totals": totals, "packages": rows}
    (out / "coverage.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    lines = ["# internal 覆盖审计", "", "覆盖率是执行过的语句比例，不能证明所有输入和分支均正确。",
             "后台 regression 与单元测试单独统计；未执行语句不会因为单元测试通过而算作 regression 覆盖。", "",
             "| 模块 | 语句数 | regression | 两层合并 |", "| --- | ---: | ---: | ---: |"]
    for name, row in sorted(rows.items()):
        n = row["statements"]
        percentages = [f'{row[key] / n:.1%}' if n else "无可执行语句" for key in ("regression", "combined")]
        lines.append(f'| {name.removeprefix(MODULE)} | {n} | {percentages[0]} | {percentages[1]} |')
    lines.extend(["", "所有未被 regression 执行的源码块、位置及是否已被单元测试执行，见 coverage.json。",
                  "真实供应商 live 测试需要外部服务及凭据；本报告不将跳过的 live 测试计为通过。", ""])
    (out / "coverage.md").write_text("\n".join(lines))
    merged = ["mode: set"]
    for key in sorted(r.keys() | u.keys()):
        count = (r.get(key) or u[key])[0]
        hit = int(r.get(key, (0, False))[1] or u.get(key, (0, False))[1])
        merged.append(f"{key[0]}:{key[1]} {count} {hit}")
    merged_path = out / "combined.cover"
    merged_path.write_text("\n".join(merged) + "\n")
    for label, profile in [("regression", Path(args.regression)), ("combined", merged_path)]:
        subprocess.run(["go", "tool", "cover", "-html=" + str(profile.resolve()),
                        "-o", str((out / (label + ".html")).resolve())], cwd=root, check=True)
        functions = subprocess.check_output(["go", "tool", "cover", "-func=" + str(profile.resolve())], cwd=root, text=True)
        (out / (label + "-functions.txt")).write_text(functions)
    print(json.dumps(totals, ensure_ascii=False))


if __name__ == "__main__":
    main()
