#!/usr/bin/env python3
"""按 docs 数据清单构建真实租户并验收；凭据仅来自环境，证据不含密钥。"""
import argparse
from contextlib import nullcontext
from checklist import Checklist
import base64
import hashlib
import json
import math
import os
from pathlib import Path
import random
import re
import secrets
import string
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "docs/testdata/real-acceptance/dataset.json"


def acceptance_run_id(length=12):
    """用途：生成只含小写字母的本轮唯一标识；参数为长度，返回随机字符串；验收标记会进入真实护栏，避免连续数字被手机号规则改写后造成观察器误判。"""
    return "".join(secrets.choice(string.ascii_lowercase) for _ in range(length))


def temporary_retry_delay(summary, default_delay, rate_limit_delay, attempt=1):
    """用途：按真实上游错误选择有限重试间隔；参数为错误摘要、普通间隔、限流间隔和当前次数，返回等待秒数；429 使用完整冷却，连接类 5xx 指数退避并以同一上限封顶。"""
    if re.search(r"(?:^|\D)429(?:\D|$)", summary):
        return rate_limit_delay
    return min(default_delay * (2 ** max(attempt - 1, 0)), rate_limit_delay)


def chat_probe_result(status, reply):
    """用途：为供应商探测生成不含正文的判定证据；参数为 HTTP 状态和任意 JSON 响应，返回通过标记及结构诊断；供候选选择调用，只有非空文本回答和有限正数计量可通过，异常结构按失败返回且无副作用。"""
    reply = reply if isinstance(reply, dict) else {}
    choices = reply.get("choices")
    choice = choices[0] if isinstance(choices, list) and choices and isinstance(choices[0], dict) else {}
    message = choice.get("message")
    message = message if isinstance(message, dict) else {}
    usage = reply.get("usage")
    usage = usage if isinstance(usage, dict) else {}
    content, reasoning = message.get("content"), message.get("reasoning_content")
    finish = choice.get("finish_reason")
    # 仅保留协议枚举和长度，不能把供应商正文、错误文本或凭据写入探测报告。
    finish = finish if isinstance(finish, str) and finish in ("stop", "length", "tool_calls", "content_filter", "function_call") else "unknown"
    prompt, completion = usage.get("prompt_tokens", 0), usage.get("completion_tokens", 0)
    valid_usage = all(type(value) in (int, float) and value > 0 and math.isfinite(value)
                      for value in (prompt, completion))
    content_chars = len(content.strip()) if isinstance(content, str) else 0
    return {
        "passed": status == 200 and content_chars > 0 and valid_usage and not reply.get("error"),
        "prompt_tokens": prompt if type(prompt) in (int, float) and math.isfinite(prompt) else 0,
        "completion_tokens": completion if type(completion) in (int, float) and math.isfinite(completion) else 0,
        "finish_reason": finish, "content_chars": content_chars,
        "reasoning_chars": len(reasoning.strip()) if isinstance(reasoning, str) else 0,
        "response_error": bool(reply.get("error")),
    }


def fallback_acceptance_deployments(deployments, model, fallback_model, fallback_provider):
    """用途：从基线部署选择跨模型验收目标；参数为部署列表、主备公开模型和备用供应商，返回主模型全部健康部署 ID 与备用 ID；供回退验收调用，同名模型、缺失或含糊目标抛 ValueError，不修改数据。"""
    if model == fallback_model:
        raise ValueError("跨模型回退要求不同的公开模型")
    baseline = [row for row in deployments if not row.get("temporary") and not row.get("temporary_fault")]
    healthy_ids = [row["id"] for row in baseline if row["public_name"] == model]
    targets = [row["id"] for row in baseline
               if row["public_name"] == fallback_model and row["provider"] == fallback_provider]
    if not healthy_ids or len(targets) != 1:
        raise ValueError("回退验收缺少主模型部署或唯一指定供应商备用部署")
    return healthy_ids, targets[0]


def fallback_acceptance_route(model, fault_id, healthy_ids, fallback_model, fallback_id, timeout_seconds):
    """用途：构造零权重与跨模型回退的临时密钥路由；参数为主备真实模型、故障 ID、全部健康 ID（兼容单个字符串）、备用 ID 和超时，返回完整路由模板正文；同名模型抛 ValueError，所有同名健康部署显式置零，避免未列出部署继承非零默认权重。"""
    if model == fallback_model:
        raise ValueError("跨模型回退要求不同的公开模型")
    healthy_ids = [healthy_ids] if isinstance(healthy_ids, str) else list(healthy_ids)
    return {
        "model_routes": [
            {"model": model, "strategy": "traffic-split", "allocations": [
                {"deployment_id": fault_id, "weight": 100},
                *[{"deployment_id": ident, "weight": 0} for ident in healthy_ids],
            ]},
            {"model": fallback_model, "strategy": "traffic-split", "allocations": [
                {"deployment_id": fallback_id, "weight": 100},
            ]},
        ],
        "retry_policy": {"max_attempts": 1, "timeout_seconds": timeout_seconds,
                         "failure_threshold": 0, "cooldown_seconds": 60},
        "fallbacks": [{model: [fallback_model]}],
        "context_window_fallbacks": [],
        "content_policy_fallbacks": [],
    }


def weighted_acceptance_route(model, first_id, second_id, weights, timeout_seconds):
    """用途：构造同一真实模型两个部署的确定性权重路由；参数为模型、两个部署 ID、两项权重和超时，返回完整模板正文；验收只使用 100:0 与 0:100，避免随机抽样结论。"""
    return {
        "model_routes": [{
            "model": model,
            "strategy": "traffic-split",
            "allocations": [
                {"deployment_id": first_id, "weight": weights[0]},
                {"deployment_id": second_id, "weight": weights[1]},
            ],
        }],
        "retry_policy": {"max_attempts": 1, "timeout_seconds": timeout_seconds,
                         "failure_threshold": 0, "cooldown_seconds": 60},
        "fallbacks": [],
        "context_window_fallbacks": [],
        "content_policy_fallbacks": [],
    }


def header_value(headers, name):
    """用途：按 HTTP 大小写不敏感规则读取响应头；参数为头字典和名称，返回字符串或空值；供缓存与请求 ID 验收复用。"""
    return next((value for key, value in headers.items() if key.lower() == name.lower()), None)


def response_cache_hit(headers):
    """用途：识别网关两种兼容缓存命中头；参数为响应头，返回布尔值；只接受显式 true，不把缺失或其他值视为命中。"""
    return any(str(header_value(headers, name) or "").lower() == "true"
               for name in ("cache_hit", "x-litellm-cache-hit"))


def cache_bill_check(bill, key, expected_usage):
    """用途：核对缓存命中的独立零费用日志；参数为账单、密钥和首次响应 usage，返回无；验证五级归属、usage 一致、命中标记、零费用及价格明细。"""
    for field, expected in (("api_key", key["token_id"]), ("user", key["user_id"]),
                            ("team_id", key["team_id"]), ("project_id", key["project_id"]),
                            ("organization_id", key["organization_id"])):
        if bill.get(field) != expected:
            raise AssertionError("缓存账单归属错误: " + field)
    if any(int(bill.get(field, -1)) != int(expected_usage.get(field, -2))
           for field in ("prompt_tokens", "completion_tokens")):
        raise AssertionError("缓存命中 usage 与首次真实响应不一致")
    if not bill.get("cache_hit"):
        raise AssertionError("缓存命中日志缺少 cache_hit=true")
    if not math.isclose(float(bill.get("spend", -1)), 0, rel_tol=0, abs_tol=1e-12):
        raise AssertionError("缓存命中仍产生费用")
    if not bill.get("metadata", {}).get("cost_breakdown"):
        raise AssertionError("缓存命中日志缺少价格明细")


def load_manifest(path=MANIFEST):
    """用途：读取并校验固定层级；参数为清单路径，返回字典；供启动/单测调用，非法或重复数据拒绝写库。"""
    data = json.loads(Path(path).read_text())
    if data.get("version") != 1 or len(data["organizations"]) != 3 or len(data["teams"]) != 3:
        raise ValueError("数据集必须为 v1，包含三个组织和三个团队定义")
    for field in ("organizations", "teams", "providers"):
        slugs = [row.get("slug", row.get("id")) for row in data[field]]
        if len(set(slugs)) != len(slugs) or not all(re.fullmatch(r"[a-z][a-z0-9-]*", s or "") for s in slugs):
            raise ValueError(field + " 标识必须唯一且可用于邮箱")
    if any(len(o["members"]) != 9 or len(set(o["members"])) != 9 for o in data["organizations"]):
        raise ValueError("每个组织必须列出九个不同成员")
    if data["key_profiles"] != ["inherit", "fenno-only", "qiniu-only"]:
        raise ValueError("每个成员的三个密钥策略必须完整")
    rates = data["billing"]
    if any(not math.isfinite(rates[k]) or rates[k] <= 0 for k in ("input_cost_per_token", "output_cost_per_token")):
        raise ValueError("验收费率必须为有限正数")
    if len(data["providers"]) != 2 or data["limits"]["max_upstream_attempts"] < 100:
        raise ValueError("需要两个供应商及至少 100 次请求额度")
    agent = data.get("agent_conversation", {})
    if agent.get("agent_type") != "codex" or agent.get("endpoint") != "/v1/responses" \
            or agent.get("turns") != 3 or not str(agent.get("user_agent", "")).startswith("codex_cli_rs/") \
            or not agent.get("instructions") or not isinstance(agent.get("max_output_tokens"), int) \
            or agent["max_output_tokens"] < 32:
        raise ValueError("必须声明 Codex Responses 三轮会话、客户端标识、指令和输出额度")
    if data["limits"]["max_upstream_attempts"] < 81 * agent["turns"] + 100:
        raise ValueError("上游额度必须覆盖 81 个 Codex 三轮会话及其他验收请求")
    chains = data.get("logic_chains", {})
    cache_chain = chains.get("cache_consistency", {})
    weight_chain = chains.get("weighted_route_switch", {})
    fallback_chain = chains.get("fallback_cache", {})
    if cache_chain.get("identical_replays", 0) < 2 or not cache_chain.get("cross_key_isolation") \
            or not cache_chain.get("route_change_invalidates") or cache_chain.get("cache_hit_cost") != 0:
        raise ValueError("缓存一致性链必须覆盖重复请求、跨密钥隔离、路由失效和零费用")
    initial = weight_chain.get("initial_weights")
    updated = weight_chain.get("updated_weights")
    if initial != [100, 0] or updated != [0, 100] or any(
            not isinstance(value, (int, float)) or value < 0 for value in (initial or []) + (updated or [])):
        raise ValueError("权重切换链必须固定为 100:0 到 0:100")
    if fallback_chain.get("primary_failure_status") != 429 or fallback_chain.get("repeat_requests", 0) < 2 \
            or not fallback_chain.get("cache_must_remain_disabled"):
        raise ValueError("回退链必须重复验证 429、真实回退和缓存禁用")
    required_roles = {"platform_admin", "organization_admin", "team_admin", "member"}
    if {p["role"] for p in data.get("personas", [])} != required_roles:
        raise ValueError("必须包含四类可登录身份")
    if {s["role"] for s in data.get("permission_scenarios", [])} != required_roles:
        raise ValueError("四类角色必须具有可执行权限场景")
    for guard in data["guardrails"]:
        params = guard["litellm_params"]
        if params.get("guardrail") == "custom_code" and (params.get("custom_code_language") != "xgo" or
                "func ApplyGuardrail(" not in params.get("custom_code", "") or not guard.get("acceptance")):
            raise ValueError("XGo必须包含完整源码及执行验收场景")
    required_models = {
        "fenno-chat", "qiniu-chat", "qiniu-glm", "fenno-image", "qiniu-ark-seedance",
        "qiniu-fal-seedance", "qiniu-fal-kling",
    }
    definitions = {row.get("id"): row for row in data.get("models", [])}
    if set(definitions) != required_models:
        raise ValueError("真实验收必须声明 GPT 双供应商、GLM、图片和三种视频模型")
    if data.get("verification_order") != ["gpt-5.6-sol", "z-ai/glm-5", "fenno-image",
                                               "qiniu-ark-seedance", "qiniu-fal-seedance", "qiniu-fal-kling"]:
        raise ValueError("验收顺序必须为 GPT、GLM、图片、Seedance 和 Kling")
    glm = definitions["qiniu-glm"]
    if any(glm.get(field) != value for field, value in {
            "provider": "qiniu", "kind": "chat", "selection": "exact",
            "public_name": "z-ai/glm-5", "upstream_model": "z-ai/glm-5",
            "stored_model": "z-ai/glm-5", "transport": "bypass_openai_chat",
            "endpoint_types": ["chat", "responses"]}.items()):
        raise ValueError("GLM 必须明确声明七牛 z-ai/glm-5 的 Chat/Responses 部署")
    expected = {
        "fenno-image": ("gpt-image-2", "bypass_openai_image_generation", "bypass:openai-images"),
        "qiniu-ark-seedance": ("bytedance/doubao-seedance-2-0-mini-260615", "qiniu_contents_generation", "bypass:ark-video"),
        "qiniu-fal-seedance": ("bytedance/seedance-2.0/mini/text-to-video", "qiniu_fal_doubao_20", "fal:queue"),
        "qiniu-fal-kling": ("fal-ai/kling-video/v2.5-turbo/pro/text-to-video", "qiniu_fal_kling", "fal:queue"),
    }
    for ident, (model, transport, endpoint) in expected.items():
        row = definitions[ident]
        if row.get("public_name") != model or row.get("upstream_model") != model:
            raise ValueError(ident + " 的公开模型名必须等于真实上游模型 ID")
        if row.get("transport") != transport or row.get("endpoint_types") != [endpoint]:
            raise ValueError(ident + " 的传输协议或端点类型不正确")
    if any(row.get("public_name") and (not row["public_name"].isascii() or re.search(r"[\s\u4e00-\u9fff]", row["public_name"])) for row in data["models"]):
        raise ValueError("公开模型名必须是英文真实上游模型 ID")
    shared = data.get("shared_chat_model", {})
    if shared.get("public_name") != "gpt-5.6-sol" or shared.get("default_weights") != [3, 7] \
            or shared.get("deployments") != {"fennoai": "gpt-5.6-sol", "qiniu": "openai/gpt-5.6-sol"}:
        raise ValueError("共享聊天模型必须明确声明 Fenno/七牛 gpt-5.6-sol 及 3:7 默认权重")
    for ident, provider, upstream in (("fenno-chat", "fennoai", "gpt-5.6-sol"),
                                      ("qiniu-chat", "qiniu", "openai/gpt-5.6-sol")):
        row = definitions[ident]
        if row.get("provider") != provider or row.get("selection") != "exact" \
                or row.get("upstream_model") != upstream or row.get("stored_model") != upstream \
                or row.get("public_name") != shared["public_name"]:
            raise ValueError(ident + " 必须映射到共享 gpt-5.6-sol 的精确真实部署")
        if row.get("endpoint_types") != ["chat", "responses"]:
            raise ValueError(ident + " 必须同时开放 Chat 和 Codex Responses 入口")
    variables = {"shared_chat_model": "gpt-5.6-sol",
                 "fenno_deployment": "f", "qiniu_deployment": "q"}
    for profile in ("organization-default", "team-default", "fenno-only", "qiniu-only"):
        resolve_document(data["routing"]["templates"][profile], variables)
    return data


