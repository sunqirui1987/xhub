# 公开模型表

- Route: `/model_hub_table`
- Nav: PUBLIC
- Status: `specified`

## 目的

公开 Hub 的**表格形态**（`ModelHubTable publicPage=true`）。未登录可浏览已公开模型。Admin 在登录 AI Hub 复制的 URL 就是本页。无 Create。若 `require_auth_for_public_ai_hub=true` 且 cookie token 无效，跳登录。

## 布局

`publicPage=true`：公开 Navbar + AI Hub 表。无控制台五组侧栏。可选 `?key=` 当 accessToken（只读）。

### 顶栏

标题 AI Hub。Model Hub URL 展示 `{base}/ui/model_hub_table`。无 Useful Links 管理（那是登录 Admin 的 `/model-hub-table`）。无 `Select Models to Make Public`。

### 筛选

`ModelFilters`：搜索、provider、mode、capabilities。Agent/MCP Tab 搜索。匿名拉 `GET /public/model_hub` 填模型表。

### 表

与登录 AI Hub 同列（Public Model Name、Provider、mode、价格、tokens、capabilities），但数据源是公开目录，行菜单只有 View details / Copy model name。无 is_public 切换。Agent/MCP/Skill 只读。

### Tab

Model Hub / Agent Hub / MCP Hub / Skill Hub。公开页 Agent/MCP 数据在 `publicPage` 下不走登录 `GET /v1/agents`（那些 effect 在 `!publicPage` 才 fetch）；模型走 `modelHubPublicModelsCall`。

### 抽屉

无。详情 Dialog。

### 模态

只读模型详情。无 Make Public 表单。

### URL

`/model_hub_table`。`?key=` 可选。`require_auth_for_public_ai_hub` 为真且 token 无效 → 登录页。

## 交互

进入：`GET` UI config + `GET /public/model_hub`。点行只读详情。Copy 名称。无写。鉴权开关来自 `GET /get/ui_settings` 的 `require_auth_for_public_ai_hub`。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 公开模型 | `GET /public/model_hub` | 表 |
| UI 设置（是否要登录） | `GET /get/ui_settings` | 可能跳登录 |
| Hub 信息 / 主题 | public info + ui config | 标题与 Navbar |

无 Create。提供商字段说明若需要来自公开 schema，随列渲染，不单独开「列说明」页。

## 字段

| 字段 | 含义 |
|---|---|
| `model_group` | 公开名 |
| `providers` | 供应商 |
| `mode` | chat/embedding 等 |
| `is_public_model_group` | 公开页应全为已公开条目 |
| `require_auth_for_public_ai_hub` | 是否强制登录 |

## 状态

loading：Loading models。empty：Inbox。token 无效且要求登录：整页替换到 login。无写按钮。窄屏表横向滚。

## 验收

匿名打开能看到表、无 Create；管理员复制的 URL 即本路由；强制登录开启时无 token 进登录；桌面与窄屏；不得 mock 表格。
