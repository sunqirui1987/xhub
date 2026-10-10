#!/usr/bin/env python3
"""OpenAI-compatible fake upstream for browser e2e."""
from __future__ import annotations

import argparse
import json
import socket
import uuid
from email.parser import BytesParser
from email.policy import default
from urllib.parse import urlsplit
from http.server import BaseHTTPRequestHandler, HTTPServer


ERROR_LOG_JSON_DIAGNOSTIC = (
    "E2E upstream JSON diagnostic begin | "
    + "0123456789abcdef" * 128
    + " | E2E upstream JSON diagnostic end"
)
ERROR_LOG_TEXT_DIAGNOSTIC = (
    "E2E upstream plain-text diagnostic begin | "
    + "fedcba9876543210" * 128
    + " | E2E upstream plain-text diagnostic end"
)


def codex_answer(req: dict) -> str:
    """用途：校验隔离 Codex 模拟器真实外发的历史；参数为 Chat 上游正文，返回项目标记。

    浏览器 E2E 专属模型调用；三轮必须依次包含 system、用户和助手历史，缺轮、乱序或泄漏代理字段抛 ValueError，无持久化副作用。
    """
    messages = req.get("messages", [])
    if "previous_response_id" in req or "store" in req or len(messages) not in (2, 4, 6):
        raise ValueError("Codex history or proxy fields invalid")
    first = messages[1].get("content", "")
    prefix, suffix = "Remember the project marker ", ". Reply with only that marker."
    if not first.startswith(prefix) or not first.endswith(suffix):
        raise ValueError("Codex initial project marker missing")
    marker = first[len(prefix):-len(suffix)]
    if not marker or messages[0].get("role") != "system":
        raise ValueError("Codex instructions or marker missing")
    prompts = [first, "What project marker did I give you? Reply with only the marker.",
               "Confirm the same project marker once more. Reply with only the marker."]
    expected = [messages[0]]
    for index in range(len(messages) // 2):
        expected.append({"role": "user", "content": prompts[index]})
        if index < len(messages) // 2 - 1:
            expected.append({"role": "assistant", "content": marker})
    if messages != expected:
        raise ValueError("Codex upstream did not receive full conversation")
    return marker


def continuation_answer(req: dict) -> str:
    """校验续接用例的完整历史；参数为真实上游正文，返回本轮标记或抛出 ValueError。

    浏览器 E2E 的专属模型调用；校验三轮用户/助手顺序和代理字段隔离，无外部调用或持久化。
    """
    messages = req.get("messages", [])
    if "previous_response_id" in req or "store" in req or len(messages) not in (1, 3, 5):
        raise ValueError("continuation history or proxy fields invalid")
    turn = (len(messages) + 1) // 2
    expected = []
    for index in range(1, turn + 1):
        expected.append({"role": "user", "content": f"continuation-{index}"})
        if index < turn:
            expected.append({"role": "assistant", "content": f"history-ok-{index}"})
    if messages != expected:
        raise ValueError("upstream did not receive full conversation")
    return f"history-ok-{turn}"


def continuation_usage(req: dict) -> dict:
    """返回续接夹具的原始统计；参数为上游请求，返回含供应商扩展字段的 usage。

    供本地三轮浏览器测试调用，首轮零缓存、后续读4个token；无外部服务或持久化副作用。
    """
    turn = (len(req.get("messages", [])) + 1) // 2
    return {"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10,
            "prompt_tokens_details": {"cached_tokens": 0 if turn == 1 else 4},
            "provider_statistics": {"turn": turn, "cache_source": "local-fixture"}}


class Handler(BaseHTTPRequestHandler):
    ark_tasks: dict[str, dict] = {}
    fal_tasks: dict[str, dict] = {}

    def _ark(self, req: dict | None = None) -> bool:
        """校验官方 Ark 本地任务请求；req 为正文或 None，返回路径是否匹配。

        调用：浏览器 E2E；只接受 Bearer 和显式模型、非空 content；保存内存任务，进程结束清理。
        """
        roots = ("/api/v3/contents/generations/tasks", "/v3/contents/generations/tasks")
        root = next((candidate for candidate in roots if self.path.startswith(candidate)), None)
        if root is None:
            return False
        if self.headers.get("Authorization") != "Bearer sk-fake":
            self.send_error(401)
            return True
        if req is not None and self.path == root:
            if not req.get("model") or not req.get("content"):
                self.send_error(400, "invalid Ark request body")
                return True
            task = "ark-e2e-" + uuid.uuid4().hex
            self.ark_tasks[task] = req
            self._json({"id": task})
            return True
        task = self.path.removeprefix(root + "/")
        if req is None and task in self.ark_tasks:
            # 特定生命周期夹具第一次查询保持运行，下一次完成；其他既有调用保持立即完成。
            facts = self.ark_tasks[task]
            controlled = any(item.get("text") in ("e2e-task-lifecycle", "e2e-task-failure") for item in facts.get("content", []))
            failed = any(item.get("text") == "e2e-task-failure" for item in facts.get("content", []))
            polls = facts.get("_test_polls", 0)
            facts["_test_polls"] = polls + 1
            state = "running" if controlled and polls == 0 else ("failed" if failed else "succeeded")
            self._json({"id": task, "status": state, "content": {"video_url": "https://example.invalid/ark-e2e.mp4"}, "usage": {"completion_tokens": 100}})
        else:
            self.send_error(404)
        return True

    def _fal(self, req: dict | None = None) -> bool:
        """验证已实现 Fal 队列与 Key 鉴权；req 为创建正文或 None，返回是否匹配。

        仅用于真实网关 E2E，覆盖 Doubao 与 Dreamina Seedance 路径，内存任务随服务退出清理。
        """
        root = next((prefix for prefix in ("/queue/bytedance/seedance-2.0", "/queue/byteplus/seedance-2.0") if self.path.startswith(prefix)), "")
        if not root:
            return False
        if self.headers.get("Authorization") != "Key sk-fake":
            self.send_error(401)
            return True
        if req is not None and self.path == root + "/text-to-video":
            if "model" in req or not req.get("prompt"):
                self.send_error(400)
                return True
            task = "fal-e2e-" + uuid.uuid4().hex
            self.fal_tasks[task] = req
            self._json({"request_id": task})
        elif req is None and self.path.removeprefix(root + "/requests/") in self.fal_tasks:
            self._json({"status": "COMPLETED", "video_url": "https://example.invalid/fal.mp4"})
        else:
            self.send_error(404)
        return True

    def do_GET(self) -> None:
        """按本地供应商路径返回目录，验证 /v1 双向回退、200 网站正文及失败降级。

        参数：无，使用当前 HTTP 请求；返回：无，写入目录或 404 响应。
        调用：浏览器模型配置 E2E；不访问外部服务，服务器结束即清理。
        """
        if self.path.rstrip("/") == "/v1/market/models":
            # 确定性市场数据覆盖公开、私有、退役、多模态与完整价格，测试进程结束回收。
            rows = []
            for model_id, private, retired, output in [
                ("e2e-market-chat", False, "", "text"),
                ("e2e-market-video", False, "", "video"),
                ("e2e-market-private", True, "", "text"),
                ("e2e-market-retired", False, "2020-01-01T00:00:00Z", "text"),
            ]:
                rows.append({"id": model_id, "name": model_id, "private": private,
                    "features": ["工具调用" if output == "text" else "视频生成"], "hot_tags": ["热门"],
                    "retirement_at": retired, "issuer": {"name": "OpenAI" if output == "text" else "ByteDance"},
                    "architecture": {"input_modalities": ["text"], "output_modalities": [output],
                        "function_calling": {"supported": output == "text"}},
                    "model_constraints": {"context_length": 128000},
                    "pricing_rules_v2": [{"input_range": [0, 128000], "details_v2": {
                        "text_input": {"name": "文本输入", "unit_name": "token", "unit_size": 1000000, "unit_price_usd": 2, "unit_price": 14},
                        "text_output": {"name": "文本输出", "unit_name": "token", "unit_size": 1000000, "unit_price_usd": 6, "unit_price": 42}}}],
                    "support_api_protocols": ["openai"]})
            self._json({"status": True, "data": rows})
            return
        if self._ark() or self._fal():
            return
        if self.path.rstrip("/") in ("/discovery-html/models", "/discovery-invalid/models", "/discovery-invalid/v1/models"):
            # 复现供应商网站的兜底路由：不存在的 API 目录仍返回 200 HTML。
            body = b"<!doctype html><html><title>Provider website</title></html>"
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        if self.path.rstrip("/") in ("/models", "/v1/models",
                                           "/discovery-v1/v1/models", "/discovery-root/models", "/discovery-html/v1/models"):
            # 完整返回155条上游模型，验证列表不会被截断或附加隐式专用模型。
            self._json({"data": [{"id": "gpt-4o-mini"}] + [{"id": f"e2e-model-{i}"} for i in range(154)]})
            return
        self.send_error(404)

    def do_POST(self) -> None:
        """读取当前 POST JSON 并返回对应本地推理结果，供浏览器 E2E 调用。

        参数：无；返回：无，写 HTTP 响应；非法 JSON 按空对象处理。
        Ark 任务由 _ark 保存；原生图片和 Google 用例校验字段与上游鉴权，进程结束清理数据。
        """
        n = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(n)
        try:
            req = json.loads(raw.decode() or "{}")
        except (json.JSONDecodeError, UnicodeDecodeError):
            req = {}
        if self._ark(req) or self._fal(req):
            return
        model = req.get("model") or "gpt-4o-mini"
        if self._error_log_response(str(model)):
            return
        # 兼容层允许保留 openai/ 前缀；只对这两个专属夹具规范化，避免影响其他模型行为。
        history_model = str(model).removeprefix("openai/")
        if history_model in ("e2e-responses-history", "e2e-codex-agent"):
            try:
                text = codex_answer(req) if history_model == "e2e-codex-agent" else continuation_answer(req)
            except ValueError as error:
                self.send_error(400, str(error))
                return
            response_id = "chatcmpl-" + uuid.uuid4().hex
            usage = continuation_usage(req) if history_model == "e2e-responses-history" else {"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10}
            if req.get("stream") is True:
                chunks = [
                    {"id": response_id, "choices": [{"index": 0, "delta": {"content": text}}]},
                    {"id": response_id, "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}], "usage": usage},
                ]
                body = ("".join("data: " + json.dumps(chunk) + "\n\n" for chunk in chunks) + "data: [DONE]\n\n").encode()
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self._diagnostic_headers(history_model)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
            else:
                self._json({"id": response_id, "object": "chat.completion", "model": model,
                    "choices": [{"index": 0, "message": {"role": "assistant", "content": text}, "finish_reason": "stop"}], "usage": usage}, diagnostic_model=history_model)
            return
        # 仅测试自建型号前缀触发回退错误，不影响其他浏览器用例或已有模型。
        for prefix, status, code in (("e2e-fallback-500-", 500, "api_error"),
                                     ("e2e-fallback-429-", 429, "rate_limit_exceeded"),
                                     ("e2e-fallback-context-", 400, "context_length_exceeded"),
                                     ("e2e-fallback-content-", 400, "content_policy_violation"),
                                     ("e2e-fallback-normal-", 400, "invalid_request")):
            if str(model).startswith(prefix):
                payload = json.dumps({"error": {"code": code, "message": code}}).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)
                return
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
        if path.endswith("/messages"):
            payload = {"id": "msg_e2e", "type": "message", "role": "assistant", "model": model,
                       "content": [{"type": "text", "text": "e2e-ok"}], "stop_reason": "end_turn",
                       "usage": {"input_tokens": 8, "output_tokens": 2}}
            if req.get("stream") is True:
                self._sse([
                    {"type": "message_start", "message": {"id": "msg_e2e", "usage": {"input_tokens": 8}}},
                    {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}},
                    {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "e2e-ok"}},
                    {"type": "message_delta", "delta": {"stop_reason": "end_turn"}, "usage": {"output_tokens": 2}},
                    {"type": "message_stop"},
                ])
            else:
                self._json(payload)
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
            if req.get("model", "").startswith("e2e-native-real-") or b"e2e-native-real-image_edit" in raw:
                if self.headers.get("Authorization") != "Bearer sk-fake":
                    self.send_error(401)
                    return
                if path.endswith("/edits"):
                    message = BytesParser(policy=default).parsebytes(
                        ("Content-Type: " + self.headers.get("Content-Type", "") + "\r\n\r\n").encode() + raw)
                    fields = {part.get_param("name", header="content-disposition"): part.get_payload(decode=True) for part in message.iter_parts()}
                    if fields.get("model") != b"e2e-native-real-image_edit" or not fields.get("image") or fields.get("prompt") != b"hello":
                        self.send_error(400, "invalid native image edit")
                        return
                elif req.get("prompt") != "hello":
                    self.send_error(400, "invalid native image generation")
                    return
                self._json({"created": 1, "data": [{"b64_json": "e2e-ok"}], "usage": {"input_tokens": 8, "output_tokens": 2}})
                return
            self._json({"created": 1, "data": [{"url": "https://example.invalid/img/1"}]})
            return
        if "/videos" in path:
            self._json({"id": "video_e2e", "object": "video", "status": "queued", "model": model})
            return
        if "generateContent" in path or "streamGenerateContent" in path or "countTokens" in path:
            if "e2e-native-real-" in path:
                is_gemini = path.startswith("/v1beta/")
                authenticated = self.headers.get("x-goog-api-key") == "sk-fake" if is_gemini else self.headers.get("Authorization") == "Bearer sk-fake"
                if not authenticated or "model" in req or not req.get("contents") or urlsplit(path).query not in ("", "alt=sse"):
                    self.send_error(400, "invalid Google native request")
                    return
            if "streamGenerateContent" in path:
                self._sse([{"candidates": [{"content": {"parts": [{"text": "e2e-ok"}]}, "finishReason": "STOP"}],
                            "usageMetadata": {"promptTokenCount": 8, "candidatesTokenCount": 2}}])
                return
            self._json({
                "candidates": [{"finishReason": "STOP", "content": {"parts": [{"text": "e2e-ok"}]}}],
                "usageMetadata": {"promptTokenCount": 8, "candidatesTokenCount": 2, "totalTokenCount": 10},
            })
            return
        if req.get("stream") is True:
            body = (
                b'data: {"id":"c1","object":"chat.completion.chunk","choices":[{"delta":{"content":"e2e-ok"}}]}\n\n'
                b'data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":2}}\n\n'
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
        # 浏览器验收同时覆盖 UI 自定义替换文本和 API 默认替换文本；命中任一值都证明
        # 上游收到的是护栏处理后的正文，而不是仅有管理接口调试结果。
        if "[手机号已隐藏]" in message_text or "[REDACTED]" in message_text:
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

    def _error_log_response(self, model: str) -> bool:
        """为错误日志 E2E 返回可辨识的上游失败；参数为请求模型，返回是否已处理。

        调用场景：数据面转发到本地上游后，在页面验证 JSON 长正文、纯文本正文或断网失败。
        副作用与边界：只匹配 e2e-error-log-* 测试模型；断网分支主动关闭当前连接，服务仍可处理后续请求。
        """
        if model.startswith("e2e-error-log-json-"):
            payload = {
                "error": {
                    "type": "upstream_gateway_error",
                    "code": "E2E_UPSTREAM_502",
                    "message": ERROR_LOG_JSON_DIAGNOSTIC,
                    "details": {
                        "upstream_request_id": "e2e-upstream-json-request",
                        "retryable": False,
                        "terminal_cause": "e2e-json-terminal-cause",
                    },
                }
            }
            self._response(502, json.dumps(payload).encode(), "application/json")
            return True
        if model.startswith("e2e-error-log-text-"):
            self._response(502, ERROR_LOG_TEXT_DIAGNOSTIC.encode(), "text/plain; charset=utf-8")
            return True
        if model.startswith("e2e-error-log-network-"):
            self.close_connection = True
            try:
                self.connection.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            self.connection.close()
            return True
        return False

    def _response(self, status: int, body: bytes, content_type: str) -> None:
        """写出指定 HTTP 响应；参数为状态码、完整正文和类型，无返回值。

        调用场景：错误日志测试需要保留非 200 上游的原始正文和 Content-Type。
        异常与副作用：直接写当前连接；调用方必须在写出后结束请求处理。
        """
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _sse(self, events: list[dict]) -> None:
        """输出本地原厂协议事件；参数为完整事件列表，无返回值，E2E 数据面转换调用，进程结束清理。"""
        body = "".join("data: " + json.dumps(event) + "\n\n" for event in events).encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _diagnostic_headers(self, model: str) -> None:
        """为续接模型加入多值响应头；参数为模型，无返回，供本地 JSON/SSE 验证完整记录及脱敏。"""
        if model == "e2e-responses-history":
            self.send_header("X-Upstream-Trace", "trace-first")
            self.send_header("X-Upstream-Trace", "trace-second")
            self.send_header("Set-Cookie", "upstream-session=fixture-secret")

    def _json(self, payload: dict, diagnostic_model: str = "") -> None:
        """发送本地 JSON 响应；参数为正文和可选诊断模型，无返回，写入头及正文后结束响应。"""
        body = json.dumps(payload).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self._diagnostic_headers(diagnostic_model)
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
