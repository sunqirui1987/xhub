# gateway/guard

Guardrail checks for chat. Mounted as `guard.Module`.

POST `/apply_guardrail` and POST `/guardrails/apply_guardrail` run one check and return the action. The inference path does not use those routes. `dataplane.Serve` calls `Host.GuardrailBlocks` only when `op` is `chat` or empty, before the cache and before the upstream.

`listGuardrails` reads key-value kinds `guardrails` and `guardrail` from `RecordStore`. No store means the check is skipped and logged at debug.

`guardrailText` is the text that is checked: `text`, else `input`, else `prompt`, else the concatenated `messages[].content`.

`matchGuardrail` reads `litellm_params.guardrail` or the top-level `guardrail`:

- `block` or `always_block` returns action `block` and the original text.
- Otherwise the word lists `blocked_words` and `keywords` (on the params or on the guardrail) are tested case-insensitively. A hit with kind `redact` or `litellm_params.mode == redact` returns action `redact` and the text with that word replaced by `[REDACTED]`. Any other hit returns `block`.
- No hit returns `allow` and the original text.

A block in `Serve` is HTTP 400 `guardrail_failed`. The exchange is still stored and `RecordSpend` is called with success false, so the logs drawer can show which guardrail stopped the call.

The console garden (`frontend` guardrails components) edits these key-value documents. Patterns in the garden include financial, medical, legal, violence, jailbreak, and PII helpers. This package does not call Azure Content Safety. There is no Azure severity threshold of 0, 2, 4, 6 in the Go matcher.

## What this package does not do

It does not run on bypass. Official contents generation is not scanned for blocked words. It does not implement the plugin `Decision` type. Plugins run later, inside `Serve`, and can refuse even when every guardrail allowed the text.

中文说明见同目录 `readme_cn.md`。
