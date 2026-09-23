#!/usr/bin/env python3
"""Rewrite L1 contract files so every catalog path appears with 8 template sections."""
from __future__ import annotations

import json
from collections import defaultdict
from pathlib import Path

from schemas import REQ_FIELDS, REQ_JSON, RESP_FIELDS, RESP_JSON

DOCS = Path(__file__).resolve().parents[1]
CAT = json.loads((DOCS / "_inventory" / "catalog.json").read_text())
CONTRACT_ROOT = DOCS / "backend-api" / "contracts"

META: dict[str, tuple[str, str, str, str, bool]] = {
    "data.chat": ("data/chat-completions.md", "Chat Completions", "virtual-key", "/playground", True),
    "data.completions": ("data/completions.md", "Text Completions", "virtual-key", "/playground", True),
    "data.messages": ("data/messages.md", "Anthropic Messages", "virtual-key", "/playground", True),
    "data.responses": ("data/responses.md", "Responses API", "virtual-key", "/playground", True),
    "data.embeddings": ("data/embeddings.md", "Embeddings", "virtual-key", "/playground", True),
    "data.images": ("data/images.md", "Images", "virtual-key", "/playground", True),
    "data.audio": ("data/audio.md", "Audio", "virtual-key", "/playground", True),
    "data.moderations": ("data/moderations.md", "Moderations", "virtual-key", "—", True),
    "data.rerank": ("data/rerank.md", "Rerank", "virtual-key", "—", True),
    "data.files": ("data/files.md", "Files", "virtual-key", "/vector-stores", True),
    "data.batches": ("data/batches.md", "Batches", "virtual-key", "—", True),
    "data.assistants_threads": ("data/assistants-threads.md", "Assistants and Threads", "virtual-key", "—", True),
    "data.fine_tuning": ("data/fine-tuning.md", "Fine-tuning", "virtual-key", "—", True),
    "data.containers": ("data/containers.md", "Containers", "virtual-key", "—", True),
    "data.vector_stores": ("data/vector-stores.md", "Vector Stores (data plane)", "virtual-key", "/vector-stores", True),
    "data.videos": ("data/videos.md", "Videos", "virtual-key", "—", True),
    "data.realtime": ("data/realtime.md", "Realtime", "virtual-key", "—", True),
    "data.search_ocr_rag": ("data/search-ocr-rag.md", "Search, OCR, RAG", "virtual-key", "/search-tools", True),
    "data.skills_tools_memory": ("data/skills-tools-memory.md", "Skills, Tools, Memory", "mixed", "/skills /memory /tool-policies", True),
    "data.evals": ("data/evals.md", "Evals", "virtual-key", "/cost-optimization", True),
    "data.workflows": ("data/workflows.md", "Workflows", "mixed", "/workflows", True),
    "data.agents": ("data/agents.md", "Agents", "mixed", "/agents", True),
    "data.access_groups": ("data/access-groups.md", "Access Groups (data plane)", "mixed", "/access-groups", False),
    "data.mcp": ("data/mcp.md", "MCP data plane", "mixed", "/mcp-servers /mcp/oauth/callback", True),
    "data.a2a": ("data/a2a.md", "A2A", "virtual-key", "/agents", True),
    "data.gemini_v1beta": ("data/gemini.md", "Gemini v1beta", "virtual-key", "/playground", True),
    "data.passthrough": ("data/passthrough.md", "Provider Passthrough", "virtual-key", "—", True),
    "data.model_hub": ("data/models.md", "Models list and Model Hub", "virtual-key", "/model-hub-table /model_hub", False),
    "data.interactions": ("data/interactions.md", "Interactions", "virtual-key", "/agents", True),
    "data.claude_code": ("data/claude-code.md", "Claude Code and plugins", "mixed", "/skills", True),
    "mgmt.keys": ("management/keys.md", "Virtual Keys", "management", "/api-keys /chat/api-keys", False),
    "mgmt.users": ("management/users.md", "Internal Users", "management", "/users", False),
    "mgmt.teams": ("management/teams.md", "Teams", "management", "/teams", False),
    "mgmt.organizations": ("management/organizations.md", "Organizations", "management", "/organizations", False),
    "mgmt.projects": ("management/projects.md", "Projects", "management", "/projects", False),
    "mgmt.budgets": ("management/budgets.md", "Budgets", "management", "/budgets", False),
    "mgmt.models": ("management/models.md", "Model admin", "management", "/models-and-endpoints", False),
    "mgmt.guardrails": ("management/guardrails.md", "Guardrails admin", "management", "/guardrails /guardrails-monitor", False),
    "mgmt.policies": ("management/policies.md", "Policies", "management", "/policies /tool-policies", False),
    "mgmt.prompts": ("management/prompts.md", "Prompts", "management", "/prompts", False),
    "mgmt.credentials": ("management/credentials.md", "Credentials", "management", "/models-and-endpoints /chat/credentials", False),
    "mgmt.tags": ("management/tags.md", "Tags", "management", "/tag-management", False),
    "mgmt.spend": ("management/spend-usage.md", "Spend, usage, logs", "management", "/usage /logs /old-usage /chat/usage /chat/logs", False),
    "mgmt.config": ("management/config.md", "Config and YAML", "management", "/models-and-endpoints /router-settings", False),
    "mgmt.sso": ("management/sso.md", "SSO and login", "mixed", "/login /admin-panel /onboarding", False),
    "mgmt.scim": ("management/scim.md", "SCIM", "management", "/admin-panel", False),
    "mgmt.mcp_servers": ("management/mcp-servers.md", "MCP server admin", "management", "/mcp-servers", False),
    "mgmt.search_tools": ("management/search-tools.md", "Search tool admin", "management", "/search-tools", False),
    "mgmt.vector_stores_admin": ("management/vector-stores.md", "Vector store admin", "management", "/vector-stores", False),
    "mgmt.health": ("management/health.md", "Health and diagnostics", "mixed", "shell", False),
    "mgmt.callbacks": ("management/callbacks-alerting.md", "Callbacks and logging hooks", "management", "/logging-and-alerts /chat/integrations", False),
    "mgmt.cache": ("management/cache.md", "Cache admin", "management", "/caching", False),
    "mgmt.router": ("management/router.md", "Router admin", "management", "/router-settings /cost-optimization", False),
    "mgmt.jwt_oidc": ("management/jwt.md", "JWT key mapping", "management", "/admin-panel", False),
    "mgmt.customers": ("management/customers.md", "Customers and end users", "management", "—", False),
    "mgmt.invitations": ("management/invitations.md", "Invitations", "management", "/onboarding /users", False),
    "mgmt.compliance": ("management/compliance.md", "Compliance", "management", "/admin-panel", False),
    "mgmt.cost_export": ("management/cost-export.md", "CloudZero and Vantage export", "management", "/cost-tracking", False),
    "mgmt.ui_settings": ("management/ui-settings.md", "UI settings and theme", "management", "/ui-theme /admin-panel", False),
    "mgmt.public": ("management/public.md", "Public discovery hubs", "public-or-key", "/model_hub /connect", False),
    "mgmt.debug": ("management/debug.md", "Debug and memory", "master-key", "—", False),
    "mgmt.ops_schedules": ("management/schedules.md", "Ops schedules", "master-key", "—", False),
    "mgmt.audit": ("management/audit.md", "Audit logs", "management", "/admin-panel", False),
    "mgmt.email": ("management/email.md", "Email event settings", "management", "/logging-and-alerts", False),
    "mgmt.enterprise_misc": ("management/enterprise-misc.md", "Enterprise miscellaneous", "management", "—", False),
    "mgmt.utils": ("management/utils.md", "Utils and transform", "management", "/transform-request /api-reference", False),
    "mgmt.allowed_ips": ("management/allowed-ips.md", "Allowed IPs", "management", "/admin-panel", False),
    "mgmt.alerting": ("management/alerting.md", "Alerting settings", "management", "/logging-and-alerts", False),
    "mgmt.placeholders": ("management/placeholders.md", "Placeholders", "management", "—", False),
    "mgmt.onboarding": ("management/onboarding.md", "Onboarding tokens", "mixed", "/onboarding", False),
}

