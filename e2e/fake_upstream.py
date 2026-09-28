#!/usr/bin/env python3
"""OpenAI-compatible fake upstream for browser e2e."""
from __future__ import annotations

import argparse
import json
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_POST(self) -> None:
        n = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(n)
        try:
            req = json.loads(raw.decode() or "{}")
        except json.JSONDecodeError:
            req = {}
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
