"""视频验收日志恢复的 PostgreSQL 回归；显式启用后创建私有 schema，不调用供应商。"""
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
import uuid

from real_dataset import Dataset, load_manifest

SPEC = importlib.util.spec_from_file_location("media_recovery_runner", Path(__file__).parents[1] / "scripts/e2e-real-dataset.py")
RUNNER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RUNNER)


@unittest.skipUnless(os.environ.get("XHUB_MEDIA_RECOVERY_DATABASE_TEST") == "1", "需要显式启用私有 PostgreSQL 回归")
class MediaLogRecoveryDatabaseTests(unittest.TestCase):
    """在真实 PostgreSQL 上验证恢复查询、结算条件及坏证据拒绝，不修改 public 数据。"""

    def setUp(self):
        """用途：建立随机隔离 schema 及最小日志契约；无参数/返回，供各回归调用；连接失败即失败，注册清理删除 schema 和临时报告。"""
        self.schema = "media_recovery_" + uuid.uuid4().hex
        RUNNER.sql(f"CREATE SCHEMA {self.schema};")
        self.addCleanup(RUNNER.sql, f"DROP SCHEMA {self.schema} CASCADE;")
        RUNNER.sql(f"""SET search_path TO {self.schema};
CREATE TABLE usage_events (request_id text PRIMARY KEY, model text, call_type text, status text,
 task_settled boolean DEFAULT false, price_snapshot text DEFAULT '', cost numeric DEFAULT 0);
CREATE TABLE request_logs (request_id text PRIMARY KEY REFERENCES usage_events, request_body text, response_body text);""")
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.dataset = Dataset("http://127.0.0.1:1", directory.name, load_manifest(), logger=lambda _: None)

    def insert(self, request_id, task, call_type, status, response, settled=False, cost=1, price="catalog"):
        """用途：向本轮私有 schema 写入日志夹具；参数为身份、任务、调用类型、状态、响应及结算字段，返回无；供成功及坏证据回归使用，正文转义，schema 随测试清理。"""
        values = [str(value).replace("'", "''") for value in (request_id, task["model"], call_type, status, price, response)]
        identity, model, protocol, state, snapshot, body = values
        RUNNER.sql(f"""SET search_path TO {self.schema};
INSERT INTO usage_events VALUES ('{identity}', '{model}', '{protocol}', '{state}', {str(settled).lower()}, '{snapshot}', {cost});
INSERT INTO request_logs VALUES ('{identity}', '{{}}', '{body}');""")

    def test_original_completed_task_recovers_same_id_for_all_protocols(self):
        """目的：三种视频协议完成后恢复原 ID 且只计一次；前置真实私有库日志和含引号 Task ID，验证恢复、价格、费用及持久化报告，schema 和目录自动清理。"""
        for index, transport in enumerate(("qiniu_contents_generation", "qiniu_fal_doubao_20", "qiniu_fal_kling")):
            task = {"model": "video-model", "transport": transport, "task_id": f"exact'{index}"}
            identity = f"original-{index}"
            response = json.dumps({"id" if index == 0 else "request_id": task["task_id"], "status": "COMPLETED"})
            self.insert(identity, task, transport + ":create", "completed", response, settled=True)
            self.dataset.report["media_tasks"] = [task]
            self.assertEqual(RUNNER.recover_media_log_ids(self.dataset, self.schema), 1)
            self.assertEqual((task["create_call_id"], task["settlement_log_id"]), (identity, identity))
            self.assertEqual(RUNNER.audit_video_settlement(task, self.schema),
                             {"rows": 1, "priced": 1, "positive": 1, "request_id": identity})

    def test_legacy_separate_settlement_still_recovers(self):
        """目的：旧独立结算日志仍可续跑；前置三类旧创建和终态日志，验证两个 ID 不同且结算唯一，私有 schema 和报告自动清理。"""
        for index, transport in enumerate(("qiniu_contents_generation", "qiniu_fal_doubao_20", "qiniu_fal_kling")):
            task = {"model": "video-model", "transport": transport, "task_id": f"legacy-{index}"}
            response = json.dumps({"request_id": task["task_id"]})
            self.insert(f"create-{index}", task, transport + ":create", "success", response, cost=0, price="")
            suffix = "get" if index == 0 else "status"
            paid = f"official-settlement:legacy-{index}"
            self.insert(paid, task, transport + ":" + suffix, "success", response)
            self.dataset.report["media_tasks"] = [task]
            self.assertEqual(RUNNER.recover_media_log_ids(self.dataset, self.schema), 1)
            self.assertEqual(task["create_call_id"], f"create-{index}")
            self.assertEqual(task["settlement_log_id"], paid)
            self.assertEqual(RUNNER.audit_video_settlement(task, self.schema)["rows"], 1)

    def test_incomplete_and_misleading_evidence_is_rejected(self):
        """目的：失败、未结算、非法正文和 Task ID 前缀均不能恢复；前置坏证据逐条写入私有库，验证恢复及审计明确失败，测试结束删除库和目录。"""
        task = {"model": "video-model", "transport": "qiniu_fal_kling", "task_id": "exact"}
        self.dataset.report["media_tasks"] = [task]
        for index, (status, settled, response) in enumerate((
            ("completed", True, '{"request_id":"exact-suffix"}'),
            ("completed", True, '{"request_id":"other","prompt":"exact"}'),
            ("completed", True, "not-json exact"),
            ("completed", False, '{"request_id":"exact"}'),
            ("polling", True, '{"request_id":"exact"}'),
            ("failed", True, '{"request_id":"exact"}'),
        )):
            self.insert(f"bad-{index}", task, task["transport"] + ":create", status, response, settled=settled)
            with self.subTest(index=index):
                with self.assertRaisesRegex(AssertionError, "无法恢复视频"):
                    RUNNER.recover_media_log_ids(self.dataset, self.schema)
                with self.assertRaisesRegex(AssertionError, "结算未去重"):
                    RUNNER.audit_video_settlement(task, self.schema)

    def test_unpriced_or_duplicate_settlements_cannot_pass(self):
        """目的：恢复 ID 不代表结算审计通过；前置零费用、无快照及双结算任务，验证审计拒绝并识别重复条数，私有 schema 和目录自动清理。"""
        for index, (cost, price) in enumerate(((0, "catalog"), (1, ""), (1, "catalog"))):
            task = {"model": "video-model", "transport": "qiniu_contents_generation", "task_id": f"exact-{index}"}
            response = json.dumps({"id": task["task_id"]})
            self.insert(f"original-{index}", task, task["transport"] + ":create", "completed", response, settled=True, cost=cost, price=price)
            if index == 2:
                self.insert("official-settlement:duplicate", task, task["transport"] + ":get", "success", response)
            with self.subTest(index=index), self.assertRaisesRegex(AssertionError, "结算未去重"):
                RUNNER.audit_video_settlement(task, self.schema)