_DATA_GROUP = {
    "data.chat": "inference",
    "data.completions": "inference",
    "data.messages": "inference",
    "data.responses": "inference",
    "data.embeddings": "inference",
    "data.moderations": "inference",
    "data.rerank": "inference",
    "data.gemini_v1beta": "inference",
    "data.interactions": "inference",
    "data.images": "media",
    "data.audio": "media",
    "data.videos": "media",
    "data.realtime": "media",
    "data.files": "resources",
    "data.batches": "resources",
    "data.assistants_threads": "resources",
    "data.fine_tuning": "resources",
    "data.containers": "resources",
    "data.vector_stores": "resources",
    "data.search_ocr_rag": "resources",
    "data.skills_tools_memory": "platform",
    "data.evals": "platform",
    "data.workflows": "platform",
    "data.agents": "platform",
    "data.access_groups": "platform",
    "data.mcp": "platform",
    "data.a2a": "platform",
    "data.passthrough": "platform",
    "data.model_hub": "platform",
    "data.claude_code": "platform",
}
_MGMT_GROUP = {
    "mgmt.keys": "identity",
    "mgmt.users": "identity",
    "mgmt.teams": "identity",
    "mgmt.organizations": "identity",
    "mgmt.projects": "identity",
    "mgmt.budgets": "identity",
    "mgmt.customers": "identity",
    "mgmt.invitations": "identity",
    "mgmt.onboarding": "identity",
    "mgmt.models": "catalog",
    "mgmt.credentials": "catalog",
    "mgmt.guardrails": "governance",
    "mgmt.policies": "governance",
    "mgmt.prompts": "governance",
    "mgmt.tags": "governance",
    "mgmt.spend": "observability",
    "mgmt.mcp_servers": "platform",
    "mgmt.search_tools": "platform",
    "mgmt.vector_stores_admin": "platform",
    "mgmt.cache": "platform",
    "mgmt.router": "platform",
    "mgmt.config": "platform",
    "mgmt.sso": "platform",
    "mgmt.scim": "platform",
    "mgmt.jwt_oidc": "platform",
    "mgmt.ui_settings": "platform",
    "mgmt.health": "platform",
    "mgmt.callbacks": "platform",
    "mgmt.public": "platform",
    "mgmt.debug": "platform",
    "mgmt.ops_schedules": "platform",
    "mgmt.audit": "platform",
    "mgmt.email": "platform",
    "mgmt.enterprise_misc": "platform",
    "mgmt.utils": "platform",
    "mgmt.allowed_ips": "platform",
    "mgmt.alerting": "platform",
    "mgmt.placeholders": "platform",
    "mgmt.compliance": "platform",
    "mgmt.cost_export": "platform",
}

