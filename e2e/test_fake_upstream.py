"""本地 Ark 假上游的协议单元测试，不访问外部服务。"""
import json
import threading
import unittest
from http.server import HTTPServer
from urllib.error import HTTPError
from urllib.request import Request, urlopen
from e2e.fake_upstream import Handler, ERROR_LOG_JSON_DIAGNOSTIC, ERROR_LOG_TEXT_DIAGNOSTIC


class ArkUpstreamTest(unittest.TestCase):
    def test_fallback_rate_limit_is_scoped_to_test_models(self):
        """目的：为浏览器回退流程提供可控429；前置本地HTTP服务，验证故障型号返回限流码、普通型号正常及相邻前缀不误伤，tearDown关闭服务清理。"""
        with self.assertRaises(HTTPError) as error:
            self.request("/v1/chat/completions", {"model": "e2e-fallback-429-case"})
        self.assertEqual(error.exception.code, 429)
        self.assertEqual(json.load(error.exception)["error"]["code"], "rate_limit_exceeded")
        for model in ("normal-model", "e2e-fallback-429"):
            response = self.request("/v1/chat/completions", {"model": model})
            self.assertEqual(response["model"], model)

    def test_continuation_diagnostics(self):
        """前置本地HTTP夹具，验证三轮缓存统计、多值头与坏历史400；tearDown关闭服务清理数据。"""
        messages = []
        for turn in range(1, 4):
            messages.append({"role": "user", "content": f"continuation-{turn}"})
            data = json.dumps({"model": "e2e-responses-history", "messages": messages}).encode()
            with urlopen(Request(self.base + "/v1/chat/completions", data=data), timeout=2) as response:
                self.assertEqual(response.headers.get_all("X-Upstream-Trace"), ["trace-first", "trace-second"])
                usage = json.load(response)["usage"]
                self.assertEqual(usage["prompt_tokens_details"]["cached_tokens"], 0 if turn == 1 else 4)
                self.assertEqual(usage["provider_statistics"]["turn"], turn)
            messages.append({"role": "assistant", "content": f"history-ok-{turn}"})
        with self.assertRaises(HTTPError) as error:
            self.request("/v1/chat/completions", {"model": "e2e-responses-history", "messages": []})
        self.assertEqual(error.exception.code, 400)

    def test_error_log_diagnostics(self):
        """前置本地 HTTP 上游；验证长 JSON、纯文本无截断，普通模型仍成功，断网可恢复；tearDown 关闭服务并清理任务。"""
        for kind in ("json", "text"):
            with self.subTest(kind=kind), self.assertRaises(HTTPError) as error:
                self.request("/v1/chat/completions", {"model": "e2e-error-log-" + kind + "-unit", "messages": []})
            with error.exception as response:
                self.assertEqual(response.code, 502)
                body = response.read().decode()
                if kind == "json":
                    details = json.loads(body)["error"]
                    self.assertEqual(details["message"], ERROR_LOG_JSON_DIAGNOSTIC)
                    self.assertEqual(details["details"]["terminal_cause"], "e2e-json-terminal-cause")
                else:
                    self.assertEqual(body, ERROR_LOG_TEXT_DIAGNOSTIC)
        from http.client import RemoteDisconnected
        with self.assertRaises(RemoteDisconnected):
            self.request("/v1/chat/completions", {"model": "e2e-error-log-network-unit", "messages": []})
        normal = self.request("/v1/chat/completions", {"model": "e2e-error-log-success-unit", "messages": []})
        self.assertEqual(normal["choices"][0]["message"]["content"], "e2e-ok")

    def setUp(self):
        """启动随机端口本地 HTTPServer；无参数/返回，供每个用例使用，tearDown 清理。"""
        Handler.ark_tasks.clear()
        self.server = HTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever)
        self.thread.start()
        self.base = f"http://127.0.0.1:{self.server.server_port}"

    def tearDown(self):
        """结束本用例服务器、线程及任务；无参数/返回，不留下测试数据。"""
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()
        Handler.ark_tasks.clear()

    def request(self, path, body=None, key="Bearer sk-fake"):
        """向本地路径发 JSON 请求；body 决定 POST/GET，key 为鉴权。

        返回解码对象，错误状态抛 HTTPError；供本类测试使用，响应上下文自动关闭。
        """
        data = None if body is None else json.dumps(body).encode()
        with urlopen(Request(self.base + path, data=data, headers={"Authorization": key}), timeout=2) as response:
            return json.load(response)

    def test_task_failure(self):
        """前置本地上游；失败夹具先运行后失败，验证错误终态；tearDown 清理服务器与任务。"""
        path = "/api/v3/contents/generations/tasks"
        created = self.request(path, {"model": "doubao-seedance-2-0-260128", "content": [{"type": "text", "text": "e2e-task-failure"}]})
        task_path = path + "/" + created["id"]
        self.assertEqual(self.request(task_path)["status"], "running")
        self.assertEqual(self.request(task_path)["status"], "failed")

    def test_codex_model_prefix_and_history(self):
        """目的：Codex 模型带协议前缀仍必须验证历史；前置本地 HTTP 上游，验证三轮独立ID、正确标记与坏历史400，tearDown清理服务。"""
        first = "Remember the project marker project-alpha. Reply with only that marker."
        prompts = [first, "What project marker did I give you? Reply with only the marker.",
                   "Confirm the same project marker once more. Reply with only the marker."]
        for model in ("e2e-codex-agent", "openai/e2e-codex-agent"):
            messages = [{"role": "system", "content": "You are Codex."}]
            ids = set()
            for prompt in prompts:
                messages.append({"role": "user", "content": prompt})
                answer = self.request("/v1/chat/completions", {"model": model, "messages": messages})
                self.assertEqual(answer["choices"][0]["message"]["content"], "project-alpha")
                self.assertNotIn(answer["id"], ids)
                ids.add(answer["id"])
                messages.append({"role": "assistant", "content": "project-alpha"})
            with self.assertRaises(HTTPError) as error:
                self.request("/v1/chat/completions", {"model": model, "messages": messages[-2:-1]})
            self.assertEqual(error.exception.code, 400)

    def test_task_lifecycle(self):
        """前置本地上游；标记任务先运行再完成，重复查询维持完成；tearDown 清理服务器和任务。"""
        path = "/api/v3/contents/generations/tasks"
        created = self.request(path, {"model": "doubao-seedance-2-0-260128", "content": [{"type": "text", "text": "e2e-task-lifecycle"}]})
        task_path = path + "/" + created["id"]
        self.assertEqual(self.request(task_path)["status"], "running")
        self.assertEqual(self.request(task_path)["status"], "succeeded")
        self.assertEqual(self.request(task_path)["status"], "succeeded")

    def test_create_and_poll(self):
        """验证合法创建、唯一任务 ID、完成结果及用量；使用本地服务，tearDown 清理。"""
        path = "/api/v3/contents/generations/tasks"
        first = self.request(path, {"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"apple"}]})
        second = self.request(path, {"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"apple"}]})
        self.assertNotEqual(first["id"], second["id"])
        result = "/api/v3/contents/generations/tasks/" + first["id"]
        for suffix in ("", ""):
            doc = self.request(result + suffix)
            self.assertEqual(doc["status"], "succeeded")
            self.assertEqual(doc["usage"]["completion_tokens"], 100)

    def test_invalid_auth_body_and_unknown_task(self):
        """验证错误鉴权、网关字段泄漏、空输入和未知任务分别拒绝；tearDown 清理服务。"""
        root = "/api/v3/contents/generations/tasks"
        for path, body, key, status in [
            (root, {"model":"m","content":[{}]}, "Key sk-fake", 401),
            (root, {"content":[{}]}, "Bearer sk-fake", 400),
            (root, {}, "Bearer sk-fake", 400),
            (root + "/missing", None, "Bearer sk-fake", 404),
        ]:
            with self.subTest(path=path, body=body, key=key):
                with self.assertRaises(HTTPError) as error:
                    self.request(path, body, key)
                self.assertEqual(error.exception.code, status)
                error.exception.close()

    def test_existing_catalog(self):
        """验证非 Ark 的目录路径仍返回完整旧目录；本地服务与内存任务由 tearDown 清理。"""
        self.assertEqual(len(self.request("/v1/models")["data"]), 155)