def resolve_document(value, variables):
    """用途：递归解析模板占位符；参数为 JSON 和运行期 ID，返回新文档；供模板实例化，未声明变量立即失败，不改原清单。"""
    if isinstance(value, str) and value.startswith("$"):
        if value[1:] not in variables:
            raise ValueError("未声明模板变量: " + value)
        return variables[value[1:]]
    if isinstance(value, list):
        return [resolve_document(item, variables) for item in value]
    if isinstance(value, dict):
        return {resolve_document(k, variables): resolve_document(v, variables) for k, v in value.items()}
    return value


def hierarchy(data):
    """用途：展开 3×3×3 身份；参数为已校验清单，返回组织团队成员列表；邮箱使用保留域，不代表实际公司员工。"""
    return [dict(organization=o, team=t, index=i, name=o["members"][ti * 3 + i],
                 email=f'{o["slug"]}.{t["slug"]}.{i+1}@acceptance.xhub.invalid')
            for o in data["organizations"] for ti, t in enumerate(data["teams"]) for i in range(3)]


def write_private(path, data):
    """用途：保存可续跑的访问材料；参数为路径和 JSON，返回无；权限固定 0600，父目录由调用方设为 0700。"""
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    os.chmod(path, 0o600)
    with os.fdopen(fd, "w") as stream:
        json.dump(data, stream, ensure_ascii=False, indent=2)


def http(base, route, body=None, token=None, method=None, request_headers=None):
    """用途：发送真实 HTTP JSON；参数为地址/路径/正文/身份/方法及可选客户端头，返回状态、正文、头；供管理接口和 Codex 模拟调用，网络异常交给调用方。"""
    headers = {"Content-Type": "application/json", **(request_headers or {})}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = Request(base.rstrip("/") + route, data=None if body is None else json.dumps(body).encode(),
                  headers=headers, method=method or ("GET" if body is None else "POST"))
    try:
        # 图片生成可能在供应商侧同步运行数分钟；客户端窗口必须长于清单中的
        # 300 秒上游窗口，否则会由验收客户端提前断开，误报成网关失败。
        response = urlopen(req, timeout=330)
    except HTTPError as error:
        response = error
    with response:
        raw = response.read()
        try:
            value = json.loads(raw)
        except ValueError:
            value = {"non_json": True}
        return response.status, value, dict(response.headers)


def curl_json(base, route, body, token, timeout=180):
    """用途：用系统 curl 兼容供应商长连接并读取 JSON；参数为地址、路径、可空正文、令牌和超时，返回状态、正文、空响应头；异常文本不复述命令参数或令牌。"""
    command = ["curl", "--silent", "--show-error", "--max-time", str(timeout), "--output", "-",
               "--write-out", "\n%{http_code}", "-H", "Content-Type: application/json",
               "-H", "Authorization: Bearer " + token]
    if body is not None:
        command.extend(["--data-binary", json.dumps(body)])
    command.append(base.rstrip("/") + route)
    with subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE) as process:
        stdout, stderr = process.communicate()
    if process.returncode != 0:
        raise ConnectionError("curl 供应商请求失败，退出码=" + str(process.returncode) + "，错误=" + stderr.decode(errors="replace")[:200])
    raw, separator, status_text = stdout.rpartition(b"\n")
    if not separator or not status_text.isdigit():
        raise ConnectionError("curl 供应商响应缺少 HTTP 状态")
    try:
        value = json.loads(raw)
    except ValueError:
        value = {"non_json": True}
    return int(status_text), value, {}


def forward_supplier_chat(provider, body):
    """用途：把观察器收到的聊天请求原样转发给真实供应商；参数为供应商配置和请求正文，返回 HTTP 状态、JSON 正文与响应头；供 81 密钥及回退验收使用，网络错误交由观察器转换成明确的 502。"""
    base = os.environ.get(provider["base_env"], provider["base"]).rstrip("/")
    route = "/chat/completions" if base.endswith("/v1") else "/v1/chat/completions"
    return curl_json(base, route, body, os.environ[provider["key_env"]], timeout=330)


def chat_candidates(catalog, rng, preferred=()):
    """用途：从真实目录选择聊天候选；参数为目录、随机源和已知稳定真实 ID，返回去重列表；目录内的稳定模型优先，其余候选随机回退，探测成功才部署。"""
    ids = catalog.get("model_ids", [])
    candidates = [m for m in ids if re.search(r"gpt|claude|gemini|deepseek|qwen|kimi|glm|llama|hy|grok", m, re.I)
                  and not re.search(r"image|video|audio|tts|embed|rerank|realtime|whisper|vision|preview.*image", m, re.I)]
    candidates = sorted(set(candidates))
    rng.shuffle(candidates)
    preferred = [model for model in preferred if model in candidates]
    return preferred + [model for model in candidates if model not in preferred]


def bill_check(bill, key, rates):
    """用途：核对真实用量及五级归属；参数为账单、密钥记录和费率，返回费用；供 API/浏览器验收，缺字段或金额不符抛异常。"""
    for field, expected in (("api_key", key["token_id"]), ("user", key["user_id"]),
                            ("team_id", key["team_id"]), ("project_id", key["project_id"]),
                            ("organization_id", key["organization_id"])):
        if bill.get(field) != expected:
            raise AssertionError("账单归属错误: " + field)
    if int(bill.get("prompt_tokens", 0)) <= 0 or int(bill.get("completion_tokens", 0)) <= 0:
        raise AssertionError("供应商必须返回非零真实输入输出用量")
    expected = bill["prompt_tokens"] * rates["input_cost_per_token"] + bill["completion_tokens"] * rates["output_cost_per_token"]
    if not math.isclose(float(bill["spend"]), expected, rel_tol=0, abs_tol=1e-9):
        raise AssertionError("真实账单与固定验收费率不一致")
    if not bill.get("metadata", {}).get("cost_breakdown"):
        raise AssertionError("缺少历史价格快照")
    return float(bill["spend"])


class Observer:
    """真实供应商转发及可控故障观察器；正常响应均来自外网，不生成模拟补全。"""

    def __init__(self, providers, limit, port=0, logger=None):
        """用途：建立隔离转发服务；参数为供应商配置/尝试上限/端口/日志回调，返回实例；密钥仅从环境读取，服务需 close 清理。"""
        self.providers = {p["id"]: p for p in providers}
        self.limit, self.attempts, self.rows = limit, 0, []
        self.logger = logger or (lambda message: None)
        self.lock = threading.Lock()
        owner = self

        class Handler(BaseHTTPRequestHandler):
            """只转发列入清单的路径，并记录正文中可核对的请求标记。"""

            def log_message(self, *args):
                """用途：关闭默认访问日志；任意日志参数不输出，避免路径或网络错误携带私密材料。"""
                pass

            def do_POST(self):
                """用途：处理网关补全；无参数，返回 HTTP 响应；第二段路径作为部署标签写入证据，只有 fault 注入故障，其余标签仍真实转发。"""
                parts = self.path.strip("/").split("/")
                if not parts or parts[0] not in owner.providers:
                    self.send_error(404)
                    return
                provider = owner.providers[parts[0]]
                body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))))
                text = json.dumps(body.get("messages", []), ensure_ascii=False)
                # 未配置观察标签时，OpenAI 客户端会把协议路径（例如 /v1/chat/completions）
                # 直接追加到供应商前缀。这里把协议段归一为 default，避免把 v1 误当成
                # 部署标签，导致真实回退已经成功却被证据断言错误拒绝。
                candidate_tag = parts[1] if len(parts) > 1 else "default"
                route_tag = "default" if candidate_tag in {"v1", "chat", "responses"} else candidate_tag
                fault = route_tag == "fault"
                with owner.lock:
                    owner.attempts += 1
                    allowed = owner.attempts <= owner.limit
                status = 429 if fault else 0
                usage = {}
                content = b'{"error":{"message":"acceptance injected rate limit","type":"rate_limit"}}'
                if not allowed:
                    status = 429
                elif not fault:
                    try:
                        # 系统 curl 与构建期真实模型探测使用同一网络路径，避免 urllib
                        # 经本机代理建立 HTTPS 隧道时被远端提前断开。
                        status, answer, _ = forward_supplier_chat(provider, body)
                    except (ConnectionError, OSError) as error:
                        status = 502
                        answer = {"error": {"message": "acceptance supplier transport failed",
                                            "type": "upstream_transport"}}
                        owner.logger("供应商=%s 模型=%s 真实转发失败=%s" % (
                            provider["id"], body.get("model"), type(error).__name__))
                    usage = answer.get("usage", {})
                    content = json.dumps(answer).encode()
                with owner.lock:
                    owner.rows.append({"provider": provider["id"], "model": body.get("model"),
                                       "route_tag": route_tag,
                                       "messages": text, "fault": fault, "forwarded": allowed and not fault, "status": status, "usage": usage})
                owner.logger("供应商=%s 部署标签=%s 模型=%s 状态=%s 输入=%s 输出=%s%s" % (
                    provider["id"], route_tag, body.get("model"), status, usage.get("prompt_tokens", 0),
                    usage.get("completion_tokens", 0), "（注入限流）" if fault else ""))
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(content)

        self.server = ThreadingHTTPServer(("127.0.0.1", port), Handler)
        self.base = "http://127.0.0.1:" + str(self.server.server_port)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def close(self):
        """用途：结束当前观察器；无参数/返回，供 finally 调用；释放端口并等待服务器退出。"""
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()