def _nest_meta() -> None:
    for fid, (rel, title, auth, console, spend) in list(META.items()):
        name = rel.split("/")[-1]
        if fid.startswith("data."):
            META[fid] = (f"data/{_DATA_GROUP[fid]}/{name}", title, auth, console, spend)
        else:
            META[fid] = (f"management/{_MGMT_GROUP[fid]}/{name}", title, auth, console, spend)

_nest_meta()


def assign(path: str) -> str:
    p = path
    if p == "/":
        return "mgmt.health"
    if p.startswith("/.well-known/skills") or p.startswith("/.well-known/agent-skills"):
        return "data.skills_tools_memory"
    if "mcp" in p and p.startswith("/.well-known"):
        return "data.mcp"
    if "litellm-cli-auth" in p or "litellm-ui-config" in p:
        return "mgmt.sso"
    if p.startswith("/.well-known/jwks") or p.startswith("/.well-known/openid"):
        return "mgmt.sso"
    if "oauth-authorization-server" in p or "oauth-protected-resource" in p:
        return "data.mcp"
    if "/chat/completions" in p or p.startswith("/cursor/chat") or p.startswith("/queue/chat"):
        return "data.chat"
    if "/completions" in p and "chat" not in p:
        return "data.completions"
    if p.startswith("/v1/messages") or p == "/messages":
        return "data.messages"
    if "/responses" in p:
        return "data.responses"
    if "/embeddings" in p:
        return "data.embeddings"
    if "/images/" in p or p.endswith("/images"):
        return "data.images"
    if "/audio/" in p:
        return "data.audio"
    if "moderations" in p:
        return "data.moderations"
    if "rerank" in p:
        return "data.rerank"
    if "/files" in p:
        return "data.files"
    if "/batches" in p:
        return "data.batches"
    if "/assistants" in p or "/threads" in p:
        return "data.assistants_threads"
    if "/fine_tuning" in p:
        return "data.fine_tuning"
    if "/containers" in p:
        return "data.containers"
    if "vector_stores" in p:
        return "data.vector_stores"
    if p.startswith("/vector_store/") or p.startswith("/vector_store"):
        return "mgmt.vector_stores_admin"
    if p.startswith("/videos") or p.startswith("/v1/videos"):
        return "data.videos"
    if "/realtime" in p or p.startswith("/vertex_ai/live"):
        return "data.realtime"
    if (
        p.startswith("/v1/search")
        or p.startswith("/search/")
        or p == "/search"
        or "/ocr" in p
        or "/rag/" in p
        or p.startswith("/v1/indexes")
    ):
        if p.startswith("/search_tools"):
            return "mgmt.search_tools"
        return "data.search_ocr_rag"
    if p.startswith("/search_tools"):
        return "mgmt.search_tools"
    if p.startswith("/v1/skills") or p.startswith("/v1/tool") or p.startswith("/v1/memory"):
        return "data.skills_tools_memory"
    if p.startswith("/v1/evals"):
        return "data.evals"
    if "/workflows" in p:
        return "data.workflows"
    if (
        p.startswith("/v1/agents")
        or p.startswith("/v1beta/agents")
        or p.startswith("/agent/")
        or p.startswith("/make_public")
    ):
        return "data.agents"
    if p.startswith("/v1/access_group") or p.startswith("/access_group"):
        return "data.access_groups"
    if (
        p.startswith("/mcp")
        or p.startswith("/v1/mcp")
        or "{mcp_server_name}" in p
        or p.startswith("/authorize")
        or p in ("/token", "/register", "/callback", "/introspect", "/revoke", "/discover", "/enabled", "/token/generate")
    ):
        return "data.mcp"
    if p.startswith("/a2a") or "/a2a/" in p:
        return "data.a2a"
    if ":generateContent" in p or ":streamGenerateContent" in p or ":countTokens" in p:
        return "data.gemini_v1beta"
    if p.startswith("/v1beta/models"):
        return "data.gemini_v1beta"
    if p.startswith("/interactions") or p.startswith("/v1beta/interactions"):
        return "data.interactions"
    if p.startswith("/claude-code") or p.startswith("/api/plugins") or p.startswith("/api/event_logging"):
        return "data.claude_code"
    if (
        p.startswith("/openai/")
        or p.startswith("/anthropic/")
        or p.startswith("/bedrock/")
        or p.startswith("/gemini/")
        or p.startswith("/azure/")
        or p.startswith("/vertex_ai/")
        or p.startswith("/comprehendmedical")
        or p.startswith("/openai_passthrough")
        or p.startswith("/{provider}")
    ):
        return "data.passthrough"
    if (
        p in ("/v1/models", "/models")
        or p.startswith("/v1/models/")
        or p.startswith("/model_hub")
        or p.startswith("/public/model_hub")
        or p.startswith("/cursor/models")
        or "model/deprecations" in p
        or p.startswith("/models/")
    ):
        return "data.model_hub"
    if p.startswith("/key") or p.startswith("/v2/key"):
        return "mgmt.keys"
    if p.startswith("/user-credentials") or p.startswith("/user-env-vars"):
        return "mgmt.mcp_servers"
    if p.startswith("/user/available_users"):
        return "mgmt.enterprise_misc"
    if p.startswith("/user") or p.startswith("/v2/user"):
        return "mgmt.users"
    if p.startswith("/team") or p.startswith("/v2/team"):
        return "mgmt.teams"
    if p.startswith("/organization") or p.startswith("/v2/organization"):
        return "mgmt.organizations"
    if p.startswith("/project"):
        return "mgmt.projects"
    if p.startswith("/budget"):
        return "mgmt.budgets"
    if p.startswith("/model/") or p.startswith("/v1/model/") or p.startswith("/v2/model/") or p.startswith("/model_group"):
        return "mgmt.models"
    if p.startswith("/guardrails") or p.startswith("/apply_guardrail") or p.startswith("/v2/guardrails"):
        return "mgmt.guardrails"
    if p.startswith("/policies") or p.startswith("/policy"):
        return "mgmt.policies"
    if p.startswith("/prompts"):
        return "mgmt.prompts"
    if p.startswith("/credentials"):
        return "mgmt.credentials"
    if p.startswith("/tag"):
        return "mgmt.tags"
    if (
        p.startswith("/spend")
        or p.startswith("/global")
        or p.startswith("/usage")
        or p.startswith("/spend_logs")
        or p.startswith("/gateway/daily")
        or p.startswith("/cost/")
    ):
        return "mgmt.spend"
    if p.startswith("/config") or p.startswith("/reload/") or p.startswith("/config_overrides"):
        return "mgmt.config"
    if p.startswith("/sso") or p in ("/login", "/fallback/login") or p.startswith("/v2/login") or p.startswith("/v3/login"):
        return "mgmt.sso"
    if p.split("/")[1] in {"Users", "Groups", "ResourceTypes", "Schemas", "ServiceProviderConfig"} if p.startswith("/") and len(p) > 1 else False:
        return "mgmt.scim"
    if p.startswith("/Users") or p.startswith("/Groups") or p.startswith("/ResourceTypes") or p.startswith("/Schemas") or p.startswith("/ServiceProviderConfig"):
        return "mgmt.scim"
    if (
        p.startswith("/server")
        or p.startswith("/toolset")
        or p.startswith("/tools")
        or p == "/access_groups"
        or p == "/registry.json"
        or p.startswith("/openapi-registry")
    ):
        return "mgmt.mcp_servers"
    if p.startswith("/health") or p in ("/test", "/settings") or p.startswith("/test/"):
        return "mgmt.health"
    if p.startswith("/callbacks") or p.startswith("/active/callbacks") or p.startswith("/get/config/callbacks") or p == "/logs":
        return "mgmt.callbacks"
    if p.startswith("/cache") or p in ("/ping", "/flushall") or p == "/delete" or p.startswith("/redis") or p.startswith("/coordination_redis"):
        return "mgmt.cache"
    if p.startswith("/router") or p.startswith("/auto_router") or p.startswith("/adaptive_router") or p.startswith("/fallback") or p == "/routes":
        return "mgmt.router"
    if p.startswith("/jwt"):
        return "mgmt.jwt_oidc"
    if p.startswith("/customer") or p.startswith("/end_user"):
        return "mgmt.customers"
    if p.startswith("/invitation"):
        return "mgmt.invitations"
    if p.startswith("/compliance"):
        return "mgmt.compliance"
    if p.startswith("/cloudzero") or p.startswith("/vantage"):
        return "mgmt.cost_export"
    if p.startswith("/get/allowed_ips") or p.startswith("/add/allowed_ip") or p.startswith("/delete/allowed_ip"):
        return "mgmt.allowed_ips"
    if (
        p.startswith("/get/")
        or p.startswith("/update/")
        or p.startswith("/upload/")
        or p.startswith("/get_logo")
        or p.startswith("/get_favicon")
        or p.startswith("/get_image")
        or p == "/upload/logo"
    ):
        return "mgmt.ui_settings"
    if p.startswith("/public"):
        return "mgmt.public"
    if p.startswith("/debug") or p.startswith("/memory-usage") or p.startswith("/otel-spans") or p.startswith("/lazy"):
        return "mgmt.debug"
    if p.startswith("/schedule"):
        return "mgmt.ops_schedules"
    if p.startswith("/audit"):
        return "mgmt.audit"
    if p.startswith("/email"):
        return "mgmt.email"
    if p.startswith("/utils"):
        return "mgmt.utils"
    if p.startswith("/alerting"):
        return "mgmt.alerting"
    if p.startswith("/placeholders"):
        return "mgmt.placeholders"
    if p.startswith("/onboarding"):
        return "mgmt.onboarding"
    if p.startswith("/engines/"):
        if "chat" in p:
            return "data.chat"
        if "embeddings" in p:
            return "data.embeddings"
        return "data.completions"
    if p.startswith("/cursor/"):
        return "data.chat"
    if p.startswith("/queue/"):
        return "data.chat"
    if p.startswith("/v2/"):
        return "mgmt.spend"
    if p.startswith("/v3/"):
        return "mgmt.sso"
    if p.startswith("/network") or p.startswith("/litellm") or p.startswith("/provider/") or p in ("/log-event", "/robots.txt"):
        return "mgmt.enterprise_misc"
    if p.startswith("/add/") or p.startswith("/delete/"):
        return "mgmt.allowed_ips"
    return "mgmt.enterprise_misc"


