"""保存本轮验收清单；旧检查点、未执行与本轮通过分别展示。"""
from contextlib import contextmanager
import json
from pathlib import Path
import time


class Checklist:
    """由真实验收 runner 持有，用例只有完成全部断言后才能通过。"""

    def __init__(self, directory, logger, plan):
        """用途：初始化本轮编号清单；参数为目录、日志回调和唯一 ID/名称列表，返回实例；覆盖本轮旧清单，不读取历史通过状态，重复 ID 拒绝。"""
        if len({ident for ident, _ in plan}) != len(plan):
            raise ValueError("验收用例 ID 重复")
        self.directory, self.logger = Path(directory), logger
        self.rows = [dict(id=ident, name=name, status="pending") for ident, name in plan]
        self.directory.mkdir(parents=True, exist_ok=True)
        self.save()
        for index, row in enumerate(self.rows, 1):
            logger(f"[待测 {index:03d}/{len(self.rows):03d}] {row['id']} {row['name']}")

    def save(self):
        """用途：保存本轮 JSON 与人工核对表；无参数/返回，供状态变更调用；未执行和运行中不能计为通过，异常向 runner 传播。"""
        counts = {status: sum(row["status"] == status for row in self.rows)
                  for status in ("passed", "failed", "pending", "running", "revalidated", "reused")}
        (self.directory / "cases.json").write_text(json.dumps(
            dict(counts=counts, cases=self.rows), ensure_ascii=False, indent=2) + "\n")
        labels = dict(passed="通过", failed="失败", pending="未执行", running="执行中",
                      revalidated="检查点已复验", reused="沿用历史证据，本轮未执行")
        lines = ["# 本轮 E2E 核对清单", "", " | ".join(f"{labels[key]}={value}" for key, value in counts.items()),
                 "", "| 编号 | 用例 | 状态 | 耗时(s) |", "| --- | --- | --- | --- |"]
        for index, row in enumerate(self.rows, 1):
            name = row['name'].replace('|', '\\|').replace('\n', ' ')
            lines.append(f"| {index:03d} {row['id']} | {name} | {labels[row['status']]} | {row.get('seconds', '')} |")
        (self.directory / "cases.md").write_text("\n".join(lines) + "\n")

    @contextmanager
    def case(self, ident):
        """用途：包围一个真实业务用例；参数为已登记 ID，产出可标注复验/沿用的记录；成功记录耗时，任何异常或中断标失败并原样抛出，不执行后续用例。"""
        row = next(row for row in self.rows if row["id"] == ident)
        if row["status"] != "pending":
            raise ValueError("本轮用例重复执行: " + ident)
        label = f"{self.rows.index(row) + 1:03d}/{len(self.rows):03d} {ident} {row['name']}"
        row["status"] = "running"
        started = time.monotonic()
        self.save()
        self.logger("[开始 " + label + "]")
        try:
            yield row
        except BaseException as error:
            row.update(status="failed", error_type=type(error).__name__)
            self.logger("[失败 " + label + "] " + type(error).__name__)
            raise
        else:
            if row["status"] == "running":
                row["status"] = "passed"
            names = dict(passed="通过", revalidated="检查点已复验", reused="沿用历史证据，本轮未执行")
            self.logger("[结果 " + label + "] " + names[row["status"]])
        finally:
            row["seconds"] = round(time.monotonic() - started, 3)
            self.save()

    def summary(self):
        """用途：输出当前真实状态汇总和报告路径；无参数/返回，由 runner 在清理阶段调用；失败时也保留待测项，不序列化业务凭据。"""
        self.save()
        counts = json.loads((self.directory / "cases.json").read_text())["counts"]
        names = dict(passed="通过", failed="失败", pending="未执行", running="执行中", revalidated="检查点已复验", reused="沿用历史")
        self.logger("[用例汇总] " + "，".join(f"{names[key]}={value}" for key, value in counts.items()))
        self.logger("[核对报告] " + str(self.directory / "cases.md"))
