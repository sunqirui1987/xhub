"""Codex 数据集请求、检查点和模型阶段的确定性单元测试，无外部凭据。"""
import copy
import json
from pathlib import Path
import tempfile
import threading
import unittest
from unittest import mock
from real_dataset import Dataset, load_manifest
from fake_upstream import codex_answer


class CodexAgentTests(unittest.TestCase):
    def setUp(self):
        """用途：创建隔离报告目录、身份及内存账单；无参数/返回，供测试调用，目录由 addCleanup 清理。"""
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.dataset = Dataset("http://127.0.0.1:1", temporary.name, load_manifest(), logger=lambda _: None)
        self.key = dict(token_id="key", key="sk-test", user_id="user", team_id="team",
                        project_id="project", organization_id="org", profile="qiniu-only", call_model="gpt-5.6-sol")
        self.dataset.state["keys"] = [self.key]
        self.bills = {}
        self.requests = []
        self.dataset.api = mock.Mock(side_effect=self.api)
        self.dataset.wait_bill = mock.Mock(side_effect=lambda call: self.bills[call])
        self.dataset.observer = type("Observer", (), {"rows": [], "lock": threading.Lock()})()

    def api(self, route, body=None, **kwargs):
        """用途：模拟三轮传输及独立账单读取；参数与 Dataset.api 一致，返回响应及头或状态三元组。

        供单元测试检查协议，构造完整上游历史并由本地校验器验证；只修改内存证据，无持久化副作用。
        """
        if body is None:
            call = route.rsplit("/", 1)[-1]
            return self.bills.get(call, {}), {}, 200 if call in self.bills else 404
        index = len(self.requests) + 1
        self.requests.append(copy.deepcopy((body, kwargs)))
        marker = self.requests[0][0]["input"][0]["content"].split("marker ")[1].split(".")[0]
        response = dict(id="resp-" + str(index), status="completed", output=[{
            "type": "message", "role": "assistant", "content": [{"type": "output_text", "text": marker}]}],
            usage={"input_tokens": 8, "output_tokens": 2})
        call = "call-" + str(index)
        self.bills[call] = dict(api_key="key", user="user", team_id="team", project_id="project",
            organization_id="org", model=self.key["call_model"], session_id=kwargs["request_headers"]["Session_id"],
            response=response, prompt_tokens=8, completion_tokens=2, spend=0.000012,
            metadata={"cost_breakdown": {"source": "snapshot"}},
            proxy_server_request={"body": copy.deepcopy(body), "headers": kwargs["request_headers"]})
        messages = [{"role": "system", "content": body["instructions"]}]
        for prior, _ in self.requests[:-1]:
            messages.extend(prior["input"] + [{"role": "assistant", "content": marker}])
        messages.extend(body["input"])
        stored = self.bills[call]["proxy_server_request"]["body"]
        stored.pop("previous_response_id", None)
        stored.pop("store", None)
        stored["input"] = messages[1:]
        self.assertEqual(codex_answer({"messages": messages}), marker)
        self.dataset.observer.rows.append(dict(status=200, forwarded=True, provider="qiniu",
            messages=json.dumps(messages), usage={"prompt_tokens": 8, "completion_tokens": 2}))
        return response, {"x-litellm-call-id": call}

    def test_three_turns_and_strict_resume(self):
        """目的：完整三轮只发本轮输入且复验不重复付费；前置隔离内存供应商，验证上下文、头、保存与404边界，目录自动清理。"""
        session = self.dataset.codex_conversation(self.key)
        self.assertEqual(len(session["turns"]), 3)
        for index, (body, options) in enumerate(self.requests):
            self.assertEqual(len(body["input"]), 1)
            self.assertEqual(body.get("previous_response_id"), None if index == 0 else "resp-" + str(index))
            self.assertEqual(options["request_headers"]["Session_id"], session["session_id"])
            if index:
                self.assertNotIn(session["marker"], body["input"][0]["content"])
        self.assertEqual(self.dataset.codex_conversation(self.key, True), session)
        self.assertEqual(len(self.requests), 3)
        self.bills.pop("call-2")
        self.assertIsNone(self.dataset.codex_conversation(self.key, True))

    def test_corrupt_evidence_and_partial_conversation_rejected(self):
        """目的：不能用单轮、错链、错误身份或虚假usage冒充完整会话；前置合法三轮，各失败分支应返回无检查点，目录自动清理。"""
        session = self.dataset.codex_conversation(self.key)
        pristine = copy.deepcopy(self.bills)
        for field, value in (("session_id", "foreign"), ("spend", 1), ("prompt_tokens", 0),
                             ("response", {}), ("proxy_server_request", {})):
            self.bills = copy.deepcopy(pristine)
            self.bills["call-2"][field] = value
            with self.subTest(field=field):
                self.assertIsNone(self.dataset.codex_conversation(self.key, True))
        self.bills = pristine
        for field, value in (("call_id", "call-1"), ("response_id", "resp-1"), ("previous_response_id", "wrong"), ("turn", 1)):
            bad = copy.deepcopy(session)
            bad["turns"][1][field] = value
            self.dataset.report["agent_conversations"] = [bad]
            self.assertIsNone(self.dataset.codex_conversation(self.key, True))
        self.dataset.report["agent_conversations"] = [{**session, "turns": session["turns"][:1]}]
        self.assertIsNone(self.dataset.codex_conversation(self.key, True))

    def test_supplier_failure_never_records_success(self):
        """目的：上游丢历史或身份路由错误不得写成功报告；前置合法计量但错供应商，验证明确失败，目录自动清理。"""
        original = self.dataset.observed_since
        def wrong_supplier(start, marker):
            """用途：返回错误供应商观察证据；参数为起点和标记，返回副本；只用于失败输入测试，无外部副作用。"""
            return [{**row, "provider": "fennoai"} for row in original(start, marker)]
        self.dataset.observed_since = wrong_supplier
        with self.assertRaisesRegex(AssertionError, "单供应商"):
            self.dataset.codex_conversation(self.key)
        self.assertFalse(self.dataset.report.get("agent_conversations"))

    def test_stage_order_and_fail_fast(self):
        """目的：GPT、GLM、图片视频严格串行且前阶段失败停止；前置已声明GLM部署，验证调用顺序和复验分支，无外部调用，目录自动清理。"""
        self.dataset.state["deployments"] = [{"public_name": "z-ai/glm-5"}]
        events = []
        self.dataset.verify_codex_agents = mock.Mock(side_effect=lambda: events.append("gpt"))
        self.dataset.codex_conversation = mock.Mock(side_effect=lambda key, **kwargs: events.append("glm") or {})
        self.dataset.verify_media_models = mock.Mock(side_effect=lambda key: events.append("media"))
        self.dataset.verify_acceptance_models()
        self.assertEqual(events, ["gpt", "glm", "media"])
        events.clear()
        self.dataset.verify_codex_agents.side_effect = AssertionError("GPT failed")
        with self.assertRaisesRegex(AssertionError, "GPT failed"):
            self.dataset.verify_acceptance_models()
        self.assertEqual(events, [])
        self.dataset.verify_codex_agents.side_effect = lambda: events.append("gpt")
        self.dataset.codex_conversation.side_effect = AssertionError("GLM failed")
        with self.assertRaisesRegex(AssertionError, "GLM failed"):
            self.dataset.verify_acceptance_models()
        self.assertEqual(events, ["gpt"])

    def test_upgrade_missing_glm_and_catalog_failure(self):
        """目的：旧基线只补GLM部署与白名单，目录缺模型时停止；前置隔离内存资源，验证顺序与错误且不调用媒体，目录自动清理。"""
        self.dataset.state["deployments"] = [{"public_name": "gpt-5.6-sol"}]
        self.dataset.state["teams"] = [{"id": "team"}]
        self.dataset.verify_codex_agents = mock.Mock()
        self.dataset.provider_catalog = mock.Mock(return_value={"model_ids": ["z-ai/glm-5"]})
        self.dataset.api = mock.Mock(return_value=({}, {}))
        self.dataset.codex_conversation = mock.Mock(side_effect=[None, {"turns": []}])
        self.dataset.verify_media_models = mock.Mock()

        def add_glm(definition, provider):
            """用途：模拟旧基线增加GLM；参数为定义和供应商，返回部署ID；仅修改内存状态，供升级分支测试。"""
            self.assertEqual(provider["id"], "qiniu")
            self.dataset.state["deployments"].append({"public_name": definition["public_name"]})
            return "glm-deployment"

        self.dataset.deployment = mock.Mock(side_effect=add_glm)
        self.dataset.verify_acceptance_models()
        self.assertEqual(self.dataset.api.call_args_list, [
            mock.call("/team/update", {"team_id": "team", "models": ["gpt-5.6-sol", "z-ai/glm-5"]}),
            mock.call("/key/update", {"key": "key", "models": ["gpt-5.6-sol", "z-ai/glm-5"]}),
        ])
        self.assertEqual(self.dataset.codex_conversation.call_count, 2)
        self.assertEqual(self.dataset.codex_conversation.call_args.args[0]["call_model"], "z-ai/glm-5")
        self.dataset.verify_media_models.assert_called_once()
        self.dataset.state["deployments"] = [{"public_name": "gpt-5.6-sol"}]
        self.dataset.provider_catalog.return_value = {"model_ids": []}
        self.dataset.api.reset_mock()
        self.dataset.verify_media_models.reset_mock()
        with self.assertRaisesRegex(AssertionError, "缺少指定 GLM"):
            self.dataset.verify_acceptance_models()
        self.dataset.api.assert_not_called()
        self.dataset.verify_media_models.assert_not_called()

    def test_manifest_invalid_agent_glm_and_order(self):
        """目的：启动前拒绝缺失Codex、错误GLM、顺序或预算；前置清单副本，验证明确ValueError，临时目录自动清理。"""
        for kind in ("agent", "glm", "order", "budget"):
            data = load_manifest()
            if kind == "agent": data["agent_conversation"]["turns"] = 1
            elif kind == "glm": next(row for row in data["models"] if row["id"] == "qiniu-glm")["upstream_model"] = "missing"
            elif kind == "order": data["verification_order"].reverse()
            else: data["limits"]["max_upstream_attempts"] = 100
            path = self.dataset.directory / "manifest.json"
            path.write_text(json.dumps(data))
            with self.subTest(kind=kind), self.assertRaises(ValueError): load_manifest(path)

    def test_empty_keys_and_bad_upstream_history(self):
        """目的：空样本不能通过验收，缺历史和乱序不得返回成功；前置空列表及异常协议，验证失败边界，无外部数据，目录自动清理。"""
        with self.assertRaises(ValueError): codex_answer({"messages": []})
        with self.assertRaises(ValueError): codex_answer({"messages": [{"role": "system"}, {"content": "bad"}]})
        self.dataset.state["keys"] = []
        with self.assertRaisesRegex(AssertionError, "空密钥"): self.dataset.verify_codex_agents()
