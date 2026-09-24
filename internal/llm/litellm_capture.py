"""Run LiteLLM 1.102.0 and print the upstream request it would send.

No vendor network call: httpx is patched, and Vertex token refresh is stubbed
to a fixed access token so the URL and body still come from LiteLLM.
"""
import json
import os
import sys

os.environ.setdefault("LITELLM_LOCAL_MODEL_COST_MAP", "True")
ROOT = os.environ.get("LITELLM_ROOT", "/Users/sunqirui/Downloads/litellm-main")
sys.path.insert(0, ROOT)

import httpx
import litellm
from litellm.llms.vertex_ai.vertex_llm_base import VertexBase

captured = []


def _record(request: httpx.Request):
    body = request.content or b""
    try:
        text = body.decode()
    except Exception:
        text = ""
    captured.append({"url": str(request.url), "headers": dict(request.headers), "body": text})


def _send(self, request, **kwargs):
    _record(request)
    payload = {
        "id": "x",
        "object": "chat.completion",
        "choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}],
        "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
        "candidates": [{"content": {"parts": [{"text": "ok"}]}}],
        "content": [{"type": "text", "text": "ok"}],
        "message": {"content": [{"type": "text", "text": "ok"}]},
        "data": [{"url": "https://example.invalid/img"}],
    }
    return httpx.Response(200, json=payload, request=request)


httpx.Client.send = _send


async def _asend(self, request, **kwargs):
    _record(request)
    payload = {
        "value": "eph-secret",
        "expires_at": 1,
        "session": {"model": "gpt-4o-realtime-preview"},
    }
    return httpx.Response(200, json=payload, request=request)


httpx.AsyncClient.send = _asend


def _token(self, credentials=None, project_id=None, **kwargs):
    return "vk", project_id or "vertex-project"


VertexBase.get_access_token = _token

CASES = [
    {"name": "openai", "model": "openai/gpt-4o-mini", "api_base": "https://relay.example/v1", "api_key": "sk-test"},
    {"name": "azure", "model": "azure/dep", "api_base": "https://relay.example/v1", "api_key": "az-key"},
    {"name": "anthropic", "model": "anthropic/claude-3-5-sonnet-latest", "api_base": "https://relay.example", "api_key": "ant-key"},
    {"name": "gemini", "model": "gemini/gemini-2", "api_base": "https://relay.example", "api_key": "gk"},
    {"name": "vertex", "model": "vertex_ai/gemini-2", "api_base": "https://relay.example", "api_key": "vk", "vertex_project": "vertex-project", "vertex_location": "us-central1"},
    {"name": "zai", "model": "zai/glm", "api_base": "https://upstream.example/v1", "api_key": "sk-zai"},
    {"name": "deepseek", "model": "deepseek/deepseek-chat", "api_base": "https://upstream.example/v1", "api_key": "sk-ds"},
    {"name": "cohere", "model": "cohere/command", "api_base": "https://upstream.example/v1", "api_key": "ck"},
]


def auth_of(headers):
    want = {}
    for key, value in headers.items():
        lk = key.lower()
        if lk in ("authorization", "api-key", "x-api-key", "x-goog-api-key", "anthropic-version", "request-source"):
            want[lk] = value
    return want


def norm_body(text):
    try:
        return json.loads(text)
    except Exception:
        return {"raw": text}


messages = [{"role": "user", "content": "hi"}]
out = {"wires": {}, "strategies": []}
for case in CASES:
    captured.clear()
    kwargs = {
        "model": case["model"],
        "messages": messages,
        "api_base": case["api_base"],
        "api_key": case["api_key"],
        "max_tokens": 16,
    }
    if "vertex_project" in case:
        kwargs["vertex_project"] = case["vertex_project"]
        kwargs["vertex_location"] = case["vertex_location"]
    err = None
    try:
        litellm.completion(**kwargs)
    except Exception as exc:
        err = f"{type(exc).__name__}: {exc}"
    req = captured[0] if captured else None
    item = {"error": err}
    if req:
        item["url"] = req["url"]
        item["auth"] = auth_of(req["headers"])
        item["body"] = norm_body(req["body"])
    out["wires"][case["name"]] = item

