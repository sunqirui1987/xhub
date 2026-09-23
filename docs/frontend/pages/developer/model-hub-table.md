# AI Hub

- Route: `/model-hub-table`
- Nav: DEVELOPER TOOLS
- Status: `specified`

## 目的

登录控制台里的模型/Agent/MCP/Skill 目录。Admin 可把条目标成公开，并复制公开 Hub URL。非 Admin 只读，嵌入公开 Hub 视图（公司已公开的模型）。这不是未登录的 `/model_hub` / `/model_hub_table`。

## 布局

登录后控制台壳。Admin 渲染 `ModelHubTable`（`publicPage=false`）；非 Admin 渲染嵌入式 `PublicModelHub`（`isEmbedded=true`，无公开顶栏）。

### 顶栏

Admin：居中标题 `AI Hub` + 说明「Make models, agents, and MCP servers public…」。右侧 `Model Hub URL` 只读条：`{proxyBaseUrl}/ui/model_hub_table` + Copy。其下 Admin 可编辑 Useful Links（公开 Hub About 区外链）。非 Admin：嵌入说明条「These are models, agents, and MCP servers your proxy admin has indicated are available in your company.」

### 筛选

Model Hub Tab：`ModelFilters`（搜索 + provider / mode / capability 等，客户端过滤）。Agent Hub：搜索框。MCP Hub：搜索。Skill Hub：Skill 仪表盘自带列表。非 Admin 嵌入视图：Search Models、Provider、Mode、Features 四格筛选。

### 表

| Tab | 列 |
|---|---|
| Model Hub | Public Model Name（`model_group`）、Provider、mode、input/output cost（$/1M）、max tokens、TPM/RPM、capabilities（`supports_*`）、is_public、行 `⋯`（View details / Copy model name） |
| Agent Hub | agent 名、描述、skills、is_public |
| MCP Hub | server_name、transport、描述、is_public |
| Skill Hub | Skill / Claude Code plugin 列表 |

空态：`No models yet` / `No matching models`（Inbox 图标）。客户端分页与排序。

### Tab

线型：`Model Hub` / `Agent Hub` / `MCP Hub` / `Skill Hub`，默认 models。非 Admin 嵌入公开 Hub 同样四 Tab，但 Agent/MCP 仅在有公开数据时出现。

### 抽屉

无独立抽屉。行点模型名或 View details 打开详情 Dialog（等同模态详情）。

### 模态

- 模型详情：能力、价格、支持的 OpenAI params、代码片段。
- Agent / MCP 详情。
- Admin：`Select Models to Make Public` → `MakeModelPublicForm`；对应 Agent/MCP/Skill 的 Make Public 表单。`canModify` = proxy_admin（Admin Viewer 只读，不能改 is_public）。

### URL

`/model-hub-table`。Tab 与筛选不写入 URL。复制的公开 URL 指向 `/ui/model_hub_table`（公开表形态）。

## 交互

进入：Admin 拉 `GET /model_group/info` 与 `GET /config/field/info?field_name=enable_public_model_hub`；同时拉 agents、MCP servers、Claude Code plugins。点模型名开详情；Copy 写剪贴板 toast。Admin 勾选模型提交 Make Public，表 `is_public_model_group` 更新。非 Admin 不出现 Make Public。Useful Links 增删改走 `POST /model_hub/update_useful_links`。窄屏 Provider 列可隐藏（`hidden md:table-cell`）。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| Admin 模型目录 | `GET /model_group/info` | Model Hub 表 |
| 是否启用公开 Hub | `GET /config/field/info?field_name=enable_public_model_hub` | 决定公开 URL 是否可用 |
| Agent 目录 | `GET /v1/agents` | Agent Hub 表 |
| MCP 目录 | `GET /v1/mcp/server` | MCP Hub 表 |
| Skill / plugin | `GET /claude-code/plugins` | Skill Hub |
| 更新 useful links | `POST /model_hub/update_useful_links` | 公开 Hub About 外链更新 |
| 非 Admin 公开目录 | `GET /public/v1/model_hub` 等公开读 | 嵌入只读表 |
| 标公开 | Make Public 表单写对应 agent/MCP/model 公开字段 | 行 `is_public` 徽章 |

## 字段

| 字段 | 含义 |
|---|---|
| `model_group` | 客户端公开别名 |
| `providers` | 上游供应商列表 |
| `mode` | chat / embedding / image 等 |
| `is_public_model_group` | 是否出现在未登录 Hub |
| `input_cost_per_token` / `output_cost_per_token` | 单价，表内按 /1M 展示 |
| `supports_vision` / `supports_function_calling` 等 | 能力徽章 |
| `agent_id` / `is_public` | Agent 公开标记 |
| `server_name` / `transport` | MCP 条目 |

## 状态

loading：表 `Loading models…`。empty：Inbox 空态。forbidden：无登录不进；Admin Viewer 能看不能 Make Public。筛选无匹配：`No matching models`。窄屏表横向滚动。

## 验收

Admin 能列出模型并打开详情、复制公开 URL；非 Admin 只读嵌入目录、无 Make Public；桌面与窄屏走通上表动作；不得 mock 表格。
