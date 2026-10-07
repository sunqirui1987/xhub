# gateway/guard

聊天的护栏。以 `guard.Module` 挂上。

POST `/apply_guardrail` 和 POST `/guardrails/apply_guardrail` 做一次检查并返回动作。推理路径不走这两条路由。`dataplane.Serve` 只在 `op` 是 `chat` 或空时调用 `Host.GuardrailBlocks`，在缓存和上游之前。

`listGuardrails` 从 `RecordStore` 读键值种类 `guardrails` 和 `guardrail`。没有库时跳过检查，并打一条 debug 日志。

`guardrailText` 是被检查的文本：先 `text`，否则 `input`，否则 `prompt`，再否则把 `messages[].content` 拼起来。

`matchGuardrail` 读 `litellm_params.guardrail` 或顶层的 `guardrail`：

- `block` 或 `always_block` 返回动作 `block` 和原文。
- 否则对词表 `blocked_words` 和 `keywords`（在参数上或在护栏上）做不区分大小写的包含判断。种类是 `redact` 或 `litellm_params.mode == redact` 时，返回动作 `redact`，并把命中的词换成 `[REDACTED]`。其他命中返回 `block`。
- 没有命中返回 `allow` 和原文。

`Serve` 里的拦截是 HTTP 400 `guardrail_failed`。交换仍会存下来，并以失败调用 `RecordSpend`，这样日志抽屉能看出是哪条护栏拦住的。

控制台的护栏园（`frontend` 的 guardrails 组件）编辑这些键值文档。园里的模式包括金融、医疗、法律、暴力、越狱和 PII 辅助。这个包不调用 Azure Content Safety。Go 匹配器里没有 0、2、4、6 的 Azure 严重级别。

## 这个包不做什么

它不跑在 bypass 上。官方内容生成不会扫敏感词。它也不实现插件的 `Decision`。插件在 `Serve` 里更晚运行，即使每条护栏都放行，插件仍可以拒绝。

English notes are in `readme.md` in this directory.