def norm(path: str) -> str:
    return path.replace(":path", "").rstrip("/") or "/"


def request_section(fam: str) -> str:
    table = {
        "data.chat": (
            "Content-Type `application/json`。必填 `model`（Router 别名）、`messages`。\n"
            "网关差集：发上游前替换为 deployment 真实模型名；`n` 计入费用与 TPM，Provider 不支持则 400；"
            "`stream` / `stream_options.include_usage`；`user`/`end_user` 进 End User 预算；可选 `Idempotency-Key`；"
            "`tools`/`tool_choice`/`response_format`/`temperature`/`max_tokens`/`max_completion_tokens`/`reasoning_effort` 按 capability 与 `drop_params`。\n"
            "网关扩展字段：`guardrails`、`caching`、`fallbacks`、`metadata.tags`、`session_id`。禁止发明自有字段名。"
        ),
        "data.completions": "必填 `model`、`prompt`。`max_tokens` 默认 16，允许 0。`n`/`best_of`/`echo`/`logprobs`/`stream` 按 OpenAI Completions。",
        "data.messages": "Anthropic 原生：`model`、`messages`、`max_tokens`、`tools`、`thinking`、`output_config.effort`。`reasoning_effort` 映射到 thinking budget。`/v1/messages/count_tokens` 只计 token。",
        "data.responses": "`model`、`input`、`instructions`、`tools`、`previous_response_id`、`store`、`stream`。Azure 走 `/openai/responses`，默认 version `preview`。",
        "data.embeddings": "必填 `model`、`input`。",
        "data.images": "Generations：JSON `model`、`prompt`、`size`、`n`。Edits：multipart `image` + `prompt`。",
        "data.audio": "Speech：JSON `model`、`input`、`voice`。Transcriptions：multipart `file` + `model`。",
        "data.files": "POST multipart `file`、`purpose`。",
        "data.batches": "`input_file_id`、`endpoint`、`completion_window`、`metadata`。",
        "mgmt.keys": (
            "JSON 对齐 `GenerateKeyRequest`：`key_alias`、`duration`、`models`、`max_budget`、`soft_budget`、"
            "`user_id`、`team_id`、`organization_id`、`project_id`、`agent_id`、`tpm_limit`、`rpm_limit`、"
            "`max_parallel_requests`、`budget_duration`、`metadata`、`permissions`、`guardrails`、`policies`、"
            "`object_permission`、`allowed_routes`、`key_type`、`auto_rotate`、`rotation_interval`、`tags`、"
            "`model_rpm_limit`、`model_tpm_limit`、`budget_id`。update 未出现字段保持原值。"
        ),
        "mgmt.users": "`user_email` 必填。`user_role`、`user_alias`、`max_budget`、`models`、`teams`。",
        "mgmt.teams": "`team_alias`、`organization_id`、`models`、`max_budget`、成员 `user_id`/`role`。",
        "mgmt.models": "`model_name` + `litellm_params` + `model_info`。",
        "mgmt.scim": "SCIM 2.0 JSON：User/Group schemas as in `scim_v2.py`。",
    }
    fields = REQ_FIELDS.get(fam)
    if fields:
        return f"冻结字段（不得改名）：`{fields.replace(', ', '`、`')}`。"
    return (
        "冻结字段见请求体 JSON 的每个键；GET 用 query：`page`、`page_size`、`start_date`、`end_date`。"
        " 禁止把字段权威推给 Python handler。"
    )


