#!/usr/bin/env python3
"""按 docs 数据清单构建真实租户并验收；凭据仅来自环境，证据不含密钥。"""
import argparse
import concurrent.futures
import json
import math
import os
from pathlib import Path
import random
import re
import secrets
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "docs/testdata/real-acceptance/dataset.json"


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
    if data["key_profiles"] != ["inherit", "fenno-only", "modelink-only"]:
        raise ValueError("每个成员的三个密钥策略必须完整")
    rates = data["billing"]
    if any(not math.isfinite(rates[k]) or rates[k] <= 0 for k in ("input_cost_per_token", "output_cost_per_token")):
        raise ValueError("验收费率必须为有限正数")
    if len(data["providers"]) != 2 or data["limits"]["max_upstream_attempts"] < 100:
        raise ValueError("需要两个供应商及至少 100 次请求额度")
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
    for profile in ("organization-default", "team-default", "fenno-only", "modelink-only"):
        resolve_document(data["routing"]["templates"][profile], {"alias": "a", "backup_alias": "b",
                         "fenno_deployment": "f", "modelink_deployment": "m"})
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


def http(base, route, body=None, token=None, method=None):
    """用途：发送真实 HTTP JSON；参数为地址/路径/正文/身份/方法，返回状态、正文、头；错误正文不输出，网络异常交给调用方。"""
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = Request(base.rstrip("/") + route, data=None if body is None else json.dumps(body).encode(),
                  headers=headers, method=method or ("GET" if body is None else "POST"))
    try:
        response = urlopen(req, timeout=110)
    except HTTPError as error:
        response = error
    with response:
        raw = response.read()
        try:
            value = json.loads(raw)
        except ValueError:
            value = {"non_json": True}
        return response.status, value, dict(response.headers)


