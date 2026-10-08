# gateway/guard

Mounted as guard.Module. Management trial routes /apply_guardrail and /guardrails/apply_guardrail require management access. Inference calls Evaluate before cache lookup and upstream work for chat and Responses.

Rules come from guardrails and guardrail records. A configured store read failure produces a blocking rule. A nil store keeps the unconfigured-host behavior of skipping checks. Default-on rules with pre-call or redact mode run in sequence; blocking stops later rules.

Text traversal includes text/input/prompt/messages/contents/system/instructions/systemInstruction and supported nested text/content/parts arrays. All supported fields are scanned, rather than choosing the first field. Image URLs and message roles are preserved. Case-insensitive blocked words can block or redact; redaction replaces all matching terms and mutates request text before encoding.

Blocks return 400 guardrail_failed and retain failure logs with zero local charge. Findings describe rules that actually executed. This is a local word matcher, not Azure Content Safety or a general multimodal classifier. It does not scan official bypass, tool/function arguments, or image contents.

Tests: guard_test.go, guard_regression_test.go, and dataplane cache/exchange tests. 中文说明见 readme_cn.md。