class Dataset:
    """通过当前公开管理接口创建基线，再通过个人密钥验收数据面。"""

    def __init__(self, gateway, directory, data, observer=None, logger=None):
        """用途：准备运行状态；参数为网关/私有目录/清单/观察器/日志回调，返回实例；保存密钥独立于公开证据。"""
        self.gateway, self.directory, self.data, self.observer = gateway, Path(directory), data, observer
        self.logger = logger or (lambda message: print(message, flush=True))
        self.log_lock, self.api_sequence = threading.Lock(), 0
        self.directory.mkdir(parents=True, exist_ok=True)
        os.chmod(self.directory, 0o700)
        self.state = {"keys": [], "organizations": [], "teams": [], "users": [], "projects": [], "deployments": []}
        self.report = {"checks": [], "probe_attempts": [], "calls": [], "models": [], "status": "running"}
        self.run = acceptance_run_id()
        self.admin = None
        self.representative = False
        self.checklist = None

    def acceptance_keys(self):
        """用途：选择不同路由规则的代表密钥；无参数，返回稳定列表；共享验收每类一把，其他入口保留全量，不修改基线，空列表由会话验收拒绝。"""
        if not self.representative:
            return self.state["keys"]
        chosen = {}
        for key in self.state["keys"]:
            profile = key["profile"]
            category = ("organization" if key["team_index"] == 0 else "team") if profile == "inherit" else profile
            chosen.setdefault(category, key)
        if set(chosen) != {"organization", "team", "fenno-only", "qiniu-only"}:
            raise AssertionError("代表密钥必须覆盖组织继承、团队继承和两种单供应商配置")
        return list(chosen.values())

    def case(self, ident):
        """用途：进入编号场景；参数为清单 ID，返回上下文；shared runner 保存结果，其他调用兼容原行为，无业务副作用。"""
        return self.checklist.case(ident) if self.checklist else nullcontext({})

    def run_case(self, ident, callback, *args):
        """用途：执行独立业务场景；参数为 ID、函数及位置参数，返回函数结果；失败写清单并原样抛出，不掩盖断言或清理错误。"""
        with self.case(ident):
            return callback(*args)

    def start_checklist(self, with_regression=True):
        """用途：公布去重验收计划；参数决定是否登记后台套件，无返回；模型调用前启动，每种路由只登记一个代表，内部三轮仍完整。"""
        plan = [("gpt-" + str(index), "GPT 三轮上下文、粘滞与五级账单：" + key["profile"] +
                 (" / 组织继承" if key.get("team_index") == 0 else " / 团队继承") if key["profile"] == "inherit"
                 else "GPT 三轮上下文、粘滞与五级账单：" + key["profile"])
                for index, key in enumerate(self.acceptance_keys(), 1)]
        plan.insert(0, ("entities", "实体基线及团队成员"))
        plan += [("glm", "GLM 三轮上下文与账单"), ("image", "图片生成、文件与精确请求日志"),
                 ("ark-video", "Ark 视频创建、终态、日志与重复结算"),
                 ("fal-video", "FAL Seedance 视频创建、终态、日志与重复结算"),
                 ("kling-video", "FAL Kling 视频创建、终态、日志与重复结算"),
                 ("inheritance", "组织/团队/个人路由继承"),
                 ("chat-bills", "Chat 入口代表请求与价格快照"),
                 ("permissions", "四类角色权限与跨组织拒绝"), ("xgo", "XGo 持久化、调试、拦截、脱敏与放行"),
                 ("limits", "五级预算、RPM/TPM、模型白名单与密钥生命周期"),
                 ("model-lifecycle", "模型创建、下架、恢复与删除"), ("guard-lifecycle", "护栏编辑、默认启用与删除"),
                 ("global-guards", "全局拦截无外发与个人信息转发前脱敏"), ("profile-isolation", "单供应商路由隔离"),
                 ("cache-weight", "缓存零费用、跨密钥隔离、封禁及权重更新失效"),
                 ("fallback", "429 回退、不缓存、目标权限与禁用回退"),
                 ("media-logs", "精确媒体日志与终态证据复验"), ("database", "事件、日报、token、五级金额与结算去重"),
                 ("browser", "真实数据的控制台、日志、护栏与角色页面"),
                 ("business-browser", "去重浏览器业务：密钥、模型、用户、护栏、Playground、错误及任务日志")]
        if with_regression:
            plan.append(("regression", "后台接口契约、权限、预算、协议与持久化回归"))
        self.checklist = Checklist(self.directory, self.logger, plan)

    def save(self):
        """用途：落盘检查点和脱敏报告；无参数/返回，供每个阶段调用；访问材料保存在忽略目录 0600 文件。"""
        write_private(self.directory / "access.json", {**self.state, "admin": self.admin, "gateway": self.gateway})
        (self.directory / "report.json").write_text(json.dumps(self.report, ensure_ascii=False, indent=2) + "\n")

    def progress(self, message):
        """用途：把不含凭据的构建与验收进度立即输出到终端；参数为日志正文，无返回值；供长时间真实请求显示当前步骤。"""
        self.logger(message)

    def action_start(self, model, protocol, action, detail):
        """用途：记录单项验收开始；参数为模型、协议、动作和脱敏说明，无返回值；调用方在真实操作前使用。"""
        self.progress(f"    ▶ [{model} / {protocol} / {action}] {detail}")

    def action_wait(self, model, protocol, action, detail):
        """用途：记录异步任务等待状态；参数为模型、协议、动作和脱敏说明，无返回值；不会把轮询当作成功。"""
        self.progress(f"    ⏳ [{model} / {protocol} / {action}] {detail}")

    def action_ok(self, model, protocol, action, detail, record=True):
        """用途：记录一个已验证成功项；参数含是否写报告，返回无；只允许在断言完成后调用。"""
        self.progress(f"    ✅ [{model} / {protocol} / {action}] {detail}")
        if record:
            self.report["checks"].append({"name": f"{model}:{protocol}:{action}", "model": model,
                                          "protocol": protocol, "action": action, "passed": True})

    def action_fail(self, model, protocol, action, error):
        """用途：记录单项验收失败；参数为模型、协议、动作和异常，无返回值；不输出请求正文与密钥。"""
        self.progress(f"    ❌ [{model} / {protocol} / {action}] {type(error).__name__}: {error}")

    def result_json(self, filename, value):
        """用途：把 bypass 返回结果保存为可复核 JSON；参数为文件名和脱敏后的响应，返回绝对路径；文件权限为0600。"""
        directory = self.directory / "artifacts"
        directory.mkdir(parents=True, exist_ok=True)
        path = directory / filename
        write_private(path, value)
        return str(path.resolve())

    def image_result(self, model, answer):
        """用途：提取并落盘真实图片 bypass 结果；参数为模型和响应，返回可打印证据；base64 图片保存为二进制且 JSON 不重复保存大正文。"""
        first = answer["data"][0]
        compact = {key: value for key, value in answer.items() if key != "data"}
        compact["data"] = [{key: value for key, value in first.items() if key != "b64_json"}]
        if first.get("b64_json"):
            raw = base64.b64decode(first["b64_json"], validate=True)
            if raw.startswith(b"\x89PNG\r\n\x1a\n"):
                extension = ".png"
            elif raw.startswith(b"\xff\xd8\xff"):
                extension = ".jpg"
            else:
                extension = ".bin"
            image_path = self.directory / "artifacts" / (model.replace("/", "_") + extension)
            image_path.parent.mkdir(parents=True, exist_ok=True)
            image_path.write_bytes(raw)
            os.chmod(image_path, 0o600)
            evidence = {"type": "b64_json", "bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest(),
                        "artifact": str(image_path.resolve())}
            compact["data"][0]["b64_json"] = evidence
        else:
            evidence = {"type": "url", "url": first["url"]}
        evidence["response_json"] = self.result_json(model.replace("/", "_") + "-bypass-result.json", compact)
        return evidence

    def api(self, route, body=None, token=None, method=None, expected=200, include_status=False, request_headers=None):
        """用途：验证管理或数据接口；参数为请求、期望状态、状态码开关及可选客户端头，返回 JSON/头或 JSON/头/状态；正文不出现在失败日志，避免凭据泄露。"""
        verb = method or ("GET" if body is None else "POST")
        safe_route = route.split("?", 1)[0]
        with self.log_lock:
            self.api_sequence += 1
            sequence = self.api_sequence
        started = time.monotonic()
        expected_values = (expected,) if isinstance(expected, int) else tuple(expected)
        expected_label = "/".join(str(value) for value in expected_values)
        self.progress(f"    [接口 {sequence:04d}] {verb} {safe_route} 开始，预期={expected_label}")
        try:
            options = {"request_headers": request_headers} if request_headers else {}
            status, value, headers = http(self.gateway, route, body, token or self.admin, method, **options)
        except Exception as error:
            self.progress(f"    [接口 {sequence:04d}] {verb} {safe_route} 网络失败，耗时={time.monotonic() - started:.3f}s，错误={type(error).__name__}")
            raise
        self.progress(f"    [接口 {sequence:04d}] {verb} {safe_route} 完成，状态={status}，耗时={time.monotonic() - started:.3f}s")
        if status not in expected_values:
            raise AssertionError(f"{verb} {safe_route} 状态 {status}，预期 {expected_label}")
        if include_status:
            return value, headers, status
        return value, headers

    def image_generation(self, definition, key, attempts=3, delay=10):
        """用途：执行真实图片生成并对明确的临时上游错误有限重试；参数为模型定义、个人密钥、最大次数和间隔，返回成功响应与头；每次尝试均独立记录，最终仍失败时抛错且不算通过。"""
        model, protocol = definition["public_name"], definition["transport"]
        body = {"model": model, "prompt": "A red apple on a white background.",
                "n": 1, "size": "1024x1024"}
        retryable = (429, 502, 503, 504)
        for attempt in range(1, attempts + 1):
            self.action_start(model, protocol, f"第 {attempt}/{attempts} 次真实图片生成",
                              "POST /bypass/openai/v1/images/generations，1 张 1024x1024")
            answer, headers, status = self.api(definition["create_path"], body, token=key["key"],
                                                expected=(200, *retryable), include_status=True)
            if status == 200:
                self.action_ok(model, protocol, f"第 {attempt}/{attempts} 次真实图片生成响应",
                               "HTTP=200，继续核对图片文件、请求日志和计费", record=False)
                return answer, headers
            error = answer.get("error", answer) if isinstance(answer, dict) else {}
            if isinstance(error, dict):
                summary = str(error.get("message") or error.get("code") or "无错误正文")[:300]
            else:
                summary = str(error)[:300]
            self.action_fail(model, protocol, f"第 {attempt}/{attempts} 次真实图片生成",
                             RuntimeError(f"HTTP={status}，上游临时错误={summary}"))
            if attempt < attempts:
                self.action_wait(model, protocol, "真实图片生成重试", f"{delay} 秒后进行第 {attempt + 1}/{attempts} 次")
                time.sleep(delay)
        raise AssertionError(f"gpt-image-2 连续 {attempts} 次真实生成均遇到临时上游错误")

    def discover_catalog(self, provider, attempts=3, delay=5):
        """用途：优先通过已持久化连接读取真实目录，代理失败后从同一供应商只读接口实时回退；参数为供应商定义、最大次数和间隔，返回非空目录；两条路径都不接受静态或伪造模型。"""
        name = provider["name"]
        for attempt in range(1, attempts + 1):
            self.action_start(name, "provider-catalog", f"第 {attempt}/{attempts} 次读取真实模型目录",
                              f"连接名={provider['id']}，POST /model/builtin/models")
            catalog, _ = self.api("/model/builtin/models", {"credential_name": provider["id"]})
            model_ids = catalog.get("model_ids", []) if isinstance(catalog, dict) else []
            error = catalog.get("error") if isinstance(catalog, dict) else "响应不是 JSON 对象"
            if not error and model_ids:
                self.action_ok(name, "provider-catalog", "读取真实模型目录",
                               f"连接名={provider['id']}，模型数={len(model_ids)}，第 {attempt}/{attempts} 次成功")
                return catalog
            detail = str(error or "HTTP=200 但 model_ids 为空")[:300]
            self.action_fail(name, "provider-catalog", f"第 {attempt}/{attempts} 次读取真实模型目录",
                             RuntimeError(detail))
            if attempt < attempts:
                self.action_wait(name, "provider-catalog", "真实模型目录重试",
                                 f"{delay} 秒后进行第 {attempt + 1}/{attempts} 次只读查询")
                time.sleep(delay)
        self.action_wait(name, "provider-catalog", "供应商目录直连回退",
                         "已保存连接的目录代理连续失败，改用同一连接凭据读取供应商官方 /v1/models")
        try:
            return self.discover_supplier_catalog(provider, attempts=attempts, delay=delay)
        except Exception as error:
            raise RuntimeError("真实目录发现失败: " + provider["id"] +
                               f"；连接代理和供应商官方目录各连续 {attempts} 次失败") from error

    def discover_supplier_catalog(self, provider, attempts=3, delay=5):
        """用途：从供应商官方 OpenAI 目录读取实时模型 ID；参数为供应商定义、最大次数和间隔，返回标准 model_ids；仅在已保存连接的目录代理失败后调用，所有尝试均打印结果。"""
        name = provider["name"]
        base = os.environ.get(provider["base_env"], provider["base"]).rstrip("/")
        route = "/models" if base.endswith("/v1") else "/v1/models"
        for attempt in range(1, attempts + 1):
            self.action_start(name, "provider-catalog-direct", f"第 {attempt}/{attempts} 次读取供应商官方目录",
                              f"连接名={provider['id']}，GET {route}")
            try:
                status, response, _ = curl_json(base, route, None, os.environ[provider["key_env"]], timeout=90)
                rows = response.get("data", []) if isinstance(response, dict) else []
                model_ids = [row.get("id") for row in rows if isinstance(row, dict) and row.get("id")]
                if status == 200 and model_ids:
                    catalog = {"model_ids": list(dict.fromkeys(model_ids)), "source": "supplier-official-live"}
                    self.action_ok(name, "provider-catalog-direct", "读取供应商官方目录",
                                   f"连接名={provider['id']}，HTTP=200，模型数={len(catalog['model_ids'])}，第 {attempt}/{attempts} 次成功")
                    return catalog
                error = response.get("error") if isinstance(response, dict) else None
                raise RuntimeError(f"HTTP={status}，模型数={len(model_ids)}，错误={str(error)[:200]}")
            except Exception as error:
                self.action_fail(name, "provider-catalog-direct", f"第 {attempt}/{attempts} 次读取供应商官方目录", error)
                if attempt < attempts:
                    self.action_wait(name, "provider-catalog-direct", "供应商官方目录重试",
                                     f"{delay} 秒后进行第 {attempt + 1}/{attempts} 次只读查询")
                    time.sleep(delay)
        raise RuntimeError("供应商官方目录连续失败: " + provider["id"])

    def probe_chat_candidate(self, provider, upstream, probe_index, probe_total, attempts=1, delay=3,
                             action="真实模型候选探测", timeout=180):
        """用途：直接调用供应商探测一个真实聊天模型；参数为供应商、模型 ID、候选进度、重试次数、间隔、日志动作和超时，返回是否取得非空回答和非零 usage；正文为空且因长度截断时下一次提高输出上限至 1024，每次尝试保存无正文诊断，有限重试耗尽后淘汰候选且不污染正式验收 checks。"""
        protocol = "bypass_openai_chat"
        base = os.environ.get(provider["base_env"], provider["base"]).rstrip("/")
        label = f"{action} {probe_index}/{probe_total}"
        max_tokens = 32
        for attempt in range(1, attempts + 1):
            attempt_label = label if attempts == 1 else f"{label}，尝试 {attempt}/{attempts}"
            self.action_start(upstream, protocol, attempt_label,
                              f"供应商={provider['id']}，直接调用供应商聊天端点，超时={timeout}秒，max_tokens={max_tokens}")
            try:
                status, reply, _ = curl_json(
                    base, "/chat/completions" if base.endswith("/v1") else "/v1/chat/completions",
                    {"model": upstream, "messages": [{"role": "user",
                     "content": "Reply only OK. probe " + self.run}], "max_tokens": max_tokens},
                    os.environ[provider["key_env"]], timeout=timeout)
            except Exception as error:
                self.report.setdefault("probe_attempts", []).append({"name": "real-model-probe", "provider": provider["id"],
                    "model": upstream, "attempt": attempt, "attempts": attempts, "status": None,
                    "error": type(error).__name__, "max_tokens": max_tokens, "passed": False})
                self.action_fail(upstream, protocol, attempt_label, error)
                if attempt < attempts:
                    self.action_wait(upstream, protocol, label,
                                     f"{delay} 秒后重试当前真实模型，随后仍失败则继续下一候选")
                    time.sleep(delay)
                continue
            result = chat_probe_result(status, reply)
            self.report.setdefault("probe_attempts", []).append({"name": "real-model-probe", "provider": provider["id"],
                "model": upstream, "attempt": attempt, "attempts": attempts, "status": status,
                "max_tokens": max_tokens, **result})
            detail = (f"HTTP={status}，输入={result['prompt_tokens']}，输出={result['completion_tokens']}，"
                      f"结束原因={result['finish_reason']}，正文字符数={result['content_chars']}，"
                      f"推理字符数={result['reasoning_chars']}，响应错误={result['response_error']}")
            if result["passed"]:
                self.action_ok(upstream, protocol, attempt_label, detail, record=False)
                return True
            self.action_fail(upstream, protocol, attempt_label, RuntimeError(detail))
            if attempt < attempts:
                # 推理模型的输出预算可能先被思考耗尽；仅在上游明确截断时扩容，仍要求最终回答，不能把已计费当作成功。
                truncated = status == 200 and result["finish_reason"] == "length" and result["content_chars"] == 0 and not result["response_error"]
                if truncated:
                    max_tokens = 1024
                self.action_wait(upstream, protocol, label,
                                 ("正文为空且输出被长度限制截断，提高 max_tokens 至 1024；" if truncated else "") +
                                 f"{delay} 秒后重试当前真实模型，随后仍失败则继续下一候选")
                time.sleep(delay)
        return False

    def select_lifecycle_model(self, provider, candidates, attempts=2, delay=3):
        """用途：为模型生命周期验收选择额外的真实聊天模型；参数为供应商、去重候选、单候选重试次数和间隔，返回首个真实响应且 usage 非零的模型 ID；单候选失败会继续，全部失败时明确报错。"""
        total = len(candidates)
        for index, candidate in enumerate(candidates, 1):
            if self.probe_chat_candidate(provider, candidate, index, total, attempts=attempts, delay=delay,
                                         action="生命周期探测", timeout=120):
                return candidate
        raise RuntimeError(f"{total} 个额外真实聊天模型均未通过生命周期探测")

    def deployment(self, definition, provider, upstream=None, public_name=None, fault=False, temporary=False, route_tag=None):
        """用途：按真实模型定义注册部署；参数为定义、供应商、可选上游模型、可选公开模型名、故障开关、临时标记和观察标签，返回部署 ID；允许两个供应商的精确上游 ID 归入同一公开模型，标签区分部署，临时标记供异常退出清理。"""
        rates = self.data["billing"]
        upstream = upstream or definition["upstream_model"]
        public_name = public_name or (definition.get("public_name") if upstream == definition.get("upstream_model") else upstream) or upstream
        stored_model = definition.get("stored_model", upstream) if upstream == definition.get("upstream_model") else upstream
        info = {"transport": definition["transport"], "endpoint_types": definition["endpoint_types"]}
        params = {"model": stored_model, "custom_llm_provider": "openai",
                  "litellm_credential_name": provider["id"]}
        if definition["kind"] == "chat":
            params.update(max_tokens=self.data["limits"]["max_output_tokens"],
                          input_cost_per_token=rates["input_cost_per_token"],
                          output_cost_per_token=rates["output_cost_per_token"])
            info["pricing_source"] = "manual"
        else:
            info.update(pricing_source="catalog", base_model=definition["base_model"])
        if self.observer and definition["kind"] == "chat":
            tag = "fault" if fault else route_tag
            params["api_base"] = self.observer.base + "/" + provider["id"] + ("/" + tag if tag else "")
        self.action_start(public_name, definition["transport"], "添加模型与端点",
                          f"供应商连接={provider['id']}，上游模型={stored_model}")
        row, _ = self.api("/model/new", {"model_name": public_name, "litellm_params": params, "model_info": info})
        ident = row["model_info"]["id"]
        saved = {"id": ident, "public_name": public_name, "provider": provider["id"], "model": stored_model,
                 "kind": definition["kind"], "transport": definition["transport"],
                 "endpoint_types": definition["endpoint_types"], "observed": definition["kind"] == "chat",
                 "route_tag": "fault" if fault else route_tag,
                 "temporary_fault": fault, "temporary": temporary or fault or bool(route_tag)}
        self.state["deployments"].append(saved)
        self.save()
        self.action_ok(public_name, definition["transport"], "添加模型与端点", f"部署 ID={ident}")
        return ident

    def template(self, name, ids, profile, **scope):
        """用途：实例化清单整体路由文档；参数为名称/部署ID/策略/作用域，返回模板ID；阈值零禁用被动熔断，避免用例互相污染。"""
        body = resolve_document(self.data["routing"]["templates"][profile], {
            "shared_chat_model": self.data["shared_chat_model"]["public_name"],
            "fenno_deployment": ids[0], "qiniu_deployment": ids[1]})
        row, _ = self.api("/route_template/new", {"name": name, "body": body, **scope})
        return row["id"]

    def seed(self):
        """用途：仅通过管理 API 构建清单声明的保留基线；无参数、无返回值；供 testdata 和验收准备调用，要求空租户库，不读取供应商目录、不探测或运行模型，异常保留检查点。"""
        login, _ = self.api("/v2/login", {"username": os.environ.get("E2E_DATASET_ADMIN", "admin"),
                                          "password": os.environ["E2E_DATASET_PASSWORD"]})
        self.admin = login["key"]
        self.progress("  [构建 1/5] 管理员登录成功，检查空数据库")
        orgs, _ = self.api("/organization/list")
        if orgs:
            raise RuntimeError("数据集要求空租户环境；已有组织时拒绝覆盖，请使用隔离运行入口")
        selected, ids = {}, []
        self.state["chat_models"] = {}
        self.progress("  [构建 2/5] 先添加 2 个真实模型提供商连接")
        for provider in self.data["providers"]:
            self.action_start(provider["name"], "provider", "添加模型提供商", f"连接名={provider['id']}")
            self.api("/credentials", {"credential_name": provider["id"],
                "credential_info": {"catalog_id": provider["id"], "custom_llm_provider": "openai", "provider_id": "OpenAI"},
                "credential_values": {"api_base": os.environ.get(provider["base_env"], provider["base"]),
                                      "api_key": "os.environ/" + provider["key_env"]}})
            listed, _ = self.api("/credentials")
            if provider["id"] not in {row.get("credential_name") for row in listed.get("credentials", [])}:
                raise AssertionError("模型提供商连接未持久化: " + provider["id"])
            self.action_ok(provider["name"], "provider", "添加模型提供商", "连接已持久化，凭据值保持脱敏")
        self.progress("  [构建 3/5] 按清单添加模型与端点，仅保存配置，不调用供应商")
        for provider in self.data["providers"]:
            chat_definition_id = {"fennoai": "fenno-chat", "qiniu": "qiniu-chat"}[provider["id"]]
            definition = next(row for row in self.data["models"] if row["id"] == chat_definition_id)
            found = definition["upstream_model"]
            selected[provider["id"]] = {"provider": provider, "model": found}
            self.state["chat_models"][provider["id"]] = self.data["shared_chat_model"]["public_name"]
            ids.append(self.deployment(definition, provider, found, self.data["shared_chat_model"]["public_name"]))
            self.report["models"].append({"provider": provider["id"], "upstream_model": found, "source": "manifest"})
            required = [row for row in self.data["models"] if row["provider"] == provider["id"]
                        and row["id"] not in ("fenno-chat", "qiniu-chat")]
            for definition in required:
                self.deployment(definition, provider)
                self.report["models"].append({"provider": provider["id"], "upstream_model": definition["upstream_model"],
                                               "kind": definition["kind"], "source": "manifest"})
        self.api("/model/default", {"model_name": self.data["shared_chat_model"]["public_name"],
            "weights": {"allocations": [
                {"deployment_id": ids[0], "weight": self.data["shared_chat_model"]["default_weights"][0]},
                {"deployment_id": ids[1], "weight": self.data["shared_chat_model"]["default_weights"][1]}]}}, method="PUT")
        groups, _ = self.api("/model/groups?size=200")
        group_rows = groups.get("data", groups if isinstance(groups, list) else [])
        for deployment in self.state["deployments"]:
            group = next((row for row in group_rows if row.get("model_name") == deployment["public_name"]), None)
            if not group:
                raise AssertionError("模型分组未显示真实模型 ID: " + deployment["public_name"])
            actual = next((row for row in group.get("deployments", [])
                           if row.get("id") == deployment["id"]), None)
            if not actual:
                raise AssertionError("模型分组缺少部署: " + deployment["id"])
            params, info = actual.get("litellm_params", {}), actual.get("model_info", {})
            expected = {
                "公开模型名": (actual.get("model_name"), deployment["public_name"]),
                "供应商连接": (params.get("litellm_credential_name"), deployment["provider"]),
                "上游模型": (params.get("model"), deployment["model"]),
                "传输协议": (info.get("transport"), deployment["transport"]),
                "用户入口": (info.get("endpoint_types"), deployment["endpoint_types"]),
            }
            wrong = [name for name, (got, wanted) in expected.items() if got != wanted]
            if wrong:
                raise AssertionError(deployment["public_name"] + " 部署结构错误: " + ",".join(wrong))
            self.action_ok(deployment["public_name"], deployment["transport"], "结构核对",
                           f"公开名={deployment['public_name']}，连接={deployment['provider']}，"
                           f"上游={deployment['model']}，入口={','.join(deployment['endpoint_types'])}")
        self.progress("  [构建 4/5] 创建独立真实模型路由及组织/团队/个人结构")
        fenno = self.template("route-fennoai-only", ids, "fenno-only")
        qiniu = self.template("route-qiniu-only", ids, "qiniu-only")
        allowed_models = list(dict.fromkeys(deployment["public_name"] for deployment in self.state["deployments"]))
        password = "Acceptance-" + secrets.token_urlsafe(18)
        self.state["member_password"] = password
        expanded = hierarchy(self.data)
        self.progress("    创建 3 个组织、9 个团队、27 名成员、9 个项目和 81 把密钥")
        for org in self.data["organizations"]:
            row, _ = self.api("/organization/new", {"organization_alias": org["name"], "max_budget": self.data["budgets"]["organization"]})
            oid = row["organization_id"]
            self.state["organizations"].append({"id": oid, "name": org["name"]})
            ot = self.template(org["slug"] + "-organization-models", ids, "organization-default", organization_id=oid)
            self.api("/route_template/binding", {"scope": "organization", "scope_id": oid, "route_template_id": ot})
            for ti, team in enumerate(self.data["teams"]):
                members = [m for m in expanded if m["organization"]["slug"] == org["slug"] and m["team"]["slug"] == team["slug"]]
                first = members[0]
                user, _ = self.api("/user/new", {"user_email": first["email"], "user_alias": first["name"], "password": password,
                    "user_role": "user", "max_budget": 10, "admin_organization_ids": [oid] if ti == 0 else []})
                teamrow, _ = self.api("/team/new", {"organization_id": oid, "team_alias": team["name"],
                    "admin_user_id": user["user_id"], "models": allowed_models, "max_budget": 30})
                tid = teamrow["team_id"]
                self.state["teams"].append({"id": tid, "organization_id": oid, "name": team["name"]})
                if ti:
                    tt = self.template(org["slug"] + "-" + team["slug"] + "-team-models", ids, "team-default", organization_id=oid, team_id=tid)
                    self.api("/route_template/binding", {"scope": "team", "scope_id": tid, "route_template_id": tt})
                project, _ = self.api("/project/new", {"team_id": tid, "project_alias": self.data["project_profiles"][ti]["name"], "max_budget": self.data["budgets"]["project"]})
                pid = project["project_id"]
                self.state["projects"].append({"id": pid, "team_id": tid})
                for mi, member in enumerate(members):
                    if mi:
                        user, _ = self.api("/user/new", {"user_email": member["email"], "user_alias": member["name"],
                            "password": password, "user_role": "user", "team_id": tid, "team_role": "user", "max_budget": 10})
                    uid = user["user_id"]
                    self.state["users"].append({"id": uid, "email": member["email"], "name": member["name"], "team_id": tid,
                        "organization_id": oid, "organization_slug": org["slug"], "team_slug": team["slug"], "member_index": mi})
                    for profile in self.data["key_profiles"]:
                        body = {"key_alias": org["name"] + "/" + team["name"] + "/" + member["name"] + "/" + profile,
                            "owner_type": "personal", "user_id": uid, "team_id": tid, "project_id": pid,
                            "models": allowed_models, "max_budget": 3}
                        if profile != "inherit":
                            body["route_template_id"] = fenno if profile == "fenno-only" else qiniu
                        key, _ = self.api("/key/generate", body)
                        call_model = self.data["shared_chat_model"]["public_name"]
                        self.state["keys"].append({"key": key["key"], "token_id": key["token_id"], "profile": profile,
                            "user_id": uid, "team_id": tid, "project_id": pid, "organization_id": oid, "team_index": ti})
                        self.state["keys"][-1]["call_model"] = call_model
                        self.save()
                self.progress(f"    [{org['name']}/{team['name']}] 已创建 3 名成员、1 个项目、9 把密钥；累计团队={len(self.state['teams'])}/9 密钥={len(self.state['keys'])}/81")
            self.progress(f"    [{org['name']}] 组织构建完成；累计成员={len(self.state['users'])}/27")
        self.progress(f"  [构建 5/5] 创建 {len(self.data['guardrails'])} 条护栏并保存数据集检查点")
        for guardrail in self.data["guardrails"]:
            self.api("/guardrails", {k: v for k, v in guardrail.items() if k != "acceptance"})
        # 管理员额外拥有一把无团队的个人虚拟密钥；与 81 把成员密钥分开保存，保持成员会话矩阵稳定。
        admin_key, _ = self.api("/key/generate", {"owner_type": "personal", "user_id": login["user_id"],
            "key_alias": "管理员个人密钥", "models": allowed_models, "max_budget": 3})
        self.state["admin_key"] = admin_key
        self.state["selected"] = selected
        self.report.update(status="seeded", phase="seed", admin_key_count=1, total_key_count=82)
        self.save()

    def wait_log(self, call_id, require_cost=False):
        """用途：等待请求日志详情持久化；参数为请求 ID 和是否要求价格快照，返回完整日志；媒体创建日志允许零费用，正式结算必须含价格证据。"""
        end = time.monotonic() + 90
        while time.monotonic() < end:
            status, bill, _ = http(self.gateway, "/spend/logs/ui/" + call_id, token=self.admin)
            if status == 200 and (not require_cost or bill.get("metadata", {}).get("cost_breakdown")):
                return bill
            time.sleep(0.3)
        requirement = "及价格快照" if require_cost else ""
        raise AssertionError("请求日志未持久化" + requirement + ": " + call_id)

    def wait_bill(self, call_id):
        """用途：等待可计费请求完整落库；参数为请求 ID，返回含价格快照的账单；供聊天、图片和终态结算验收使用。"""
        return self.wait_log(call_id, require_cost=True)

    def call_checkpoint(self, key):
        """用途：复验某把密钥已经落盘的真实聊天回执；参数为密钥记录，返回有效回执或空；调用方据此安全续跑，静态身份、模型、响应、usage、五级归属和价格快照任一不符都不跳过。"""
        candidates = [row for row in self.report.get("calls", []) if row.get("key_id") == key.get("token_id")]
        if len(candidates) != 1:
            return None
        row = candidates[0]
        if row.get("profile") != key.get("profile") or row.get("model") != key.get("call_model"):
            return None
        call_id, marker = row.get("call_id"), row.get("marker")
        if not call_id or not marker:
            return None
        try:
            bill, _, status = self.api("/spend/logs/ui/" + call_id, expected=(200, 404), include_status=True)
            if status != 200 or bill.get("model") != key.get("call_model"):
                return None
            cost = bill_check(bill, key, self.data["billing"])
            response, messages = bill.get("response", {}), bill.get("messages", [])
            usage = response.get("usage", {}) if isinstance(response, dict) else {}
            if not response.get("choices") or marker not in json.dumps(messages, ensure_ascii=False):
                return None
            if any(int(bill.get(field, 0)) != int(usage.get(field, -1))
                   for field in ("prompt_tokens", "completion_tokens")):
                return None
            if any(int(row.get(field, 0)) != int(bill.get(field, -1))
                   for field in ("prompt_tokens", "completion_tokens")):
                return None
            if not math.isclose(float(row.get("cost", -1)), cost, rel_tol=0, abs_tol=1e-9):
                return None
            return row
        except (AssertionError, TypeError, ValueError):
            return None

    def record_call(self, key, marker, call_id, model, bill, cost):
        """用途：原子替换一把密钥的聊天验收检查点；参数为密钥、标记、请求、模型、账单和金额，返回回执；每次真实成功核账后立即保存，后续失败不会丢失已完成项。"""
        row = {"call_id": call_id, "marker": marker, "key_id": key["token_id"],
               "profile": key["profile"], "model": model,
               "prompt_tokens": bill["prompt_tokens"],
               "completion_tokens": bill["completion_tokens"], "cost": cost}
        self.report["calls"] = [item for item in self.report.get("calls", [])
                                  if item.get("key_id") != key["token_id"]] + [row]
        self.save()
        return row

    def call(self, key, marker, extra="", model=None, expected=200, guardrails=None, defer_bill=False,
             attempts=5, retry_delay=3, rate_limit_delay=65):
        """用途：个人密钥真实请求对账；参数含标记/模型/状态/护栏/延迟对账及临时错误重试策略，返回正文或四元回执；429 先等待供应商窗口恢复，其他临时错误短退避，失败不计成功证据。"""
        model = model or key.get("call_model") or self.state["chat_models"]["fennoai"]
        body = {"model": model, "messages": [{"role": "user", "content": "Reply only OK. " + marker + " " + extra}], "max_tokens": 32}
        if guardrails is not None:
            body["guardrails"] = guardrails
        if expected == 200:
            retryable = (429, 502, 503, 504)
            for attempt in range(1, attempts + 1):
                self.action_start(model, "bypass_openai_chat", f"第 {attempt}/{attempts} 次真实 AI 请求",
                                  f"profile={key.get('profile', 'temporary')}，预期 HTTP=200")
                answer, headers, status = self.api("/v1/chat/completions", body, token=key["key"],
                                                    expected=(200, *retryable), include_status=True)
                if status == 200:
                    break
                error = answer.get("error", answer) if isinstance(answer, dict) else {}
                summary = (str(error.get("message") or error.get("code") or "无错误正文")
                           if isinstance(error, dict) else str(error))[:300]
                self.action_fail(model, "bypass_openai_chat", f"第 {attempt}/{attempts} 次真实 AI 请求",
                                 RuntimeError(f"HTTP={status}，上游临时错误={summary}"))
                if attempt < attempts:
                    wait_seconds = temporary_retry_delay(summary, retry_delay, rate_limit_delay, attempt)
                    self.action_wait(model, "bypass_openai_chat", "真实 AI 请求重试",
                                     f"{wait_seconds} 秒后进行第 {attempt + 1}/{attempts} 次")
                    time.sleep(wait_seconds)
            else:
                raise AssertionError(f"{model} 连续 {attempts} 次真实请求均遇到临时上游错误")
        else:
            self.action_start(model, "bypass_openai_chat", "真实 AI 请求",
                              f"profile={key.get('profile', 'temporary')}，预期 HTTP={expected}")
            answer, headers = self.api("/v1/chat/completions", body, token=key["key"], expected=expected)
        if expected == 200:
            call_id = next((v for k, v in headers.items() if k.lower() == "x-litellm-call-id"), None)
            if not call_id or not answer.get("choices"):
                raise AssertionError("真实响应缺少 request_id 或 choices")
            usage = answer.get("usage", {})
            if int(usage.get("prompt_tokens", 0)) <= 0 or int(usage.get("completion_tokens", 0)) <= 0:
                raise AssertionError("真实响应必须包含非零输入和输出 usage")
            if self.observer:
                observed = [r for r in self.observer.rows if marker in r["messages"] and r["status"] == 200]
                if observed and observed[-1].get("usage") and any(observed[-1]["usage"].get(k) != answer.get("usage", {}).get(k)
                        for k in ("prompt_tokens", "completion_tokens")):
                    raise AssertionError("网关响应计量与真实供应商不一致")
            if defer_bill:
                self.action_ok(model, "bypass_openai_chat", "真实 AI 响应",
                               f"输入={answer.get('usage', {}).get('prompt_tokens', 0)}，输出={answer.get('usage', {}).get('completion_tokens', 0)}，等待批量计量核对", record=False)
                return (key, marker, call_id, answer.get("usage", {}))
            bill = self.wait_bill(call_id)
            if any(bill.get(k) != answer.get("usage", {}).get(k) for k in ("prompt_tokens", "completion_tokens")):
                raise AssertionError("账单计量与真实响应用量不一致")
            cost = bill_check(bill, key, self.data["billing"])
            self.record_call(key, marker, call_id, model, bill, cost)
            self.action_ok(model, "bypass_openai_chat", "请求日志、计量与计费",
                           f"输入={bill['prompt_tokens']}，输出={bill['completion_tokens']}，金额={cost:.9f}")
        else:
            self.action_ok(model, "bypass_openai_chat", "本地拒绝且无上游调用", f"HTTP={expected}")
        return answer

    def chat_request(self, key, body, expected=200):
        """用途：发送正文完全固定的聊天请求；参数为密钥、请求正文和期望状态，返回正文、响应头、状态和可空请求 ID；成功响应必须具有 choices 与非零 usage，供缓存重放、权重和回退链使用。"""
        answer, headers, status = self.api("/v1/chat/completions", body, token=key["key"],
                                            expected=expected, include_status=True)
        call_id = header_value(headers, "x-litellm-call-id")
        if status == 200:
            usage = answer.get("usage", {}) if isinstance(answer, dict) else {}
            if not call_id or not answer.get("choices"):
                raise AssertionError("真实响应缺少 request_id 或 choices")
            if int(usage.get("prompt_tokens", 0)) <= 0 or int(usage.get("completion_tokens", 0)) <= 0:
                raise AssertionError("真实响应必须包含非零输入和输出 usage")
        return answer, headers, status, call_id

    def codex_conversation(self, key, revalidate=False):
        """用途：执行或复验一把个人密钥的 Codex 三轮 Responses 会话；参数为密钥及只复验开关，返回完整会话证据或空；逐轮检查上下文、会话头、五级归属、usage 和价格，完整成功后保存，失败不写完整成功回执。"""
        config = self.data["agent_conversation"]
        saved = next((row for row in self.report.get("agent_conversations", [])
                      if row.get("key_id") == key["token_id"] and row.get("model") == key["call_model"]), None)
        if revalidate:
            if not saved or saved.get("agent_type") != "codex" or saved.get("model") != key["call_model"] or saved.get("profile") != key["profile"] or len(saved.get("turns", [])) != config["turns"]:
                return None
            try:
                previous = None
                ids = set()
                response_ids = set()
                for index, turn in enumerate(saved["turns"], 1):
                    if turn["call_id"] in ids or turn["response_id"] in response_ids or turn["turn"] != index or turn["previous_response_id"] != previous:
                        return None
                    bill, _, status = self.api("/spend/logs/ui/" + turn["call_id"], expected=(200, 404), include_status=True)
                    if status != 200:
                        return None
                    self.check_codex_turn(key, saved, turn, bill)
                    ids.add(turn["call_id"])
                    response_ids.add(turn["response_id"])
                    previous = turn["response_id"]
                return saved
            except (AssertionError, AttributeError, KeyError, TypeError, ValueError):
                return None
        session = "codex-" + acceptance_run_id(18)
        marker = "project-" + acceptance_run_id(12)
        conversation = {"agent_type": "codex", "session_id": session, "marker": marker,
                        "key_id": key["token_id"], "profile": key["profile"],
                        "model": key["call_model"], "turns": []}
        prompts = ["Remember the project marker " + marker + ". Reply with only that marker.",
                   "What project marker did I give you? Reply with only the marker.",
                   "Confirm the same project marker once more. Reply with only the marker."]
        previous = None
        provider = None
        for index, prompt in enumerate(prompts, 1):
            self.progress(f"    [会话第 {index}/3 轮] {key['call_model']} / {key['profile']}：" +
                          ("建立上下文" if index == 1 else "续接历史、核对上下文与独立账单"))
            body = {"model": key["call_model"], "instructions": config["instructions"],
                    "input": [{"role": "user", "content": prompt}], "store": False,
                    "stream": False, "max_output_tokens": config["max_output_tokens"]}
            if previous:
                body["previous_response_id"] = previous
            start = len(self.observer.rows) if self.observer else 0
            answer, headers = self.api(config["endpoint"], body, token=key["key"], request_headers={
                "User-Agent": config["user_agent"], "Session_id": session, "Originator": "codex_cli_rs"})
            call_id = header_value(headers, "x-litellm-call-id")
            if (not call_id or not answer.get("id")
                    or any(row["call_id"] == call_id or row["response_id"] == answer["id"]
                           for row in conversation["turns"])):
                raise AssertionError("Codex 第 %d 轮缺少独立请求或响应 ID" % index)
            bill = self.wait_bill(call_id)
            turn = {"turn": index, "call_id": call_id, "response_id": answer["id"],
                    "previous_response_id": previous, "prompt": prompt,
                    "request_body": body,
                    "prompt_tokens": bill["prompt_tokens"], "completion_tokens": bill["completion_tokens"],
                    "cost": bill_check(bill, key, self.data["billing"])}
            self.check_codex_turn(key, conversation, turn, bill, answer)
            if self.observer:
                observed = self.observed_since(start, marker)
                successful = [row for row in observed if row["status"] == 200 and row.get("forwarded")]
                if len(successful) != 1:
                    raise AssertionError("Codex 每轮必须具有一次真实供应商转发证据")
                row = successful[0]
                messages = json.loads(row["messages"])
                if len(messages) != 2 * index or messages[0].get("role") != "system" or messages[-1].get("content") != prompt:
                    raise AssertionError("Codex 上游历史条数、指令或当前输入错误")
                for prior_index, prior in enumerate(conversation["turns"]):
                    if messages[2 * prior_index + 1].get("content") != prior["prompt"] or marker not in messages[2 * prior_index + 2].get("content", ""):
                        raise AssertionError("Codex 上游丢失前轮用户或助手历史")
                if provider is not None and provider != row["provider"]:
                    raise AssertionError("Codex 同一会话未保持供应商粘滞")
                if any(int(row.get("usage", {}).get(field, -1)) != int(turn[field])
                       for field in ("prompt_tokens", "completion_tokens")):
                    raise AssertionError("Codex 网关计量与真实供应商不一致")
                provider = row["provider"]
                expected_provider = {"fenno-only": "fennoai", "qiniu-only": "qiniu"}.get(key["profile"])
                if expected_provider and provider != expected_provider:
                    raise AssertionError("Codex 会话违反单供应商路由配置")
            conversation["turns"].append(turn)
            self.progress(f"    [会话第 {index}/3 轮通过] 上下文、usage、归属、费用与供应商粘滞已核对")
            previous = answer["id"]
        conversation["provider"] = provider
        self.report["agent_conversations"] = [row for row in self.report.get("agent_conversations", [])
                                               if (row.get("key_id"), row.get("model")) != (key["token_id"], key["call_model"])] + [conversation]
        self.save()
        return conversation

    def check_codex_turn(self, key, conversation, turn, bill, answer=None):
        """用途：核对 Codex 一轮持久化证据；参数为身份、会话、轮次、账单和可选实时响应，无返回值；执行及断点复验共用，缺失上下文、客户端头、关联或计费时抛断言，无写入副作用。"""
        response = bill.get("response", {})
        if not conversation.get("session_id") or not conversation.get("marker") or not isinstance(response, dict):
            raise AssertionError("Codex 会话或响应证据缺失")
        usage = response.get("usage", {})
        text = "".join(part.get("text", "") for item in response.get("output", [])
                       for part in item.get("content", []) if part.get("type") == "output_text")
        proxy = bill.get("proxy_server_request", {})
        body = proxy.get("body", {})
        headers = proxy.get("headers", {})
        if bill.get("session_id") != conversation["session_id"] or bill.get("model") != key["call_model"]:
            raise AssertionError("Codex 会话或模型归属错误")
        if response.get("id") != turn["response_id"] or response.get("status") != "completed" or conversation["marker"] not in text or response.get("error"):
            raise AssertionError("Codex 第 %d 轮响应未完成或无法记住前轮项目标记：status=%s，response_id=%s，text=%s" %
                                 (turn["turn"], response.get("status"), response.get("id"), text[:160]))
        if answer is not None and answer != response:
            raise AssertionError("Codex 持久化响应与客户端收到的响应不一致")
        if not str(header_value(headers, "user-agent") or "").startswith("codex_cli_rs/") or header_value(headers, "session_id") != conversation["session_id"] or header_value(headers, "originator") != "codex_cli_rs":
            raise AssertionError("Codex 客户端身份或稳定会话头缺失")
        request = turn.get("request_body", {})
        if (request.get("input") != [{"role": "user", "content": turn["prompt"]}]
                or request.get("previous_response_id") != turn["previous_response_id"]
                or request.get("store") is not False or request.get("stream") is not False
                or request.get("model") != key["call_model"]
                or request.get("instructions") != self.data["agent_conversation"]["instructions"]):
            raise AssertionError("Codex 请求必须只发送本轮输入并正确续接响应")
        # 网关日志记录经过历史恢复与护栏处理的请求，因此必须验证完整历史；
        # 客户端增量输入另外保存在本轮检查点，不把两种正文混为同一协议。
        inputs = body.get("input", [])
        if (not isinstance(inputs, list) or len(inputs) != 2 * turn["turn"] - 1
                or inputs[-1] != request["input"][0] or "previous_response_id" in body
                or "store" in body or body.get("model") != key["call_model"]
                or body.get("instructions") != request["instructions"]):
            raise AssertionError("Codex 持久化请求未恢复完整历史或泄漏代理字段")
        for index, prior in enumerate(conversation["turns"][:turn["turn"] - 1]):
            if (inputs[index * 2] != {"role": "user", "content": prior["prompt"]}
                    or inputs[index * 2 + 1].get("role") != "assistant"
                    or conversation["marker"] not in json.dumps(inputs[index * 2 + 1].get("content", ""))):
                raise AssertionError("Codex 持久化请求丢失前轮上下文")
        for stored, native in (("prompt_tokens", "input_tokens"), ("completion_tokens", "output_tokens")):
            if int(bill.get(stored, 0)) != int(usage.get(native, -1)) or bill.get(stored) != turn[stored]:
                raise AssertionError("Codex 逐轮 usage 与持久化账单不一致")
        if not math.isclose(bill_check(bill, key, self.data["billing"]), turn["cost"], rel_tol=0, abs_tol=1e-9):
            raise AssertionError("Codex 逐轮费用与检查点不一致")

    def verify_codex_agents(self):
        """用途：生成或严格复验 Codex 三轮数据；无参数/返回，由模型验收调用；共享入口每种路由选一把代表，三轮请求必须独立，保留全部基线和历史会话。"""
        if not self.state["keys"]:
            raise AssertionError("Codex 验收不能使用空密钥列表")
        executed = 0
        ids = set()
        keys = self.acceptance_keys()
        for index, key in enumerate(keys, 1):
            with self.case("gpt-" + str(index)) as result:
                conversation = self.codex_conversation(key, revalidate=True)
                if conversation is None:
                    conversation = self.codex_conversation(key)
                    executed += 1
                else:
                    result["status"] = "revalidated"
                for turn in conversation["turns"]:
                    if turn["call_id"] in ids:
                        raise AssertionError("Codex 不同会话复用了请求 ID")
                    ids.add(turn["call_id"])
        self.report["acceptance_key_ids"] = [key["token_id"] for key in keys]
        self.report["checks"] = [row for row in self.report["checks"] if row.get("name") != "codex-agent-multi-turn"]
        self.report["checks"].append({"name": "codex-agent-multi-turn", "agent_type": "codex",
                                      "sessions": len(keys), "requests": len(ids),
                                      "executed_this_run": executed, "passed": True})
        self.save()

    def observed_since(self, start, marker):
        """用途：读取某一步之后属于固定请求标记的观察记录；参数为起始下标和标记，返回匹配行副本；供断言缓存未外发、权重命中与回退顺序。"""
        if not self.observer:
            return []
        with self.observer.lock:
            return [dict(row) for row in self.observer.rows[start:] if marker in row.get("messages", "")]

    def verify_cache_weight_chain(self, key):
        """用途：组合验证固定请求缓存、跨密钥隔离、封禁权限和 100:0 到 0:100 权重切换；参数为基线密钥，返回无；创建两部署、一路由和两把临时密钥，finally 全部删除。"""
        model = self.state["chat_models"]["fennoai"]
        protocol = "bypass_openai_chat"
        provider = self.state["selected"]["fennoai"]["provider"]
        definition = next(row for row in self.data["models"] if row["id"] == "fenno-chat")
        weights = self.data["logic_chains"]["weighted_route_switch"]
        timeout = self.data["routing"]["timeout_seconds"]
        deployment_ids, route_id, temporary_keys = [], None, []
        try:
            first_id = self.deployment(definition, provider, model, temporary=True, route_tag="weight-a")
            second_id = self.deployment(definition, provider, model, temporary=True, route_tag="weight-b")
            deployment_ids.extend((first_id, second_id))
            initial = weighted_acceptance_route(model, first_id, second_id,
                                                weights["initial_weights"], timeout)
            route, _ = self.api("/route_template/new", {
                "name": "real-acceptance-cache-weight-" + self.run, "body": initial})
            route_id = route["id"]
            for suffix in ("primary", "isolation"):
                created, _ = self.api("/key/generate", {"key_alias": "real-cache-weight-" + suffix,
                    "owner_type": "personal", "user_id": key["user_id"], "team_id": key["team_id"],
                    "project_id": key["project_id"], "models": [model], "route_template_id": route_id})
                temporary_keys.append({**key, **created, "profile": "logic-chain", "call_model": model})
            primary, isolated = temporary_keys
            marker = "cache-weight-" + self.run
            body = {"model": model, "messages": [{"role": "user",
                    "content": "Reply only OK. " + marker}], "max_tokens": 32}

            before = len(self.observer.rows)
            self.action_start(model, protocol, "缓存首次未命中与权重 100:0",
                              "固定正文首次请求，要求只调用 weight-a 的真实上游")
            first, first_headers, _, first_call = self.chat_request(primary, body)
            rows = self.observed_since(before, marker)
            if (response_cache_hit(first_headers) or len(rows) != 1
                    or rows[0].get("route_tag") != "weight-a" or rows[0].get("status") != 200):
                raise AssertionError("首次请求没有按 100:0 只进入 weight-a 真实部署")
            first_bill = self.wait_bill(first_call)
            first_cost = bill_check(first_bill, primary, self.data["billing"])
            self.action_ok(model, protocol, "缓存首次未命中与权重 100:0",
                           f"cache_hit=false，weight-a 上游=1，weight-b 上游=0，金额={first_cost:.9f}")

            before = len(self.observer.rows)
            second, second_headers, _, second_call = self.chat_request(primary, body)
            if not response_cache_hit(second_headers) or self.observed_since(before, marker):
                raise AssertionError("相同请求没有由缓存回答或仍发生真实上游调用")
            if second != first or second_call == first_call:
                raise AssertionError("缓存响应不一致或缓存请求没有独立 call ID")
            second_bill = self.wait_bill(second_call)
            cache_bill_check(second_bill, primary, first["usage"])
            self.action_ok(model, protocol, "相同请求缓存命中与响应一致",
                           "响应 JSON 与首次完全一致，上游增量=0，独立请求日志 cache_hit=true，spend=0，usage 一致")

            before = len(self.observer.rows)
            _, isolated_headers, _, isolated_call = self.chat_request(isolated, body)
            rows = self.observed_since(before, marker)
            if response_cache_hit(isolated_headers) or len(rows) != 1 or rows[0].get("route_tag") != "weight-a":
                raise AssertionError("另一把密钥错误复用了第一把密钥的缓存")
            isolated_cost = bill_check(self.wait_bill(isolated_call), isolated, self.data["billing"])
            self.action_ok(model, protocol, "跨密钥缓存隔离",
                           f"第二把密钥 cache_hit=false，weight-a 真实上游=1，独立计费={isolated_cost:.9f}")

            self.api("/key/block", {"key": primary["key"]})
            before = len(self.observer.rows)
            _, blocked_headers, blocked_status, _ = self.chat_request(primary, body, expected=401)
            if response_cache_hit(blocked_headers) or self.observed_since(before, marker):
                raise AssertionError("封禁密钥读取了缓存或发生上游调用")
            self.action_ok(model, protocol, "封禁权限先于缓存读取",
                           f"HTTP={blocked_status}，cache_hit=false，上游增量=0")
            self.api("/key/unblock", {"key": primary["key"]})

            updated = weighted_acceptance_route(model, first_id, second_id,
                                                weights["updated_weights"], timeout)
            self.api("/route_template/" + route_id + "/update", {"body": updated})
            before = len(self.observer.rows)
            switched, switched_headers, _, switched_call = self.chat_request(primary, body)
            rows = self.observed_since(before, marker)
            if response_cache_hit(switched_headers) or len(rows) != 1 or rows[0].get("route_tag") != "weight-b":
                raise AssertionError("路由更新未使旧缓存失效并切换到 weight-b")
            switched_cost = bill_check(self.wait_bill(switched_call), primary, self.data["billing"])
            self.action_ok(model, protocol, "路由更新使缓存失效并切换权重 0:100",
                           f"cache_hit=false，weight-a 上游=0，weight-b 上游=1，金额={switched_cost:.9f}")

            before = len(self.observer.rows)
            repeated, repeated_headers, _, repeated_call = self.chat_request(primary, body)
            if repeated != switched or not response_cache_hit(repeated_headers) or self.observed_since(before, marker):
                raise AssertionError("切换后相同请求没有命中新路由缓存")
            cache_bill_check(self.wait_bill(repeated_call), primary, switched["usage"])
            self.action_ok(model, protocol, "新权重缓存命中与零权重排除",
                           "响应一致，weight-a 上游=0，weight-b 上游增量=0，cache_hit=true，spend=0")
        finally:
            for subject in temporary_keys:
                self.api("/key/delete", {"keys": [subject["key"]]})
            if route_id:
                self.api("/route_template/" + route_id + "/delete", {})
            for deployment_id in deployment_ids:
                self.api("/model/delete", {"id": deployment_id})
            if deployment_ids:
                self.state["deployments"] = [row for row in self.state["deployments"]
                                               if row["id"] not in deployment_ids]
                self.save()

    def verify_fallback_policy_chain(self, key):
        """用途：组合验证 429 跨真实模型回退、重复请求禁用缓存、回退目标白名单复核及 disable_fallbacks；参数为基线密钥，返回无；临时故障部署、模板和密钥均 finally 删除。"""
        model = self.state["chat_models"]["fennoai"]
        # chat_models 记录两家供应商共用的公开模型，不能作为跨模型回退目标；
        # 使用清单中独立的七牛 GLM 模型，并把主模型两家的健康部署全部排除。
        fallback_definition = next(row for row in self.data["models"] if row["id"] == "qiniu-glm")
        fallback_model = fallback_definition["public_name"]
        protocol = "bypass_openai_chat"
        provider = self.state["selected"]["fennoai"]["provider"]
        definition = next(row for row in self.data["models"] if row["id"] == "fenno-chat")
        healthy_ids, fallback_id = fallback_acceptance_deployments(
            self.state["deployments"], model, fallback_model, fallback_definition["provider"])
        fault_id = self.deployment(definition, provider, model, fault=True, temporary=True)
        route_id, temporary_keys = None, []
        try:
            route, _ = self.api("/route_template/new", {"name": "real-acceptance-fallback-" + self.run,
                "body": fallback_acceptance_route(model, fault_id, healthy_ids, fallback_model, fallback_id,
                                                    self.data["routing"]["timeout_seconds"])})
            route_id = route["id"]
            for suffix, models in (("allowed", [model, fallback_model]), ("primary-only", [model])):
                created, _ = self.api("/key/generate", {"key_alias": "real-fallback-" + suffix,
                    "owner_type": "personal", "user_id": key["user_id"], "team_id": key["team_id"],
                    "project_id": key["project_id"], "models": models, "route_template_id": route_id})
                temporary_keys.append({**key, **created, "profile": "logic-chain", "call_model": model})
            allowed, primary_only = temporary_keys
            marker = "fallback-repeat-" + self.run
            body = {"model": model, "messages": [{"role": "user",
                    "content": "Reply only OK. " + marker}], "max_tokens": 32}
            repeat_count = self.data["logic_chains"]["fallback_cache"]["repeat_requests"]
            for repeat in range(1, repeat_count + 1):
                before = len(self.observer.rows)
                answer, headers, _, call_id = self.chat_request(allowed, body)
                rows = self.observed_since(before, marker)
                route = [(row.get("provider"), row.get("route_tag"), row.get("status")) for row in rows]
                if response_cache_hit(headers) or route != [("fennoai", "fault", 429), ("qiniu", "default", 200)]:
                    raise AssertionError("回退请求没有按 Fenno 429 到七牛真实 200 执行，或错误命中缓存")
                cost = bill_check(self.wait_bill(call_id), allowed, self.data["billing"])
                self.action_ok(model + " → " + fallback_model, protocol,
                               f"第 {repeat}/{repeat_count} 次 429 跨真实模型回退",
                               f"Fenno fault=429，Fenno 健康零权重=0 次，Qiniu 真实=200，cache_hit=false，金额={cost:.9f}，usage={answer['usage']}")
            self.action_ok(model + " → " + fallback_model, protocol, "配置回退禁用响应缓存",
                           f"完全相同正文执行={repeat_count} 次，每次均重新发生 fault 429 和真实回退 200")

            denied_marker = "fallback-denied-" + self.run
            denied_body = {"model": model, "messages": [{"role": "user",
                "content": "Reply only OK. " + denied_marker}], "max_tokens": 32}
            before = len(self.observer.rows)
            denied, denied_headers, denied_status, _ = self.chat_request(primary_only, denied_body, expected=401)
            rows = self.observed_since(before, denied_marker)
            if response_cache_hit(denied_headers) or [(r.get("provider"), r.get("route_tag"), r.get("status")) for r in rows] != [("fennoai", "fault", 429)]:
                raise AssertionError("回退目标白名单拒绝前发生了备用模型外发")
            if "model not in allowed model list" not in json.dumps(denied, ensure_ascii=False):
                raise AssertionError("回退目标白名单拒绝没有返回明确原因")
            self.action_ok(fallback_model, protocol, "回退目标重新执行模型白名单",
                           f"主模型故障后 HTTP={denied_status}，Qiniu 上游增量=0，拒绝原因已核对")

            disabled_marker = "fallback-disabled-" + self.run
            disabled_body = {"model": model, "messages": [{"role": "user",
                "content": "Reply only OK. " + disabled_marker}], "max_tokens": 32,
                "disable_fallbacks": True}
            before = len(self.observer.rows)
            _, disabled_headers, disabled_status, _ = self.chat_request(allowed, disabled_body, expected=502)
            rows = self.observed_since(before, disabled_marker)
            if response_cache_hit(disabled_headers) or [(r.get("provider"), r.get("route_tag"), r.get("status")) for r in rows] != [("fennoai", "fault", 429)]:
                raise AssertionError("disable_fallbacks=true 后仍执行了备用模型")
            self.action_ok(model, protocol, "disable_fallbacks 停止回退链",
                           f"HTTP={disabled_status}，仅 Fenno fault=429，Qiniu 上游增量=0，cache_hit=false")
        finally:
            for subject in temporary_keys:
                self.api("/key/delete", {"keys": [subject["key"]]})
            if route_id:
                self.api("/route_template/" + route_id + "/delete", {})
            self.api("/model/delete", {"id": fault_id})
            self.state["deployments"] = [row for row in self.state["deployments"] if row["id"] != fault_id]
            self.save()

    def verify_image_model(self, key):
        """用途：通过 Fenno gpt-image-2 真实生成一张图片；参数为个人密钥，返回无；核对响应、请求日志和目录计价证据。"""
        definition = next(row for row in self.data["models"] if row["id"] == "fenno-image")
        model, protocol = definition["public_name"], definition["transport"]
        answer, headers = self.image_generation(definition, key)
        images = answer.get("data", [])
        if not images or not (images[0].get("url") or images[0].get("b64_json")):
            raise AssertionError("gpt-image-2 真实响应缺少图片结果")
        result = self.image_result(model, answer)
        call_id = next((v for k, v in headers.items() if k.lower() == "x-litellm-call-id"), None)
        if not call_id:
            raise AssertionError("gpt-image-2 响应缺少请求日志 ID")
        bill = self.wait_bill(call_id)
        if not bill.get("metadata", {}).get("cost_breakdown") or float(bill.get("spend", 0)) <= 0:
            raise AssertionError("gpt-image-2 缺少目录价格快照或费用")
        self.report["media_calls"] = self.report.get("media_calls", []) + [{"model": model, "kind": "image",
            "call_id": call_id, "cost": bill["spend"], "bypass_result": result}]
        self.verify_image_log(self.report["media_calls"][-1])
        self.action_ok(model, protocol, "真实图片生成、日志与计费",
                       f"bypass 结果={json.dumps(result, ensure_ascii=False)}；请求日志={call_id}，金额={float(bill['spend']):.9f}")

    def verify_image_log(self, call):
        """用途：复核 gpt-image-2 的真实日志正文；参数为图片检查点，返回完整日志；验证模型、精确 prompt、图片正文和价格快照，已有付费结果可安全续跑。"""
        model, prompt = call["model"], "A red apple on a white background."
        bill = self.wait_bill(call["call_id"])
        proxy = bill.get("proxy_server_request", {})
        request = proxy.get("body", {}) if isinstance(proxy, dict) else {}
        response = bill.get("response", {})
        images = response.get("data", []) if isinstance(response, dict) else []
        if bill.get("model") != model or request.get("model") != model or request.get("prompt") != prompt:
            raise AssertionError("gpt-image-2 请求日志模型或 prompt 不完整")
        if not images or not isinstance(images[0], dict) or not (images[0].get("url") or images[0].get("b64_json")):
            raise AssertionError("gpt-image-2 请求日志缺少真实图片响应")
        if not bill.get("metadata", {}).get("cost_breakdown"):
            raise AssertionError("gpt-image-2 请求日志缺少价格快照")
        self.action_ok(model, "bypass_openai_image_generation", "请求日志正文",
                       f"request_id={call['call_id']}，模型、prompt、图片响应和价格快照完整", record=False)
        return bill

    def verify_video_logs(self, task, definition):
        """用途：复核视频的原始请求与终态证据；参数为任务检查点与模型定义，返回创建和结算详情（原日志更新时为同一条）；验证 prompt、Task ID、完成状态、最终 URL、usage 或时长，证据不全抛断言，只读日志接口。"""
        model, protocol = task["model"], task["transport"]
        prompt = "A red apple on a white table, static camera, gentle natural light."
        create_id, settlement_id = task.get("create_call_id"), task.get("settlement_log_id")
        if not create_id or not settlement_id:
            raise AssertionError(model + " 缺少创建或终态日志 ID")
        created = self.wait_log(create_id)
        if create_id == settlement_id and created.get("status") != "completed":
            raise AssertionError(model + " 原任务日志尚未完成")
        proxy = created.get("proxy_server_request", {})
        request = proxy.get("body", {}) if isinstance(proxy, dict) else {}
        response = created.get("response", {})
        request_prompt = request.get("prompt")
        if protocol == "qiniu_contents_generation":
            request_prompt = "\n".join(str(row.get("text", "")) for row in request.get("content", []) if isinstance(row, dict))
        response_task = str(response.get("id") or response.get("request_id") or "") if isinstance(response, dict) else ""
        if created.get("model") != model or request_prompt != prompt or response_task != task["task_id"]:
            raise AssertionError(model + " 创建日志的模型、prompt 或 Task ID 不完整")
        self.action_ok(model, protocol, "视频创建日志正文",
                       f"request_id={create_id}，Task ID={task['task_id']}，模型、prompt、状态和查询地址完整", record=False)

        settled = self.wait_bill(settlement_id)
        final = settled.get("response", {})
        result = final.get("result", {}) if isinstance(final, dict) and isinstance(final.get("result"), dict) else {}
        content = final.get("content", {}) if isinstance(final, dict) and isinstance(final.get("content"), dict) else {}
        video = result.get("video", {}) if isinstance(result.get("video"), dict) else {}
        url = content.get("video_url") or video.get("url")
        final_task = str(final.get("id") or final.get("request_id") or "") if isinstance(final, dict) else ""
        status = str(final.get("status") or "") if isinstance(final, dict) else ""
        expected_url = task.get("bypass_result", {}).get("video_url")
        usage = (final.get("usage") or result.get("usage")) if isinstance(final, dict) else None
        if settled.get("model") != model or final_task != task["task_id"] or not status or not url or url != expected_url:
            raise AssertionError(model + " 终态日志缺少 Task ID、状态或真实视频 URL")
        if protocol in ("qiniu_contents_generation", "qiniu_fal_doubao_20") and not isinstance(usage, dict):
            raise AssertionError(model + " 终态日志缺少真实 usage")
        if protocol == "qiniu_fal_kling" and float(video.get("duration", 0)) <= 0:
            raise AssertionError(model + " 终态日志缺少按时长结算依据")
        usage_text = f"usage={json.dumps(usage, ensure_ascii=False)}" if usage else f"duration={video.get('duration')}（供应商未返回 token usage）"
        self.action_ok(model, protocol, "视频终态日志正文",
                       f"request_id={settlement_id}，Task ID={task['task_id']}，状态={status}，最终 URL 已核对，{usage_text}", record=False)
        return created, settled

    def poll_video(self, key, definition, task_id, status_path, result_path, terminal, failure):
        """用途：轮询真实视频任务并取得终态；参数含密钥、定义、任务和路径/状态集合，返回最终 JSON；十秒输出一次等待日志。"""
        model, protocol = definition["public_name"], definition["transport"]
        deadline = time.monotonic() + 600
        while time.monotonic() < deadline:
            time.sleep(10)
            expected = (200, 202) if definition["transport"].startswith("qiniu_fal_") else 200
            status_doc, _, http_status = self.api(status_path, token=key["key"], expected=expected, include_status=True)
            status = str(status_doc.get("status", ""))
            self.action_wait(model, protocol, "轮询视频任务",
                             f"任务={task_id}，HTTP={http_status}，状态={status or 'unknown'}")
            if status.lower() in failure:
                raise AssertionError("真实视频任务失败，状态=" + status)
            if status.upper() != terminal:
                continue
            if result_path == status_path:
                return status_doc
            result = self.video_result_query(key, definition, result_path, "读取真实视频终态")
            return result.get("result", result)
        raise TimeoutError("真实视频任务十分钟内未完成，任务=" + task_id)

    def video_result_query(self, key, definition, result_path, action, attempts=3, delay=5):
        """用途：幂等读取真实视频终态并处理供应商临时错误；参数为密钥、模型、结果路径、动作名、次数和间隔，返回一次HTTP 200结果；所有失败尝试有日志，耗尽后抛错。"""
        model, protocol = definition["public_name"], definition["transport"]
        retryable = (429, 502, 503, 504)
        for attempt in range(1, attempts + 1):
            answer, _, status = self.api(result_path, token=key["key"], expected=(200, *retryable), include_status=True)
            if status == 200:
                return answer
            error = answer.get("error", answer) if isinstance(answer, dict) else {}
            if isinstance(error, dict):
                summary = str(error.get("message") or error.get("code") or "无错误正文")[:300]
            else:
                summary = str(error)[:300]
            self.action_fail(model, protocol, f"{action}第 {attempt}/{attempts} 次",
                             RuntimeError(f"HTTP={status}，上游临时错误={summary}"))
            if attempt < attempts:
                self.action_wait(model, protocol, action, f"{delay} 秒后重试第 {attempt + 1}/{attempts} 次")
                time.sleep(delay)
        raise AssertionError(f"{model} {action}连续 {attempts} 次遇到临时上游错误")

    def verify_video_model(self, key, definition):
        """用途：创建并完成最小真实视频任务；参数为个人密钥和模型定义，返回无；验证本地 URL、终态、视频及重复查询，并将创建与结算 ID 同设为原日志 ID 保存检查点。调用付费供应商，缺失 ID 或产物、终态失败及超时均抛异常。"""
        model, protocol = definition["public_name"], definition["transport"]
        if protocol == "qiniu_contents_generation":
            body = {"model": model, "content": [{"type": "text", "text": "A red apple on a white table, static camera, gentle natural light."}],
                    "duration": 4, "resolution": "480p", "ratio": "16:9", "generate_audio": False}
        elif protocol == "qiniu_fal_doubao_20":
            body = {"prompt": "A red apple on a white table, static camera, gentle natural light.",
                    "duration": "4", "resolution": "480p", "aspect_ratio": "16:9", "generate_audio": False}
        else:
            body = {"prompt": "A red apple on a white table, static camera, gentle natural light.", "duration": "5"}
        self.action_start(model, protocol, "创建真实视频任务", "使用最小时长和最低验收规格")
        created, create_headers = self.api(definition["create_path"], body, token=key["key"])
        create_call_id = header_value(create_headers, "x-litellm-call-id")
        if not create_call_id:
            raise AssertionError(model + " 创建响应缺少请求日志 ID")
        task_id = str(created.get("id") or created.get("request_id") or "")
        if not task_id:
            raise AssertionError("视频创建响应缺少任务 ID")
        if protocol == "qiniu_contents_generation":
            result_path = status_path = definition["query_prefix"] + task_id
            terminal, failures = "SUCCEEDED", {"failed", "expired", "cancelled"}
        else:
            result_path = definition["query_prefix"] + task_id
            status_path = result_path + "/status"
            if created.get("response_url") != result_path or created.get("status_url") != status_path:
                raise AssertionError("FAL 返回的任务 URL 未改写为 XHub 本地路径")
            terminal, failures = "COMPLETED", {"failed", "error", "cancelled"}
        self.action_ok(model, protocol, "创建真实视频任务", f"任务={task_id}，查询路径已验证")
        result = self.poll_video(key, definition, task_id, status_path, result_path, terminal, failures)
        if protocol == "qiniu_contents_generation":
            content = result.get("content", {})
            usage = result.get("usage", {})
            if not content.get("video_url") or int(usage.get("completion_tokens", 0)) <= 0:
                raise AssertionError("Ark Seedance 终态缺少 video_url 或非零 usage")
            video_url, duration = content["video_url"], content.get("duration", result.get("duration"))
        else:
            video = result.get("video", {})
            if not video.get("url") or (protocol == "qiniu_fal_kling" and float(video.get("duration", 0)) <= 0):
                raise AssertionError("FAL 视频终态缺少视频 URL 或时长")
            video_url, duration = video["url"], video.get("duration")
        artifact = self.result_json(definition["id"] + "-bypass-result.json", result)
        bypass_result = {"task_id": task_id, "terminal_status": terminal, "video_url": video_url,
                         "duration": duration, "response_json": artifact}
        for repeat in range(1, 4):
            self.video_result_query(key, definition, result_path, f"第 {repeat}/3 次终态查询")
            self.action_ok(model, protocol, "重复查询去重前置", f"第 {repeat}/3 次终态查询成功", record=False)
        self.report["media_tasks"] = self.report.get("media_tasks", []) + [{"model": model, "kind": "video",
            "transport": protocol, "task_id": task_id, "result_path": result_path,
            "create_call_id": create_call_id, "settlement_log_id": create_call_id, "bypass_result": bypass_result}]
        self.action_ok(model, protocol, "真实视频终态",
                       f"bypass 结果={json.dumps(bypass_result, ensure_ascii=False)}；已重复查询 3 次")

    def verify_media_models(self, key):
        """用途：依次验收图片和三种视频 bypass；参数为允许全部模型的密钥，返回无；有效检查点直接复核文件，每种付费协议仅创建一个最小任务。"""
        with self.case("image") as evidence:
            image_model = next(row for row in self.data["models"] if row["id"] == "fenno-image")["public_name"]
            image_rows = [row for row in self.report.get("media_calls", [])
                          if row.get("model") == image_model and row.get("kind") == "image"]
            image = image_rows[-1] if image_rows else None
            image_result = image.get("bypass_result", {}) if image else {}
            if image and all(Path(image_result.get(field, "")).is_file() for field in ("artifact", "response_json")):
                self.report["media_calls"] = [row for row in self.report.get("media_calls", [])
                                                if not (row.get("model") == image_model and row.get("kind") == "image")] + [image]
                self.action_ok(image_model, "bypass_openai_image_generation", "复核已有真实图片结果",
                               f"bytes={image_result.get('bytes')}，sha256={image_result.get('sha256')}，文件与响应 JSON 均存在", record=False)
                self.verify_image_log(image)
                evidence["status"] = "revalidated"
            else:
                self.verify_image_model(key)
        for ident in self.data["verification_order"][3:]:
            case_id = {"qiniu-ark-seedance": "ark-video", "qiniu-fal-seedance": "fal-video", "qiniu-fal-kling": "kling-video"}[ident]
            with self.case(case_id) as evidence:
                definition = next(row for row in self.data["models"] if row["id"] == ident)
                model = definition["public_name"]
                rows = [row for row in self.report.get("media_tasks", [])
                        if row.get("model") == model and row.get("kind") == "video"]
                task = rows[-1] if rows else None
                result = task.get("bypass_result", {}) if task else {}
                response_json = result.get("response_json", "")
                if task and result.get("video_url") and result.get("duration") and Path(response_json).is_file():
                    self.report["media_tasks"] = [row for row in self.report.get("media_tasks", [])
                                                    if not (row.get("model") == model and row.get("kind") == "video")] + [task]
                    self.action_ok(model, definition["transport"], "复核已有真实视频结果",
                                   f"任务={result.get('task_id')}，终态={result.get('terminal_status')}，时长={result.get('duration')}，真实 URL 与响应 JSON 均存在", record=False)
                    self.verify_video_logs(task, definition)
                    evidence["status"] = "revalidated"
                else:
                    self.verify_video_model(key, definition)
        self.save()

    def verify_acceptance_models(self):
        """用途：执行有序模型验收；无参数/返回；GPT 按路由代表、GLM 三轮及四类媒体各测一次，严格复验检查点，前阶段失败停止后续付费任务。"""
        self.progress("  [模型阶段 1/6] GPT-5.6-sol：Codex 三轮会话")
        self.verify_codex_agents()
        with self.case("glm") as result:
            if self.verify_glm_model():
                result["status"] = "revalidated"
        self.progress("  [模型阶段 3–6/6] 图片 → Ark Seedance → FAL Seedance → Kling")
        self.verify_media_models(self.state["keys"][0])

    def verify_glm_model(self):
        """用途：验收 GLM 三轮会话；无参数，返回是否严格复验旧会话，供模型阶段调用；旧基线补齐模型和白名单，复验失败才发真实请求，目录缺模型抛断言。"""
        definition = next(row for row in self.data["models"] if row["id"] == "qiniu-glm")
        if not any(row["public_name"] == definition["public_name"] for row in self.state["deployments"]):
            # 旧保留基线缺少 GLM 时只新增明确的模型和白名单，不重建租户或清理原数据。
            provider = next(row for row in self.data["providers"] if row["id"] == definition["provider"])
            catalog = self.provider_catalog(provider)
            if definition["upstream_model"] not in catalog["model_ids"]:
                raise AssertionError("七牛目录缺少指定 GLM 模型：" + definition["upstream_model"])
            self.deployment(definition, provider)
            models = list(dict.fromkeys(row["public_name"] for row in self.state["deployments"]
                                        if not row.get("temporary")))
            for team in self.state["teams"]:
                self.api("/team/update", {"team_id": team["id"], "models": models})
            for owner in self.state["keys"]:
                self.api("/key/update", {"key": owner["token_id"], "models": models})
        key = {**self.state["keys"][0], "call_model": definition["public_name"], "profile": "qiniu-only"}
        self.progress("  [模型阶段 2/6] GLM：Codex 三轮会话")
        revalidated = self.codex_conversation(key, revalidate=True) is not None
        if not revalidated:
            self.codex_conversation(key)
        self.report["checks"] = [row for row in self.report["checks"] if row.get("name") != "glm-codex-multi-turn"]
        self.report["checks"].append({"name": "glm-codex-multi-turn", "model": key["call_model"], "passed": True})
        self.save()
        return revalidated

    def verify_saved_media_logs(self):
        """用途：统一复核报告中所有真实媒体日志；无参数和返回值；由 runner 在恢复精确日志 ID 后调用，确保图片和三种视频每个子项都经过详情接口验证。"""
        # 动态探测的聊天定义没有固定 public_name；这里仅建立视频模型映射，避免续跑媒体复核误读聊天定义。
        definitions = {
            row["public_name"]: row
            for row in self.data["models"]
            if row.get("kind") == "video" and row.get("public_name")
        }
        for call in self.report.get("media_calls", []):
            if call.get("kind") == "image":
                self.verify_image_log(call)
        for task in self.report.get("media_tasks", []):
            definition = definitions.get(task.get("model"))
            if task.get("kind") == "video" and definition:
                self.verify_video_logs(task, definition)

    def verify(self):
        """用途：验收实体基线与不同业务类型；无参数/返回，由 runner 调用；共享模式按路由去重，逐项断言和保存，失败中止，历史成功不能代替本轮业务执行。"""
        with self.case("entities"):
            counts = {name: len(self.state[name]) for name in ("organizations", "teams", "users", "projects", "keys")}
            if list(counts.values()) != [3, 9, 27, 9, 81]:
                raise AssertionError("实体数量错误")
            for team in self.state["teams"]:
                members, _ = self.api("/team/member_list?team_id=" + team["id"])
                if len(members["members"]) != 3:
                    raise AssertionError("团队必须恰好包含三名成员")
            self.report["counts"] = counts
        self.verify_acceptance_models()
        keys = self.acceptance_keys()
        with self.case("inheritance"):
            for key in keys:
                binding, _ = self.api("/route_template/binding?scope=key&scope_id=" + key["token_id"])
                expected = "key" if key["profile"] != "inherit" else ("organization" if key["team_index"] == 0 else "team")
                if binding["effective"]["scope_type"] != expected:
                    raise AssertionError("路由继承层级错误: " + expected)
                self.progress("    [继承通过] " + key["profile"] + " / " + expected)
        with self.case("chat-bills") as result:
            executed = 0
            for index, key in enumerate(keys, 1):
                self.progress(f"    [Chat {index}/{len(keys)}] {key['profile']}：真实响应、usage、五级归属与价格快照")
                if self.call_checkpoint(key):
                    self.progress("    [Chat 检查点已复验] 持久化请求及账单一致")
                    continue
                self.report["calls"] = [row for row in self.report.get("calls", []) if row.get("key_id") != key["token_id"]]
                self.call(key, f"dataset-{self.run}-{index - 1}")
                executed += 1
                self.save()
            evidence = [row for row in self.report.get("calls", []) if row.get("key_id") in {key["token_id"] for key in keys}]
            if len(evidence) != len(keys) or len({row.get("key_id") for row in evidence}) != len(keys):
                raise AssertionError("代表密钥的唯一真实调用证据不完整")
            name = "representative-keys-real-calls-and-billing" if self.representative else "81-personal-keys-real-calls-and-billing"
            self.report["checks"] = [row for row in self.report["checks"] if row.get("name") != name]
            self.report["checks"].append(dict(name=name, keys=len(keys), executed_this_run=executed, resumed=len(keys)-executed, passed=True))
            if executed == 0:
                result["status"] = "revalidated"
            self.save()
        key = keys[0]
        self.run_case("permissions", self.verify_permissions)
        self.run_case("xgo", self.verify_xgo, key)
        self.run_case("limits", self.verify_limits, key)
        self.run_case("model-lifecycle", self.verify_model_lifecycle, key)
        self.run_case("guard-lifecycle", self.verify_guardrail_lifecycle, key)
        with self.case("global-guards"):
            before = len(self.observer.rows) if self.observer else None
            blocked = self.call(key, "block-" + self.run, "验收禁止外发", expected=400)
            if "guardrail_failed" not in json.dumps(blocked) or (self.observer and len(self.observer.rows) != before):
                raise AssertionError("护栏拦截必须在上游调用前发生")
            self.report["checks"].append({"name": "global-guardrail-block-no-egress", "passed": True})
            marker = "redact-" + self.run
            self.call(key, marker, "电话13800138000 邮箱qa@example.invalid")
            if self.observer:
                rows = [r for r in self.observer.rows if marker in r["messages"]]
                if not rows or any("13800138000" in r["messages"] or "qa@example.invalid" in r["messages"] for r in rows):
                    raise AssertionError("个人信息在真实供应商转发前没有脱敏")
                self.report["checks"].append({"name": "redaction-before-real-supplier", "passed": True})
        if self.observer:
            self.run_case("profile-isolation", self.verify_profile_route_isolation)
            self.run_case("cache-weight", self.verify_cache_weight_chain, key)
            self.run_case("fallback", self.verify_fallback_policy_chain, key)
        self.report["status"] = "passed"
        self.save()

    def verify_profile_route_isolation(self):
        """用途：用当前运行的新请求验收两个单供应商密钥配置；无参数/返回；分别调用真实英文模型 ID，并要求观察器中只有声明的供应商收到该标记，证明不同上游模型没有再被合并到业务别名。"""
        if self.checked("profile-routes-real-models-and-vendors"):
            return
        for profile, provider in (("fenno-only", "fennoai"), ("qiniu-only", "qiniu")):
            key = next(row for row in self.state["keys"] if row["profile"] == profile)
            marker = f"route-isolation-{profile}-{self.run}"
            self.call(key, marker, model=key["call_model"])
            rows = [row for row in self.observer.rows if marker in row["messages"]]
            successes = [row for row in rows if row["status"] == 200]
            if not successes or any(row["provider"] != provider for row in rows):
                raise AssertionError(f"{profile} 没有隔离到真实模型和供应商 {provider}")
            self.action_ok(key["call_model"], "bypass_openai_chat", "真实模型路由隔离",
                           f"profile={profile}，仅供应商={provider} 收到请求，响应包含非零 usage")
        self.report["checks"].append({"name": "profile-routes-real-models-and-vendors", "passed": True})
        self.save()


    def verify_permissions(self):
        """用途：按清单登录四角色验收读写；无参数/返回；验证列表真实隔离、团队配置和护栏管理拒绝，临时护栏 finally 删除。"""
        personas = []
        for scenario in self.data["permission_scenarios"]:
            role = scenario["role"]
            self.progress(f"    [权限] 角色={role} 开始核对登录、团队可见范围、编辑边界与护栏权限")
            definition = next(p for p in self.data["personas"] if p["role"] == role)
            if role == "platform_admin":
                token, user = self.admin, self.state["users"][0]
            else:
                user = next(u for u in self.state["users"] if u["organization_slug"] == definition["organization"]
                            and u["team_slug"] == definition["team"] and u["member_index"] == definition["member_index"])
                login, _ = self.api("/v2/login", {"username": user["email"], "password": self.state["member_password"]})
                token = login["key"]
            teams, _ = self.api("/team/list", token=token)
            if len(teams) != scenario["visible_teams"]:
                raise AssertionError(role + " 团队可见范围不正确")
            foreign = next(t for t in self.state["teams"] if t["organization_id"] != user["organization_id"])
            for tid, field in ((user["team_id"], "own_team_update"), (foreign["id"], "foreign_team_update")):
                self.api("/team/update", {"team_id": tid, "team_description": "真实验收权限验证"},
                         token=token, expected=scenario[field])
            # 组织/团队管理员可编辑描述，但扩大预算和模型边界仍属于平台权限。
            self.api("/team/update", {"team_id": user["team_id"], "max_budget": 30}, token=token,
                     expected=200 if role == "platform_admin" else 403)
            created = None
            try:
                created, _ = self.api("/guardrails", {"guardrail_name": "权限临时-" + role,
                    "litellm_params": {"guardrail": "local", "mode": "pre_call", "default_on": False,
                                       "blocked_words": ["权限标记"], "action": "block"}},
                    token=token, expected=scenario["global_guardrail_create"])
            finally:
                if created and created.get("guardrail_id"):
                    self.api("/guardrails/" + created["guardrail_id"], method="DELETE")
            personas.append({"role": role, "email": "admin" if role == "platform_admin" else user["email"],
                             "team_id": user["team_id"], "visible_teams": len(teams)})
            self.report["checks"].append({"name": "permissions-" + role, "passed": True})
            self.action_ok("gpt-5.6-sol + z-ai/glm-5", "identity-rbac", role + " 权限矩阵",
                           f"登录、可见团队={len(teams)}、本团队编辑、跨组织拒绝、预算边界和全局护栏权限均已核对",
                           record=False)
        self.state["personas"] = personas
        self.save()

    def verify_xgo(self, key):
        """用途：验收清单XGo源码及真实执行；参数为个人密钥，返回无；验证源码往返、调试、无外发拦截、真实脱敏和审计放行。"""
        guards, _ = self.api("/guardrails/list")
        for definition in self.data["guardrails"]:
            scenario = definition.get("acceptance")
            if not scenario:
                continue
            name = definition["guardrail_name"]
            self.progress(f"    [XGo] 护栏={name} 动作={scenario['action']} 开始")
            stored = next(g for g in guards["guardrails"] if g["guardrail_name"] == name)
            detail, _ = self.api("/guardrails/" + stored["guardrail_id"])
            if detail["litellm_params"]["custom_code"] != definition["litellm_params"]["custom_code"]:
                raise AssertionError("XGo源码持久化不一致")
            trial, _ = self.api("/guardrails/apply_guardrail", {"guardrail_name": name, "text": scenario["text"]})
            if trial.get("action") != scenario["action"]:
                raise AssertionError(name + " 调试动作不一致")
            if scenario["action"] == "modify":
                trial_text = trial.get("response_text", trial.get("text", ""))
                if scenario["text"] in trial_text or scenario["replacement"] not in trial_text:
                    raise AssertionError(name + " 调试响应没有返回脱敏后的正文")
            marker = "xgo-" + self.run + "-" + scenario["action"]
            before = len(self.observer.rows) if self.observer else 0
            self.call(key, marker, scenario["text"], expected=scenario["expected_http_status"], guardrails=[name])
            rows = self.observed_since(before, marker)
            if self.observer and scenario["action"] == "block" and rows:
                raise AssertionError("XGo拦截发生了上游外发")
            if self.observer and scenario["action"] == "modify" and (not rows or any(
                    scenario["text"] in r["messages"] or scenario["replacement"] not in r["messages"] for r in rows)):
                original = sum(scenario["text"] in row.get("messages", "") for row in rows)
                replaced = sum(scenario["replacement"] in row.get("messages", "") for row in rows)
                raise AssertionError(
                    f"XGo脱敏没有作用于真实上游正文: 当前请求观察记录={len(rows)}，"
                    f"仍含原文={original}，含替换文本={replaced}")
            self.report["checks"].append({"name": name + "-persist-debug-real-request", "passed": True})
            result = ("本地阻断且上游增量=0" if scenario["action"] == "block" else
                      f"真实上游正文已替换为 {scenario['replacement']}" if scenario["action"] == "modify" else
                      "审计标记后真实上游继续响应")
            self.action_ok(key["call_model"], "xgo_guardrail", name + " / " + scenario["action"],
                           "源码持久化、调试响应和真实请求一致；" + result, record=False)


    def refuse(self, key, marker, expected, needle=None, model=None):
        """用途：验证本地拒绝无外发；参数为密钥、标记、状态及错误关键词，返回错误；用于额度/撤销/下架，不产生收费调用。"""
        before = len(self.observer.rows) if self.observer else 0
        answer = self.call(key, marker, model=model, expected=expected)
        if needle and needle not in json.dumps(answer, ensure_ascii=False):
            raise AssertionError("拒绝响应未指明原因: " + needle)
        if self.observer and len(self.observer.rows) != before:
            raise AssertionError("本地拒绝发生上游外发: " + marker)
        return answer

    def verify_limits(self, key):
        """用途：验证五级预算和RPM/TPM；参数为已成功计费密钥，返回无；逐层设零并finally恢复，临时密钥全部删除。"""
        for scenario in self.data["limit_scenarios"]["budgets"]:
            scope = scenario["scope"]
            if self.checked("budget-" + scope + "-no-egress"):
                continue
            identity = key[scope + "_id"] if scope != "key" else key["key"]
            self.progress(f"    [预算] scope={scope} 设为零并验证本地拒绝，随后恢复")
            route, field, method = scenario["route"], scenario["field"], scenario["method"]
            try:
                self.api(route, {field: identity, "max_budget": 0}, method=method)
                self.refuse(key, "budget-" + scope + self.run, 429, "budget")
            finally:
                self.api(route, {field: identity, "max_budget": self.data["budgets"][scope]}, method=method)
            self.report["checks"].append({"name": "budget-" + scope + "-no-egress", "passed": True})
            self.action_ok(key["call_model"], "identity-budget", scope + " 预算链",
                           "预算设为零后 HTTP=429 且上游增量=0；恢复后配置已写回", record=False)
        for field in ("rpm_limit", "tpm_limit"):
            if self.checked(field + "-reject-and-real-recovery"):
                continue
            temp = None
            self.progress(f"    [限流] {field} 验证拒绝、恢复和真实调用")
            try:
                temp, _ = self.api("/key/generate", {"owner_type": "personal", "user_id": key["user_id"],
                    "team_id": key["team_id"], "project_id": key["project_id"], "key_alias": "临时-" + field, field: 0})
                subject = {**key, **temp}
                self.refuse(subject, field + self.run, 429, field)
                self.api("/key/update", {"key": temp["key"], field: 100000})
                self.call(subject, field + "-restored-" + self.run)
                self.report["checks"].append({"name": field + "-reject-and-real-recovery", "passed": True})
                self.action_ok(key["call_model"], "identity-rate-limit", field + " 限流链",
                               "限制为零时本地拒绝且无外发；提高限额后真实模型响应、usage 与计费均通过", record=False)
            finally:
                if temp:
                    self.api("/key/delete", {"keys": [temp["key"]]})
        # 基线81把钥匙保持不变，生命周期使用成员自己签发的临时密钥。
        if not self.checked("model-allowlist-no-egress"):
            self.progress("    [模型白名单] 验证非白名单英文模型 ID 在本地拒绝且无上游外发")
            scenario = self.data["limit_scenarios"]["model_allowlist"]
            self.refuse(key, "allowlist-" + self.run, scenario["expected_status"], model=scenario["disallowed_model"])
            self.report["checks"].append({"name": "model-allowlist-no-egress", "passed": True})
            self.action_ok(scenario["disallowed_model"], "identity-model-allowlist", "模型白名单拒绝",
                           f"HTTP={scenario['expected_status']}，拒绝发生在外发前，上游增量=0", record=False)
        if self.checked("member-key-generate-block-rotate-real-calls"):
            return
        user = next(u for u in self.state["users"] if u["id"] == key["user_id"])
        self.progress("    [密钥生命周期] 验证成员签发、封禁、解封、轮换、旧密钥失效和删除")
        login, _ = self.api("/v2/login", {"username": user["email"], "password": self.state["member_password"]})
        token, temp = login["key"], None
        try:
            temp, _ = self.api("/key/generate", {"team_id": key["team_id"], "project_id": key["project_id"], "key_alias": "成员生命周期"}, token=token)
            self.call({**key, **temp}, "lifecycle-new-" + self.run)
            self.api("/key/block", {"key": temp["key"]}, token=token)
            self.refuse({**key, **temp}, "lifecycle-block-" + self.run, 401)
            self.api("/key/unblock", {"key": temp["key"]}, token=token)
            old = dict(temp)
            rotated, _ = self.api("/key/regenerate", {"key": temp["key"]}, token=token)
            temp = {**temp, **rotated}
            self.refuse({**key, **old}, "lifecycle-old-" + self.run, 401)
            self.call({**key, **temp}, "lifecycle-rotated-" + self.run)
            self.report["checks"].append({"name": "member-key-generate-block-rotate-real-calls", "passed": True})
        finally:
            if temp:
                self.api("/key/delete", {"keys": [temp["key"]]}, token=token)
        self.refuse({**key, **temp}, "lifecycle-deleted-" + self.run, 401)

    def verify_model_lifecycle(self, key):
        """用途：在验收阶段读取真实目录并验证模型添加上架下架删除；参数为个人密钥，返回无；目录失败即停止，下架无外发，恢复后真实回答，finally 删除临时部署。"""
        if self.checked("model-add-disable-refuse-enable-real-request"):
            return
        provider = self.state["selected"]["qiniu"]["provider"]
        # 建数阶段只依赖清单；额外模型的实时目录发现归属于有供应商凭据的验收阶段。
        catalog = self.discover_catalog(provider)
        (self.directory / "qiniu-catalog.json").write_text(json.dumps(catalog, ensure_ascii=False, indent=2))
        deployed = {row["public_name"] for row in self.state["deployments"]}
        candidates = [row for row in chat_candidates(catalog, random.Random(self.run + "-lifecycle"))
                      if row not in deployed][:self.data["limits"]["max_probe_models_per_provider"]]
        if not candidates:
            raise RuntimeError("真实目录中没有未部署的额外聊天模型可用于生命周期验收")
        model = self.select_lifecycle_model(provider, candidates)
        self.action_ok(model, "bypass_openai_chat", "生命周期探测",
                       "真实供应商响应包含非空回答和非零 usage，允许进入创建、下架、恢复和删除流程")
        self.progress(f"    [模型生命周期] 真实模型={model} 创建、调用、下架、恢复、删除")
        definition = next(row for row in self.data["models"] if row["id"] == "qiniu-chat")
        ident = self.deployment(definition, provider, model, temporary=True)
        allowed = [row["public_name"] for row in self.state["deployments"] if row["id"] != ident]
        temp = None
        try:
            self.api("/team/update", {"team_id": key["team_id"], "models": allowed + [model]})
            temp, _ = self.api("/key/generate", {"owner_type": "personal", "user_id": key["user_id"],
                "team_id": key["team_id"], "project_id": key["project_id"], "key_alias": "模型生命周期", "models": [model]})
            subject = {**key, **temp}
            self.call(subject, "model-listed-" + self.run, model=model)
            self.api("/model/disable", {"model_name": model})
            # 禁用返回的具体状态是现有网关契约；断言拒绝且没有绕行备用部署。
            status, answer, _ = http(self.gateway, "/v1/chat/completions", {"model": model,
                "messages": [{"role": "user", "content": "model-unlisted-" + self.run}]}, subject["key"])
            if status < 400 or any("model-unlisted-" + self.run in r["messages"] for r in self.observer.rows):
                raise AssertionError("下架模型仍然外发或成功回答")
            self.api("/model/enable", {"model_name": model})
            self.call(subject, "model-relisted-" + self.run, model=model)
            self.report["checks"].append({"name": "model-add-disable-refuse-enable-real-request", "disabled_status": status, "passed": True})
        finally:
            if temp:
                self.api("/key/delete", {"keys": [temp["key"]]})
            self.api("/model/delete", {"id": ident})
            self.state["deployments"] = [d for d in self.state["deployments"] if d["id"] != ident]
            self.api("/team/update", {"team_id": key["team_id"], "models": allowed})

    def checked(self, name):
        """用途：判断续跑场景已有成功证据；参数为唯一场景名，返回布尔；只跳过明确通过项，失败和未执行项继续运行。"""
        # 编号清单表示本轮执行，不能把旧通过记录当成本轮执行结果；非清单续跑仍保留历史兼容。
        return not self.checklist and any(c["name"] == name and c.get("passed") for c in self.report["checks"])

    def verify_guardrail_lifecycle(self, key):
        """用途：临时XGo编辑启用删除验收；参数为个人密钥，返回无；调试和真实脱敏均验证，finally删除临时规则。"""
        if self.checked("xgo-edit-default-enable-delete"):
            return
        definition = next(g for g in self.data["guardrails"] if g.get("acceptance", {}).get("action") == "modify")
        self.progress("    [护栏生命周期] 创建临时 XGo、编辑源码、默认启用、真实脱敏并删除")
        guard, _ = self.api("/guardrails", {"guardrail_name": "临时-XGo-编辑与默认启用",
            "litellm_params": {**definition["litellm_params"], "default_on": False}})
        ident = guard["guardrail_id"]
        try:
            source = definition["litellm_params"]["custom_code"].replace("[工单已隐藏]", "[编辑后已隐藏]")
            self.api("/guardrails/" + ident, {"litellm_params": {"custom_code": source, "default_on": True}}, method="PATCH")
            stored, _ = self.api("/guardrails/" + ident)
            if stored["litellm_params"]["custom_code"] != source or not stored["litellm_params"]["default_on"]:
                raise AssertionError("XGo编辑或默认启用未持久化")
            marker = "xgo-edited-" + self.run
            self.call(key, marker, "XGO_PRIVATE")
            if self.observer:
                rows = [r for r in self.observer.rows if marker in r["messages"]]
                if not rows or any("XGO_PRIVATE" in r["messages"] or "[编辑后已隐藏]" not in r["messages"] for r in rows):
                    raise AssertionError("编辑启用后的XGo未改写真实外发正文")
        finally:
            self.api("/guardrails/" + ident, method="DELETE")
        self.api("/guardrails/" + ident, expected=404)
        self.report["checks"].append({"name": "xgo-edit-default-enable-delete", "passed": True})
        self.save()


def main():
    """用途：供已启动 E2E 网关加载清单；命令行输入地址/目录，返回退出码；所有异常保留失败报告，不打印凭据。"""
    parser = argparse.ArgumentParser()
    parser.add_argument("--gateway", required=True)
    parser.add_argument("--directory", required=True)
    parser.add_argument("--seed-only", action="store_true")
    args = parser.parse_args()
    dataset = Dataset(args.gateway, args.directory, load_manifest())
    try:
        dataset.seed()
        if not args.seed_only:
            dataset.verify()
    except Exception as error:
        dataset.report.update(status="failed", error=type(error).__name__ + ": " + str(error))
        dataset.save()
        raise SystemExit("真实数据集失败，查看私有运行目录 report.json")
    print("真实数据集完成，报告: " + str(dataset.directory / "report.json"))


if __name__ == "__main__":
    main()