# Image generation uses the same patched client. Response shape is not the comparison;
# the upstream URL and body are.
captured.clear()
img_err = None
try:
    litellm.image_generation(
        model="openai/dall-e-3",
        prompt="a cat",
        api_base="https://relay.example/v1",
        api_key="sk-test",
    )
except Exception as exc:
    img_err = f"{type(exc).__name__}: {exc}"
if captured:
    out["wires"]["images"] = {
        "error": img_err,
        "url": captured[0]["url"],
        "auth": auth_of(captured[0]["headers"]),
        "body": norm_body(captured[0]["body"]),
    }
else:
    out["wires"]["images"] = {"error": img_err}

strategy_dir = os.path.join(ROOT, "litellm", "router_strategy")
names = []
for entry in sorted(os.listdir(strategy_dir)):
    if entry.startswith(("__", ".")):
        continue
    stem = entry[:-3] if entry.endswith(".py") else entry
    names.append(stem)
def _one(name, fn):
    captured.clear()
    err = None
    try:
        fn()
    except Exception as exc:
        err = f"{type(exc).__name__}: {exc}"
    item = {"error": err}
    if captured:
        item["url"] = captured[0]["url"]
        item["auth"] = auth_of(captured[0]["headers"])
        item["body"] = norm_body(captured[0]["body"])
    out["wires"][name] = item

_one("rerank", lambda: litellm.rerank(
    model="cohere/rerank-english-v3.0",
    query="q",
    documents=["a", "b"],
    api_base="https://upstream.example",
    api_key="ck",
    top_n=2,
))
_one("audio_speech", lambda: litellm.speech(
    model="openai/tts-1",
    input="hi",
    voice="alloy",
    api_base="https://relay.example/v1",
    api_key="sk-test",
))
_one("moderations", lambda: litellm.moderation(
    model="openai/omni-moderation-latest",
    input="hi",
    api_base="https://relay.example/v1",
    api_key="sk-test",
))
_one("responses", lambda: litellm.responses(
    model="openai/gpt-4o-mini",
    input="hi",
    api_base="https://relay.example/v1",
    api_key="sk-test",
))

out["strategies"] = names

captured.clear()
import asyncio

async def _realtime():
    return await litellm.acreate_realtime_client_secret(
        model="openai/gpt-4o-realtime-preview",
        api_base="https://relay.example/v1",
        api_key="sk-test",
    )

rt_err = None
rt_val = None
try:
    rt_val = asyncio.run(_realtime())
except Exception as exc:
    rt_err = f"{type(exc).__name__}: {exc}"
rt_item = {"error": rt_err}
if captured:
    rt_item["url"] = captured[-1]["url"]
    rt_item["auth"] = auth_of(captured[-1]["headers"])
    rt_item["body"] = norm_body(captured[-1]["body"])
if rt_val is not None:
    if hasattr(rt_val, "model_dump"):
        rt_item["returned"] = rt_val.model_dump()
    elif isinstance(rt_val, dict):
        rt_item["returned"] = rt_val
    else:
        rt_item["returned"] = str(rt_val)
out["wires"]["realtime_client_secret"] = rt_item

try:
    info = litellm.get_model_info(model="gpt-4o-mini")
    out["model_info"] = {"max_input_tokens": info.get("max_input_tokens"), "mode": info.get("mode")}
except Exception as exc:
    out["model_info"] = {"error": f"{type(exc).__name__}: {exc}"}
dest = sys.argv[1] if len(sys.argv) > 1 else None
payload = json.dumps(out)
if dest:
    with open(dest, "w") as fh:
        fh.write(payload)
else:
    sys.stdout.write(payload + "\n")
