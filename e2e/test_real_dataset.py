"""真实数据集规划的确定性单元测试；不连接外网、不写入用户数据库。"""
import copy
import base64
from contextlib import redirect_stdout
import io
import json
import os
from pathlib import Path
import random
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest import mock
import importlib.util
from real_dataset import Dataset, Observer, acceptance_run_id, bill_check, cache_bill_check, chat_candidates, chat_probe_result, curl_json, fallback_acceptance_deployments, fallback_acceptance_route, forward_supplier_chat, header_value, hierarchy, http, load_manifest, response_cache_hit, temporary_retry_delay, weighted_acceptance_route, write_private, resolve_document

RUNNER_SPEC = importlib.util.spec_from_file_location("e2e_real_dataset_runner", Path(__file__).parents[1] / "scripts/e2e-real-dataset.py")
RUNNER = importlib.util.module_from_spec(RUNNER_SPEC)
RUNNER_SPEC.loader.exec_module(RUNNER)


class DatasetTests(unittest.TestCase):
    """验证层级唯一性、输入失败和账单边界，临时文件由上下文清理。"""

    def test_permissions_separate_limits_from_models_and_status(self):
        """目的：防止组织管理员分配额度再次被旧验收拒绝；前置四角色及原值含 null/零的团队，核对每项请求的状态码和原值，并验证意外放行立即失败；不连接后台，临时报告自动清理。"""
        for unexpected_allow in (False, True):
            with self.subTest(unexpected_allow=unexpected_allow), tempfile.TemporaryDirectory() as directory:
                dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda _: None)
                dataset.admin = "platform-session"
                dataset.state["member_password"] = "unit-password"
                for persona in dataset.data["personas"]:
                    if persona["role"] == "platform_admin":
                        continue
                    dataset.state["users"].append({
                        "email": persona["role"], "organization_slug": persona["organization"],
                        "team_slug": persona["team"], "member_index": persona["member_index"],
                        "organization_id": "own-org", "team_id": "own-team"})
                dataset.state["teams"] = [{"id": "foreign-team", "organization_id": "foreign-org"}]
                info = {"max_budget": 30, "rpm_limit": None, "tpm_limit": 0,
                        "models": ["unit-model"], "status": "active"}
                calls = []

                def api(route, body=None, token=None, expected=200, method=None):
                    """用途：按独立权限矩阵模拟接口并记录请求；参数兼容 Dataset.api，返回正文和响应头；状态码不符抛错，仅写内存，供权限单测调用。"""
                    role = "platform_admin" if token in (None, "platform-session") else token
                    if route == "/v2/login":
                        return {"key": body["username"]}, {}
                    if route == "/team/list":
                        return [{}] * {"platform_admin": 9, "organization_admin": 3,
                                       "team_admin": 1, "member": 1}[role], {}
                    if route.startswith("/team/info?"):
                        return {"team_info": info.copy()}, {}
                    allowed = role == "platform_admin"
                    if route == "/team/update":
                        calls.append((role, body.copy(), expected))
                        own = body["team_id"] == "own-team"
                        if "team_description" in body:
                            allowed |= own and role in ("organization_admin", "team_admin")
                        elif any(limit in body for limit in ("max_budget", "rpm_limit", "tpm_limit")):
                            allowed |= own and role == "organization_admin"
                        if unexpected_allow and role == "organization_admin" and "models" in body:
                            allowed = True
                    actual = 200 if allowed else 403
                    if actual != expected:
                        raise AssertionError(f"{role} {route}: actual={actual}, expected={expected}")
                    return {}, {}

                dataset.api = api
                if unexpected_allow:
                    with self.assertRaisesRegex(AssertionError, "organization_admin.*actual=200, expected=403"):
                        dataset.verify_permissions()
                else:
                    dataset.verify_permissions()
                    self.assertEqual(len(dataset.state["personas"]), 4)
                    for role in ("platform_admin", "organization_admin", "team_admin", "member"):
                        for tid in ("own-team", "foreign-team"):
                            for limit in ("max_budget", "rpm_limit", "tpm_limit"):
                                matches = [(body, status) for caller, body, status in calls
                                           if caller == role and body["team_id"] == tid and limit in body]
                                self.assertEqual(len(matches), 1, f"{role}/{tid}/{limit} 必须独立验收")
                                self.assertEqual(matches[0][0][limit], info[limit], "不得改变团队原有配置")

    def test_permission_manifest_requires_distinct_limit_and_model_expectations(self):
        """目的：缺失或非法权限预期必须在写库前失败；前置真实清单副本，覆盖四个必填字段及无效状态码，临时清单自动清理。"""
        for field in ("own_team_limits", "foreign_team_limits", "team_models_update", "team_status_update"):
            for value in (None, 201):
                with self.subTest(field=field, value=value), tempfile.TemporaryDirectory() as directory:
                    data = load_manifest()
                    data["permission_scenarios"][0][field] = value
                    path = Path(directory) / "manifest.json"
                    path.write_text(json.dumps(data))
                    with self.assertRaisesRegex(ValueError, "预期状态码"):
                        load_manifest(path)

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

    def test_business_browser_reads_private_report_and_rejects_missing_or_failed_results(self):
        """目的：排队浏览器必须读取锁内保存的专属报告；前置模拟编排返回成功、失败或缺报告，验证路径和错误响应；临时目录清理，不启动外部服务。"""
        for mode in ["passed", "failed", "missing"]:
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as directory:
                report_dir = Path(directory)

                def command(command, logger, env):
                    """用途：模拟浏览器退出前保存专属报告；参数为命令、日志器和环境，无返回；供编排测试调用，仅写临时证据。"""
                    path = Path(env["E2E_BROWSER_REPORT_DIR"])
                    self.assertEqual(path, (report_dir / "business-browser").resolve())
                    if mode != "missing":
                        path.mkdir()
                        (path / "results.json").write_text(json.dumps({"stats": {
                            "expected": 19 if mode == "passed" else 18,
                            "unexpected": 0 if mode == "passed" else 1, "flaky": 0, "skipped": 0}}))

                with mock.patch.object(RUNNER, "command_stream", side_effect=command):
                    if mode == "passed":
                        result = RUNNER.run_business_browser(report_dir, mock.Mock())
                        self.assertEqual(result["cases"], 19)
                    else:
                        with self.assertRaisesRegex(AssertionError, "缺少本轮浏览器报告" if mode == "missing" else "未全部执行通过"):
                            RUNNER.run_business_browser(report_dir, mock.Mock())
                self.assertEqual((report_dir / "business-browser-results.json").exists(), mode != "missing")

    def test_acceptance_run_id_cannot_be_redacted_as_phone_number(self):
        """目的：运行标记不能被默认手机号护栏改写；前置固定长度，验证仅含小写字母且长度准确，无外部数据需要清理。"""
        for _ in range(100):
            value = acceptance_run_id(18)
            self.assertEqual(len(value), 18)
            self.assertTrue(value.isascii() and value.isalpha() and value.islower())

    def test_temporary_retry_delay_backs_off_and_honors_rate_limit_window(self):
        """目的：真实供应商短时断连不能在数秒内耗尽全部重试；前置普通 502 与上游 429 摘要，验证指数退避、封顶和完整限流冷却，无外部数据需要清理。"""
        self.assertEqual([temporary_retry_delay("upstream 502", 3, 65, attempt)
                          for attempt in range(1, 7)], [3, 6, 12, 24, 48, 65])
        self.assertEqual(temporary_retry_delay("upstream 429 rate limit", 3, 65, 1), 65)

    def test_xgo_modify_checks_debug_output_and_current_real_upstream_body(self):
        """目的：XGo modify 必须同时证明调试响应和当前真实外发正文已脱敏；前置单条真实清单规则和内存观察器，验证原文被替换且旧观察记录不参与断言，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            definition = next(row for row in dataset.data["guardrails"]
                              if row.get("acceptance", {}).get("action") == "modify")
            dataset.data["guardrails"] = [definition]
            dataset.observer = type("ObserverStub", (), {"rows": [
                {"messages": "旧请求 XGO_PRIVATE", "status": 200}],
                "lock": __import__("threading").Lock()})()
            dataset.api = mock.Mock(side_effect=[
                ({"guardrails": [{"guardrail_name": definition["guardrail_name"],
                                    "guardrail_id": "guard-modify"}]}, {}),
                ({"litellm_params": definition["litellm_params"]}, {}),
                ({"action": "modify", "response_text": definition["acceptance"]["replacement"]}, {}),
            ])

            def real_call(key, marker, extra, expected, guardrails):
                """模拟护栏处理后的当前真实上游正文；参数与 Dataset.call 一致，无返回；只把脱敏结果写入观察器。"""
                dataset.observer.rows.append({
                    "messages": marker + " " + extra.replace(
                        definition["acceptance"]["text"], definition["acceptance"]["replacement"]),
                    "status": 200,
                })

            key = {"call_model": "gpt-5.6-sol"}
            with mock.patch.object(dataset, "call", side_effect=real_call):
                dataset.verify_xgo(key)
            self.assertTrue(dataset.checked(definition["guardrail_name"] + "-persist-debug-real-request"))

            dataset.report["checks"] = []
            dataset.api = mock.Mock(side_effect=[
                ({"guardrails": [{"guardrail_name": definition["guardrail_name"],
                                    "guardrail_id": "guard-modify"}]}, {}),
                ({"litellm_params": definition["litellm_params"]}, {}),
                ({"action": "modify", "response_text": definition["acceptance"]["text"]}, {}),
            ])
            with self.assertRaisesRegex(AssertionError, "调试响应没有返回脱敏后的正文"):
                dataset.verify_xgo(key)

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
        """目的：真实目录优先稳定真实 ID 且只保留聊天候选；前置混合目录，验证优先级、去重、空目录边界，无副作用。"""
        rows = chat_candidates({"model_ids": ["gpt-real", "gpt-real", "qwen-live", "gpt-image", "qwen-embedding", "tts"]},
                               random.Random(1), ["qwen-live", "missing-model"])
        self.assertEqual(set(rows), {"gpt-real", "qwen-live"})
        self.assertEqual(rows[0], "qwen-live")
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

    def test_cache_headers_and_zero_cost_bill(self):
        """目的：缓存命中兼容两种大小写不敏感响应头并核对零费用日志；前置完整五级身份和首次 usage，验证合法账单通过及非零费用、错误归属、usage、命中标记和价格明细失败，无持久化数据。"""
        self.assertTrue(response_cache_hit({"Cache_Hit": "TRUE"}))
        self.assertTrue(response_cache_hit({"X-LiteLLM-Cache-Hit": "true"}))
        self.assertFalse(response_cache_hit({"cache_hit": "false"}))
        self.assertFalse(response_cache_hit({}))
        self.assertEqual(header_value({"X-Test": "value"}, "x-test"), "value")
        key = dict(token_id="k", user_id="u", team_id="t", project_id="p", organization_id="o")
        usage = {"prompt_tokens": 11, "completion_tokens": 7}
        bill = dict(api_key="k", user="u", team_id="t", project_id="p", organization_id="o",
                    **usage, spend=0, cache_hit=True, metadata={"cost_breakdown": {"source": "cache"}})
        self.assertIsNone(cache_bill_check(bill, key, usage))
        for field, value in (("spend", 0.1), ("team_id", "wrong"), ("completion_tokens", 6),
                             ("cache_hit", False), ("metadata", {})):
            with self.subTest(field=field), self.assertRaises(AssertionError):
                cache_bill_check({**bill, field: value}, key, usage)

    def test_weighted_acceptance_route_switches_only_the_active_deployment(self):
        """目的：确定性权重链只在两个真实部署间切换；前置同一模型和两个部署 ID，验证 100:0、0:100、无回退及单次尝试，纯内存结果无需清理。"""
        first = weighted_acceptance_route("gpt-5.6-sol", "a", "b", [100, 0], 300)
        second = weighted_acceptance_route("gpt-5.6-sol", "a", "b", [0, 100], 300)
        self.assertEqual(first["model_routes"][0]["allocations"], [
            {"deployment_id": "a", "weight": 100}, {"deployment_id": "b", "weight": 0}])
        self.assertEqual(second["model_routes"][0]["allocations"], [
            {"deployment_id": "a", "weight": 0}, {"deployment_id": "b", "weight": 100}])
        self.assertEqual(first["fallbacks"], [])
        self.assertEqual(first["retry_policy"]["max_attempts"], 1)

    def test_manifest_rejects_incomplete_logic_chains(self):
        """目的：真实组合链配置不能弱化缓存、权重或回退断言；前置完整清单副本，逐项破坏重复次数、隔离、失效、零费用、权重和429要求，验证加载失败，临时文件自动清理。"""
        mutations = {
            "missing": lambda data: data.pop("logic_chains"),
            "replays": lambda data: data["logic_chains"]["cache_consistency"].update(identical_replays=1),
            "isolation": lambda data: data["logic_chains"]["cache_consistency"].update(cross_key_isolation=False),
            "cost": lambda data: data["logic_chains"]["cache_consistency"].update(cache_hit_cost=1),
            "weights": lambda data: data["logic_chains"]["weighted_route_switch"].update(initial_weights=[99, 1]),
            "negative": lambda data: data["logic_chains"]["weighted_route_switch"].update(updated_weights=[-1, 101]),
            "status": lambda data: data["logic_chains"]["fallback_cache"].update(primary_failure_status=500),
            "fallback_replays": lambda data: data["logic_chains"]["fallback_cache"].update(repeat_requests=1),
        }
        for name, mutate in mutations.items():
            data = copy.deepcopy(load_manifest())
            mutate(data)
            with tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "data.json"
                path.write_text(json.dumps(data))
                with self.subTest(name=name), self.assertRaises(ValueError):
                    load_manifest(path)

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
        source = {"$model": [{"id": "$deployment", "weight": 0}]}
        self.assertEqual(resolve_document(source, {"model": "gpt-5.4", "deployment": "deployment-id"}),
                         {"gpt-5.4": [{"id": "deployment-id", "weight": 0}]})
        self.assertIn("$model", source)
        self.assertEqual(resolve_document([], {}), [])
        with self.assertRaisesRegex(ValueError, "未声明模板变量"):
            resolve_document(source, {"model": "gpt-5.4"})

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

    def test_manifest_declares_exact_real_models_and_protocols(self):
        """目的：锁定真实聊天、图片和视频验收矩阵；前置完整清单，验证模型 ID、协议、入口和创建路径，不产生外部数据。"""
        data = load_manifest()
        models = {row["id"]: row for row in data["models"]}
        self.assertEqual(set(models), {"fenno-chat", "qiniu-chat", "qiniu-glm", "fenno-image",
                         "qiniu-ark-seedance", "qiniu-fal-seedance", "qiniu-fal-kling"})
        expected = {
            "fenno-image": ("gpt-image-2", "bypass_openai_image_generation", "bypass:openai-images", "/bypass/openai/v1/images/generations"),
            "qiniu-ark-seedance": ("bytedance/doubao-seedance-2-0-mini-260615", "qiniu_contents_generation", "bypass:ark-video", "/v3/contents/generations/tasks"),
            "qiniu-fal-seedance": ("bytedance/seedance-2.0/mini/text-to-video", "qiniu_fal_doubao_20", "fal:queue", "/queue/bytedance/seedance-2.0/mini/text-to-video"),
            "qiniu-fal-kling": ("fal-ai/kling-video/v2.5-turbo/pro/text-to-video", "qiniu_fal_kling", "fal:queue", "/queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video"),
        }
        for ident, (name, transport, endpoint, path) in expected.items():
            with self.subTest(model=ident):
                row = models[ident]
                self.assertEqual(row["public_name"], name)
                self.assertEqual(row["upstream_model"], name)
                self.assertTrue(row["public_name"].isascii())
                self.assertEqual(row["transport"], transport)
                self.assertEqual(row["endpoint_types"], [endpoint])
                self.assertEqual(row["create_path"], path)
        self.assertEqual(models["fenno-chat"]["upstream_model"], "gpt-5.6-sol")
        self.assertEqual(models["qiniu-chat"]["upstream_model"], "openai/gpt-5.6-sol")
        self.assertEqual(models["qiniu-glm"]["upstream_model"], "z-ai/glm-5")
        for ident in ("fenno-chat", "qiniu-chat", "qiniu-glm"):
            self.assertEqual(models[ident]["endpoint_types"], ["chat", "responses"])
        self.assertEqual(data["limit_scenarios"]["model_allowlist"]["disallowed_model"],
                         "xhub/non-allowlisted-model")

    def test_live_route_timeout_covers_synchronous_media_generation(self):
        """目的：给同步图片生成保留完整供应商处理窗口；前置真实清单，验证所有作用域均为300秒，不访问外网或写库。"""
        data = load_manifest()
        self.assertEqual(data["routing"]["timeout_seconds"], 300)
        for name, template in data["routing"]["templates"].items():
            with self.subTest(template=name):
                self.assertEqual(template["retry_policy"]["timeout_seconds"], 300)

    def test_manifest_rejects_business_alias_as_public_model(self):
        """目的：禁止把业务别名冒充真实模型 ID；前置篡改图片公开名，验证加载失败，临时文件自动清理。"""
        data = copy.deepcopy(load_manifest())
        next(row for row in data["models"] if row["id"] == "fenno-image")["public_name"] = "Enterprise Assistant"
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "data.json"
            path.write_text(json.dumps(data))
            with self.assertRaisesRegex(ValueError, "公开模型名必须等于真实上游模型 ID"):
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

    def test_seed_provider_validation_requires_no_credentials_but_validates_urls(self):
        """目的：建数允许完全没有供应商密钥，但拒绝非法地址；前置清单与空环境，验证默认地址、空密钥边界及非法 URL，无网络或数据需要清理。"""
        data = load_manifest()
        RUNNER.validate_providers(data, {}, require_credentials=False)
        with self.assertRaisesRegex(ValueError, "HTTPS"):
            RUNNER.validate_providers(data, {data["providers"][0]["base_env"]: "http://invalid"},
                                      require_credentials=False)

    def test_seed_rejects_existing_tenants_before_supplier_or_model_calls(self):
        """目的：建数不能覆盖已有租户或进入供应商执行；前置登录成功但已有组织，验证失败后无目录、探测、会话和媒体调用，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda message: None)
            dataset.api = mock.Mock(side_effect=[({"key": "admin"}, {}), ([{"organization_id": "existing"}], {})])
            with mock.patch.dict(os.environ, {"E2E_DATASET_PASSWORD": "password"}), \
                    mock.patch.object(dataset, "discover_catalog") as catalog, \
                    mock.patch.object(dataset, "probe_chat_candidate") as probe, \
                    mock.patch.object(dataset, "verify_acceptance_models") as models:
                with self.assertRaisesRegex(RuntimeError, "已有组织"):
                    dataset.seed()
            catalog.assert_not_called()
            probe.assert_not_called()
            models.assert_not_called()

    def test_shared_seed_never_starts_observer_or_verification(self):
        """目的：共享 testdata 编排只启动网关和建数，无真实上游观察器或验收；前置替代进程及建数器，验证报告、零调用提示和进程退出，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            args = type("Args", (), {"phase": "seed", "directory": directory})()
            dataset = mock.Mock()
            dataset.report = {}
            dataset.state = {name: [] for name in ("organizations", "teams", "users", "projects", "keys")}
            with mock.patch.object(RUNNER, "reset_shared_target"), \
                    mock.patch.object(RUNNER, "command"), \
                    mock.patch.object(RUNNER, "ready"), \
                    mock.patch.object(RUNNER.subprocess, "Popen") as process, \
                    mock.patch.object(RUNNER.module, "Dataset", return_value=dataset), \
                    mock.patch.object(RUNNER.module, "Observer") as observer, \
                    mock.patch.dict(RUNNER.os.environ, {}, clear=True), redirect_stdout(io.StringIO()):
                self.assertEqual(RUNNER.shared_main_locked(args, load_manifest()), 0)
            dataset.seed.assert_called_once_with()
            dataset.verify.assert_not_called()
            observer.assert_not_called()
            process.return_value.terminate.assert_called_once_with()
            self.assertEqual(dataset.report["status"], "seeded")
            self.assertIn("模型调用=0", (Path(directory) / "testdata.log").read_text())

    def test_seed_creates_admin_key_without_running_models(self):
        """目的：建数只走管理接口并额外创建管理员本人的个人密钥；前置内存管理 API 与空租户，验证 81+1 归属、报告及零供应商调用，临时检查点自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda message: None)

            def management_api(route, body=None, **options):
                """模拟确定性管理响应；参数为路径、正文及请求选项，返回记录和空响应头；仅供建数单测，未声明路径立即失败，无外部副作用。"""
                if route == "/v2/login":
                    return {"key": "session", "user_id": "admin-id"}, {}
                if route == "/organization/list":
                    return [], {}
                if route == "/credentials":
                    return {"credentials": [{"credential_name": p["id"]} for p in dataset.data["providers"]]}, {}
                if route == "/model/groups?size=200":
                    groups = {}
                    for row in dataset.state["deployments"]:
                        group = groups.setdefault(row["public_name"], {"model_name": row["public_name"], "deployments": []})
                        group["deployments"].append({"id": row["id"], "model_name": row["public_name"],
                         "litellm_params": {"litellm_credential_name": row["provider"], "model": row["model"]},
                         "model_info": {"transport": row["transport"], "endpoint_types": row["endpoint_types"]}})
                    return {"data": list(groups.values())}, {}
                fields = {"/organization/new": "organization_id", "/team/new": "team_id",
                          "/user/new": "user_id", "/project/new": "project_id"}
                if route in fields:
                    return {fields[route]: route + str(dataset.api.call_count)}, {}
                if route == "/key/generate":
                    return {**body, "key": "plain", "token_id": str(dataset.api.call_count)}, {}
                if route in ("/model/default", "/route_template/binding", "/guardrails"):
                    return {}, {}
                raise AssertionError("建数出现未声明管理路径或数据面调用: " + route)

            def register_model(definition, provider, upstream=None, public_name=None):
                """模拟部署登记并填充状态；参数与 deployment 一致，返回唯一 ID；只隔离持久化细节，管理模型结构仍由 seed 验证。"""
                row = {**definition, "id": str(len(dataset.state["deployments"])),
                       "provider": provider["id"], "public_name": public_name or definition["public_name"],
                       "model": definition.get("stored_model", upstream or definition["upstream_model"])}
                dataset.state["deployments"].append(row)
                return row["id"]

            dataset.api = mock.Mock(side_effect=management_api)
            with mock.patch.dict(os.environ, {"E2E_DATASET_PASSWORD": "password"}), \
                    mock.patch.object(dataset, "deployment", side_effect=register_model), \
                    mock.patch.object(dataset, "template", return_value="template"), \
                    mock.patch.object(dataset, "discover_catalog") as catalog, \
                    mock.patch.object(dataset, "probe_chat_candidate") as probe, \
                    mock.patch.object(dataset, "verify_acceptance_models") as models:
                dataset.seed()
            self.assertEqual(len(dataset.state["keys"]), 81)
            self.assertEqual(dataset.state["admin_key"]["user_id"], "admin-id")
            self.assertEqual(dataset.state["admin_key"]["owner_type"], "personal")
            self.assertNotIn("team_id", dataset.state["admin_key"])
            self.assertEqual(dataset.report["total_key_count"], 82)
            self.assertEqual(dataset.report["calls"], [])
            catalog.assert_not_called()
            probe.assert_not_called()
            models.assert_not_called()

    def test_verify_starts_ordered_model_acceptance_after_count_check(self):
        """目的：模型验收前先核对基线；前置错误数量和完整实体，验证数量错误先拒绝、成员关系核对后模型失败中止，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda message: None)
            with mock.patch.object(dataset, "verify_acceptance_models", side_effect=RuntimeError("模型阶段失败")) as models, \
                    mock.patch.object(dataset, "api") as api:
                with self.assertRaisesRegex(AssertionError, "实体数量错误"):
                    dataset.verify()
                models.assert_not_called()
                for name, count in {"organizations": 3, "teams": 9, "users": 27, "projects": 9, "keys": 81}.items():
                    dataset.state[name] = [{"id": str(index)} for index in range(count)]
                api.return_value = ({"members": [{}, {}, {}]}, {})
                with self.assertRaisesRegex(RuntimeError, "模型阶段失败"):
                    dataset.verify()
                models.assert_called_once_with()
                self.assertEqual(api.call_count, 9)

    def test_progress_log_is_visible_and_contains_no_state(self):
        """目的：长时间真实验收会立即显示阶段日志；前置内存输出，验证正文可见且状态不被序列化，无外部数据需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            output = io.StringIO()
            with redirect_stdout(output):
                dataset.progress("构建阶段可见")
            self.assertEqual(output.getvalue(), "构建阶段可见\n")
            self.assertNotIn("keys", output.getvalue())

    def test_response_usage_audit_accepts_only_complete_consistent_rows(self):
        """目的：普通成功响应必须是合法 JSON 且 usage 与计量一致；前置模拟数据库分类结果，验证正常、空证据、非法 JSON、非法 token 和数值不一致，模拟查询无需清理。"""
        valid = {"successful_responses": 12, "invalid_json": 0, "with_usage": 8,
                 "invalid_usage": 0, "mismatches": 0}
        with mock.patch.object(RUNNER, "sql", return_value=json.dumps(valid)) as query:
            self.assertEqual(RUNNER.audit_response_usage(), valid)
            statement = query.call_args.args[0]
            self.assertIn("pg_input_is_valid(l.response_body, 'jsonb')", statement)
            self.assertIn("CASE WHEN pg_input_is_valid", statement)
            self.assertIn("u.status='success'", statement)
            self.assertIn("u.call_type IN ('chat', 'responses')", statement)
            self.assertIn("response->'usage'->>'input_tokens'", statement)
            self.assertIn("response->'usage'->>'output_tokens'", statement)
            self.assertIn("official-settlement:%", statement)
        failures = (
            ({**valid, "with_usage": 0}, "没有可核对的 usage"),
            ({**valid, "invalid_json": 1}, "非法 JSON"),
            ({**valid, "invalid_usage": 1}, "非法 usage token"),
            ({**valid, "mismatches": 1}, "持久化计量不一致"),
        )
        for row, message in failures:
            with self.subTest(message=message), mock.patch.object(RUNNER, "sql", return_value=json.dumps(row)):
                with self.assertRaisesRegex(AssertionError, message):
                    RUNNER.audit_response_usage()

    def test_video_settlement_audit_is_scoped_to_exact_task(self):
        """目的：视频结算去重必须按具体任务而非模型历史总数判断；前置固定 Ark 任务和模拟数据库结果，验证任务 ID、终态调用类型、价格及重复结算失败，无外部数据需清理。"""
        task = {"model": "bytedance/doubao-seedance-2-0-mini-260615",
                "transport": "qiniu_contents_generation", "task_id": "qvideo-exact-123"}
        proof = {"rows": 1, "priced": 1, "positive": 1, "request_id": "official-settlement:exact"}
        with mock.patch.object(RUNNER, "sql", return_value=json.dumps(proof)) as query:
            self.assertEqual(RUNNER.audit_video_settlement(task), proof)
            statement = query.call_args.args[0]
            self.assertIn("u.call_type='qiniu_contents_generation:get'", statement)
            self.assertIn("l.response_body::jsonb->>'request_id')='qvideo-exact-123'", statement)
            self.assertIn("u.status='completed' AND u.task_settled", statement)
            self.assertIn("min(u.request_id)", statement)
        with mock.patch.object(RUNNER, "sql", return_value=json.dumps({**proof, "rows": 2})):
            with self.assertRaisesRegex(AssertionError, "结算未去重"):
                RUNNER.audit_video_settlement(task)

    def test_wait_log_accepts_zero_cost_media_creation(self):
        """目的：媒体创建日志无需价格快照也能被读取，正式账单仍要求价格证据；前置模拟零费用详情，验证普通日志立即返回而账单继续轮询，模拟无外部数据需清理。"""
        dataset = Dataset("http://127.0.0.1:1", tempfile.mkdtemp(), load_manifest())
        empty_cost = (200, {"model": "video-model", "metadata": {}}, {})
        with mock.patch("real_dataset.http", return_value=empty_cost) as request:
            self.assertEqual(dataset.wait_log("create-id"), empty_cost[1])
            request.assert_called_once()
        with mock.patch("real_dataset.http", side_effect=[empty_cost, (200, {"metadata": {"cost_breakdown": {"source": "catalog"}}}, {})]), \
             mock.patch("real_dataset.time.sleep"):
            self.assertIn("cost_breakdown", dataset.wait_bill("settlement-id")["metadata"])

    def test_image_log_requires_real_response_body(self):
        """目的：图片日志必须包含真实模型、精确 prompt、图片正文和价格快照；前置模拟完整详情，验证通过及缺图失败，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda _: None)
            call = {"model": "gpt-image-2", "call_id": "image-log"}
            bill = {"model": "gpt-image-2", "proxy_server_request": {"body": {
                "model": "gpt-image-2", "prompt": "A red apple on a white background."}},
                "response": {"data": [{"b64_json": "real-base64"}]},
                "metadata": {"cost_breakdown": {"source": "catalog"}}}
            with mock.patch.object(dataset, "wait_bill", return_value=bill):
                self.assertEqual(dataset.verify_image_log(call), bill)
            with mock.patch.object(dataset, "wait_bill", return_value={**bill, "response": {"data": []}}):
                with self.assertRaisesRegex(AssertionError, "缺少真实图片响应"):
                    dataset.verify_image_log(call)

    def test_video_logs_require_exact_task_url_and_usage_boundary(self):
        """目的：视频日志分别验证创建与终态证据；前置 Ark 完整日志，验证精确 Task ID、URL、usage 通过及错误 URL 失败，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda _: None)
            model = "bytedance/doubao-seedance-2-0-mini-260615"
            task = {"model": model, "transport": "qiniu_contents_generation", "task_id": "qvideo-exact",
                    "create_call_id": "create-id", "settlement_log_id": "settlement-id",
                    "bypass_result": {"video_url": "https://media.example/video.mp4"}}
            definition = next(row for row in load_manifest()["models"] if row.get("public_name") == model)
            created = {"model": model, "proxy_server_request": {"body": {"model": model, "content": [{
                "type": "text", "text": "A red apple on a white table, static camera, gentle natural light."}]}},
                "response": {"id": "qvideo-exact"}}
            settled = {"model": model, "response": {"id": "qvideo-exact", "status": "succeeded",
                "content": {"video_url": "https://media.example/video.mp4"},
                "usage": {"completion_tokens": 40594}}, "metadata": {"cost_breakdown": {"source": "catalog"}}}
            with mock.patch.object(dataset, "wait_log", return_value=created), mock.patch.object(dataset, "wait_bill", return_value=settled):
                self.assertEqual(dataset.verify_video_logs(task, definition), (created, settled))
            wrong = copy.deepcopy(settled)
            wrong["response"]["content"]["video_url"] = "https://media.example/wrong.mp4"
            with mock.patch.object(dataset, "wait_log", return_value=created), mock.patch.object(dataset, "wait_bill", return_value=wrong):
                with self.assertRaisesRegex(AssertionError, "真实视频 URL"):
                    dataset.verify_video_logs(task, definition)

    def test_saved_media_log_verification_ignores_dynamic_chat_definitions(self):
        """目的：续跑媒体复核兼容没有固定 public_name 的动态聊天定义；前置真实清单及一条图片和视频记录，验证只分派媒体日志且不误读聊天配置，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda _: None)
            video_model = "bytedance/doubao-seedance-2-0-mini-260615"
            image = {"kind": "image", "model": "gpt-image-2", "call_id": "image-id"}
            task = {"kind": "video", "model": video_model, "task_id": "video-id"}
            dataset.report["media_calls"] = [image]
            dataset.report["media_tasks"] = [task]
            with mock.patch.object(dataset, "verify_image_log") as verify_image, \
                 mock.patch.object(dataset, "verify_video_logs") as verify_video:
                dataset.verify_saved_media_logs()
            verify_image.assert_called_once_with(image)
            definition = next(row for row in dataset.data["models"] if row.get("public_name") == video_model)
            verify_video.assert_called_once_with(task, definition)

    def test_recover_media_log_ids_uses_exact_task_and_call_types(self):
        """目的：续跑只按精确模型、Task ID 与协议恢复媒体日志；前置一个 FAL 任务和模拟查询，验证创建及终态 ID 回填和 SQL 约束，临时报告自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda _: None)
            task = {"model": "bytedance/seedance-2.0/mini/text-to-video", "transport": "qiniu_fal_doubao_20",
                    "task_id": "qvideo-exact", "kind": "video"}
            dataset.report["media_tasks"] = [task]
            with mock.patch.object(RUNNER, "sql", return_value="create-exact|official-settlement:exact") as query:
                self.assertEqual(RUNNER.recover_media_log_ids(dataset), 1)
            self.assertEqual(task["create_call_id"], "create-exact")
            self.assertEqual(task["settlement_log_id"], "official-settlement:exact")
            statement = query.call_args.args[0]
            self.assertIn("qiniu_fal_doubao_20:create", statement)
            self.assertIn("qiniu_fal_doubao_20:status", statement)
            self.assertIn("qvideo-exact", statement)

    def test_video_log_conditions_validate_and_escape_exact_identity(self):
        """目的：共享查询条件支持三类协议且拒绝不完整输入；前置正常及含引号任务，验证精确 JSON 匹配、转义和失败路径，无外部数据需清理。"""
        task = {"model": "model'quoted", "task_id": "task'quoted"}
        for transport in ("qiniu_contents_generation", "qiniu_fal_doubao_20", "qiniu_fal_kling"):
            with self.subTest(transport=transport):
                scope, create, settled = RUNNER.video_log_conditions({**task, "transport": transport})
                self.assertIn("model''quoted", scope)
                self.assertIn("task''quoted", scope)
                self.assertIn("pg_input_is_valid", scope)
                self.assertEqual(create, f"u.call_type='{transport}:create'")
                self.assertIn("u.status='completed' AND u.task_settled", settled)
                self.assertIn("official-settlement:%", settled)
        for invalid in ({}, {**task, "transport": "unknown"},
                        {**task, "task_id": "", "transport": "qiniu_fal_kling"}):
            with self.subTest(invalid=invalid), self.assertRaisesRegex(ValueError, "Task ID"):
                RUNNER.video_log_conditions(invalid)

    def test_recover_media_log_ids_supports_original_and_missing_evidence(self):
        """目的：原任务完成后恢复相同创建与结算 ID，缺失证据明确失败；前置临时报告及模拟查询，验证正常、空任务列表和两侧 ID 缺失，不调用供应商，目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda _: None)
            with mock.patch.object(RUNNER, "sql") as query:
                self.assertEqual(RUNNER.recover_media_log_ids(dataset), 0)
                query.assert_not_called()
            task = {"model": "video-model", "transport": "qiniu_contents_generation", "task_id": "exact"}
            dataset.report["media_tasks"] = [task]
            with mock.patch.object(RUNNER, "sql", return_value="original|original"):
                self.assertEqual(RUNNER.recover_media_log_ids(dataset), 1)
            self.assertEqual(task["create_call_id"], task["settlement_log_id"])
            for row in ("", "original|", "|settlement"):
                with self.subTest(row=row), mock.patch.object(RUNNER, "sql", return_value=row):
                    with self.assertRaisesRegex(AssertionError, "无法恢复视频创建或终态日志"):
                        RUNNER.recover_media_log_ids(dataset)

    def test_video_logs_revalidate_completed_original_and_reject_pending(self):
        """目的：复验同一日志的原始 prompt 与完成证据，待处理不可算通过；前置 FAL 详情及临时目录，验证完整 URL、usage 及状态失败，模拟接口不写外部数据，目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda _: None)
            task = {"model": "video-model", "transport": "qiniu_fal_doubao_20", "task_id": "exact",
                    "create_call_id": "original", "settlement_log_id": "original",
                    "bypass_result": {"video_url": "https://example.invalid/video.mp4"}}
            detail = {"model": task["model"], "status": "completed",
                      "proxy_server_request": {"body": {"prompt": "A red apple on a white table, static camera, gentle natural light."}},
                      "response": {"request_id": "exact", "status": "COMPLETED", "result": {
                          "video": {"url": task["bypass_result"]["video_url"]}, "usage": {"completion_tokens": 100}}}}
            with mock.patch.object(dataset, "wait_log", return_value=detail), mock.patch.object(dataset, "wait_bill", return_value=detail):
                self.assertEqual(dataset.verify_video_logs(task, {}), (detail, detail))
            with mock.patch.object(dataset, "wait_log", return_value={**detail, "status": "polling"}):
                with self.assertRaisesRegex(AssertionError, "原任务日志尚未完成"):
                    dataset.verify_video_logs(task, {})

    def test_verify_database_includes_completed_video_costs(self):
        """目的：视频原日志的 completed 费用必须计入日报对账；前置模拟聚合及空媒体列表，验证查询包括两种成功状态和金额不一致失败，临时报告自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda _: None)
            counter = json.dumps({"requests": 1, "prompt_tokens": 0, "completion_tokens": 100})
            summary = json.dumps({"events": 1, "unique_calls": 1, "cost": 0.1})
            with mock.patch.object(RUNNER, "sql", side_effect=[summary + "\n0.1", counter + "\n" + counter, *(["0"] * 5)]) as query, \
                 mock.patch.object(RUNNER, "audit_response_usage", return_value={"successful_responses": 1, "with_usage": 1}):
                self.assertEqual(RUNNER.verify_database(dataset)["cost"], 0.1)
                self.assertIn("status IN ('success', 'completed')", query.call_args_list[0].args[0])
            with mock.patch.object(RUNNER, "sql", return_value=summary + "\n0.2"):
                with self.assertRaisesRegex(AssertionError, "每日聚合金额"):
                    RUNNER.verify_database(dataset)

    def test_video_creation_checkpoint_keeps_original_settlement_id(self):
        """目的：新视频检查点立即保存原结算 ID，避免续跑前无终态引用；前置 Ark 创建及终态响应，验证成功路径、缺失创建 ID 与缺失任务 ID 失败，模拟调用不收费，临时文件自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda _: None)
            definition = next(row for row in dataset.data["models"] if row["id"] == "qiniu-ark-seedance")
            result = {"id": "exact", "content": {"video_url": "https://example.invalid/video.mp4", "duration": 4},
                      "usage": {"completion_tokens": 100}}
            with mock.patch.object(dataset, "api", return_value=({"id": "exact"}, {"x-litellm-call-id": "original"})), \
                 mock.patch.object(dataset, "poll_video", return_value=result), \
                 mock.patch.object(dataset, "video_result_query", return_value=result):
                dataset.verify_video_model({"key": "fake"}, definition)
            task = dataset.report["media_tasks"][0]
            self.assertEqual(task["create_call_id"], "original")
            self.assertEqual(task["settlement_log_id"], "original")
            for answer, headers, message in (({"id": "exact"}, {}, "创建响应缺少请求日志 ID"),
                                              ({}, {"x-litellm-call-id": "original"}, "视频创建响应缺少任务 ID")):
                with self.subTest(message=message), mock.patch.object(dataset, "api", return_value=(answer, headers)):
                    with self.assertRaisesRegex(AssertionError, message):
                        dataset.verify_video_model({"key": "fake"}, definition)

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

    def test_image_bypass_result_is_saved_as_binary_and_compact_json(self):
        """目的：图片 bypass 结果可直接查看且日志不塞入大段base64；前置微型PNG响应，验证文件、摘要和紧凑JSON，临时目录自动清理。"""
        raw = b"\x89PNG\r\n\x1a\nreal-image-bytes"
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            result = dataset.image_result("gpt-image-2", {"created": 1, "data": [{"b64_json": base64.b64encode(raw).decode()}]})
            self.assertEqual(Path(result["artifact"]).read_bytes(), raw)
            self.assertEqual(result["bytes"], len(raw))
            saved = Path(result["response_json"]).read_text()
            self.assertIn(result["sha256"], saved)
            self.assertNotIn(base64.b64encode(raw).decode(), saved)

    def test_image_bypass_result_recognizes_jpeg_artifact(self):
        """目的：真实图片JPEG响应以可直接预览的扩展名保存；前置微型JPEG字节，验证文件后缀和内容，临时目录自动清理。"""
        raw = b"\xff\xd8\xff\xe1real-jpeg-bytes"
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            result = dataset.image_result("gpt-image-2", {"data": [{"b64_json": base64.b64encode(raw).decode()}]})
            self.assertEqual(Path(result["artifact"]).suffix, ".jpg")
            self.assertEqual(Path(result["artifact"]).read_bytes(), raw)

    def test_image_generation_retries_only_documented_temporary_failures(self):
        """目的：真实图片生成遇到供应商临时错误时可恢复且不虚报通过；前置两次502后成功，验证三次独立调用和等待日志，使用模拟响应无需清理外部数据。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lines.append)
            definition = next(row for row in dataset.data["models"] if row["id"] == "fenno-image")
            responses = [
                ({"error": {"message": "temporary upstream failure"}}, {}, 502),
                ({"error": {"message": "temporary upstream failure"}}, {}, 502),
                ({"data": [{"url": "https://example.invalid/image.png"}]}, {"x-litellm-call-id": "call-1"}, 200),
            ]
            with mock.patch.object(dataset, "api", side_effect=responses) as api, mock.patch("real_dataset.time.sleep") as sleep:
                answer, headers = dataset.image_generation(definition, {"key": "secret"}, delay=1)
            self.assertEqual(answer["data"][0]["url"], "https://example.invalid/image.png")
            self.assertEqual(headers["x-litellm-call-id"], "call-1")
            self.assertEqual(api.call_count, 3)
            self.assertEqual(sleep.call_count, 2)
            output = "\n".join(lines)
            self.assertIn("❌ [gpt-image-2 / bypass_openai_image_generation / 第 1/3 次真实图片生成]", output)
            self.assertIn("✅ [gpt-image-2 / bypass_openai_image_generation / 第 3/3 次真实图片生成响应]", output)

    def test_image_generation_does_not_retry_non_temporary_failure(self):
        """目的：真实图片生成不能掩盖未声明的错误状态；前置供应商400响应，验证接口契约立即抛错且不等待，不访问或清理外部数据。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            definition = next(row for row in dataset.data["models"] if row["id"] == "fenno-image")
            with mock.patch.object(dataset, "api", side_effect=AssertionError("状态 400")) as api, \
                    mock.patch("real_dataset.time.sleep") as sleep:
                with self.assertRaisesRegex(AssertionError, "状态 400"):
                    dataset.image_generation(definition, {"key": "secret"}, delay=1)
            self.assertEqual(api.call_count, 1)
            sleep.assert_not_called()

    def test_provider_catalog_retries_empty_read_only_response(self):
        """目的：真实供应商目录偶发返回空列表时有限重试且不伪造模型；前置一次空目录后取得两个真实 ID，验证结果、调用次数和等待次数，模拟调用无需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            dataset.admin = "admin-token"
            provider = dataset.data["providers"][0]
            responses = [({"model_ids": []}, {}), ({"model_ids": ["gpt-6-sol", "gpt-image-2"]}, {})]
            with mock.patch.object(dataset, "api", side_effect=responses) as api, \
                    mock.patch("real_dataset.time.sleep") as wait:
                catalog = dataset.discover_catalog(provider, attempts=3, delay=1)
            self.assertEqual(catalog["model_ids"], ["gpt-6-sol", "gpt-image-2"])
            self.assertEqual(api.call_count, 2)
            wait.assert_called_once_with(1)

    def test_provider_catalog_fails_after_declared_attempts(self):
        """目的：连接代理和官方目录都失败时停止构建且不能退回自定义模型；前置三次错误代理及直连失败，验证明确失败和固定代理次数，模拟调用无需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            dataset.admin = "admin-token"
            provider = dataset.data["providers"][0]
            with mock.patch.object(dataset, "api", return_value=({"error": "temporary unavailable"}, {})) as api, \
                    mock.patch.object(dataset, "discover_supplier_catalog", side_effect=RuntimeError("direct unavailable")), \
                    mock.patch("real_dataset.time.sleep") as wait:
                with self.assertRaisesRegex(RuntimeError, "连接代理和供应商官方目录"):
                    dataset.discover_catalog(provider, attempts=3, delay=1)
            self.assertEqual(api.call_count, 3)
            self.assertEqual(wait.call_count, 2)

    def test_provider_catalog_falls_back_to_live_supplier_directory(self):
        """目的：连接目录代理持续失败时仍从同一供应商官方接口取得真实模型；前置代理三次错误和官方目录成功，验证回退结果与来源，不访问外网且无需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            provider = dataset.data["providers"][1]
            expected = {"model_ids": ["google/gemini-3.6-flash"], "source": "supplier-official-live"}
            with mock.patch.object(dataset, "api", return_value=({"error": "context deadline exceeded"}, {})), \
                    mock.patch.object(dataset, "discover_supplier_catalog", return_value=expected) as direct, \
                    mock.patch("real_dataset.time.sleep"):
                catalog = dataset.discover_catalog(provider, attempts=3, delay=1)
            self.assertEqual(catalog, expected)
            direct.assert_called_once_with(provider, attempts=3, delay=1)

    def test_direct_supplier_catalog_normalizes_openai_models(self):
        """目的：官方 /v1/models 响应只提取真实模型 ID 并去重；前置重复 OpenAI 目录响应，验证来源、数量和 GET 调用，不访问外网且无需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            provider = dataset.data["providers"][1]
            response = {"object": "list", "data": [{"id": "google/gemini-3.6-flash"}, {"id": "google/gemini-3.6-flash"}, {"id": "gpt-image"}]}
            with mock.patch.dict("os.environ", {provider["key_env"]: "secret"}), \
                    mock.patch("real_dataset.curl_json", return_value=(200, response, {})) as request:
                catalog = dataset.discover_supplier_catalog(provider, attempts=1)
            self.assertEqual(catalog["model_ids"], ["google/gemini-3.6-flash", "gpt-image"])
            self.assertEqual(catalog["source"], "supplier-official-live")
            self.assertIsNone(request.call_args.args[2])

    def test_chat_probe_records_network_failure_without_stopping_candidates(self):
        """目的：单个真实聊天候选网络断开时继续候选选择；前置供应商连接被远端关闭，验证失败证据和返回值，不访问真实外网且无需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lines.append)
            provider = dataset.data["providers"][0]
            with mock.patch.dict("os.environ", {provider["key_env"]: "secret"}), \
                    mock.patch("real_dataset.curl_json", side_effect=ConnectionError("remote closed")):
                passed = dataset.probe_chat_candidate(provider, "gpt-6-sol", 1, 5)
            self.assertFalse(passed)
            self.assertEqual(dataset.report["probe_attempts"][-1]["error"], "ConnectionError")
            self.assertFalse(any(row.get("name") == "real-model-probe" for row in dataset.report["checks"]))
            self.assertIn("❌ [gpt-6-sol / bypass_openai_chat / 真实模型候选探测 1/5]", "\n".join(lines))

    def test_chat_probe_marks_only_nonzero_real_usage_as_success(self):
        """目的：真实聊天候选必须同时返回回答和非零计量；前置成功供应商响应，验证打勾日志和通过证据，不访问真实外网且无需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lines.append)
            provider = dataset.data["providers"][0]
            reply = {"choices": [{"message": {"content": "OK"}}], "usage": {"prompt_tokens": 8, "completion_tokens": 1}}
            with mock.patch.dict("os.environ", {provider["key_env"]: "secret"}), \
                    mock.patch("real_dataset.curl_json", return_value=(200, reply, {})):
                passed = dataset.probe_chat_candidate(provider, "gpt-6-sol", 2, 5)
            self.assertTrue(passed)
            self.assertTrue(dataset.report["probe_attempts"][-1]["passed"])
            self.assertFalse(any(row.get("name") == "real-model-probe" for row in dataset.report["checks"]))
            self.assertIn("✅ [gpt-6-sol / bypass_openai_chat / 真实模型候选探测 2/5]", "\n".join(lines))

    def test_lifecycle_model_selection_continues_after_failed_candidate(self):
        """目的：模型生命周期探测遇到首个候选网络失败时继续选择；前置首个返回失败且第二个返回成功，验证选中第二个真实模型和完整候选顺序，模拟调用无需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            provider = dataset.data["providers"][1]
            with mock.patch.object(dataset, "probe_chat_candidate", side_effect=[False, True]) as probe:
                selected = dataset.select_lifecycle_model(provider, ["first-real", "second-real"], attempts=2, delay=0)
            self.assertEqual(selected, "second-real")
            self.assertEqual([call.args[1] for call in probe.call_args_list], ["first-real", "second-real"])
            self.assertEqual(probe.call_args_list[0].kwargs["action"], "生命周期探测")

    def test_chat_probe_result_handles_success_boundaries_and_malformed_responses(self):
        """目的：探测判定不能把计费、推理或畸形结构当作回答；前置正常、空白、错误和计量边界响应，验证安全失败及无正文证据；纯内存测试无需清理。"""
        good = {"choices": [{"message": {"content": "OK"}, "finish_reason": "stop"}],
                "usage": {"prompt_tokens": 19, "completion_tokens": 1}}
        self.assertTrue(chat_probe_result(200, good)["passed"])
        for status, reply in [(502, good), (200, None), (200, []), (200, {"choices": [None]}),
                              (200, {"choices": "invalid"}), (200, {**good, "error": {"message": "secret"}})]:
            with self.subTest(status=status, reply=reply):
                self.assertFalse(chat_probe_result(status, reply)["passed"])
        for content in [None, "", " \n", ["OK"], {"text": "OK"}]:
            with self.subTest(content=content):
                reply = copy.deepcopy(good)
                reply["choices"][0]["message"] = {"content": content, "reasoning_content": "private reasoning"}
                result = chat_probe_result(200, reply)
                self.assertFalse(result["passed"])
                self.assertNotIn("private reasoning", json.dumps(result))
                self.assertEqual(result["reasoning_chars"], 17)
        for usage in [None, [], {"prompt_tokens": 19, "completion_tokens": 0},
                      {"prompt_tokens": -1, "completion_tokens": 1},
                      {"prompt_tokens": True, "completion_tokens": 1},
                      {"prompt_tokens": "secret", "completion_tokens": 1},
                      {"prompt_tokens": float("nan"), "completion_tokens": 1}]:
            with self.subTest(usage=usage):
                result = chat_probe_result(200, {**good, "usage": usage})
                self.assertFalse(result["passed"])
                json.dumps(result, allow_nan=False)
                self.assertNotIn("secret", json.dumps(result))

    def test_chat_probe_retries_truncated_reasoning_with_larger_budget(self):
        """目的：推理耗尽32 token后应自适应重试而非重复相同请求；前置首轮仅有推理且 length、次轮有回答，验证预算32→1024和结构诊断；临时目录自动清理且不访问外网。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lines.append)
            provider = dataset.data["providers"][1]
            truncated = {"choices": [{"message": {"content": None, "reasoning_content": "private reasoning"},
                                      "finish_reason": "length"}],
                         "usage": {"prompt_tokens": 19, "completion_tokens": 32}}
            good = {"choices": [{"message": {"content": "OK"}, "finish_reason": "stop"}],
                    "usage": {"prompt_tokens": 19, "completion_tokens": 60}}
            with mock.patch.dict("os.environ", {provider["key_env"]: "secret"}), \
                    mock.patch("real_dataset.curl_json", side_effect=[(200, truncated, {}), (200, good, {})]) as request:
                self.assertTrue(dataset.probe_chat_candidate(provider, "moonshotai/kimi-k2.6", 1, 2, attempts=2, delay=0))
            self.assertEqual([call.args[2]["max_tokens"] for call in request.call_args_list], [32, 1024])
            probes = dataset.report["probe_attempts"]
            self.assertEqual([row["passed"] for row in probes], [False, True])
            self.assertEqual(probes[0]["finish_reason"], "length")
            self.assertEqual(probes[0]["content_chars"], 0)
            self.assertIn("提高 max_tokens 至 1024", "\n".join(lines))
            self.assertNotIn("private reasoning", json.dumps(probes) + "\n".join(lines))

    def test_chat_probe_does_not_expand_budget_for_unrelated_failures(self):
        """目的：网络失败、非截断空回答和错误响应不能触发预算增加；前置失败响应及两次重试，验证始终32 token且失败单列；临时目录清理，无外部调用。"""
        for status, reply in [(502, {}), (200, {"choices": [{"message": {"content": ""}, "finish_reason": "stop"}]}),
                              (200, {"error": "private", "choices": [{"message": {}, "finish_reason": "length"}]})]:
            with self.subTest(status=status, reply=reply), tempfile.TemporaryDirectory() as directory:
                dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda line: None)
                provider = dataset.data["providers"][1]
                with mock.patch.dict("os.environ", {provider["key_env"]: "secret"}), \
                        mock.patch("real_dataset.curl_json", return_value=(status, reply, {})) as request:
                    self.assertFalse(dataset.probe_chat_candidate(provider, "candidate", 1, 1, attempts=2, delay=0))
                self.assertEqual([call.args[2]["max_tokens"] for call in request.call_args_list], [32, 32])
                self.assertFalse(dataset.report["checks"])

    def test_lifecycle_probe_uses_real_http_and_continues_after_truncation_limit(self):
        """目的：真实HTTP探测扩容后仍无正文必须继续候选；前置本地供应商端点，验证curl载荷、32→1024预算上限和最终选中可用模型；服务、线程及临时目录均自动清理。"""
        requests = []

        class Supplier(BaseHTTPRequestHandler):
            """为探测提供本地协议边界；请求记录留在内存，服务由测试finally关闭。"""

            def do_POST(self):
                """用途：按候选返回截断推理或最终回答；无参数或返回值，供HTTP服务调用，读取请求并写JSON，正文和凭据不落盘。"""
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                requests.append((self.path, body))
                truncated = body["model"] == "truncated-model"
                reply = {"choices": [{"message": {"content": None if truncated else "OK",
                                                     "reasoning_content": "thinking" if truncated else None},
                                       "finish_reason": "length" if truncated else "stop"}],
                         "usage": {"prompt_tokens": 19, "completion_tokens": body["max_tokens"] if truncated else 1}}
                raw = json.dumps(reply).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)

            def log_message(self, format, *args):
                """用途：关闭服务默认日志以保持测试报告干净；参数为格式及值，无返回，无副作用。"""
                return

        server = ThreadingHTTPServer(("127.0.0.1", 0), Supplier)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            with tempfile.TemporaryDirectory() as directory:
                dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lambda line: None)
                provider = dataset.data["providers"][1]
                with mock.patch.dict("os.environ", {provider["key_env"]: "local-only",
                                                     provider["base_env"]: f"http://127.0.0.1:{server.server_port}/v1"}):
                    selected = dataset.select_lifecycle_model(provider, ["truncated-model", "working-model"], delay=0)
                self.assertEqual(selected, "working-model")
                self.assertEqual([body["max_tokens"] for path, body in requests], [32, 1024, 32])
                self.assertTrue(all(path == "/v1/chat/completions" for path, body in requests))
                self.assertEqual([row["passed"] for row in dataset.report["probe_attempts"]], [False, False, True])
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)

    def test_lifecycle_model_selection_fails_when_all_candidates_fail(self):
        """目的：模型生命周期不得把全部供应商失败误报为通过；前置两个候选均失败，验证明确异常和每个候选均已探测，模拟调用无需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            provider = dataset.data["providers"][1]
            with mock.patch.object(dataset, "probe_chat_candidate", return_value=False) as probe:
                with self.assertRaisesRegex(RuntimeError, "2 个额外真实聊天模型"):
                    dataset.select_lifecycle_model(provider, ["first-real", "second-real"], attempts=2, delay=0)
            self.assertEqual(probe.call_count, 2)

    def test_fallback_acceptance_route_pins_zero_weight_and_real_fallback(self):
        """目的：零权重验收不能被现有组织或团队模板覆盖；前置主模型故障/健康部署及真实备用模型，验证临时密钥模板显式写入100:0、备用部署和回退链，纯内存结果无需清理。"""
        body = fallback_acceptance_route("gpt-5.6-sol", "fault", "healthy", "z-ai/glm-5", "qiniu", 300)
        self.assertEqual(body["model_routes"][0]["allocations"], [
            {"deployment_id": "fault", "weight": 100},
            {"deployment_id": "healthy", "weight": 0},
        ])
        self.assertEqual(body["model_routes"][1]["allocations"], [
            {"deployment_id": "qiniu", "weight": 100},
        ])
        self.assertEqual(body["fallbacks"], [{"gpt-5.6-sol": ["z-ai/glm-5"]}])
        self.assertEqual(body["retry_policy"]["max_attempts"], 1)

    def test_fallback_acceptance_deployments_excludes_all_shared_model_suppliers(self):
        """目的：同一公开模型跨供应商时全部健康部署必须置零；前置七牛部署排在 Fenno 之前及临时部署，验证选择独立 GLM 目标并排除临时记录，纯内存无需清理。"""
        deployments = [
            {"id": "qiniu-shared", "public_name": "gpt-5.6-sol", "provider": "qiniu"},
            {"id": "fenno-shared", "public_name": "gpt-5.6-sol", "provider": "fennoai"},
            {"id": "other-glm", "public_name": "z-ai/glm-5", "provider": "fennoai"},
            {"id": "qiniu-glm", "public_name": "z-ai/glm-5", "provider": "qiniu"},
            {"id": "old-fault", "public_name": "gpt-5.6-sol", "provider": "fennoai", "temporary_fault": True},
            {"id": "temp-glm", "public_name": "z-ai/glm-5", "provider": "qiniu", "temporary": True},
        ]
        healthy, target = fallback_acceptance_deployments(deployments, "gpt-5.6-sol", "z-ai/glm-5", "qiniu")
        self.assertEqual(healthy, ["qiniu-shared", "fenno-shared"])
        self.assertEqual(target, "qiniu-glm")
        body = fallback_acceptance_route("gpt-5.6-sol", "fault", healthy, "z-ai/glm-5", target, 300)
        self.assertEqual(body["model_routes"][0]["allocations"], [
            {"deployment_id": "fault", "weight": 100},
            {"deployment_id": "qiniu-shared", "weight": 0},
            {"deployment_id": "fenno-shared", "weight": 0},
        ])

    def test_fallback_acceptance_deployments_rejects_invalid_baseline(self):
        """目的：无效基线须在创建临时数据前明确失败；前置同名主备、缺失主备或重复备用，验证抛 ValueError，纯内存无需清理。"""
        primary = {"id": "healthy", "public_name": "primary", "provider": "fennoai"}
        backup = {"id": "backup", "public_name": "backup", "provider": "qiniu"}
        for rows, source, target in (([primary], "primary", "primary"),
                                     ([primary], "primary", "backup"),
                                     ([backup], "primary", "backup"),
                                     ([primary, backup, {**backup, "id": "duplicate"}], "primary", "backup")):
            with self.subTest(rows=rows, source=source, target=target), self.assertRaises(ValueError):
                fallback_acceptance_deployments(rows, source, target, "qiniu")
        with self.assertRaisesRegex(ValueError, "不同的公开模型"):
            fallback_acceptance_route("shared", "fault", ["healthy"], "shared", "backup", 300)

    def test_fallback_policy_chain_uses_manifest_glm_and_cleans_up_on_failure(self):
        """目的：验收入口不能重用两家共用的 chat_models，且异常必须清理临时数据；前置真实清单及两家同名基线，在模板失败与聊天失败处断言 GLM 目标、全部零权重与删除调用，临时目录自动清理。"""
        for fail_at in ("template", "chat"):
            with self.subTest(fail_at=fail_at), tempfile.TemporaryDirectory() as directory:
                dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
                dataset.state["chat_models"] = {"fennoai": "gpt-5.6-sol", "qiniu": "gpt-5.6-sol"}
                dataset.state["selected"] = {"fennoai": {"provider": {"id": "fennoai"}}}
                baseline = [
                    {"id": "qiniu", "public_name": "gpt-5.6-sol", "provider": "qiniu"},
                    {"id": "fenno", "public_name": "gpt-5.6-sol", "provider": "fennoai"},
                    {"id": "glm", "public_name": "z-ai/glm-5", "provider": "qiniu"},
                ]
                dataset.state["deployments"] = baseline + [
                    {"id": "fault", "public_name": "gpt-5.6-sol", "provider": "fennoai", "temporary": True}]
                dataset.observer = mock.MagicMock(rows=[])

                def api(route, body):
                    """模拟管理接口并核对完整路由参数；参数为路径和正文，返回创建结果或抛故障，所有状态仅在当前临时实例内。"""
                    if route == "/route_template/new":
                        self.assertEqual(body["body"]["fallbacks"], [{"gpt-5.6-sol": ["z-ai/glm-5"]}])
                        self.assertEqual(body["body"]["model_routes"][0]["allocations"], [
                            {"deployment_id": "fault", "weight": 100},
                            {"deployment_id": "qiniu", "weight": 0},
                            {"deployment_id": "fenno", "weight": 0}])
                        if fail_at == "template":
                            raise AssertionError("injected failure")
                        return {"id": "route"}, {}
                    return {"key": "temporary-" + body.get("key_alias", "")}, {}

                with mock.patch.object(dataset, "deployment", return_value="fault"), \
                     mock.patch.object(dataset, "api", side_effect=api) as requests, \
                     mock.patch.object(dataset, "chat_request", side_effect=AssertionError("injected failure")):
                    with self.assertRaisesRegex(AssertionError, "injected failure"):
                        dataset.verify_fallback_policy_chain({"user_id": "user", "team_id": "team", "project_id": "project"})
                self.assertEqual(dataset.state["deployments"], baseline)
                requests.assert_any_call("/model/delete", {"id": "fault"})
                if fail_at == "chat":
                    requests.assert_any_call("/route_template/route/delete", {})
                    self.assertEqual(sum(call.args[0] == "/key/delete" for call in requests.call_args_list), 2)

    def test_profile_route_isolation_calls_both_real_models_and_rejects_cross_provider(self):
        """目的：真实模型 ID 分离后仍主动验证两种单供应商路由；前置两个配置密钥和模拟观察记录，验证两次真实调用目标及跨供应商记录会失败，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            dataset.state["keys"] = [
                {"profile": "fenno-only", "call_model": "gpt-5.6-sol"},
                {"profile": "qiniu-only", "call_model": "z-ai/glm-5"},
            ]
            dataset.observer = mock.MagicMock(rows=[])

            def observed_call(key, marker, model=None):
                """模拟当前调用进入与配置匹配的供应商；参数为密钥、标记和模型，无返回；仅写入内存观察记录。"""
                provider = {"fenno-only": "fennoai", "qiniu-only": "qiniu"}[key["profile"]]
                dataset.observer.rows.append({"provider": provider, "messages": marker, "status": 200})

            with mock.patch.object(dataset, "call", side_effect=observed_call) as call:
                dataset.verify_profile_route_isolation()
            self.assertEqual([row.kwargs["model"] for row in call.call_args_list], ["gpt-5.6-sol", "z-ai/glm-5"])
            self.assertTrue(dataset.checked("profile-routes-real-models-and-vendors"))

            dataset.report["checks"] = []
            dataset.observer.rows = []

            def crossed_call(key, marker, model=None):
                """模拟错误跨供应商转发；参数为密钥、标记和模型，无返回；用于证明隔离断言会拒绝错误路径。"""
                dataset.observer.rows.append({"provider": "qiniu", "messages": marker, "status": 200})

            with mock.patch.object(dataset, "call", side_effect=crossed_call):
                with self.assertRaisesRegex(AssertionError, "fenno-only"):
                    dataset.verify_profile_route_isolation()

    def test_curl_json_parses_status_and_rejects_transport_failure(self):
        """目的：供应商直连使用兼容长连接的 curl 且正确分离 JSON 与状态；前置模拟成功及网络失败进程，验证解析和异常，不执行真实命令且无需清理。"""
        success = mock.MagicMock()
        success.__enter__.return_value = success
        success.__exit__.return_value = False
        success.communicate.return_value = (b'{"choices":[{"message":{"content":"OK"}}]}\n200', b"")
        success.returncode = 0
        with mock.patch("real_dataset.subprocess.Popen", return_value=success) as popen:
            status, value, headers = curl_json("https://example.invalid", "/v1/chat/completions", {"model": "gpt-real"}, "secret")
        self.assertEqual((status, headers), (200, {}))
        self.assertTrue(value["choices"])
        self.assertEqual(popen.call_count, 1)

        failed = mock.MagicMock()
        failed.__enter__.return_value = failed
        failed.__exit__.return_value = False
        failed.communicate.return_value = (b"", b"remote closed")
        failed.returncode = 52
        with mock.patch("real_dataset.subprocess.Popen", return_value=failed):
            with self.assertRaisesRegex(ConnectionError, "退出码=52") as raised:
                curl_json("https://example.invalid", "/v1/chat/completions", {}, "secret")
        self.assertNotIn("secret", str(raised.exception))

    def test_forward_supplier_chat_uses_curl_and_normalizes_v1_route(self):
        """目的：81 密钥观察器使用与构建探测相同的 curl 网络路径；前置带/v1及不带/v1的供应商地址，验证路由、令牌和超时，模拟调用不访问外网且无需清理。"""
        provider = {"base_env": "TEST_ACCEPTANCE_BASE", "base": "https://fallback.invalid",
                    "key_env": "TEST_ACCEPTANCE_KEY"}
        reply = (200, {"choices": [{}], "usage": {"prompt_tokens": 1, "completion_tokens": 1}}, {})
        with mock.patch.dict("os.environ", {"TEST_ACCEPTANCE_BASE": "https://provider.invalid/v1",
                                                "TEST_ACCEPTANCE_KEY": "secret"}), \
                mock.patch("real_dataset.curl_json", return_value=reply) as request:
            self.assertEqual(forward_supplier_chat(provider, {"model": "gpt-real"}), reply)
        request.assert_called_once_with("https://provider.invalid/v1", "/chat/completions",
                                        {"model": "gpt-real"}, "secret", timeout=330)

        with mock.patch.dict("os.environ", {"TEST_ACCEPTANCE_BASE": "https://provider.invalid",
                                                "TEST_ACCEPTANCE_KEY": "secret"}), \
                mock.patch("real_dataset.curl_json", return_value=reply) as request:
            forward_supplier_chat(provider, {"model": "gpt-real"})
        self.assertEqual(request.call_args.args[1], "/v1/chat/completions")

    def test_forward_supplier_chat_preserves_transport_failure(self):
        """目的：真实供应商网络失败不能伪造成成功响应；前置 curl 连接错误，验证异常原样上抛供观察器记录 502，不访问外网且无需清理。"""
        provider = {"base_env": "TEST_ACCEPTANCE_BASE", "base": "https://provider.invalid",
                    "key_env": "TEST_ACCEPTANCE_KEY"}
        with mock.patch.dict("os.environ", {"TEST_ACCEPTANCE_KEY": "secret"}), \
                mock.patch("real_dataset.curl_json", side_effect=ConnectionError("remote closed")):
            with self.assertRaisesRegex(ConnectionError, "remote closed"):
                forward_supplier_chat(provider, {"model": "gpt-real"})

    def test_observer_records_route_tags_and_only_fault_tag_injects_429(self):
        """目的：观察器必须区分同一真实模型的权重部署且只为 fault 标签注入故障；前置模拟真实供应商 200，验证 weight-a 被转发、fault 不转发并分别保存 route_tag，finally 关闭本地服务。"""
        providers = [{"id": "fennoai", "base": "https://example.invalid",
                      "base_env": "TEST_BASE", "key_env": "TEST_KEY"}]
        reply = (200, {"choices": [{"message": {"content": "OK"}}],
                       "usage": {"prompt_tokens": 3, "completion_tokens": 1}}, {})
        with mock.patch("real_dataset.forward_supplier_chat", return_value=reply) as forward:
            observer = Observer(providers, 10)
            try:
                body = {"model": "gpt-5.6-sol", "messages": [{"role": "user", "content": "tagged"}]}
                self.assertEqual(http(observer.base, "/fennoai/weight-a", body)[0], 200)
                self.assertEqual(http(observer.base, "/fennoai/fault", body)[0], 429)
            finally:
                observer.close()
        self.assertEqual(forward.call_count, 1)
        self.assertEqual([row["route_tag"] for row in observer.rows], ["weight-a", "fault"])
        self.assertEqual([row["forwarded"] for row in observer.rows], [True, False])

    def test_observer_normalizes_untagged_openai_protocol_path(self):
        """目的：无观察标签的真实部署必须记录为 default；前置模拟真实供应商成功并请求 /provider/v1/chat/completions，验证协议版本不会被误认成部署标签，finally 关闭本地服务。"""
        providers = [{"id": "qiniu", "base": "https://example.invalid",
                      "base_env": "TEST_BASE", "key_env": "TEST_KEY"}]
        reply = (200, {"choices": [{"message": {"content": "OK"}}],
                       "usage": {"prompt_tokens": 3, "completion_tokens": 1}}, {})
        with mock.patch("real_dataset.forward_supplier_chat", return_value=reply):
            observer = Observer(providers, 10)
            try:
                body = {"model": "z-ai/glm-5", "messages": [{"role": "user", "content": "fallback"}]}
                self.assertEqual(http(observer.base, "/qiniu/v1/chat/completions", body)[0], 200)
            finally:
                observer.close()
        self.assertEqual(observer.rows[0]["route_tag"], "default")
        self.assertTrue(observer.rows[0]["forwarded"])

    def test_deployment_route_tag_builds_distinct_observer_address(self):
        """目的：同模型临时部署必须生成可观察的独立地址；前置模拟观察器和模型创建接口，验证 weight-b 写入 api_base、状态保存标签及临时资源标记，不创建真实部署。"""
        with tempfile.TemporaryDirectory() as directory:
            observer = type("ObserverStub", (), {"base": "http://127.0.0.1:4567"})()
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), observer=observer)
            definition = next(row for row in dataset.data["models"] if row["id"] == "fenno-chat")
            provider = dataset.data["providers"][0]
            captured = []

            def api(route, body):
                """记录部署请求并返回固定 ID；参数为路由和正文，返回模拟管理响应；仅用于内存断言。"""
                captured.append((route, body))
                return {"model_info": {"id": "deployment-tagged"}}, {}

            dataset.api = api
            ident = dataset.deployment(definition, provider, "gpt-5.6-sol", route_tag="weight-b")
            self.assertEqual(ident, "deployment-tagged")
            self.assertEqual(captured[0][1]["litellm_params"]["api_base"],
                             "http://127.0.0.1:4567/fennoai/weight-b")
            self.assertEqual(dataset.state["deployments"][0]["route_tag"], "weight-b")
            self.assertTrue(dataset.state["deployments"][0]["temporary"])

    def test_real_chat_call_retries_temporary_upstream_and_requires_nonzero_usage(self):
        """目的：81密钥真实调用可从明确的临时502恢复且只把非零usage计为成功；前置一次502和一次真实结构200，验证重试日志与回执，模拟接口无需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lines.append)
            key = {"key": "secret", "profile": "inherit", "call_model": "gpt-real"}
            responses = [
                ({"error": {"message": "upstream closed"}}, {}, 502),
                ({"choices": [{"message": {"content": "OK"}}],
                  "usage": {"prompt_tokens": 8, "completion_tokens": 1}},
                 {"x-litellm-call-id": "call-real"}, 200),
            ]
            with mock.patch.object(dataset, "api", side_effect=responses) as api, \
                    mock.patch("real_dataset.time.sleep") as sleep:
                receipt = dataset.call(key, "retry-marker", defer_bill=True, retry_delay=1)
            self.assertEqual(receipt[2], "call-real")
            self.assertEqual(receipt[3], {"prompt_tokens": 8, "completion_tokens": 1})
            self.assertEqual(api.call_count, 2)
            sleep.assert_called_once_with(1)
            output = "\n".join(lines)
            self.assertIn("❌ [gpt-real / bypass_openai_chat / 第 1/5 次真实 AI 请求]", output)
            self.assertIn("✅ [gpt-real / bypass_openai_chat / 真实 AI 响应]", output)

    def test_real_chat_call_uses_long_cooldown_for_upstream_rate_limit(self):
        """目的：供应商429必须等待完整冷却窗口后再试；前置网关转译的上游429和随后成功响应，验证65秒退避及最终真实usage，模拟接口无需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lines.append)
            key = {"key": "secret", "profile": "inherit", "call_model": "gpt-real"}
            responses = [
                ({"error": {"message": "upstream 429"}}, {}, 502),
                ({"choices": [{"message": {"content": "OK"}}],
                  "usage": {"prompt_tokens": 8, "completion_tokens": 1}},
                 {"x-litellm-call-id": "call-rate-recovered"}, 200),
            ]
            with mock.patch.object(dataset, "api", side_effect=responses), \
                    mock.patch("real_dataset.time.sleep") as sleep:
                receipt = dataset.call(key, "rate-marker", defer_bill=True, rate_limit_delay=65)
            self.assertEqual(receipt[2], "call-rate-recovered")
            sleep.assert_called_once_with(65)
            self.assertIn("65 秒后进行第 2/5 次", "\n".join(lines))

    def test_real_chat_call_rejects_zero_usage_after_success_status(self):
        """目的：HTTP成功但没有真实输出用量时不能打勾；前置带choices但completion为零的响应，验证立即失败且不写成功回执，模拟接口无需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            key = {"key": "secret", "profile": "inherit", "call_model": "gpt-real"}
            response = ({"choices": [{"message": {"content": ""}}],
                         "usage": {"prompt_tokens": 8, "completion_tokens": 0}},
                        {"x-litellm-call-id": "call-empty"}, 200)
            with mock.patch.object(dataset, "api", return_value=response):
                with self.assertRaisesRegex(AssertionError, "非零输入和输出 usage"):
                    dataset.call(key, "zero-marker", defer_bill=True)

    def test_chat_checkpoint_revalidates_complete_real_evidence(self):
        """目的：断点续跑只跳过完整的真实聊天证据；前置匹配模型、身份、请求标记、响应usage和价格快照，验证日志接口复验后返回原回执，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            key = {"token_id": "key-1", "profile": "inherit", "call_model": "gpt-5.6-sol",
                   "user_id": "user-1", "team_id": "team-1", "project_id": "project-1",
                   "organization_id": "org-1"}
            marker = "dataset-a1b2-3"
            row = {"call_id": "call-1", "marker": marker, "key_id": "key-1",
                   "profile": "inherit", "model": "gpt-5.6-sol",
                   "prompt_tokens": 11, "completion_tokens": 7, "cost": 0.000025}
            bill = {"api_key": "key-1", "user": "user-1", "team_id": "team-1",
                    "project_id": "project-1", "organization_id": "org-1",
                    "model": "gpt-5.6-sol", "prompt_tokens": 11, "completion_tokens": 7,
                    "spend": 0.000025, "metadata": {"cost_breakdown": {"source": "snapshot"}},
                    "messages": [{"role": "user", "content": marker}],
                    "response": {"choices": [{"message": {"content": "OK"}}],
                                 "usage": {"prompt_tokens": 11, "completion_tokens": 7}}}
            dataset.report["calls"] = [row]
            with mock.patch.object(dataset, "api", return_value=(bill, {}, 200)) as api:
                self.assertEqual(dataset.call_checkpoint(key), row)
            api.assert_called_once_with("/spend/logs/ui/call-1", expected=(200, 404), include_status=True)

    def test_chat_checkpoint_rejects_incomplete_or_mismatched_evidence(self):
        """目的：模型、身份、usage、价格或真实响应任一不符都必须补跑；前置逐项破坏已保存证据，验证检查点返回空且不会误判通过，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            key = {"token_id": "key-1", "profile": "inherit", "call_model": "gpt-5.6-sol",
                   "user_id": "user-1", "team_id": "team-1", "project_id": "project-1",
                   "organization_id": "org-1"}
            marker = "dataset-a1b2-3"
            base_row = {"call_id": "call-1", "marker": marker, "key_id": "key-1",
                        "profile": "inherit", "model": "gpt-5.6-sol",
                        "prompt_tokens": 11, "completion_tokens": 7, "cost": 0.000025}
            base_bill = {"api_key": "key-1", "user": "user-1", "team_id": "team-1",
                         "project_id": "project-1", "organization_id": "org-1",
                         "model": "gpt-5.6-sol", "prompt_tokens": 11, "completion_tokens": 7,
                         "spend": 0.000025, "metadata": {"cost_breakdown": {"source": "snapshot"}},
                         "messages": [{"content": marker}],
                         "response": {"choices": [{}],
                                      "usage": {"prompt_tokens": 11, "completion_tokens": 7}}}
            cases = {
                "模型": ({**base_row, "model": "wrong-model"}, base_bill),
                "身份": (base_row, {**base_bill, "team_id": "wrong-team"}),
                "usage": (base_row, {**base_bill, "response": {"choices": [{}],
                           "usage": {"prompt_tokens": 11, "completion_tokens": 6}}}),
                "价格": (base_row, {**base_bill, "metadata": {}}),
                "响应": (base_row, {**base_bill, "response": {"choices": [],
                           "usage": {"prompt_tokens": 11, "completion_tokens": 7}}}),
            }
            for name, (row, bill) in cases.items():
                with self.subTest(name=name):
                    dataset.report["calls"] = [row]
                    with mock.patch.object(dataset, "api", return_value=(bill, {}, 200)):
                        self.assertIsNone(dataset.call_checkpoint(key))

    def test_record_call_replaces_same_key_and_persists_immediately(self):
        """目的：长批次中每把密钥成功后立即形成唯一检查点；前置同一密钥两次成功账单，验证第二次替换第一次且报告立刻落盘，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            key = {"token_id": "key-1", "profile": "inherit"}
            dataset.record_call(key, "dataset-a1-1", "call-1", "gpt-5.6-sol",
                                {"prompt_tokens": 8, "completion_tokens": 1}, 0.000010)
            dataset.record_call(key, "dataset-a1-2", "call-2", "gpt-5.6-sol",
                                {"prompt_tokens": 9, "completion_tokens": 2}, 0.000013)
            saved = json.loads((Path(directory) / "report.json").read_text())
            self.assertEqual(len(saved["calls"]), 1)
            self.assertEqual(saved["calls"][0]["call_id"], "call-2")
            self.assertEqual(saved["calls"][0]["model"], "gpt-5.6-sol")

    def test_completed_checkpoint_survives_a_later_call_failure(self):
        """目的：后续供应商失败不能丢失此前已核账的密钥；前置先保存成功回执再模拟下一次调用失败，验证磁盘报告仍保留首条证据，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            first = {"token_id": "key-1", "profile": "inherit"}
            dataset.record_call(first, "dataset-a1-1", "call-1", "gpt-5.6-sol",
                                {"prompt_tokens": 8, "completion_tokens": 1}, 0.000010)
            failing = {"key": "secret", "token_id": "key-2", "profile": "inherit",
                       "call_model": "z-ai/glm-5"}
            with mock.patch.object(dataset, "api", side_effect=AssertionError("upstream failed")):
                with self.assertRaisesRegex(AssertionError, "upstream failed"):
                    dataset.call(failing, "dataset-a1-2")
            saved = json.loads((Path(directory) / "report.json").read_text())
            self.assertEqual([row["call_id"] for row in saved["calls"]], ["call-1"])

    def test_recover_shared_checkpoints_selects_latest_valid_row_per_key(self):
        """目的：共享数据库恢复为每把当前密钥选择最新合法真实记录；前置一条较新坏响应和一条较旧完整响应，验证跳过坏记录并恢复完整证据，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            key = {"token_id": "key-1", "profile": "inherit", "call_model": "gpt-5.6-sol",
                   "user_id": "user-1", "team_id": "team-1", "project_id": "project-1",
                   "organization_id": "org-1"}
            dataset.state["keys"] = [key]
            base = {"api_key": "key-1", "user": "user-1", "team_id": "team-1",
                    "project_id": "project-1", "organization_id": "org-1",
                    "model": "gpt-5.6-sol", "prompt_tokens": 11, "completion_tokens": 7,
                    "spend": 0.000025, "metadata": {"cost_breakdown": {"source": "snapshot"}},
                    "messages": [{"content": "dataset-a1b2-3"}]}
            newest_bad = {**base, "request_id": "call-bad",
                          "response": {"choices": [], "usage": {"prompt_tokens": 11, "completion_tokens": 7}}}
            older_good = {**base, "request_id": "call-good",
                           "response": {"choices": [{}], "usage": {"prompt_tokens": 11, "completion_tokens": 7}}}
            output = "\n".join(json.dumps(row) for row in (newest_bad, older_good))
            with mock.patch.object(RUNNER, "sql", return_value=output):
                self.assertEqual(RUNNER.recover_shared_chat_checkpoints(dataset), 1)
            self.assertEqual(dataset.report["calls"][0]["call_id"], "call-good")

    def test_recover_shared_checkpoints_ignores_incomplete_rows(self):
        """目的：共享恢复不能把缺少价格、归属或响应usage的历史行当作通过；前置不完整数据库行，验证恢复数量为零且报告为空，临时目录自动清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            dataset.state["keys"] = [{"token_id": "key-1", "profile": "inherit",
                "call_model": "gpt-5.6-sol", "user_id": "user-1", "team_id": "team-1",
                "project_id": "project-1", "organization_id": "org-1"}]
            row = {"request_id": "call-bad", "api_key": "key-1", "user": "user-1",
                   "team_id": "team-1", "project_id": "project-1", "organization_id": "org-1",
                   "model": "gpt-5.6-sol", "prompt_tokens": 11, "completion_tokens": 7,
                   "spend": 0.000025, "metadata": {}, "messages": [{"content": "dataset-a1b2-3"}],
                   "response": {"choices": [{}], "usage": {"prompt_tokens": 11, "completion_tokens": 7}}}
            with mock.patch.object(RUNNER, "sql", return_value=json.dumps(row)):
                self.assertEqual(RUNNER.recover_shared_chat_checkpoints(dataset), 0)
            self.assertEqual(dataset.report["calls"], [])

    def test_fal_video_poll_accepts_202_until_completed(self):
        """目的：FAL 队列等待态的HTTP 202不会误判失败；前置一次202等待和一次200完成，验证轮询继续并读取最终结果，模拟调用不产生外部任务。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lines.append)
            definition = next(row for row in dataset.data["models"] if row["id"] == "qiniu-fal-seedance")
            responses = [
                ({"status": "IN_PROGRESS"}, {}, 202),
                ({"status": "COMPLETED"}, {}, 200),
                ({"result": {"video": {"url": "https://example.invalid/video.mp4"}}}, {}, 200),
            ]
            with mock.patch.object(dataset, "api", side_effect=responses) as api, mock.patch("real_dataset.time.sleep"):
                result = dataset.poll_video({"key": "secret"}, definition, "task-1",
                                            "/task-1/status", "/task-1", "COMPLETED", {"failed"})
            self.assertEqual(result["video"]["url"], "https://example.invalid/video.mp4")
            self.assertEqual(api.call_count, 3)
            self.assertIn("HTTP=202，状态=IN_PROGRESS", "\n".join(lines))

    def test_video_result_query_retries_idempotent_temporary_failure(self):
        """目的：视频终态GET遇到临时502后仍可完成去重核对；前置一次502后200，验证重试、等待和成功结果，模拟调用不产生外部任务。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            dataset = Dataset("http://127.0.0.1", directory, load_manifest(), logger=lines.append)
            definition = next(row for row in dataset.data["models"] if row["id"] == "qiniu-fal-seedance")
            responses = [({"error": {"message": "upstream timeout"}}, {}, 502),
                         ({"video": {"url": "https://example.invalid/video.mp4"}}, {}, 200)]
            with mock.patch.object(dataset, "api", side_effect=responses) as api, mock.patch("real_dataset.time.sleep") as sleep:
                result = dataset.video_result_query({"key": "secret"}, definition, "/result", "终态查询", delay=1)
            self.assertEqual(result["video"]["url"], "https://example.invalid/video.mp4")
            self.assertEqual(api.call_count, 2)
            sleep.assert_called_once_with(1)
            self.assertIn("❌ [bytedance/seedance-2.0/mini/text-to-video / qiniu_fal_doubao_20 / 终态查询第 1/3 次]", "\n".join(lines))

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

    def test_restore_supplier_addresses_uses_deployment_provider_ids(self):
        """目的：退出清理恢复仍存在的聊天部署并覆盖旧诊断；前置模拟部署和接口，验证按供应商还原地址、成功日志及未知供应商失败记录，不访问真实服务或数据库。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            dataset.admin = "admin-token"
            dataset.state["deployments"] = [
                {"id": "deployment-a", "provider": "fennoai", "kind": "chat", "observed": True},
                {"id": "deployment-b", "provider": "qiniu", "kind": "chat", "observed": True},
                {"id": "deployment-media", "provider": "qiniu", "kind": "video", "observed": False},
                {"id": "deployment-unknown", "provider": "missing", "kind": "chat", "observed": True},
            ]
            calls = []
            lines = []
            dataset.logger = lines.append
            dataset.report["retention_error"] = "上一轮遗留错误"
            dataset.api = lambda route, body: calls.append((route, body))
            RUNNER.restore_supplier_addresses(dataset, load_manifest())
            self.assertEqual([body["model_info"]["id"] for _, body in calls], ["deployment-a", "deployment-b"])
            self.assertEqual(calls[0][1]["litellm_params"]["api_base"], "https://api.fenno.ai")
            self.assertEqual(calls[1][1]["litellm_params"]["api_base"], "https://api.modelink.ai")
            self.assertIn("未知供应商", dataset.report["retention_error"])
            self.assertNotIn("上一轮遗留错误", dataset.report["retention_error"])
            self.assertEqual(sum("✅" in line and "恢复供应商地址" in line for line in lines), 2)

    def test_restore_supplier_addresses_clears_stale_error_after_success(self):
        """目的：上一轮恢复错误不得污染本轮成功报告；前置一个有效聊天部署和旧 retention_error，验证恢复成功后字段被删除并输出打勾日志，不访问真实服务或数据库。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lines.append)
            dataset.admin = "admin-token"
            dataset.report["retention_error"] = "旧错误"
            dataset.state["deployments"] = [
                {"id": "deployment-a", "public_name": "gpt-5.6-sol", "provider": "fennoai",
                 "kind": "chat", "observed": True},
            ]
            dataset.api = lambda route, body: ({}, {})
            RUNNER.restore_supplier_addresses(dataset, load_manifest())
            self.assertNotIn("retention_error", dataset.report)
            self.assertIn("✅ [gpt-5.6-sol / bypass_openai_chat / 恢复供应商地址]", "\n".join(lines))

    def test_restore_supplier_addresses_ignores_deleted_temporary_deployment(self):
        """目的：异常退出时已删除的生命周期部署不得造成恢复误报；前置一个带 temporary 标记且更新返回404的聊天部署，验证报告无 retention_error 并记录无需恢复，不访问真实服务或数据库。"""
        with tempfile.TemporaryDirectory() as directory:
            lines = []
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest(), logger=lines.append)
            dataset.admin = "admin-token"
            dataset.state["deployments"] = [
                {"id": "deployment-deleted", "public_name": "qwen/temporary-real-model",
                 "provider": "qiniu", "kind": "chat", "observed": True, "temporary": True},
            ]
            dataset.api = mock.Mock(side_effect=AssertionError("POST /model/update 状态 404，预期 200"))
            RUNNER.restore_supplier_addresses(dataset, load_manifest())
            self.assertNotIn("retention_error", dataset.report)
            self.assertIn("临时部署 deployment-deleted 已删除，无需恢复", "\n".join(lines))

    def test_attach_chat_observer_leaves_native_media_on_supplier_addresses(self):
        """目的：验收观察器只接管聊天请求，防止图片和视频被改发聊天端点；前置聊天、图片和视频部署，验证仅 observed 部署更新，无网络或持久化数据需清理。"""
        with tempfile.TemporaryDirectory() as directory:
            dataset = Dataset("http://127.0.0.1:1", directory, load_manifest())
            dataset.state["deployments"] = [
                {"id": "chat-a", "provider": "fennoai", "kind": "chat", "observed": True},
                {"id": "image-a", "provider": "fennoai", "kind": "image", "observed": False},
                {"id": "video-a", "provider": "qiniu", "kind": "video", "observed": False},
            ]
            calls = []
            dataset.api = lambda route, body: calls.append((route, body))
            observer = type("ObserverStub", (), {"base": "http://127.0.0.1:4567"})()
            RUNNER.attach_chat_observer(dataset, observer)
            self.assertEqual(len(calls), 1)
            self.assertEqual(calls[0][1]["model_info"]["id"], "chat-a")
            self.assertEqual(calls[0][1]["litellm_params"]["api_base"], "http://127.0.0.1:4567/fennoai")


if __name__ == "__main__":
    unittest.main()
