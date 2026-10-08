# dataplane

Serve handles adapted inference; ServeBypass forwards registered official APIs; State and Flush provide routing state and queued usage persistence. The package consumes the interfaces in host.go and does not import gateway.

## Adapted inference

Authentication and hierarchical limits run before guardrails, extensions, cache lookup, and upstream work. Chat and Responses guardrails scan supported text fields and mutate them through redaction. Requests are serialized again so encoding, cache keys, and exchange logs use the redacted content. A configured guardrail store read failure blocks requests.

Route-template resolution errors return 503. Settings are resolved per request. Missing retry count means one attempt per deployment; missing timeout means 60 seconds per attempt. Paused deployments and incompatible endpoint types are excluded before routing. Pins can only promote an eligible deployment. Weighted splits exclude zero, negative, invalid, and cooled weights; other strategies avoid cooled candidates when an alternative exists.

Each subsequent HTTP attempt reauthenticates and checks limits, including retries on the same deployment. Transport errors, 429, and 5xx can retry. After a stream has emitted output, a read failure is recorded as failure and cannot trigger a second upstream response. Failures cannot populate the response cache or update successful-session pins. Retry does not prove that the provider performed no billable work.

Non-stream successful responses populate the process-local 8192-entry LRU. The key hashes caller, operation, alias, body, model configuration, effective route settings, session, and query. Reads and writes copy bytes. Configuration changes therefore miss prior entries, including price or paused-state changes. Cache hits retain reported token counts and charge zero. There is no TTL or distributed invalidation.

Usage extraction accepts provider-reported totals, cached tokens, and fragmented SSE. Missing token usage uses an estimate; max_tokens is not consumed input. A stream read failure records a failed usage/log row with zero local charge, even if its emitted HTTP status can no longer be changed.

## Official APIs

Official forwarding preserves the provider request format, strips client authentication and hop headers, bounds buffered responses, and uses the effective template. Creation retries only explicit 429/5xx responses; an ambiguous transport failure cannot safely create another task. Successful task creation pins a credential-aware deployment under a caller/transport/task scope. Polling checks the same scope and endpoint; another caller or transport cannot reuse the pin.

Creation, pending polls, failed polls, and lists do not infer usage charges. Terminal successful usage is settled through the gateway billing path. See gateway/spend.go and the billing documentation for deduplication and persistence guarantees. Official forwarding does not run text guardrails or use the response cache. Task pins expire after seven days; a process-local fallback is lost on restart.

## Runtime and evidence

With Redis, cooldown/latency/usage feed routing and spend rows are queued. Flush commits PostgreSQL records before acknowledging queue entries. Without Redis, spend is persisted directly. These mechanisms do not reserve budgets before concurrent calls.

Regression evidence includes cache_scope_test.go, stream_failure_regression_test.go, usage_stream_test.go, official_template_test.go, and bypass_logic_test.go. Current behavior and limits are in the [runtime reference](../../docs/development/runtime.md).

中文说明见 readme_cn.md。