def response_section(fam: str) -> str:
    table = {
        "data.chat": "非流式 OpenAI `chat.completion`，`model` 为对外别名。流式 `text/event-stream`，结束 `data: [DONE]`。",
        "data.messages": "Anthropic `message`；SSE 为官方 `content_block_delta` 等。",
        "data.responses": "`object=response`，含 `id`、`status`、`output`、`usage`。",
        "mgmt.keys": "`GenerateKeyResponse` 含一次明文 `key`。list/info 不得返回完整明文。",
        "mgmt.spend": "logs 为请求行；`/global/spend` 与 daily activity 为聚合。`spend` 为 USD float。",
    }
    fields = RESP_FIELDS.get(fam)
    if fields:
        return f"冻结响应字段：`{fields.replace(', ', '`、`')}`。"
    return "响应字段以响应体 JSON 键为准，不得省略。"


def frozen_schema(fam: str) -> str:
    table = {
        "data.chat": "OpenAI Chat Completions + 网关扩展字段",
        "mgmt.keys": "`GenerateKeyRequest` / `UpdateKeyRequest` 字段集",
        "mgmt.scim": "SCIM 2.0 User/Group",
    }
    return table.get(fam, "该路由 handler 的请求体模型与上游 SDK 透传字段")


def req_headers(auth: str, spend: bool) -> str:
    if auth in ("virtual-key", "mixed") and spend:
        rows = [
            ("Authorization", "是*", "`Bearer sk-...`"),
            ("x-litellm-api-key", "否", "若出现则优先于 Authorization"),
            ("Content-Type", "是", "`application/json` 或 `multipart/form-data`"),
            ("Idempotency-Key", "否", "有则重放"),
            ("x-litellm-tags", "否", "标签"),
            ("x-litellm-end-user-id", "否", "终端用户"),
        ]
    else:
        rows = [
            ("Authorization", "是", "`Bearer` master key 或 session"),
            ("Content-Type", "JSON 时是", "`application/json` 或 multipart"),
        ]
    lines = ["| Header | 必填 | 说明 |", "|---|---|---|"]
    for h, req, note in rows:
        lines.append(f"| `{h}` | {req} | {note} |")
    return "\n".join(lines)


