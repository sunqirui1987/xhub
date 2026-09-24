"""Execute LiteLLM token_counter and get_supported_openai_params. No network."""
import json
import os
import sys

os.environ.setdefault("LITELLM_LOCAL_MODEL_COST_MAP", "True")
ROOT = os.environ.get("LITELLM_ROOT", "/Users/sunqirui/Downloads/litellm-main")
sys.path.insert(0, ROOT)

import litellm
from litellm.utils import _select_tokenizer

model = "gpt-4o-mini"
prompt_tokens = litellm.token_counter(model=model, text="hello")
message_tokens = litellm.token_counter(model=model, messages=[{"role": "user", "content": "hi"}])
tokenizer = _select_tokenizer(model=model)["type"]
params = litellm.get_supported_openai_params(model=model, custom_llm_provider="openai")
with open(sys.argv[1], "w") as fh:
    json.dump(
        {
            "model": model,
            "prompt_tokens": prompt_tokens,
            "message_tokens": message_tokens,
            "tokenizer_type": tokenizer,
            "supported_openai_params": params,
        },
        fh,
    )
