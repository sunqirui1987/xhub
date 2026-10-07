# 护栏组件

控制台 `/guardrails` 用的 React 组件。它们编辑网关键值种类 `guardrails`（以及读取时一并列出的 `guardrail`）。保存之后，只有聊天推理会读到：`dataplane.Serve` 在 `op` 为 `chat` 或空时调用 `GuardrailBlocks`。官方内容生成不跑这些规则。

匹配在网关 `gateway/guard`，不在浏览器里。动作是 `allow`、`block` 或 `redact`。`block` 和 `always_block` 直接拦截。`blocked_words` 和 `keywords` 做不区分大小写的包含判断。`redact` 把命中的词换成 `[REDACTED]`。拦住的聊天仍会记一条失败用量，状态 400，类型 `guardrail_failed`。

## 这个目录里有什么

| 文件 | 作用 |
| --- | --- |
| `GuardrailsPanel.tsx` | 列表页。拉护栏列表，打开添加表单和测试面板 |
| `guardrail_table.tsx`、`guardrailTableColumns.tsx` | 表格列 |
| `add_guardrail_form.tsx` | 新建或编辑一条护栏 |
| `guardrail_info.tsx` | 单条详情 |
| `guardrail_garden.tsx` 和 `guardrail_garden_data.ts` | 预置模式：金融、医疗、法律建议，暴力、自我伤害，偏见，越狱和数据外带 |
| `content_filter/` | 词表和分类过滤的表单 |
| `pii_components.tsx`、`pii_configuration.tsx` | PII 规则 |
| `llm_judge/` | 用模型做判定的配置 |
| `custom_code/` | 自定义代码护栏的表单字段 |
| `tool_permission/` | 工具调用是否允许 |
| `GuardrailTestPlayground.tsx` | 对 `POST /guardrails/apply_guardrail` 发一条样例文本 |

没有 Azure Content Safety 的配置组件，也没有按 0、2、4、6 分档的全局严重级别。那些字段不属于这个网关的匹配器。

组件测试和文件放在一起（`*.test.tsx`）。它们渲染表单，不启动网关。
