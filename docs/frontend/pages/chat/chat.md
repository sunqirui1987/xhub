# Chat 终端壳

- Route: `/chat`
- Nav: CHAT
- Status: `specified`

## 目的

给终端用户的对话 UI。独立壳，**不是**管理员 Playground（`/playground`）：无接口 Tab、无 temperature/tools JSON 调试栏、无管理侧栏、不登记模型。需 `enable_chat_ui`；关闭则跳回控制台根。会话存在浏览器本地，不是服务端 thread。

## 布局

独立 Chat 壳：顶栏仍是控制台 Navbar，其下 **不是** 五组管理侧栏。左 260px Chat 侧栏 + 右主区。页顶 pre-v0 警告条。

### 顶栏

Navbar（用户菜单）。Chat 壳内：`New Chat` 按钮。无对话时主区居中问候 `Good morning/afternoon/evening, {email local-part}`。localStorage 不可用时黄条 `Chat history won't be saved in this browser session`（可关）。

### 筛选

无管理筛选。模型选择在输入条左侧 Popover：搜索框 + 滚动列表（当前模型置顶，provider logo）。MCP 工具 `+` Popover（`MCPConnectPicker`）。

### 表

无表。主区气泡对话（`ChatMessages`）：user/assistant、reasoning、MCP events、usage、可编辑重发。空态：问候 + 宽输入条 + 快捷 chip（Write / Learn / Code / Brainstorm）。

### Tab

无。左栏导航是路由：Chats / Integrations / Credentials / API Keys / Logs / Usage，不是页内 Tab。

### 抽屉

无。左栏 ConversationList：选中、重命名、删除。详情不走抽屉。

### 模态

无发送模态。模型/MCP 是 Popover。

### URL

`/chat`。新对话 `createConversation` 后 `?id={convId}`（`history.pushState`）。无效/过期 id：`staleId` 时 `replace` 回 `/chat`。切会话清 `previous_response_id`。

## 交互

进入：`GET /model_group/info` 填模型；默认 localStorage `litellm_chat_selected_model` 或第一项。无会话显示空态。Enter 发送（Shift+Enter 换行）；Send 在无文本/无模型/流式中 disabled。发送：无 conv 则创建并写 `?id=`；先 append user + 空 assistant；`POST /v1/responses`（OpenAI Responses，`stream: true`，`input` 而非 messages）。有 `previous_response_id` 时只传本轮 user，避免重复历史。可选 MCP tools。流式打字；Stop abort，内容加 `[stopped]`。失败保存 `[Something went wrong…]`。编辑某条 user：截断其后历史并重发（清 session）。滚动锁：流式中保持用户滚动位置；新消息才滚到底。MCP 事件仅在干净完成时写入。本页不调管理写接口，不打开 Playground 协议 Tab。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 拉模型 | `GET /model_group/info` | 模型 Popover |
| 发送 | `POST /v1/responses`（stream） | 助手气泡 SSE；记录 `response_id` |
| 取消 | AbortController | 部分文本 + `[stopped]` |
| MCP 列表 | `GET /v1/mcp/server` | `+` 选择器 |

不是 `POST /v1/chat/completions`，也不是 Playground 的 completions/embeddings/messages Tab。

## 字段

| 字段 | 含义 |
|---|---|
| `model` | 模型组，存 localStorage |
| `input` | Responses API 消息 |
| `stream` | 默认 true |
| `previous_response_id` | 会话链 |
| `tools[].type=mcp` | 选中的 MCP server_url |
| `id` query | 本地 conversation id |

## 状态

loading models：触发器 Skeleton。empty：问候空态。forbidden：`enable_chat_ui=false` 重定向；未登录走登录。流式中 Send 变 Stop。storage 不可用：黄条，会话不持久。窄屏左栏仍 260px，主区 `max-width: 760`。

## 验收

能选模型发流式回复；`?id=` 刷新回到同一本地会话；Stop 中止；不出现 Playground 协议 Tab 或模型编辑；`enable_chat_ui` 关闭进不了；桌面与窄屏。
