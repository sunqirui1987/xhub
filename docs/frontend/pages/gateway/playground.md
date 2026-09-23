# 调试台

- Route: `/playground`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

管理员用真实数据面调试协议。

## 布局

- **顶栏 CTA**：无独立创建按钮。顶层 Tab：Chat、Compare、Compliance、Agent Builder (Experimental)。
- **筛选 / 左栏 Configurations**（Chat）：Virtual Key Source（Current UI Session / Virtual Key）；Endpoint `SearchSelect`（`/v1/chat/completions`、`/v1/responses`、`/v1/messages`、`/v1/images/generations`、`/v1/images/edits`、`/v1/embeddings`、`/v1/audio/speech`、`/v1/audio/transcriptions`、`/v1/a2a/message/send`、`/mcp-rest/tools/call`、`/v1/realtime`、`/v1beta/interactions`）；Model 下拉（来自模型列表，可 Enter custom model）；Tags；MCP Servers / 直连 tool；Guardrails、Policies；Vector Stores；图片/音频上传；Advanced 弹出：temperature、max_tokens、stream、tools JSON。控件按模型 `mode` 能力显示。
- **表/卡片/Tab**：中栏消息列表 + ChatComposer（Send / 取消）。流式打字、embeddings 向量预览、Anthropic / Responses 块、usage 与 `x-litellm-response-cost` 事件。Compare 双模型对照；Compliance 合规用例；Agent Builder 实验向导。
- **抽屉/模态**：MCP toolsets 说明；BYOK 凭证；代码片段（OpenAI SDK / Azure SDK）。
- **URL query**：无页面级 query。会话选择（endpoint、MCP servers、api key source）落在 sessionStorage。

## 交互

- 进入 Chat → 拉模型填下拉；选 endpoint 过滤兼容模型；填消息点 Send → 对应 `POST` 数据面；stream 开则 SSE 打字，关则一次输出。取消中止 fetch。
- completion-mode 模型映射到 Chat endpoint（无独立 Completions Tab，契约仍走 completions）。
- **view-only**：整页 Access Denied：「Your role does not have access to the Playground. Ask your proxy admin for access to test models.」
- **loading**：模型下拉 `Loading models...`；发送中 composer 禁用、取消可用。
- **empty**：无模型时下拉 empty；A2A 无 agent 时提示 Create agents via /v1/agents。
- **forbidden**：view-only 如上；缺 Virtual Key 时 embeddings 等报 Virtual Key is required。
- 校验失败可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 拉模型 | `GET /v1/models` 与 `GET /model/info` | 填充下拉 |
| 发送 Chat | `POST /v1/chat/completions` | 流式打字 |
| Completions | `POST /v1/completions` | 文本输出 |
| Responses | `POST /v1/responses` | output 块 |
| Embeddings | `POST /v1/embeddings` | 向量预览 |
| Messages | `POST /v1/messages` | Anthropic 块 |
| 取消 | `中止 fetch` | 服务端取消上游 |

## 字段

| 字段 | 含义 |
|---|---|
| `model` | 别名 |
| `messages` | 对话 |
| `temperature` | 采样 |
| `max_tokens` | 上限 |
| `stream` | SSE |
| `tools` | 工具 JSON |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
