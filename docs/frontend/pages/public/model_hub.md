# 公开模型目录

- Route: `/model_hub`
- Nav: PUBLIC
- Status: `specified`

## 目的

**未登录可浏览**的公开 Hub（卡片/表混合：模型表 + Agent/MCP/Skill）。无写按钮、无 Make Public、无控制台五组侧栏。若 UI 设置 `require_auth_for_public_ai_hub`（本路由本身不强制；表格形态 `/model_hub_table` 会检查），管理员可要求登录。`?key=` 可选带 access token 仅用于主题，不是写授权。

## 布局

公开壳：`Navbar isPublicPage=true`（无管理菜单）。宽 `px-8 py-12`。嵌入控制台时（`isEmbedded`）无 Navbar、无 About。

### 顶栏

公开 Navbar。About 卡：`docs_title`（默认 LiteLLM Gateway）、`custom_docs_description` 或默认「Proxy Server to call 100+ LLMs…」、版本 `Built with litellm: v{version}`。Useful Links 卡（管理员在 AI Hub 配置的外链）。Health：`I'm alive! ✓` 或 `Service unavailable`。

### 筛选

Models Tab 四格：Search Models（跨页名字包含）、Provider 多选、Mode 多选、Features 多选。Agent：搜索 + skill tags。MCP：搜索 + transport。筛选进 list query（`/public/v1/model_hub`），不是写接口。

### 表

Models：公开列（model_group、provider、mode、价格、能力）。Agent/MCP：对应列。Skill Hub：plugin 列表。空：Inbox `No models yet` / 无匹配。客户端/服务端分页 page_size=50。点行开只读详情 Dialog（能力、价格、代码片段）。**无 Create / Make Public。**

### Tab

`Model Hub` 常在；`Agent Hub` 仅当 `GET /public/agent_hub` 非空；`MCP Hub` 仅当公开 MCP 非空；`Skill Hub` 常在。

### 抽屉

无。

### 模态

只读详情（模型/Agent/MCP）。无写表单。Copy model name。

### URL

`/model_hub`。可选 `?key=`。Tab/筛选默认不作为登录态。匿名 GET 公开接口。

## 交互

进入先 `GET` UI config / `GET /public/model_hub/info`（标题、链接、版本），再并行公开 agents/MCP/skills 与 `GET /public/v1/model_hub`。搜索防抖重查。点行只读。外链 `window.open`。无写按钮。列表失败 About 仍可显示；模型 list error → Service unavailable。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| Hub 元信息 | `GET /public/model_hub/info` | About / Useful Links / 版本 |
| 模型分页 | `GET /public/v1/model_hub` | 表 |
| 公开 Agent | `GET /public/agent_hub` | Agent Tab |
| 公开 MCP | `GET /public/mcp_hub` | MCP Tab |
| 公开 Skill | skill hub public | Skill Tab |
| UI config | well-known / ui config | 根路径、主题 |

无 POST。旧 `GET /public/model_hub` 仍存在，本页列表走 `/public/v1/model_hub`。

## 字段

| 字段 | 含义 |
|---|---|
| `docs_title` / `custom_docs_description` / `litellm_version` | About |
| `useful_links` | 标题 → url（可带 index 排序） |
| `model_group` | 公开别名 |
| `providers` / `mode` / `supports_*` | 筛选与列 |
| `input_cost_per_token` / `output_cost_per_token` | 价格 |

## 状态

loading：表 Loading。empty：Inbox。error：Health `Service unavailable`，不假装有模型。无 forbidden 写态（匿名只读）。窄屏筛选 1 列，表横向滚。

## 验收

未登录能打开并看到公开模型；页面上无 Create/Make Public；筛选只影响 GET 查询；桌面与窄屏；不得 mock 表格。
