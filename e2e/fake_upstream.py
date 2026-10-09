#!/usr/bin/env python3
"""OpenAI-compatible fake upstream for browser e2e."""
from __future__ import annotations

import argparse
import json
import uuid
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    fal_tasks: dict[str, dict] = {}

    def _fal(self, req: dict | None = None) -> bool:
        """处理本地 Dreamina Fal 创建及查询，供旧七牛凭据的浏览器回归调用。

        参数 req：POST JSON，GET 时为 None；返回：匹配 Fal 路径时为 True。
        检查 Key 鉴权、固定路径及网关字段移除；保存任务到进程内，服务结束清理，无付费调用。
        """
        root = "/queue/byteplus/seedance-2.0"
        if not self.path.startswith(root):
            return False
        if self.headers.get("Authorization") != "Key sk-fake":
            self.send_error(401)
            return True
        if req is not None and self.path == root + "/text-to-video":
            if "model" in req or req.get("prompt") != "legacy-qiniu-e2e":
                self.send_error(400, "invalid Fal request body")
                return True
            task = "fal-e2e-" + uuid.uuid4().hex
            self.fal_tasks[task] = req
            self._json({"request_id": task, "response_url": "https://unused.invalid/" + task,
                        "status_url": "https://unused.invalid/" + task + "/status"})
            return True
        task = self.path.removeprefix(root + "/requests/").removesuffix("/status")
        if req is None and task in self.fal_tasks:
            self._json({"status": "COMPLETED", "video": {"url": "https://example.invalid/fal-e2e.mp4"},
                        "usage": {"completion_tokens": 100}, "resolution": "1080p"})
        else:
            self.send_error(404)
        return True

    def do_GET(self) -> None:
        """按本地供应商路径返回目录，验证 /v1 双向回退及失败降级。

        参数：无，使用当前 HTTP 请求；返回：无，写入目录或 404 响应。
        调用：浏览器模型配置 E2E；不访问外部服务，服务器结束即清理。
        """
        if self._fal():
            return
        if self.path.rstrip("/") in ("/models", "/v1/models",
                                           "/discovery-v1/v1/models", "/discovery-root/models"):
            # Deliberately exceed the old 80-option UI limit. Native Fal paths
            # come from the gateway registry, just as with a real supplier.
            self._json({"data": [{"id": "gpt-4o-mini"}] + [{"id": f"e2e-model-{i}"} for i in range(154)]})
            return
        self.send_error(404)

    def do_POST(self) -> None:
        """读取当前 POST JSON 并返回对应本地推理结果，供浏览器 E2E 调用。

        参数：无；返回：无，写 HTTP 响应；非法 JSON 按空对象处理。
        Fal 任务由 _fal 校验和保存，其余兼容接口保持原行为，进程结束清理数据。
        """
        n = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(n)
        try:
            req = json.loads(raw.decode() or "{}")
        except json.JSONDecodeError:
            req = {}
        if self._fal(req):
            return
        model = req.get("model") or "gpt-4o-mini"
        path = self.path
        if "embeddings" in path:
            payload = {
                "object": "list",
                "model": model,
                "data": [{"object": "embedding", "index": 0, "embedding": [0.1, 0.2]}],
                "usage": {"prompt_tokens": 3, "total_tokens": 3},
            }
            self._json(payload)
            return
        if "transcription" in path or "translation" in path:
            self._json({"text": "e2e-ok", "language": "en", "duration": 1.0, "segments": []})
            return
        if "moderations" in path:
            self._json({
                "id": "modr_e2e",
                "model": model,
                "results": [{"flagged": False, "categories": {}, "category_scores": {}}],
            })
            return
        if "rerank" in path:
            self._json({
                "id": "rerank_e2e",
                "results": [{"index": 0, "relevance_score": 0.9}],
                "meta": {"tokens": {"input_tokens": 1}},
            })
            return
        if "responses" in path:
            payload = {
                "id": "resp_e2e",
                "object": "response",
                "status": "completed",
                "model": model,
                "output": [{"type": "message", "content": [{"type": "output_text", "text": "e2e-ok"}]}],
                "usage": {"input_tokens": 8, "output_tokens": 2, "total_tokens": 10},
            }
            if req.get("stream") is True:
                event = json.dumps({"type": "response.output_text.delta", "delta": "e2e-ok"})
                done = json.dumps({"type": "response.completed", "response": payload})
                body = f"data: {event}\n\ndata: {done}\n\ndata: [DONE]\n\n".encode()
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
            self._json(payload)
            return
        if "/images" in path:
            self._json({"created": 1, "data": [{"url": "https://example.invalid/img/1"}]})
            return
        if "/videos" in path:
            self._json({"id": "video_e2e", "object": "video", "status": "queued", "model": model})
            return
        if "generateContent" in path or "countTokens" in path:
            self._json({
                "candidates": [{"content": {"parts": [{"text": "e2e-ok"}]}}],
                "usageMetadata": {"promptTokenCount": 8, "candidatesTokenCount": 2, "totalTokenCount": 10},
            })
            return
        if req.get("stream") is True:
            body = (
                b'data: {"id":"c1","object":"chat.completion.chunk","choices":[{"delta":{"content":"e2e-ok"}}]}\n\n'
                b"data: [DONE]\n\n"
            )
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        # 护栏 E2E 需要观察上游实际收到的请求内容，证明脱敏发生在
        # 数据面转发之前，而不是只有管理接口的试运行结果正确。
        message_text = ""
        for message in req.get("messages", []):
            if isinstance(message, dict) and isinstance(message.get("content"), str):
                message_text += message["content"]
        if "[手机号已隐藏]" in message_text:
            self._json({
                "id": "chatcmpl_e2e_redacted",
                "object": "chat.completion",
                "model": model,
                "choices": [{"index": 0, "message": {"role": "assistant", "content": "upstream-saw-redacted"}, "finish_reason": "stop"}],
                "usage": {"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
            })
            return
        payload = {
            "id": "chatcmpl_e2e",
            "object": "chat.completion",
            "model": model,
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": "e2e-ok"},
                    "finish_reason": "stop",
                }
            ],
            "usage": {"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
        }
        body = json.dumps(payload).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _json(self, payload: dict) -> None:
        body = json.dumps(payload).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt: str, *args) -> None:
        return


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("--port", type=int, default=4010)
    args = p.parse_args()
    HTTPServer(("127.0.0.1", args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
