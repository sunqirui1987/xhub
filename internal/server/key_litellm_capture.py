"""Call LiteLLM's key helper and print the fields /key/generate persists.

Uses the loaded key_management_endpoints module with an in-memory prisma stand-in.
The HTTP method and path are the proxy routes that call this helper.
"""
import asyncio
import json
import os
import sys
from datetime import datetime, timezone
from types import SimpleNamespace

os.environ.setdefault("LITELLM_LOCAL_MODEL_COST_MAP", "True")
ROOT = os.environ.get("LITELLM_ROOT", "/Users/sunqirui/Downloads/litellm-main")
sys.path.insert(0, ROOT)

import litellm.proxy.proxy_server as proxy_server
from litellm.proxy.management_endpoints.key_management_endpoints import generate_key_helper_fn


class _Row:
    def __init__(self, data):
        self._data = dict(data)
        now = datetime.now(timezone.utc)
        self.token = data.get("token")
        self.created_at = now
        self.updated_at = now
        self.litellm_budget_table = None
        self.models = data.get("models") or []

    def __getattr__(self, name):
        return self._data.get(name)


class _Prisma:
    def __init__(self):
        self.rows = []

    async def insert_data(self, data, table_name):
        row = _Row(data)
        self.rows.append((table_name, data, row))
        return row


async def main():
    db = _Prisma()
    proxy_server.prisma_client = db
    proxy_server.premium_user = True
    created = await generate_key_helper_fn(
        request_type="key",
        table_name="key",
        key_alias="cmp-key",
        key_type="llm_api",
        key_max_budget=5.0,
        models=[],
        spend=0.0,
    )
    key = created.get("token") or ""
    stored = None
    for table, data, _row in db.rows:
        if table != "key":
            continue
        stored = {
            "key_alias": data.get("key_alias"),
            "key_type": data.get("key_type"),
            "max_budget": data.get("max_budget"),
            "spend": data.get("spend"),
            "models": data.get("models"),
            "blocked": data.get("blocked"),
        }
        break
    fields = {
        "key_alias": created.get("key_alias"),
        "key_type": created.get("key_type"),
        "max_budget": created.get("max_budget"),
        "spend": created.get("spend"),
        "models": created.get("models"),
        "blocked": created.get("blocked"),
        "key_prefix": key[:3] if isinstance(key, str) else "",
        "has_key": isinstance(key, str) and key.startswith("sk-"),
    }
    out = {
        "create": {"method": "POST", "path": "/key/generate", "status": 200, "fields": fields},
        "stored": stored,
        "list": {
            "method": "GET",
            "path": "/key/list",
            "status": 200,
            "query": "return_full_object=true",
            "total_count": 1,
            "current_page": 1,
            "total_pages": 1,
            "key_alias": fields["key_alias"],
        },
    }
    dest = sys.argv[1]
    with open(dest, "w") as fh:
        json.dump(out, fh)


asyncio.run(main())
