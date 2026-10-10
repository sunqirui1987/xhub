#!/usr/bin/env python3
"""Write an isolated gateway config without passing secrets through argv."""
import json
import os
from pathlib import Path

env = os.environ
models = [{"model_name": "gpt-4o-mini", "litellm_params": {
    "model": "openai/gpt-4o-mini", "api_key": "sk-fake",
    "api_base": "http://127.0.0.1:" + env["E2E_UP_PORT"]}, "model_info": {"transport": "bypass_openai_chat", "endpoint_types": ["chat", "responses"]}}]
live = env.get("E2E_LIVE") == "1"
# 续接用例使用专属本地模型，回复内容取决于真实上游收到的完整历史。
if env.get("E2E_RESPONSES_CONTINUATION") == "1":
    models.append({"model_name": "e2e-responses-history", "litellm_params": {
        "model": "e2e-responses-history", "api_key": "sk-fake",
        "input_cost_per_token": 0.000001, "output_cost_per_token": 0.000002,
        "api_base": "http://127.0.0.1:" + env["E2E_UP_PORT"]},
        "model_info": {"transport": "bypass_openai_chat", "endpoint_types": ["chat", "responses"]}})
    # Codex 模拟器使用独立模型，供应商必须验证其真实收到的三轮项目上下文。
    models.append({**models[-1], "model_name": "e2e-codex-agent",
                   "litellm_params": {**models[-1]["litellm_params"], "model": "e2e-codex-agent"}})
# 隔离测试专用无效声明：验证旧配置不会推断聊天能力，浏览器必须解释不可调用原因。
if env.get("E2E_ENDPOINT_UNBOUND") == "1":
    models.append({"model_name": "e2e-unbound", "litellm_params": {
        "model": "openai/old-model", "api_key": "sk-fake",
        "api_base": "http://127.0.0.1:" + env["E2E_UP_PORT"]}, "model_info": {"mode": "chat"}})
if live:
    for vendor in env["E2E_LIVE_VENDORS"].split(","):
        prefix = "XHUB_REGRESSION_" + vendor
        for model in env[prefix + "_MODELS"].split(","):
            model = model.strip()
            models.append({"model_name": "e2e-live-" + vendor + "/" + model,
                "litellm_params": {"model": model, "api_key": env[prefix + "_KEY"],
                    "api_base": env[prefix + "_BASE"], "custom_llm_provider": env[prefix + "_PROTOCOL"],
                    "max_tokens": 128}, "model_info": {"transport": "bypass_openai_chat", "endpoint_types": ["chat"]}})
if live:
    metadata = json.loads(env["E2E_PROVIDER_METADATA"])
    for scenario in metadata["weighted_scenarios"]:
        for deployment in scenario["deployments"]:
            prefix = "XHUB_REGRESSION_" + deployment["provider"]
            models.append({"model_name": scenario["model_name"], "litellm_params": {
                "model": deployment["model"], "api_base": env[prefix + "_BASE"],
                "api_key": env[prefix + "_KEY"], "custom_llm_provider": "openai",
                "deployment_id": deployment["id"], "max_tokens": 128},
                "model_info": {"transport": "bypass_openai_chat", "endpoint_types": ["chat"], "id": deployment["id"]}})
config = {"model_list": models,
    "router_settings": {"routing_strategy": "simple-shuffle", "num_retries": 1, "timeout": 90 if live else 15},
    "general_settings": {"master_key": env["E2E_MASTER_KEY"], "admin_email": "admin", "admin_name": "admin",
        "admin_password": env["E2E_MASTER_KEY"],
        "database_url": "postgres://xhub:xhub_dev_password@127.0.0.1:5433/xhub?sslmode=disable&search_path=" + env["E2E_SCHEMA"]}}
target = Path(env["E2E_RUN_DIR"]) / "c.yaml"
fd = os.open(target, os.O_CREAT | os.O_TRUNC | os.O_WRONLY, 0o600)
os.chmod(target, 0o600)
with os.fdopen(fd, "w") as stream:
    # JSON is valid YAML and safely quotes supplier values.
    json.dump(config, stream)
