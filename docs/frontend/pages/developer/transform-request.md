# 请求转换调试

- Route: `/transform-request`
- Nav: DEVELOPER TOOLS
- Status: `specified`

## 目的

把将发给网关 `/chat/completions` 的 JSON，转成实际上游 cURL（含 api_base、headers、body）。用于核对 provider 映射。标题文案是 Playground，但 IA 属于 Developer Tools，不是 AI Gateway `/playground`，也不是 Chat 壳。

## 布局

登录后控制台壳。内容 `p-2`：标题 `Playground` + 一句说明。下方两列卡片（`lg:grid-cols-2`，窄屏单列）。

### 顶栏

标题 + 描述：「See how LiteLLM transforms your request for the specified provider.」无主按钮。页底右下 GitHub issue 外链。

### 筛选

无。模型写在左侧 JSON 的 `model` 字段里。

### 表

无表。左：Original Request（JSON textarea，高 `h-72`，等宽）。右：Transformed Request（只读 `<pre>`，敏感 header 不展示）。

### Tab

无。

### 抽屉

无。

### 模态

无。

### URL

`/transform-request`。编辑器内容不写入 URL；刷新回到内置示例 JSON。

## 交互

默认左侧示例：`model=openai/gpt-4o`、messages、temperature、max_tokens、stream。点 Transform 或 `Cmd/Ctrl+Enter`：先 `JSON.parse`，非法 JSON toast `Invalid JSON in request body` 且不发请求。合法则 `POST /utils/transform_request`，body `{ call_type: "completion", request_body }`。成功把 `raw_request_api_base` + `raw_request_body` + `raw_request_headers` 拼成 cURL 填右侧，toast success。无 token toast error。右侧 Copy 复制当前 cURL（空则复制空串）。加载中 Transform 按钮 spinner 且 disabled。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 转换 | `POST /utils/transform_request` | 右侧 cURL；Note: Sensitive headers are not shown |

本页不调用 `GET /utils/supported_openai_params`。

## 字段

| 字段 | 含义 |
|---|---|
| `call_type` | 固定 `completion` |
| `request_body.model` | 网关别名（可带 provider 前缀） |
| `request_body.messages` | OpenAI messages |
| `request_body.temperature` / `max_tokens` / `stream` | 采样与流式 |
| `raw_request_api_base` | 上游 URL |
| `raw_request_body` | 转换后 body |
| `raw_request_headers` | 脱敏后的请求头 |

## 状态

loading：Transform 按钮 spinner。empty：右侧占位示例 cURL（openai.com）。forbidden：无 token 不请求。JSON 非法：toast，原文本保留。接口失败：toast `Failed to transform request`，右侧不清空。窄屏两卡上下排列。

## 验收

合法 JSON 转换出上游 cURL；非法 JSON 不发请求；Copy 可用；桌面与窄屏；不得 mock 表格。
