"""Frozen request/response field lists per API family. No handler-deferral."""

REQ_JSON = {}
RESP_JSON = {}
REQ_FIELDS = {}
RESP_FIELDS = {}


def _set(fam, req_fields, req_json, resp_fields, resp_json):
    REQ_FIELDS[fam] = req_fields
    REQ_JSON[fam] = req_json
    RESP_FIELDS[fam] = resp_fields
    RESP_JSON[fam] = resp_json


_set(
    "data.chat",
    "model, messages, temperature, max_tokens, max_completion_tokens, n, stream, stream_options, tools, tool_choice, parallel_tool_calls, response_format, user, seed, stop, presence_penalty, frequency_penalty, logit_bias, logprobs, top_logprobs, reasoning_effort, guardrails, caching, fallbacks, metadata",
    """{
  "model": "gpt-4o-mini",
  "messages": [{"role": "user", "content": "Hello"}],
  "temperature": 0.2,
  "max_tokens": 256,
  "stream": false,
  "n": 1,
  "tools": [],
  "user": "user_123",
  "metadata": {"tags": ["prod"]}
}""",
    "id, object, created, model, choices, usage, system_fingerprint",
    """{
  "id": "chatcmpl_01",
  "object": "chat.completion",
  "created": 1710000000,
  "model": "gpt-4o-mini",
  "choices": [{"index": 0, "message": {"role": "assistant", "content": "Hi"}, "finish_reason": "stop"}],
  "usage": {"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10}
}""",
)
_set(
    "data.completions",
    "model, prompt, max_tokens, temperature, n, best_of, echo, logprobs, stream, stop, suffix, user",
    """{"model": "gpt-4o-mini", "prompt": "Hello", "max_tokens": 16, "n": 1, "echo": false}""",
    "id, object, created, model, choices, usage",
    """{"id": "cmpl_01", "object": "text_completion", "model": "gpt-4o-mini", "choices": [{"text": " world", "index": 0, "finish_reason": "length"}]}""",
)
_set(
    "data.messages",
    "model, messages, max_tokens, system, tools, tool_choice, thinking, temperature, stream, metadata",
    """{"model": "claude-sonnet-4-20250514", "messages": [{"role": "user", "content": "Hi"}], "max_tokens": 256}""",
    "id, type, role, content, model, stop_reason, usage",
    """{"id": "msg_01", "type": "message", "role": "assistant", "content": [{"type": "text", "text": "Hi"}], "stop_reason": "end_turn", "usage": {"input_tokens": 8, "output_tokens": 2}}""",
)
_set(
    "data.responses",
    "model, input, instructions, tools, tool_choice, previous_response_id, store, stream, max_output_tokens, metadata",
    """{"model": "gpt-4o-mini", "input": "Hello", "store": true, "stream": false}""",
    "id, object, status, output, usage, model, created_at",
    """{"id": "resp_01", "object": "response", "status": "completed", "model": "gpt-4o-mini", "output": [], "usage": {"input_tokens": 8, "output_tokens": 2}}""",
)
_set(
    "data.embeddings",
    "model, input, encoding_format, dimensions, user",
    """{"model": "text-embedding-3-small", "input": ["hello"]}""",
    "object, data, model, usage",
    """{"object": "list", "data": [{"object": "embedding", "index": 0, "embedding": [0.01]}], "model": "text-embedding-3-small", "usage": {"prompt_tokens": 1, "total_tokens": 1}}""",
)
_set(
    "data.images",
    "model, prompt, n, size, quality, response_format, user; edits also image (multipart)",
    """{"model": "dall-e-3", "prompt": "a cat", "n": 1, "size": "1024x1024", "response_format": "url"}""",
    "created, data",
    """{"created": 1710000000, "data": [{"url": "https://..."}]}""",
)
_set(
    "data.audio",
    "speech: model, input, voice, response_format, speed; transcriptions: file, model, language, prompt, response_format, temperature",
    """{"model": "tts-1", "input": "Hello", "voice": "alloy", "response_format": "mp3"}""",
    "speech: audio bytes; transcriptions: text, language, duration, segments",
    """{"text": "Hello world"}""",
)
_set(
    "data.moderations",
    "model, input",
    """{"model": "omni-moderation-latest", "input": "text"}""",
    "id, model, results",
    """{"id": "modr_01", "model": "omni-moderation-latest", "results": [{"flagged": false, "categories": {}, "category_scores": {}}]}""",
)
_set(
    "data.rerank",
    "model, query, documents, top_n, return_documents",
    """{"model": "rerank-english-v3.0", "query": "q", "documents": ["a", "b"], "top_n": 2}""",
    "id, results, meta",
    """{"results": [{"index": 1, "relevance_score": 0.9}, {"index": 0, "relevance_score": 0.2}]}""",
)
_set(
    "data.files",
    "file (multipart), purpose, expires_after",
    """{"purpose": "batch"}""",
    "id, object, bytes, created_at, filename, purpose, status",
    """{"id": "file_01", "object": "file", "bytes": 12, "filename": "in.jsonl", "purpose": "batch", "status": "processed"}""",
)
_set(
    "data.batches",
    "input_file_id, endpoint, completion_window, metadata",
    """{"input_file_id": "file_01", "endpoint": "/v1/chat/completions", "completion_window": "24h"}""",
    "id, object, endpoint, status, input_file_id, output_file_id, request_counts, created_at",
    """{"id": "batch_01", "object": "batch", "endpoint": "/v1/chat/completions", "status": "validating", "input_file_id": "file_01", "request_counts": {"total": 0, "completed": 0, "failed": 0}}""",
)
_set(
    "data.assistants_threads",
    "assistants: model, name, instructions, tools; threads: messages; runs: assistant_id",
    """{"model": "gpt-4o-mini", "name": "helper", "instructions": "Be brief"}""",
    "id, object, created_at, plus resource-specific fields",
    """{"id": "asst_01", "object": "assistant", "model": "gpt-4o-mini"}""",
)
_set(
    "data.fine_tuning",
    "model, training_file, hyperparameters, suffix, validation_file",
    """{"model": "gpt-4o-mini", "training_file": "file_01"}""",
    "id, object, model, status, created_at, fine_tuned_model",
    """{"id": "ftjob_01", "object": "fine_tuning.job", "status": "queued", "model": "gpt-4o-mini"}""",
)
_set(
    "data.containers",
    "name, expires_after, file_ids",
    """{"name": "sandbox"}""",
    "id, object, name, status, created_at",
    """{"id": "cntr_01", "object": "container", "name": "sandbox", "status": "running"}""",
)
_set(
    "data.vector_stores",
    "name, file_ids, expires_after, metadata; search: query, max_num_results",
    """{"name": "docs"}""",
    "id, object, name, status, file_counts, created_at",
    """{"id": "vs_01", "object": "vector_store", "name": "docs", "status": "completed"}""",
)
_set(
    "data.videos",
    "model, prompt, seconds, size; remix: prompt",
    """{"model": "sora-2", "prompt": "a cat walking"}""",
    "id, object, status, model, created_at",
    """{"id": "video_01", "object": "video", "status": "queued"}""",
)
_set(
    "data.realtime",
    "session: model, modalities, voice, instructions; client_secrets: expires_after",
    """{"model": "gpt-4o-realtime-preview", "voice": "alloy"}""",
    "WebSocket session events type+event_id; HTTP client_secret",
    """{"client_secret": {"value": "ek_...", "expires_at": 1710000000}}""",
)
_set(
    "data.search_ocr_rag",
    "search: query, max_results; ocr: file/url; rag ingest: documents; rag query: query",
    """{"query": "what is xhub", "max_results": 5}""",
    "results[], usage",
    """{"results": [{"title": "doc", "url": "https://...", "snippet": "..."}]}""",
)
_set(
    "data.skills_tools_memory",
    "skills: name, source; memory: key, value; tool policy: tool_name, allowed",
    """{"key": "pref", "value": "dark"}""",
    "id / key / value / skill objects",
    """{"key": "pref", "value": "dark"}""",
)
_set(
    "data.evals",
    "name, data_source_config, testing_criteria",
    """{"name": "qa-eval"}""",
    "id, object, name, status, created_at",
    """{"id": "eval_01", "object": "eval", "name": "qa-eval"}""",
)
_set(
    "data.workflows",
    "messages content; patch: status",
    """{"content": "continue"}""",
    "run_id, status, events[], messages[]",
    """{"id": "wfrun_01", "status": "running", "created_at": 1710000000}""",
)
_set(
    "data.agents",
    "agent_name, litellm_params, model, description, is_public",
    """{"agent_name": "researcher", "litellm_params": {"model": "gpt-4o-mini"}}""",
    "agent_id, agent_name, litellm_params, created_at",
    """{"agent_id": "agent_01", "agent_name": "researcher"}""",
)
_set(
    "data.access_groups",
    "access_group_id, models, budget_id",
    """{"access_group_id": "prod-chat", "models": ["gpt-4o-mini"]}""",
    "access_group_id, models, budget_id, created_at",
    """{"access_group_id": "prod-chat", "models": ["gpt-4o-mini"]}""",
)
_set(
    "data.mcp",
    "JSON-RPC method/params; OAuth: client_id, redirect_uri, code, code_verifier",
    """{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}}""",
    "JSON-RPC result/error; token: access_token, token_type, expires_in",
    """{"jsonrpc": "2.0", "id": 1, "result": {"tools": []}}""",
)
_set(
    "data.a2a",
    "message/send: message, metadata",
    """{"message": {"role": "user", "parts": [{"type": "text", "text": "hi"}]}}""",
    "task id, status, artifacts",
    """{"id": "task_01", "status": {"state": "completed"}}""",
)
_set(
    "data.gemini_v1beta",
    "contents, generationConfig, tools, safetySettings, systemInstruction",
    """{"contents": [{"role": "user", "parts": [{"text": "Hi"}]}]}""",
    "candidates, usageMetadata",
    """{"candidates": [{"content": {"parts": [{"text": "Hello"}]}}], "usageMetadata": {"promptTokenCount": 1, "candidatesTokenCount": 1}}""",
)
_set(
    "data.passthrough",
    "上游原生 body；认证用虚拟 Key，上游密钥由部署提供",
    """{"model": "gpt-4o-mini"}""",
    "上游原生 JSON 或字节",
    """{"id": "upstream"}""",
)
_set(
    "data.model_hub",
    "无 body（GET）；update_useful_links: useful_links",
    """{"useful_links": {"docs": "https://example.com"}}""",
    "object, data[].id, object, created, owned_by",
    """{"object": "list", "data": [{"id": "gpt-4o-mini", "object": "model", "owned_by": "openai"}]}""",
)
_set(
    "data.interactions",
    "model, input, store",
    """{"model": "gemini-2.5-flash", "input": "hi"}""",
    "id, status, output",
    """{"id": "interaction_01", "status": "completed"}""",
)
_set(
    "data.claude_code",
    "plugin_name, enabled; event_logging batch: events[]",
    """{"plugin_name": "my-plugin", "enabled": true}""",
    "name, version, enabled",
    """{"name": "my-plugin", "enabled": true}""",
)
_set(
    "mgmt.keys",
    "key_alias, duration, models, max_budget, soft_budget, user_id, team_id, organization_id, project_id, agent_id, tpm_limit, rpm_limit, max_parallel_requests, budget_duration, metadata, permissions, guardrails, policies, object_permission, allowed_routes, key_type, auto_rotate, rotation_interval, tags, model_rpm_limit, model_tpm_limit, budget_id, blocked, send_invite_email",
    """{"key_alias": "checkout-prod", "models": ["gpt-4o-mini"], "max_budget": 10.0, "tpm_limit": 100000, "rpm_limit": 60, "duration": "30d", "key_type": "llm_api"}""",
    "key, key_name, key_alias, expires, token_id, user_id, team_id, models, max_budget, spend",
    """{"key": "sk-live-...", "key_name": "sk-...xxxx", "key_alias": "checkout-prod", "expires": null, "token_id": "hash", "spend": 0}""",
)
_set(
    "mgmt.users",
    "user_email, user_alias, user_role, max_budget, models, teams, tpm_limit, rpm_limit, metadata, send_invite_email",
    """{"user_email": "dev@example.com", "user_role": "internal_user", "user_alias": "dev", "max_budget": 50}""",
    "user_id, user_email, user_role, spend, max_budget, teams, created_at",
    """{"user_id": "user_01", "user_email": "dev@example.com", "user_role": "internal_user", "spend": 0, "max_budget": 50}""",
)
_set(
    "mgmt.teams",
    "team_alias, organization_id, models, max_budget, tpm_limit, rpm_limit, members_with_roles, guardrails, object_permission, metadata, blocked",
    """{"team_alias": "platform", "models": ["gpt-4o-mini"], "max_budget": 100, "members_with_roles": [{"user_id": "user_01", "role": "admin"}]}""",
    "team_id, team_alias, organization_id, models, spend, max_budget, members_with_roles",
    """{"team_id": "team_01", "team_alias": "platform", "spend": 0, "max_budget": 100}""",
)
_set(
    "mgmt.organizations",
    "organization_alias, models, budget_id, metadata, members",
    """{"organization_alias": "acme", "models": []}""",
    "organization_id, organization_alias, spend, models, created_at",
    """{"organization_id": "org_01", "organization_alias": "acme"}""",
)
_set(
    "mgmt.projects",
    "project_alias, team_id, organization_id, max_budget, models, metadata",
    """{"project_alias": "checkout", "team_id": "team_01", "max_budget": 20}""",
    "project_id, project_alias, team_id, spend, max_budget, blocked, created_at",
    """{"project_id": "proj_01", "project_alias": "checkout", "team_id": "team_01", "blocked": false}""",
)
_set(
    "mgmt.budgets",
    "max_budget, soft_budget, tpm_limit, rpm_limit, model_max_budget, budget_duration, max_parallel_requests",
    """{"max_budget": 100, "tpm_limit": 100000, "rpm_limit": 60, "budget_duration": "30d"}""",
    "budget_id, max_budget, tpm_limit, rpm_limit, budget_duration, budget_reset_at",
    """{"budget_id": "budget_01", "max_budget": 100, "tpm_limit": 100000, "rpm_limit": 60}""",
)
_set(
    "mgmt.models",
    "model_name, litellm_params (model, api_base, api_key, custom_llm_provider, rpm, tpm, timeout, stream_timeout), model_info (id, mode, base_model)",
    """{"model_name": "gpt-4o-mini", "litellm_params": {"model": "openai/gpt-4o-mini", "api_key": "os.environ/OPENAI_API_KEY", "rpm": 480, "timeout": 60}}""",
    "model_name, litellm_params, model_info, blocked",
    """{"model_name": "gpt-4o-mini", "model_info": {"id": "model_01", "mode": "chat"}}""",
)
_set(
    "mgmt.guardrails",
    "guardrail_name, litellm_params.guardrail, mode, default_on, guardrail_info",
    """{"guardrail_name": "pii", "litellm_params": {"guardrail": "presidio", "mode": "pre_call", "default_on": true}}""",
    "guardrail_id, guardrail_name, litellm_params, created_at",
    """{"guardrail_id": "gr_01", "guardrail_name": "pii"}""",
)
_set(
    "mgmt.policies",
    "policy_name, description, statements, status",
    """{"policy_name": "prod-block-pii", "status": "active"}""",
    "policy_id, policy_name, version, status, created_at",
    """{"policy_id": "pol_01", "policy_name": "prod-block-pii", "version": 1, "status": "active"}""",
)
_set(
    "mgmt.prompts",
    "prompt_id, prompt_name, prompt_info, content, version",
    """{"prompt_id": "greet", "prompt_info": {"prompt": "Hello {{name}}"}}""",
    "prompt_id, version, created_at",
    """{"prompt_id": "greet", "version": 1}""",
)
_set(
    "mgmt.credentials",
    "credential_name, credential_info, credential_values",
    """{"credential_name": "openai-prod", "credential_info": {"custom_llm_provider": "openai"}}""",
    "credential_name, credential_info (values never returned)",
    """{"credential_name": "openai-prod", "credential_info": {"custom_llm_provider": "openai"}}""",
)
_set(
    "mgmt.tags",
    "name, description, models, budget_id",
    """{"name": "batch-job", "description": "offline eval"}""",
    "name, spend, created_at",
    """{"name": "batch-job", "spend": 0}""",
)
_set(
    "mgmt.spend",
    "query: start_date, end_date, api_key, user_id, team_id, request_id, page, page_size",
    """{"start_date": "2026-01-01", "end_date": "2026-01-31"}""",
    "request_id, model, spend, prompt_tokens, completion_tokens, startTime, api_key (hashed)",
    """{"data": [{"request_id": "req_01", "model": "gpt-4o-mini", "spend": 0.0001, "prompt_tokens": 8, "completion_tokens": 2}]}""",
)
_set(
    "mgmt.config",
    "config YAML / JSON: model_list, router_settings, litellm_settings, general_settings",
    """{"general_settings": {"master_key": "os.environ/LITELLM_MASTER_KEY"}}""",
    "status, version",
    """{"status": "ok"}""",
)
_set(
    "mgmt.sso",
    "login: username/password or SSO; update sso_settings fields",
    """{"username": "admin", "password": "..."}""",
    "token / session; sso_settings object",
    """{"token": "session_..."}""",
)
_set(
    "mgmt.scim",
    "SCIM User: userName, emails, active, name; Group: displayName, members",
    """{"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"], "userName": "dev@example.com", "active": true}""",
    "id, userName, meta, schemas",
    """{"id": "user_01", "userName": "dev@example.com", "active": true}""",
)
_set(
    "mgmt.mcp_servers",
    "server_name, url, transport, auth_type, credentials, mcp_info",
    """{"server_name": "github", "url": "https://mcp.example/sse", "transport": "sse", "auth_type": "none"}""",
    "server_id, server_name, url, transport, status, tools",
    """{"server_id": "mcp_01", "server_name": "github", "status": "active"}""",
)
_set(
    "mgmt.search_tools",
    "search_tool_name, litellm_params (search_provider, api_key)",
    """{"search_tool_name": "web", "litellm_params": {"search_provider": "tavily"}}""",
    "search_tool_id, search_tool_name",
    """{"search_tool_id": "st_01", "search_tool_name": "web"}""",
)
_set(
    "mgmt.vector_stores_admin",
    "vector_store_name, litellm_params, vector_store_description",
    """{"vector_store_name": "kb", "litellm_params": {"vector_store_id": "vs_upstream"}}""",
    "vector_store_id, vector_store_name, created_at",
    """{"vector_store_id": "vs_01", "vector_store_name": "kb"}""",
)
_set(
    "mgmt.health",
    "test_connection: model, mode, api_base, api_key",
    """{"model": "gpt-4o-mini", "mode": "chat"}""",
    "status, healthy_count, unhealthy_count, details",
    """{"status": "healthy"}""",
)
_set(
    "mgmt.callbacks",
    "callback name, callback_vars",
    """{"langfuse": true}""",
    "callbacks[], success_callback[], failure_callback[]",
    """{"success_callback": ["langfuse"], "failure_callback": []}""",
)
_set(
    "mgmt.cache",
    "type, host, port, ttl, namespace, password",
    """{"type": "redis", "ttl": 600}""",
    "status, redis_info",
    """{"status": "ok", "ttl": 600}""",
)
_set(
    "mgmt.router",
    "routing_strategy, num_retries, timeout, allowed_fails, fallbacks, context_window_fallbacks, enable_pre_call_checks",
    """{"routing_strategy": "simple-shuffle", "num_retries": 2, "timeout": 60}""",
    "router_settings object",
    """{"routing_strategy": "simple-shuffle", "num_retries": 2}""",
)
_set(
    "mgmt.jwt_oidc",
    "jwt_claim_name, jwt_claim_value, token, description",
    """{"jwt_claim_name": "sub", "jwt_claim_value": "user@idp", "token": "hashed_key"}""",
    "id, jwt_claim_name, jwt_claim_value",
    """{"id": "map_01", "jwt_claim_name": "sub"}""",
)
_set(
    "mgmt.customers",
    "user_id, alias, max_budget, blocked, budget_id",
    """{"user_id": "end_123", "max_budget": 5}""",
    "user_id, spend, max_budget, blocked",
    """{"user_id": "end_123", "spend": 0, "blocked": false}""",
)
_set(
    "mgmt.invitations",
    "user_id, user_email",
    """{"user_email": "new@example.com"}""",
    "id, user_id, is_accepted, expires",
    """{"id": "inv_01", "is_accepted": false}""",
)
_set(
    "mgmt.compliance",
    "query: regime, start_date, end_date",
    """{"regime": "eu-ai-act", "start_date": "2026-01-01"}""",
    "status, report fields",
    """{"status": "ok"}""",
)
_set(
    "mgmt.cost_export",
    "api_key, connection settings; export: start_date, end_date",
    """{"api_key": "..."}""",
    "status, exported_count",
    """{"status": "ok"}""",
)
_set(
    "mgmt.ui_settings",
    "logo_url, primary_color, enabled_pages, default_team_settings",
    """{"primary_color": "#0f172a"}""",
    "ui_settings object",
    """{"logo_url": null, "primary_color": "#0f172a"}""",
)
_set(
    "mgmt.public",
    "GET 无 body；部分 facet 用 query `facet`",
    """{"facet": "providers"}""",
    "hub lists: name, description, provider fields",
    """{"data": [{"name": "gpt-4o-mini", "provider": "openai"}]}""",
)
_set(
    "mgmt.debug",
    "gc configure: enabled",
    """{"enabled": true}""",
    "rss_mb, heap, asyncio_tasks",
    """{"rss_mb": 256}""",
)
_set(
    "mgmt.ops_schedules",
    "enabled, interval",
    """{"enabled": true, "interval": "1h"}""",
    "enabled, last_run, next_run",
    """{"enabled": true}""",
)
_set(
    "mgmt.audit",
    "query: start_date, end_date, changed_by, table_name",
    """{"start_date": "2026-01-01", "table_name": "LiteLLM_VerificationToken"}""",
    "id, updated_at, changed_by, changed_by_api_key, table_name, object_id, action, before_value, updated_values",
    """{"id": "audit_01", "action": "created", "table_name": "LiteLLM_VerificationToken"}""",
)
_set(
    "mgmt.email",
    "event, enabled, logo_url, support_contact",
    """{"event": "key_created", "enabled": true}""",
    "event_settings[]",
    """{"event": "key_created", "enabled": true}""",
)
_set(
    "mgmt.enterprise_misc",
    "log-event payload; available_users query",
    """{"event": "ui_click", "payload": {}}""",
    "status or users list",
    """{"status": "ok"}""",
)
_set(
    "mgmt.utils",
    "token_counter: model, messages; transform_request: model, messages, call_type",
    """{"model": "gpt-4o-mini", "messages": [{"role": "user", "content": "hi"}]}""",
    "total_tokens; transformed request JSON",
    """{"total_tokens": 8}""",
)
_set(
    "mgmt.allowed_ips",
    "ip",
    """{"ip": "10.0.0.1"}""",
    "allowed_ips[]",
    """{"allowed_ips": ["10.0.0.1"]}""",
)
_set(
    "mgmt.alerting",
    "alerting_threshold, alerting_args, slack_webhook",
    """{"alerting_threshold": 100}""",
    "alerting settings object",
    """{"alerting_threshold": 100}""",
)
_set(
    "mgmt.placeholders",
    "SCIM filter query `filter`",
    """{"filter": "userName eq \\"dev@example.com\\""}""",
    "Resources[], totalResults",
    """{"Resources": [], "totalResults": 0}""",
)
_set(
    "mgmt.onboarding",
    "claim: invitation_id, password, user_email",
    """{"invitation_id": "inv_01", "password": "..."}""",
    "token, user_id",
    """{"token": "session_...", "user_id": "user_01"}""",
)
