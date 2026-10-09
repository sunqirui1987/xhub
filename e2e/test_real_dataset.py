"""真实数据集规划的确定性单元测试；不连接外网、不写入用户数据库。"""
import copy
from contextlib import redirect_stdout
import io
import json
from pathlib import Path
import random
import tempfile
import unittest
from unittest import mock
import importlib.util
from real_dataset import Dataset, bill_check, chat_candidates, hierarchy, load_manifest, write_private, resolve_document

RUNNER_SPEC = importlib.util.spec_from_file_location("e2e_real_dataset_runner", Path(__file__).parents[1] / "scripts/e2e-real-dataset.py")
RUNNER = importlib.util.module_from_spec(RUNNER_SPEC)
RUNNER_SPEC.loader.exec_module(RUNNER)


class DatasetTests(unittest.TestCase):
    """验证层级唯一性、输入失败和账单边界，临时文件由上下文清理。"""

    def test_hierarchy(self):
        """目的：固定清单展开27个唯一成员和81把钥匙；前置真实清单，断言团队均三人，无外部数据清理。"""
        data = load_manifest()
        rows = hierarchy(data)
        self.assertEqual(len(rows), 27)
        self.assertEqual(len({r["email"] for r in rows}), 27)
        self.assertEqual(len(rows) * len(data["key_profiles"]), 81)
        for org in data["organizations"]:
            for team in data["teams"]:
                self.assertEqual(sum(r["organization"] == org and r["team"] == team for r in rows), 3)

    def test_invalid_manifest(self):
        """目的：拒绝重复身份、错误版本和非法费率；前置清单副本，验证失败输入，不写库，临时文件自动清理。"""
        for kind in ("version", "duplicate", "rate", "profiles"):
            data = copy.deepcopy(load_manifest())
            if kind == "version":
                data["version"] = 2
            elif kind == "duplicate":
                data["organizations"][1]["slug"] = data["organizations"][0]["slug"]
            elif kind == "rate":
                data["billing"]["input_cost_per_token"] = -1
            else:
                data["key_profiles"] = ["inherit"]
            with tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "data.json"
                path.write_text(json.dumps(data))
                with self.subTest(kind=kind), self.assertRaises(ValueError):
                    load_manifest(path)

    def test_candidates(self):
        """目的：真实目录随机选择只保留聊天候选；前置混合目录，验证去重、空目录边界，无副作用。"""
        rows = chat_candidates({"model_ids": ["gpt-real", "gpt-real", "qwen-live", "gpt-image", "qwen-embedding", "tts"]}, random.Random(1))
        self.assertEqual(set(rows), {"gpt-real", "qwen-live"})
        self.assertEqual(chat_candidates({}, random.Random(1)), [])

    def test_bill(self):
        """目的：验证五级账单与价格快照；前置固定费率，核对成功、归属错、零用量和错误金额，不产生数据库数据。"""
        rates = load_manifest()["billing"]
        key = dict(token_id="k", user_id="u", team_id="t", project_id="p", organization_id="o")
        bill = dict(api_key="k", user="u", team_id="t", project_id="p", organization_id="o",
                    prompt_tokens=11, completion_tokens=7, spend=0.000025, metadata={"cost_breakdown": {"source": "snapshot"}})
        self.assertAlmostEqual(bill_check(bill, key, rates), 0.000025)
        for field, value in (("user", "foreign"), ("prompt_tokens", 0), ("spend", 1), ("metadata", {})):
            with self.subTest(field=field), self.assertRaises(AssertionError):
                bill_check({**bill, field: value}, key, rates)

    def test_private_access(self):
        """目的：访问文件每次保持0600；前置临时目录，验证保存后与覆盖后权限，目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "access.json"
            write_private(path, {"key": "private-example"})
            path.chmod(0o644)
            write_private(path, {})
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)

    def test_document_resolution(self):
        """目的：完整路由文档替换键值变量且不修改原件；前置嵌套JSON，验证正常/未声明失败/空边界，无持久化清理。"""
        source = {"$alias": [{"id": "$deployment", "weight": 0}]}
        self.assertEqual(resolve_document(source, {"alias": "助手", "deployment": "真实ID"}),
                         {"助手": [{"id": "真实ID", "weight": 0}]})
        self.assertIn("$alias", source)
        self.assertEqual(resolve_document([], {}), [])
        with self.assertRaisesRegex(ValueError, "未声明模板变量"):
            resolve_document(source, {"alias": "助手"})

    def test_complete_manifest_rejects_missing_policy(self):
        """目的：缺失角色、源码或变量时在写库前拒绝；前置完整清单副本，验证失败边界，临时目录自动清理。"""
        for kind in ("role", "xgo", "variable"):
            data = copy.deepcopy(load_manifest())
            if kind == "role":
                data["personas"].pop()
            elif kind == "xgo":
                data["guardrails"][-1]["litellm_params"]["custom_code"] = ""
            else:
                data["routing"]["templates"]["fenno-only"]["unknown"] = "$missing"
            with tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "data.json"
                path.write_text(json.dumps(data))
                with self.subTest(kind=kind), self.assertRaises(ValueError):
                    load_manifest(path)

    def test_resume_only_skips_proven_checks(self):
        """目的：续跑只跳过成功场景；前置私有临时目录，验证失败/未运行边界仍需执行，目录自动清理，无网络。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            self.assertFalse(dataset.checked("quota"))
            dataset.report["checks"] = [{"name": "quota", "passed": False}, {"name": "role", "passed": True}]
            self.assertFalse(dataset.checked("quota"))
            self.assertTrue(dataset.checked("role"))
            self.assertFalse(dataset.checked("missing"))

    def test_shared_target_requires_exact_urls(self):
        """目的：破坏性清理只能命中用户指定目标；前置固定白名单，验证精确通过和数据库/Redis偏差失败，不连接或清理外部数据。"""
        RUNNER.validate_shared_target(RUNNER.SHARED_DATABASE_URL, RUNNER.SHARED_REDIS_URL)
        for database_url, redis_url in (
                (RUNNER.SHARED_DATABASE_URL.replace("/xhub?", "/postgres?"), RUNNER.SHARED_REDIS_URL),
                (RUNNER.SHARED_DATABASE_URL, "redis://127.0.0.1:6379/0")):
            with self.subTest(database_url=database_url, redis_url=redis_url), self.assertRaises(ValueError):
                RUNNER.validate_shared_target(database_url, redis_url)

    def test_shared_config_uses_public_database_and_redis_one(self):
        """目的：共享构建配置不带隔离schema且固定Redis DB 1；前置测试密码，验证配置契约，不写文件且无需清理。"""
        config = RUNNER.shared_config("test-password")["general_settings"]
        self.assertEqual(config["database_url"], RUNNER.SHARED_DATABASE_URL)
        self.assertEqual(config["redis_url"], RUNNER.SHARED_REDIS_URL)
        self.assertNotIn("search_path", config["database_url"])
        self.assertEqual(config["admin_password"], "test-password")

    def test_provider_validation_happens_without_database_access(self):
        """目的：缺少真实供应商密钥时必须在清库前失败；前置空环境，验证缺失和非法URL边界，本测试不调用重置函数。"""
        data = load_manifest()
        with self.assertRaisesRegex(ValueError, data["providers"][0]["key_env"]):
            RUNNER.validate_providers(data, {})
        environ = {provider["key_env"]: "secret" for provider in data["providers"]}
        environ[data["providers"][0]["base_env"]] = "http://insecure.invalid"
        with self.assertRaisesRegex(ValueError, "HTTPS"):
            RUNNER.validate_providers(data, environ)

    def test_progress_log_is_visible_and_contains_no_state(self):
        """目的：长时间真实验收会立即显示阶段日志；前置内存输出，验证正文可见且状态不被序列化，无外部数据需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            output = io.StringIO()
            with redirect_stdout(output):
                dataset.progress("构建阶段可见")
            self.assertEqual(output.getvalue(), "构建阶段可见\n")
            self.assertNotIn("keys", output.getvalue())

    def test_api_log_records_method_status_and_time_without_secrets(self):
        """目的：每个管理和数据接口都有可定位日志且不泄露参数；前置模拟 HTTP 成功，验证方法/路径/状态/耗时并隐藏查询和令牌，无外部数据需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lines.append)
            with mock.patch("real_dataset.http", return_value=(200, {"ok": True}, {})):
                value, _ = dataset.api("/team/list?private=value", token="secret-token")
            output = "\n".join(lines)
            self.assertEqual(value, {"ok": True})
            self.assertIn("GET /team/list 开始", output)
            self.assertIn("状态=200", output)
            self.assertIn("耗时=", output)
            self.assertNotIn("private=value", output)
            self.assertNotIn("secret-token", output)

    def test_run_logger_writes_identical_timestamped_terminal_and_file_lines(self):
        """目的：完整过程日志同时显示并持久化；前置临时日志，验证多行均带时间且正文一致，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "e2e.log"
            output = io.StringIO()
            logger = RUNNER.RunLogger(path)
            with redirect_stdout(output):
                logger.log("第一步\n第二步")
            self.assertEqual(output.getvalue(), path.read_text())
            self.assertRegex(path.read_text(), r"(?m)^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] 第一步$")
            self.assertIn("] 第二步\n", path.read_text())

    def test_shared_run_lock_rejects_concurrent_process(self):
        """目的：共享 xhub 构建与验收不能并发；前置临时锁文件，验证第二个持锁者立即失败，退出上下文后自动释放。"""
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "shared.lock"
            with RUNNER.shared_run_lock(path):
                with self.assertRaisesRegex(RuntimeError, "已有 make testdata"):
                    with RUNNER.shared_run_lock(path):
                        self.fail("竞争锁不应成功")
            with RUNNER.shared_run_lock(path):
                self.assertTrue(path.exists())


if __name__ == "__main__":
    unittest.main()
