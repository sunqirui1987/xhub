# Search Tools

- Route: `/search-tools`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

搜索工具登记与测连接。

## 布局

- **顶栏 CTA**：标题 Search Tools。Admin：`+ Add New Search Tool`。
- **筛选**：无搜索框。Provider 下拉来自 `available_providers`（创建/编辑表单内）。
- **表/卡片/Tab**：表列 Search Tool ID、Name、Provider、Created At、Updated At、Source。行菜单 View / Edit / Delete。点 ID 进 `SearchToolView`（含 Test connection）。
- **抽屉/模态**：CreateSearchTool 对话框（name、provider、api_key、description）。Edit Search Tool 对话框。Delete 确认。
- **URL query**：无。

## 交互

- 进页并行 `GET /search_tools/list` 与 `GET /search_tools/ui/available_providers`。Add → `POST /search_tools` 新行。Edit → `PUT /search_tools/{id}`。View 内 Test connection → `POST /search_tools/test_connection` 成功/失败。Delete 行消失。
- **view-only / 非 admin**：无 Add 按钮；表仍可看。
- **loading**：表 loading；provider Select 内 spinner。
- **empty**：`No search tools configured` / Add a search tool to enable web search for your models.
- **forbidden**：非 admin 无创建 CTA。
- 校验失败（缺 name/provider）可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /search_tools/list` | 表 |
| 提供商 | `GET /search_tools/ui/available_providers` | 下拉 |
| 创建 | `POST /search_tools` | 新行 |
| 测连接 | `POST /search_tools/test_connection` | 成功/失败 |

## 字段

| 字段 | 含义 |
|---|---|
| `search_tool_name` | 名称 |
| `search_provider` | tavily/serper 等 |
| `api_key` | 上游密钥引用 |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
