# 原厂协议 Bypass

选择下列独立接口，查看认证、参数、示例及响应，并打开对应在线运行弹窗。每个接口使用独立 Markdown，只有已登记路径及部署允许的模型能够调用。

## 原厂接口目录

| 方法 | 接口 | 路径 |
| --- | --- | --- |
| POST | [Bypass Anthropic 消息](native-anthropic) | /bypass/anthropic/v1/messages |
| POST | [Bypass Vertex 内容生成](native-vertex) | /bypass/vertex/v1/models/YOUR_MODEL_NAME:generateContent |
| POST | [Bypass Gemini 内容生成](native-gemini) | /bypass/gemini/v1beta/models/YOUR_MODEL_NAME:generateContent |
| POST | [Bypass OpenAI 图片生成](native-images) | /bypass/openai/v1/images/generations |
| POST | [Bypass OpenAI 图片编辑](native-image-edits) | /bypass/openai/v1/images/edits |
| POST | [Bypass OpenAI Responses 响应](native-responses) | /bypass/openai/v1/responses |
| POST | [Bypass OpenAI 对话补全](native-chat) | /bypass/openai/v1/chat/completions |

## 接口契约
使用对应 API 文章的协议字段及 XHub 虚拟密钥，部署必须选择匹配的 Bypass 传输。路由替换公开模型为上游名并应用供应商认证，保留原厂扩展字段。Gemini/Vertex 还登记 streamGenerateContent、countTokens。Bypass 不开放任意上游路径。

## 接口与认证

使用 XHub API 密钥：Authorization: Bearer <XHUB_API_KEY>。不要把供应商密钥当作网关密钥。

Base URL 使用调试台显示的网关根地址，不额外追加 /v1；实际请求使用上面的完整接口路径。

## 请求参数

本页是协议说明，具体参数以对应接口文章为准。

## 请求示例

请阅读对应接口的可执行示例。

## 成功响应

响应遵循选定原厂协议；字段取决于请求的接口和上游模型。

## 错误与排查

| HTTP | 说明 |
| --- | --- |
| 400 | 请求无效，请检查 JSON、参数和查询。 |
| 401 | XHub 凭据缺失或无效。 |
| 403 | 权限拒绝，请检查模型范围或策略。 |
| 429 | 超过限制，请检查响应详情与 Retry-After。 |

## 能力与计费边界

参数支持取决于部署模型及转换或 Bypass 路径；XHub 不保证所有上游扩展字段在所有模型中可用。以已实现的协议适配和上游响应为准。usage 的明细属于对应输入/输出总量，不能重复相加；费用以网关日志保存的计费快照为准。示例响应仅用于理解字段。

## 能力与协议选择

Bypass 是透传机制，不是模型系列。调用前确认部署支持对应已登记协议。

| Capability | Guide |
| --- | --- |
| Chat | [OpenAI](chat-completions), [Anthropic](messages), [Gemini](gemini) |
| Images | [Generation](images), [Edits](image-edits) |
| Video | [OpenAI](videos), [Content generation](ark), [Seedance Fal](fal-seedance) |