def resp_headers(spend: bool) -> str:
    if spend:
        hs = [
            "x-litellm-call-id",
            "x-litellm-model-id",
            "x-litellm-model-name",
            "x-litellm-model-api-base",
            "x-litellm-version",
            "x-litellm-response-cost",
            "x-litellm-response-cost-original",
            "x-litellm-response-cost-input",
            "x-litellm-response-cost-output",
            "x-litellm-key-tpm-limit",
            "x-litellm-key-rpm-limit",
            "x-litellm-key-max-budget",
            "x-litellm-key-spend",
            "x-litellm-cache-key",
            "x-litellm-response-duration-ms",
        ]
    else:
        hs = ["x-litellm-call-id", "Content-Type"]
    lines = ["| Header | 说明 |", "|---|---|"]
    for h in hs:
        lines.append(f"| `{h}` | 见 [错误与响应头](../../../architecture/errors-headers.md) |")
    lines.append("| `Retry-After` | 429 时 |")
    return "\n".join(lines)


def req_body_example(fam: str) -> str:
    examples = {
        "data.chat": '''```json
{
  "model": "gpt-4o-mini",
  "messages": [{"role": "user", "content": "Hello"}],
  "temperature": 0.2,
  "max_tokens": 256,
  "stream": false,
  "tools": [],
  "user": "user_123"
}
```''',
        "data.completions": '''```json
{"model": "gpt-4o-mini", "prompt": "Hello", "max_tokens": 16, "n": 1}
```''',
        "data.messages": '''```json
{"model": "claude-sonnet-4-20250514", "messages": [{"role": "user", "content": "Hi"}], "max_tokens": 256}
```''',
        "data.responses": '''```json
{"model": "gpt-4o-mini", "input": "Hello", "stream": false}
```''',
        "data.embeddings": '''```json
{"model": "text-embedding-3-small", "input": ["hello"]}
```''',
        "data.images": '''```json
{"model": "dall-e-3", "prompt": "a cat", "n": 1, "size": "1024x1024"}
```''',
        "data.audio": '''Speech JSON：
```json
{"model": "tts-1", "input": "Hello", "voice": "alloy"}
```
Transcriptions：multipart `file` + `model`。''',
        "mgmt.keys": '''```json
{
  "key_alias": "checkout-prod",
  "models": ["gpt-4o-mini"],
  "max_budget": 10.0,
  "tpm_limit": 100000,
  "rpm_limit": 60,
  "duration": "30d",
  "team_id": null
}
```''',
        "mgmt.users": '''```json
{"user_email": "dev@example.com", "user_role": "internal_user", "max_budget": 50}
```''',
        "mgmt.teams": '''```json
{"team_alias": "platform", "models": ["gpt-4o-mini"], "max_budget": 100}
```''',
        "mgmt.models": '''```json
{
  "model_name": "gpt-4o-mini",
  "litellm_params": {"model": "openai/gpt-4o-mini", "api_key": "os.environ/OPENAI_API_KEY"}
}
```''',
    }
    body = REQ_JSON.get(fam)
    if body:
        if body.strip().startswith("{"):
            return f"```json\n{body}\n```"
        return body
    return "```json\n{\n  \"page\": 1,\n  \"page_size\": 50\n}\n```"


