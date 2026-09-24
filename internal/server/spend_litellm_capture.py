"""Execute LiteLLM's spend-log payload and the UI list envelope. No vendor calls."""
import asyncio
import json
import os
import sys
from datetime import datetime, timezone

os.environ.setdefault("LITELLM_LOCAL_MODEL_COST_MAP", "True")
ROOT = os.environ.get("LITELLM_ROOT", "/Users/sunqirui/Downloads/litellm-main")
sys.path.insert(0, ROOT)

import litellm
import litellm.proxy.proxy_server as proxy_server
from litellm.proxy.db.db_spend_update_writer import DBSpendUpdateWriter
from litellm.proxy.spend_tracking.spend_management_endpoints import _build_ui_spend_logs_response


def _cost(model: str):
    response = {
        "id": "chatcmpl-x",
        "object": "chat.completion",
        "model": model,
        "choices": [{"message": {"role": "assistant", "content": "ok"}}],
        "usage": {"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
    }
    try:
        return litellm.completion_cost(completion_response=response), None
    except Exception as exc:
        return None, f"{type(exc).__name__}: {exc}"


async def _payload(response_cost, model, call_type):
    captured = {}

    async def _record(payload, prisma_client, disable_spend_logs):
        captured["payload"] = dict(payload)
        return False

    writer = DBSpendUpdateWriter(redis_cache=None)
    writer._record_spend_log = _record
    proxy_server.prisma_client = object()
    proxy_server.disable_spend_logs = False
    proxy_server.litellm_proxy_budget_name = None
    now = datetime.now(timezone.utc)
    await writer.update_database(
        token=None,
        user_id=None,
        end_user_id=None,
        team_id=None,
        org_id=None,
        kwargs={"model": model, "litellm_params": {}, "call_type": call_type, "cache_hit": False},
        completion_response={
            "id": "chatcmpl-x",
            "model": model,
            "usage": {"prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10},
            "choices": [{"message": {"role": "assistant", "content": "ok"}}],
        },
        start_time=now,
        end_time=now,
        response_cost=response_cost,
    )
    payload = captured.get("payload") or {}
    return {
        "spend": payload.get("spend"),
        "model": payload.get("model"),
        "status": payload.get("status"),
        "prompt_tokens": payload.get("prompt_tokens"),
        "completion_tokens": payload.get("completion_tokens"),
        "call_type": payload.get("call_type"),
    }


async def main():
    mystery_cost, mystery_err = _cost("mystery")
    known_cost, known_err = _cost("gpt-4o-mini")
    mystery = await _payload(mystery_cost, "mystery", "completion")
    stream = await _payload(mystery_cost, "mystery", "acompletion")
    page = await _build_ui_spend_logs_response(
        prisma_client=None,
        data=[{"request_id": "r", "model": "mystery", "session_id": None, "api_key": None, "spend": mystery["spend"]}],
        total_records=1,
        page=1,
        page_size=25,
        total_pages=1,
        enrich_session_counts=False,
        total_is_capped=False,
    )
    out = {
        "mystery_cost": mystery_cost,
        "mystery_cost_error": mystery_err,
        "known_cost": known_cost,
        "known_cost_error": known_err,
        "mystery_log": mystery,
        "stream_log": stream,
        "ui_keys": sorted(page.keys()),
        "ui_page": page.get("page"),
        "ui_page_size": page.get("page_size"),
        "ui_total": page.get("total"),
        "ui_total_pages": page.get("total_pages"),
    }
    with open(sys.argv[1], "w") as fh:
        json.dump(out, fh, default=str)


asyncio.run(main())
