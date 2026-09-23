# MCP 服务器

- Route: `/mcp-servers`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

注册 MCP、OAuth、列出 tools。

## 布局

- **顶栏 CTA**：标题 MCP Servers + 数量徽章。Admin：`Import from JSON`、`+ Add New MCP Server`。非 admin：`+ Submit MCP Server`。
- **筛选**（All Servers）：Team（All / Personal / 具体 team）、Access Group；搜索 name/alias/URL/ID；Sort（Recently created / updated / Name A→Z / Health unhealthy first）。
- **表/卡片/Tab**：
  - Tab：All Servers、Toolsets、Connect；admin 另有 Semantic Filter、Tool Search、Network Settings、Submitted MCPs。
  - All Servers 为卡片网格（非表）：server_name、url、transport、auth_type、health 徽章、缺失 user env 提示。
  - 点卡片进详情 Tab：Overview、MCP Tools、Settings（仅 proxy admin）。
- **抽屉/模态**：Discovery 选预置或 Custom → CreateMCPServer 表单（URL / transport sse|stdio|streamable-http / auth none|oauth|bearer…）。Import JSON。Delete 确认。OAuth 授权跳转。`?fill_env_vars=` 打开 UserEnvVarsModal。BYOK 凭证。
- **URL query**：`?fill_env_vars=<server_id>` 深链填 per-user env（进页后从 URL 剥掉）。OAuth 回跳用 sessionStorage 恢复 Tools Tab。

## 交互

- Add New MCP Server → Discovery → 表单 `POST /v1/mcp/server` → 卡片出现。Import → `POST /v1/mcp/server/import`。点卡片 → Overview；Tools 可测 tool；Settings 保存 `PUT /v1/mcp/server`。Delete → `DELETE /v1/mcp/server/{id}`。Health 徽章来自 `GET /v1/mcp/server/health`。OAuth → `GET /v1/mcp/server/oauth/{id}/authorize` 浏览器授权。非 admin Submit → `POST /v1/mcp/server/register`，admin 在 Submitted MCPs 审批。
- **view-only / 非 admin**：无 Import / Add New；改为 Submit；详情无 Settings。
- **loading**：`Loading MCP servers...`。
- **empty**：无数据 `No MCP servers configured. Click '+ Add New MCP Server' to get started.`；筛选无匹配 `No servers match the current filters or search.`
- **forbidden**：缺 accessToken/userRole/userID → `Missing required authentication parameters.`
- 校验失败可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /v1/mcp/server` | 卡片 |
| 创建 | `POST /v1/mcp/server` | 新卡片 |
| 更新 | `PUT /v1/mcp/server` | 保存 |
| 删除 | `DELETE /v1/mcp/server/{server_id}` | 移除 |
| 健康 | `GET /v1/mcp/server/health` | 徽章 |
| OAuth | `GET /v1/mcp/server/oauth/{server_id}/authorize` | 浏览器授权 |

## 字段

| 字段 | 含义 |
|---|---|
| `server_id` | id |
| `server_name` | 名称 |
| `url` | MCP URL |
| `transport` | sse/stdio/streamable-http |
| `auth_type` | none/oauth/bearer |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