def resp_body_example(fam: str) -> str:
    examples = {
        "data.chat": '''```json
{
  "id": "chatcmpl_01",
  "object": "chat.completion",
  "created": 1710000000,
  "model": "gpt-4o-mini",
  "choices": [{"index": 0, "message": {"role": "assistant", "content": "Hi"}, "finish_reason": "stop"}],
  "usage": {"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10}
}
```
流式：`text/event-stream`，最后 `data: [DONE]`。''',
        "data.embeddings": '''```json
{"object": "list", "data": [{"object": "embedding", "index": 0, "embedding": [0.1]}], "model": "text-embedding-3-small", "usage": {"prompt_tokens": 1, "total_tokens": 1}}
```''',
        "mgmt.keys": '''```json
{"key": "sk-...", "key_name": "sk-...xxxx", "key_alias": "checkout-prod", "expires": null, "token_id": "hash"}
```
list/info 不含完整明文 `key`。''',
        "mgmt.spend": '''```json
{"data": [{"request_id": "req_01", "model": "gpt-4o-mini", "spend": 0.0001, "prompt_tokens": 8, "completion_tokens": 2}]}
```''',
    }
    body = RESP_JSON.get(fam)
    if body:
        if body.strip().startswith("{") or body.strip().startswith("["):
            return f"```json\n{body}\n```"
        return f"```json\n{body}\n```" if body.lstrip().startswith("{") else body
    return "```json\n{\n  \"id\": \"obj_01\",\n  \"object\": \"resource\",\n  \"created_at\": 1710000000\n}\n```"


def auth_text(auth: str) -> str:
    return {
        "virtual-key": "`Authorization: Bearer sk-...` 或 `x-litellm-api-key`。`key_type` 为 `llm_api` 或 `default`。master key 默认不能调本族，除非 `general_settings` 显式允许。",
        "management": "master key、SSO session，或 `key_type=management` / 具备对应角色的虚拟 Key。按 `user_api_key_auth`。",
        "public-or-key": "公开发现路径可无 Key；写操作需要虚拟 Key 或 session。",
        "master-key": "仅 master key / proxy_admin。",
        "mixed": "数据面虚拟 Key，管理面 session/master key。OAuth 回调走浏览器 session。见 [鉴权决策](../../../architecture/auth-decision.md)。",
    }.get(auth, "见 [鉴权决策](../../../architecture/auth-decision.md)。")