def chat_candidates(catalog, rng):
    """用途：从真实目录随机选聊天候选；参数为目录和随机源，返回去重列表；过滤图像音频等模型，探测成功才部署。"""
    ids = catalog.get("model_ids", [])
    candidates = [m for m in ids if re.search(r"gpt|claude|gemini|deepseek|qwen|kimi|glm|llama|hy|grok", m, re.I)
                  and not re.search(r"image|video|audio|tts|embed|rerank|realtime|whisper|vision|preview.*image", m, re.I)]
    candidates = sorted(set(candidates))
    rng.shuffle(candidates)
    return candidates


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
                """用途：处理网关补全；无参数，返回 HTTP 响应；故障只在转发前注入，健康结果来自真实供应商。"""
                parts = self.path.strip("/").split("/")
                if not parts or parts[0] not in owner.providers:
                    self.send_error(404)
                    return
                provider = owner.providers[parts[0]]
                body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))))
                text = json.dumps(body.get("messages", []), ensure_ascii=False)
                fault = len(parts) > 1 and parts[1] == "fault"
                with owner.lock:
                    owner.attempts += 1
                    allowed = owner.attempts <= owner.limit
                status = 429 if fault else 0
                usage = {}
                content = b'{"error":{"message":"acceptance injected rate limit","type":"rate_limit"}}'
                if not allowed:
                    status = 429
                elif not fault:
                    base = os.environ.get(provider["base_env"], provider["base"]).rstrip("/")
                    route = "/chat/completions" if base.endswith("/v1") else "/v1/chat/completions"
                    status, answer, _ = http(base, route, body, os.environ[provider["key_env"]])
                    usage = answer.get("usage", {})
                    content = json.dumps(answer).encode()
                with owner.lock:
                    owner.rows.append({"provider": provider["id"], "model": body.get("model"),
                                       "messages": text, "fault": fault, "forwarded": allowed and not fault, "status": status, "usage": usage})
                owner.logger("供应商=%s 模型=%s 状态=%s 输入=%s 输出=%s%s" % (
                    provider["id"], body.get("model"), status, usage.get("prompt_tokens", 0),
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
        self.report = {"checks": [], "calls": [], "models": [], "status": "running"}
        self.run = secrets.token_hex(6)
        self.admin = None

    def save(self):
        """用途：落盘检查点和脱敏报告；无参数/返回，供每个阶段调用；访问材料保存在忽略目录 0600 文件。"""
        write_private(self.directory / "access.json", {**self.state, "admin": self.admin, "gateway": self.gateway})
        (self.directory / "report.json").write_text(json.dumps(self.report, ensure_ascii=False, indent=2) + "\n")

    def progress(self, message):
        """用途：把不含凭据的构建与验收进度立即输出到终端；参数为日志正文，无返回值；供长时间真实请求显示当前步骤。"""
        self.logger(message)

    def api(self, route, body=None, token=None, method=None, expected=200):
        """用途：验证管理或数据接口；参数为请求和期望状态，返回 JSON/头；正文不出现在失败日志，避免凭据泄露。"""
        verb = method or ("GET" if body is None else "POST")
        safe_route = route.split("?", 1)[0]
        with self.log_lock:
            self.api_sequence += 1
            sequence = self.api_sequence
        started = time.monotonic()
        self.progress(f"    [接口 {sequence:04d}] {verb} {safe_route} 开始，预期={expected}")
        try:
            status, value, headers = http(self.gateway, route, body, token or self.admin, method)
        except Exception as error:
            self.progress(f"    [接口 {sequence:04d}] {verb} {safe_route} 网络失败，耗时={time.monotonic() - started:.3f}s，错误={type(error).__name__}")
            raise
        self.progress(f"    [接口 {sequence:04d}] {verb} {safe_route} 完成，状态={status}，耗时={time.monotonic() - started:.3f}s")
        if status != expected:
            raise AssertionError(f"{verb} {safe_route} 状态 {status}，预期 {expected}")
        return value, headers

    def deployment(self, alias, provider, upstream, fault=False):
        """用途：注册已探测模型；参数为公开别名/供应商/真实模型/故障开关，返回部署 ID；故障部署用于可控回退验收。"""
        rates = self.data["billing"]
        params = {"model": upstream, "custom_llm_provider": "openai", "litellm_credential_name": provider["id"],
                  "max_tokens": self.data["limits"]["max_output_tokens"],
                  "input_cost_per_token": rates["input_cost_per_token"], "output_cost_per_token": rates["output_cost_per_token"]}
        if self.observer:
            params["api_base"] = self.observer.base + "/" + provider["id"] + ("/fault" if fault else "")
        row, _ = self.api("/model/new", {"model_name": alias, "litellm_params": params,
                         "model_info": {"transport": "bypass_openai_chat", "endpoint_types": ["chat"], "pricing_source": "manual"}})
        ident = row["model_info"]["id"]
        self.state["deployments"].append({"id": ident, "alias": alias, "provider": provider["id"], "model": upstream})
        self.save()
        return ident

    def template(self, name, ids, profile, **scope):
        """用途：实例化清单整体路由文档；参数为名称/部署ID/策略/作用域，返回模板ID；阈值零禁用被动熔断，避免用例互相污染。"""
        body = resolve_document(self.data["routing"]["templates"][profile], {
            "alias": self.data["routing"]["alias"], "backup_alias": self.data["routing"]["backup_alias"],
            "fenno_deployment": ids[0], "modelink_deployment": ids[1]})
        row, _ = self.api("/route_template/new", {"name": name, "body": body, **scope})
        return row["id"]

    def seed(self):
        """用途：建完整保留基线；无参数，返回无；先要求空租户库，真实目录探测失败立即停止，不用假模型补齐。"""
        login, _ = self.api("/v2/login", {"username": os.environ.get("E2E_DATASET_ADMIN", "admin"),
                                          "password": os.environ["E2E_DATASET_PASSWORD"]})
        self.admin = login["key"]
        self.progress("  [构建 1/5] 管理员登录成功，检查空数据库")
        orgs, _ = self.api("/organization/list")
        if orgs:
            raise RuntimeError("数据集要求空租户环境；已有组织时拒绝覆盖，请使用隔离运行入口")
        selected, ids = [], []
        rng = random.Random(os.environ.get("E2E_DATASET_RANDOM_SEED", self.run))
        self.progress("  [构建 2/5] 注册 2 个真实供应商并读取模型目录")
        for provider in self.data["providers"]:
            if not os.environ.get(provider["key_env"]):
                raise RuntimeError("缺少环境凭据 " + provider["key_env"])
            self.api("/credentials", {"credential_name": provider["id"],
                "credential_info": {"catalog_id": provider["id"], "custom_llm_provider": "openai", "provider_id": "OpenAI"},
                "credential_values": {"api_base": os.environ.get(provider["base_env"], provider["base"]),
                                      "api_key": "os.environ/" + provider["key_env"]}})
            catalog, _ = self.api("/model/builtin/models", {"credential_name": provider["id"]})
            if catalog.get("error") or not catalog.get("model_ids"):
                raise RuntimeError("真实目录发现失败: " + provider["id"])
            (self.directory / (provider["id"] + "-catalog.json")).write_text(json.dumps(catalog, ensure_ascii=False, indent=2))
            candidates = chat_candidates(catalog, rng)[:self.data["limits"]["max_probe_models_per_provider"]]
            self.progress(f"    [{provider['id']}] 目录 {len(catalog['model_ids'])} 个模型，准备探测 {len(candidates)} 个聊天候选")
            found = None
            for probe_index, upstream in enumerate(candidates, 1):
                base = os.environ.get(provider["base_env"], provider["base"]).rstrip("/")
                status, reply, _ = http(base, "/chat/completions" if base.endswith("/v1") else "/v1/chat/completions",
                    {"model": upstream, "messages": [{"role": "user", "content": "Reply only OK. probe " + self.run}], "max_tokens": 32},
                    os.environ[provider["key_env"]])
                usage = reply.get("usage", {})
                passed = status == 200 and bool(reply.get("choices")) and bool(reply["choices"][0].get("message", {}).get("content")) and usage.get("prompt_tokens", 0) > 0 and usage.get("completion_tokens", 0) > 0
                self.report["checks"].append({"name": "real-model-probe", "provider": provider["id"], "model": upstream, "status": status, "passed": passed})
                self.progress(f"    [{provider['id']} 探测 {probe_index}/{len(candidates)}] 模型={upstream} 状态={status} 输入={usage.get('prompt_tokens', 0)} 输出={usage.get('completion_tokens', 0)} {'通过' if passed else '未通过'}")
                if passed:
                    found = upstream
                    break
            if not found:
                raise RuntimeError("随机候选未通过真实调用: " + provider["id"])
            selected.append((provider, found))
            ids.append(self.deployment(self.data["routing"]["alias"], provider, found))
            self.report["models"].append({"provider": provider["id"], "upstream_model": found, "catalog_count": len(catalog["model_ids"])})
        self.progress("  [构建 3/5] 创建主模型、备用模型及组织/团队/个人路由策略")
        self.deployment(self.data["routing"]["backup_alias"], *selected[1])
        self.api("/model/default", {"model_name": self.data["routing"]["alias"], "weights": {"allocations": [
            {"deployment_id": i, "weight": w} for i, w in zip(ids, self.data["routing"]["weights"])]}}, method="PUT")
        self.api("/model/fallback", {"model_name": self.data["routing"]["alias"], "policy": {
            "fallbacks": [self.data["routing"]["backup_alias"]]}}, method="PUT")
        fenno = self.template("个人-Fenno-独占", ids, "fenno-only")
        modelink = self.template("个人-Modelink-独占", ids, "modelink-only")
        password = "Acceptance-" + secrets.token_urlsafe(18)
        self.state["member_password"] = password
        expanded = hierarchy(self.data)
        self.progress("  [构建 4/5] 创建 3 个组织、9 个团队、27 名成员、9 个项目和 81 把密钥")
        for org in self.data["organizations"]:
            row, _ = self.api("/organization/new", {"organization_alias": org["name"], "max_budget": self.data["budgets"]["organization"]})
            oid = row["organization_id"]
            self.state["organizations"].append({"id": oid, "name": org["name"]})
            ot = self.template(org["name"] + "-默认7比3", ids, "organization-default", organization_id=oid)
            self.api("/route_template/binding", {"scope": "organization", "scope_id": oid, "route_template_id": ot})
            for ti, team in enumerate(self.data["teams"]):
                members = [m for m in expanded if m["organization"]["slug"] == org["slug"] and m["team"]["slug"] == team["slug"]]
                first = members[0]
                user, _ = self.api("/user/new", {"user_email": first["email"], "user_alias": first["name"], "password": password,
                    "user_role": "user", "max_budget": 10, "admin_organization_ids": [oid] if ti == 0 else []})
                teamrow, _ = self.api("/team/new", {"organization_id": oid, "team_alias": team["name"],
                    "admin_user_id": user["user_id"], "models": [self.data["routing"]["alias"], self.data["routing"]["backup_alias"]], "max_budget": 30})
                tid = teamrow["team_id"]
                self.state["teams"].append({"id": tid, "organization_id": oid, "name": team["name"]})
                if ti:
                    tt = self.template(org["name"] + "-" + team["name"] + "-3比7", ids, "team-default", organization_id=oid, team_id=tid)
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
                            "models": [self.data["routing"]["alias"], self.data["routing"]["backup_alias"]], "max_budget": 3}
                        if profile != "inherit":
                            body["route_template_id"] = fenno if profile == "fenno-only" else modelink
                        key, _ = self.api("/key/generate", body)
                        self.state["keys"].append({"key": key["key"], "token_id": key["token_id"], "profile": profile,
                            "user_id": uid, "team_id": tid, "project_id": pid, "organization_id": oid, "team_index": ti})
                        self.save()
                self.progress(f"    [{org['name']}/{team['name']}] 已创建 3 名成员、1 个项目、9 把密钥；累计团队={len(self.state['teams'])}/9 密钥={len(self.state['keys'])}/81")
            self.progress(f"    [{org['name']}] 组织构建完成；累计成员={len(self.state['users'])}/27")
        self.progress(f"  [构建 5/5] 创建 {len(self.data['guardrails'])} 条护栏并保存数据集检查点")
        for guardrail in self.data["guardrails"]:
            self.api("/guardrails", {k: v for k, v in guardrail.items() if k != "acceptance"})
        self.state["selected"] = selected
        self.save()

    def wait_bill(self, call_id):
        """用途：等待异步用量持久化；参数为请求 ID，返回账单；等待90秒覆盖生产60秒落库周期，不把空记录当成功。"""
        end = time.monotonic() + 90
        while time.monotonic() < end:
            status, bill, _ = http(self.gateway, "/spend/logs/ui/" + call_id, token=self.admin)
            if status == 200 and bill.get("metadata", {}).get("cost_breakdown"):
                return bill
            time.sleep(0.3)
        raise AssertionError("账单未持久化: " + call_id)

    def call(self, key, marker, extra="", alias=None, expected=200, guardrails=None, defer_bill=False):
        """用途：个人密钥真实请求对账；参数含标记/模型/状态/护栏/延迟对账，返回正文或四元回执；失败不计成功证据。"""
        body = {"model": alias or self.data["routing"]["alias"], "messages": [{"role": "user", "content": "Reply only OK. " + marker + " " + extra}], "max_tokens": 32}
        if guardrails is not None:
            body["guardrails"] = guardrails
        self.progress(f"    [数据面] 标记={marker} profile={key.get('profile', 'temporary')} 模型={body['model']} 预期={expected} 开始")
        answer, headers = self.api("/v1/chat/completions", body, token=key["key"], expected=expected)
        if expected == 200:
            call_id = next((v for k, v in headers.items() if k.lower() == "x-litellm-call-id"), None)
            if not call_id or not answer.get("choices"):
                raise AssertionError("真实响应缺少 request_id 或 choices")
            if self.observer:
                observed = [r for r in self.observer.rows if marker in r["messages"] and r["status"] == 200]
                if observed and observed[-1].get("usage") and any(observed[-1]["usage"].get(k) != answer.get("usage", {}).get(k)
                        for k in ("prompt_tokens", "completion_tokens")):
                    raise AssertionError("网关响应计量与真实供应商不一致")
            if defer_bill:
                self.progress(f"    [数据面] 标记={marker} 响应完成，输入={answer.get('usage', {}).get('prompt_tokens', 0)} 输出={answer.get('usage', {}).get('completion_tokens', 0)}，等待批量计量核对")
                return (key, marker, call_id, answer.get("usage", {}))
            bill = self.wait_bill(call_id)
            if any(bill.get(k) != answer.get("usage", {}).get(k) for k in ("prompt_tokens", "completion_tokens")):
                raise AssertionError("账单计量与真实响应用量不一致")
            cost = bill_check(bill, key, self.data["billing"])
            self.report["calls"].append({"call_id": call_id, "marker": marker, "key_id": key["token_id"], "profile": key["profile"],
                "prompt_tokens": bill["prompt_tokens"], "completion_tokens": bill["completion_tokens"], "cost": cost})
            self.save()
            self.progress(f"    [数据面] 标记={marker} 计量通过，输入={bill['prompt_tokens']} 输出={bill['completion_tokens']} 金额={cost:.9f}")
        else:
            self.progress(f"    [数据面] 标记={marker} 按预期被拒绝，状态={expected}")
        return answer

    def verify(self):
        """用途：验收全部 81 密钥、真实供应商、护栏和回退；无参数/返回；每条请求独立标记，观察器证明转发与拦截。"""
        counts = {name: len(self.state[name]) for name in ("organizations", "teams", "users", "projects", "keys")}
        if list(counts.values()) != [3, 9, 27, 9, 81]:
            raise AssertionError("实体数量错误")
        self.progress("  [API 1/10] 核对实体数量和 9 个团队成员关系")
        for team in self.state["teams"]:
            members, _ = self.api("/team/member_list?team_id=" + team["id"])
            if len(members["members"]) != 3:
                raise AssertionError("团队必须恰好包含三名成员")
        self.report["counts"] = counts
        receipts = []
        self.progress("  [API 2/10] 核对 81 把密钥的组织/团队/个人路由继承")
        for index, key in enumerate(self.state["keys"]):
            binding, _ = self.api("/route_template/binding?scope=key&scope_id=" + key["token_id"])
            expected = "key" if key["profile"] != "inherit" else ("organization" if key["team_index"] == 0 else "team")
            if binding["effective"]["scope_type"] != expected:
                raise AssertionError("路由继承层级错误")
        # 并发上限三，避免对外供应商形成突发；批量发完再对账，避免每把钥匙等待一个落库周期。
        self.progress("  [API 3/10] 用 81 把密钥调用真实供应商（最大并发 3）")
        with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
            futures = [pool.submit(self.call, key, f"dataset-{self.run}-{index}", defer_bill=True)
                       for index, key in enumerate(self.state["keys"])]
            for index, future in enumerate(futures, 1):
                receipts.append(future.result())
                self.progress(f"    [真实请求 {index}/81] 已收到响应")
        self.progress("  [API 4/10] 逐笔核对 81 条响应 usage、账单归属和固定费率")
        for index, (key, marker, call_id, usage) in enumerate(receipts, 1):
            bill = self.wait_bill(call_id)
            if any(bill.get(k) != usage.get(k) for k in ("prompt_tokens", "completion_tokens")):
                raise AssertionError("批量账单计量与真实响应用量不一致")
            cost = bill_check(bill, key, self.data["billing"])
            self.report["calls"].append({"call_id": call_id, "marker": marker, "key_id": key["token_id"],
                "profile": key["profile"], "prompt_tokens": bill["prompt_tokens"], "completion_tokens": bill["completion_tokens"], "cost": cost})
            self.progress(f"    [计量核对 {index}/81] profile={key['profile']} 输入={bill['prompt_tokens']} 输出={bill['completion_tokens']} 金额={cost:.9f}")
        self.save()
        self.report["checks"].append({"name": "81-personal-keys-real-calls-and-billing", "passed": True})
        key = self.state["keys"][0]
        self.progress("  [API 5/10] 验证平台、组织、团队、成员四类权限")
        self.verify_permissions()
        self.progress("  [API 6/10] 验证 XGo 护栏持久化、调试、拦截与脱敏")
        self.verify_xgo(key)
        self.progress("  [API 7/10] 验证五级预算、RPM、TPM、模型白名单和密钥生命周期")
        self.verify_limits(key)
        self.progress("  [API 8/10] 验证模型创建、下架、恢复和删除")
        self.verify_model_lifecycle(key)
        self.progress("  [API 9/10] 验证护栏编辑、默认启用和删除")
        self.verify_guardrail_lifecycle(key)
        self.progress("  [API 10/10] 验证全局拦截、转发前脱敏、零权重路由与 429 回退")
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
            for index, k in enumerate(self.state["keys"]):
                rows = [r for r in self.observer.rows if f"dataset-{self.run}-{index} " in r["messages"] and r["status"] == 200]
                wanted = {"fenno-only": "fennoai", "modelink-only": "qiniu"}.get(k["profile"])
                if not rows or (wanted and any(r["provider"] != wanted for r in rows)):
                    raise AssertionError("零权重排除没有作用")
            self.report["checks"].append({"name": "weights-100:0-and-0:100-real-vendors", "passed": True})
        # 可控故障部署独立于保留基线，健康回退仍由真实供应商回答。
        if self.observer:
            alias = "验收限流回退"
            provider, model = self.state["selected"][0]
            fault_id = self.deployment(alias, provider, model, fault=True)
            temp = None
            try:
                self.api("/team/update", {"team_id": key["team_id"], "models": [self.data["routing"]["alias"], self.data["routing"]["backup_alias"], alias]})
                self.api("/model/fallback", {"model_name": alias, "policy": {"fallbacks": [self.data["routing"]["backup_alias"]]}}, method="PUT")
                temp, _ = self.api("/key/generate", {"key_alias": "回退临时验证", "owner_type": "personal",
                    "user_id": key["user_id"], "team_id": key["team_id"], "project_id": key["project_id"], "models": []})
                marker = "fallback-" + self.run
                self.call({**key, **temp}, marker, alias=alias)
                rows = [r for r in self.observer.rows if marker in r["messages"]]
                if not any(r["fault"] and r["status"] == 429 for r in rows) or not any(r["forwarded"] and r["status"] == 200 for r in rows):
                    raise AssertionError("回退没有从注入429到真实供应商200")
                self.report["checks"].append({"name": "429-fallback-to-real-supplier", "passed": True})
            finally:
                if temp:
                    self.api("/key/delete", {"keys": [temp["key"]]})
                self.api("/model/fallback", {"model_name": alias, "policy": {}}, method="PUT")
                self.api("/model/delete", {"id": fault_id})
                self.state["deployments"] = [d for d in self.state["deployments"] if d["id"] != fault_id]
                self.api("/team/update", {"team_id": key["team_id"], "models": [self.data["routing"]["alias"], self.data["routing"]["backup_alias"]]})
        self.report["status"] = "passed"
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
            self.progress(f"    [权限] 角色={role} 通过，可见团队={len(teams)}")
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
            marker = "xgo-" + self.run + "-" + scenario["action"]
            self.call(key, marker, scenario["text"], expected=scenario["expected_http_status"], guardrails=[name])
            rows = [r for r in self.observer.rows if marker in r["messages"]] if self.observer else []
            if self.observer and scenario["action"] == "block" and rows:
                raise AssertionError("XGo拦截发生了上游外发")
            if self.observer and scenario["action"] == "modify" and (not rows or any(
                    scenario["text"] in r["messages"] or scenario["replacement"] not in r["messages"] for r in rows)):
                raise AssertionError("XGo脱敏没有作用于真实上游正文")
            self.report["checks"].append({"name": name + "-persist-debug-real-request", "passed": True})
            self.progress(f"    [XGo] 护栏={name} 持久化、调试和真实请求通过")


    def refuse(self, key, marker, expected, needle=None, alias=None):
        """用途：验证本地拒绝无外发；参数为密钥、标记、状态及错误关键词，返回错误；用于额度/撤销/下架，不产生收费调用。"""
        before = len(self.observer.rows) if self.observer else 0
        answer = self.call(key, marker, alias=alias, expected=expected)
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
            finally:
                if temp:
                    self.api("/key/delete", {"keys": [temp["key"]]})
        # 基线81把钥匙保持不变，生命周期使用成员自己签发的临时密钥。
        if not self.checked("model-allowlist-no-egress"):
            self.progress("    [模型白名单] 验证未授权模型在本地拒绝且无上游外发")
            scenario = self.data["limit_scenarios"]["model_allowlist"]
            self.refuse(key, "allowlist-" + self.run, scenario["expected_status"], alias=scenario["disallowed_alias"])
            self.report["checks"].append({"name": "model-allowlist-no-egress", "passed": True})
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
        """用途：验收真实模型添加上架下架删除；参数为个人密钥，返回无；下架无外发，恢复后真实回答，finally删除临时部署。"""
        if self.checked("model-add-disable-refuse-enable-real-request"):
            return
        alias = self.data["model_lifecycle"]["alias"]
        self.progress(f"    [模型生命周期] 模型={alias} 创建、调用、下架、恢复、删除")
        provider, model = self.state["selected"][1]
        ident = self.deployment(alias, provider, model)
        allowed = [self.data["routing"]["alias"], self.data["routing"]["backup_alias"]]
        temp = None
        try:
            self.api("/team/update", {"team_id": key["team_id"], "models": allowed + [alias]})
            temp, _ = self.api("/key/generate", {"owner_type": "personal", "user_id": key["user_id"],
                "team_id": key["team_id"], "project_id": key["project_id"], "key_alias": "模型生命周期", "models": [alias]})
            subject = {**key, **temp}
            self.call(subject, "model-listed-" + self.run, alias=alias)
            self.api("/model/disable", {"model_name": alias})
            # 禁用返回的具体状态是现有网关契约；断言拒绝且没有绕行备用部署。
            status, answer, _ = http(self.gateway, "/v1/chat/completions", {"model": alias,
                "messages": [{"role": "user", "content": "model-unlisted-" + self.run}]}, subject["key"])
            if status < 400 or any("model-unlisted-" + self.run in r["messages"] for r in self.observer.rows):
                raise AssertionError("下架模型仍然外发或成功回答")
            self.api("/model/enable", {"model_name": alias})
            self.call(subject, "model-relisted-" + self.run, alias=alias)
            self.report["checks"].append({"name": "model-add-disable-refuse-enable-real-request", "disabled_status": status, "passed": True})
        finally:
            if temp:
                self.api("/key/delete", {"keys": [temp["key"]]})
            self.api("/model/delete", {"id": ident})
            self.state["deployments"] = [d for d in self.state["deployments"] if d["id"] != ident]
            self.api("/team/update", {"team_id": key["team_id"], "models": allowed})

    def checked(self, name):
        """用途：判断续跑场景已有成功证据；参数为唯一场景名，返回布尔；只跳过明确通过项，失败和未执行项继续运行。"""
        return any(c["name"] == name and c.get("passed") for c in self.report["checks"])

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
