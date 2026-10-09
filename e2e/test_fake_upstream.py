"""本地 Fal 假上游的协议单元测试，不访问外部服务。"""
import json
import threading
import unittest
from http.server import HTTPServer
from urllib.error import HTTPError
from urllib.request import Request, urlopen
from e2e.fake_upstream import Handler


class FalUpstreamTest(unittest.TestCase):
    def setUp(self):
        """启动随机端口本地 HTTPServer；无参数/返回，供每个用例使用，tearDown 清理。"""
        Handler.fal_tasks.clear()
        self.server = HTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever)
        self.thread.start()
        self.base = f"http://127.0.0.1:{self.server.server_port}"

    def tearDown(self):
        """结束本用例服务器、线程及任务；无参数/返回，不留下测试数据。"""
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()
        Handler.fal_tasks.clear()

    def request(self, path, body=None, key="Key sk-fake"):
        """向本地路径发 JSON 请求；body 决定 POST/GET，key 为鉴权。

        返回解码对象，错误状态抛 HTTPError；供本类测试使用，响应上下文自动关闭。
        """
        data = None if body is None else json.dumps(body).encode()
        with urlopen(Request(self.base + path, data=data, headers={"Authorization": key}), timeout=2) as response:
            return json.load(response)

    def test_create_and_poll(self):
        """验证合法创建、唯一任务 ID、完成结果及用量；使用本地服务，tearDown 清理。"""
        path = "/queue/byteplus/seedance-2.0/text-to-video"
        first = self.request(path, {"prompt": "legacy-qiniu-e2e"})
        second = self.request(path, {"prompt": "legacy-qiniu-e2e"})
        self.assertNotEqual(first["request_id"], second["request_id"])
        result = "/queue/byteplus/seedance-2.0/requests/" + first["request_id"]
        for suffix in ("", "/status"):
            doc = self.request(result + suffix)
            self.assertEqual(doc["status"], "COMPLETED")
            self.assertEqual(doc["usage"]["completion_tokens"], 100)

    def test_invalid_auth_body_and_unknown_task(self):
        """验证错误鉴权、网关字段泄漏、空输入和未知任务分别拒绝；tearDown 清理服务。"""
        root = "/queue/byteplus/seedance-2.0"
        for path, body, key, status in [
            (root + "/text-to-video", {"prompt": "legacy-qiniu-e2e"}, "Bearer sk-fake", 401),
            (root + "/text-to-video", {"prompt": "legacy-qiniu-e2e", "model": "leaked"}, "Key sk-fake", 400),
            (root + "/text-to-video", {}, "Key sk-fake", 400),
            (root + "/requests/missing", None, "Key sk-fake", 404),
        ]:
            with self.subTest(path=path, body=body, key=key):
                with self.assertRaises(HTTPError) as error:
                    self.request(path, body, key)
                self.assertEqual(error.exception.code, status)
                error.exception.close()

    def test_existing_catalog(self):
        """验证非 Fal 的目录路径仍返回完整旧目录；本地服务与内存任务由 tearDown 清理。"""
        self.assertEqual(len(self.request("/v1/models")["data"]), 155)