def main() -> None:
    by_fam: dict[str, list[dict]] = defaultdict(list)
    unassigned: list[str] = []
    for r in CAT["http_routes"]:
        fam = assign(r["path"])
        if not fam:
            unassigned.append(r["path"])
            continue
        by_fam[fam].append(r)
    print("unassigned", len(unassigned))
    for p in unassigned:
        print("UNASSIGNED", p)

    routed = {norm(r["path"]) for r in CAT["http_routes"]}
    for fam_meta in CAT["families"]:
        fid = fam_meta["id"]
        if fid not in META:
            continue
        for hp in fam_meta.get("http_paths") or []:
            if norm(hp) in routed:
                continue
            by_fam[fid].append({"method": "GET", "path": hp, "source": "catalog.families", "line": 0})
            by_fam[fid].append({"method": "POST", "path": hp, "source": "catalog.families", "line": 0})
            print("inject", fid, hp)

    enterprise = {f["id"] for f in CAT["families"] if f.get("enterprise")}

    data_dir = CONTRACT_ROOT / "data"
    mgmt_dir = CONTRACT_ROOT / "management"
    for folder in (data_dir, mgmt_dir):
        if not folder.exists():
            continue
        for p in folder.rglob("*.md"):
            if p.name != "README.md":
                p.unlink()

    for fam, rows in sorted(by_fam.items()):
        rel, title, auth, console, spend = META[fam]
        status = "independent-impl" if fam in enterprise else "specified"
        dest = CONTRACT_ROOT / rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        uniq = []
        seen: set[tuple[str, str]] = set()
        for r in sorted(rows, key=lambda x: (x["path"], x["method"])):
            k = (r["method"], r["path"])
            if k in seen:
                continue
            seen.add(k)
            uniq.append(r)
        lines = ["| Method | Path | 规范化 path | 参考 |", "|---|---|---|---|"]
        mentioned_norms = set()
        for r in uniq:
            raw = r["path"]
            n = norm(raw)
            mentioned_norms.add(n)
            src = r["source"].replace("litellm/", "", 1)
            lines.append(f"| `{r['method']}` | `{raw}` | `{n}` | `{src}:{r['line']}` |")
        extra = ""
        if spend:
            spend_txt = "数据面调用记 spend、占 RPM/TPM/并发、写 SpendLogs，可走 Guardrail 与响应缓存。见 [生命周期](../../../architecture/request-lifecycle.md) 与 [计量](../../../architecture/spend-limits.md)。"
        else:
            spend_txt = "管理写操作记 AuditLog 并失效 Auth cache。测试调用（如 `/prompts/test`、`/health/test_connection`）仍记 spend。"
        text = f"""# {title}

- Family: `{fam}`
- Status: `{status}`
- 参考: 下表定位列
- Console: {console}
- Auth: {auth}

## 目的

本族是网关下列 HTTP 路径的完整分母。字段名与默认值以冻结契约及 {frozen_schema(fam)} 为准，网关不得改名。

## HTTP 表面

实现必须注册每一行 method+path。规范化列用于覆盖校验（去掉 `:path` 与尾斜杠）。

{chr(10).join(lines)}

## 请求

{auth_text(auth)}

{request_section(fam)}

### 请求头

{req_headers(auth, spend)}

### 请求体

{req_body_example(fam)}

## 响应

{response_section(fam)}

### 响应头

{resp_headers(spend)}

### 响应体

{resp_body_example(fam)}

## 错误

OpenAI 兼容路径使用 `{{"error":{{"message","type","code","param"}}}}`。Anthropic 原生用 Anthropic 错误形状。Gemini 用 Google 错误形状。鉴权失败 401，模型不允许 401/403，预算/RPM/TPM 429 + `Retry-After`，参数 400。未实现 Provider 明确错误。共性见 [错误与响应头](../../../architecture/errors-headers.md)。

## 副作用

{spend_txt}

## 控制台绑定

Console: {console}。动作对照 [前端绑定](../../../../frontend/binding.md) 与 [页面规格](../../../../frontend/pages/README.md)。无控制台入口的路径仍须对 SDK/curl 可用。

## 验收

- 本表每一行 method+path 均已注册；未实现时返回 JSON 错误信封，不是 HTML。
- 鉴权失败与成功主路径合同测试。
- 标准客户端只改 base_url + api_key（数据面）。
"""
        dest.write_text(text, encoding="utf-8")
        print("wrote", rel, "rows", len(uniq), "bytes", dest.stat().st_size)
    print("families", len(by_fam))


if __name__ == "__main__":
    main()
