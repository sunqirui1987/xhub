"""Execute LiteLLM's passthrough URL helpers. No vendor network call."""
import json
import os
import sys

os.environ.setdefault("LITELLM_LOCAL_MODEL_COST_MAP", "True")
ROOT = os.environ.get("LITELLM_ROOT", "/Users/sunqirui/Downloads/litellm-main")
sys.path.insert(0, ROOT)

import httpx
import litellm
from litellm.proxy.pass_through_endpoints.llm_passthrough_endpoints import _join_url_paths
from litellm.proxy.pass_through_endpoints.pass_through_endpoints import HttpPassThroughEndpointHelpers

joins = [
    ("openai_responses", "https://api.openai.com/", "/responses", litellm.LlmProviders.OPENAI),
    ("gemini_generate", "https://generativelanguage.googleapis.com", "/v1beta/models/gemini-2:generateContent", litellm.LlmProviders.GEMINI),
]
urls = {}
for name, base, endpoint, provider in joins:
    urls[name] = _join_url_paths(httpx.URL(base), endpoint, provider)

subs = [
    ("with_sub", "https://upstream.example/v1", "responses", True),
    ("empty_sub", "https://upstream.example/v1", "", True),
    ("exclude_sub", "https://upstream.example/v1", "responses", False),
    ("dotdot", "https://upstream.example/v1/", "../secret", True),
]
subpaths = {}
for name, base, sub, include in subs:
    subpaths[name] = HttpPassThroughEndpointHelpers.construct_target_url_with_subpath(base, sub, include)

with open(sys.argv[1], "w") as fh:
    json.dump({"urls": urls, "subpaths": subpaths}, fh)
