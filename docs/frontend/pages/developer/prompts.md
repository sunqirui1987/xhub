# Prompts

- Route: `/prompts`
- Nav: DEVELOPER TOOLS
- Status: `specified`

## 目的

提示词版本、环境（development/staging/production）与 Prompt Studio 试跑。页顶 `DeprecationBanner`（Prompt Management）。Admin Viewer 只读；写操作仅 proxy_admin。

## 布局

登录后控制台壳。三种主视图互斥：列表、详情、编辑器（Prompt Studio）。列表 `m-8 p-2`。

### 顶栏

列表：左 `Add New Prompt`、`Upload .prompt File`（仅 canModify）；右环境 Select（All Environments / Development / Staging / Production）。详情：返回箭头、`Prompt Details`、prompt_id 可复制、Code snippets、`Prompt Studio`、Admin `Delete Prompt`。编辑器：返回、名称、环境、Save、Version history。

### 筛选

列表环境下拉，值进 `GET /prompts/list?environment=`。无文本搜索。无 URL query。

### 表

列：Prompt ID（mono，点击进详情）、Model（provider logo）、Created At、Updated At、Environment（徽章：production=error / staging=warning / development=success）、Created By、Type（`prompt_info.prompt_type`）、行 `⋯`（Copy prompt ID；Admin 另有 Delete）。

### Tab

详情：Overview / Prompt Template / Raw JSON。环境用页内按钮组（development → staging → production），不是 URL Tab。编辑器：Pretty / Dotprompt 视图切换。

### 抽屉

版本历史：编辑器右侧 `VersionHistorySidePanel`（不是全屏路由）。列表无抽屉。

### 模态

- Upload `.prompt` 文件：`AddPromptForm`。
- 删除确认 AlertDialog（列表或详情）。
- 编辑器：Publish 名称、Tools JSON、保存确认。
- 详情删除确认 Dialog。

### URL

`/prompts`。选中 prompt、环境、编辑器都不写入 URL；返回列表靠组件状态。

## 交互

进入拉 `GET /prompts/list`。点 Prompt ID 进详情（`GET /prompts/{id}/info` + `GET /prompts/{id}/versions`）。切环境重拉 info。Add New Prompt 进 Studio：左配置（模型、temperature、max_tokens、tools、developer message、messages 模板变量 `{{var}}`），右 Conversation 试跑（变量填齐后发送，`POST /prompts/test` SSE）。Save：新建 `POST /prompts`，更新 `PUT /prompts/{id}`。删除 `DELETE /prompts/{id}?environment=`。上传文件先转 JSON 再创建。非 Admin 无 Add/Upload/Delete。试跑可 Abort。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /prompts/list` | 表 |
| 详情 | `GET /prompts/{prompt_id}/info` | 详情页 |
| 版本 | `GET /prompts/{prompt_id}/versions` | 历史侧栏 / 详情版本 |
| 创建 | `POST /prompts` | 回列表新行 |
| 更新 | `PUT /prompts/{prompt_id}` | 保存并回列表或详情 |
| 删除 | `DELETE /prompts/{prompt_id}` | 行消失 |
| 文件转 JSON | 上传转换接口 | 填编辑器 |
| 试跑 | `POST /prompts/test` | 右栏助手气泡流式输出 |

## 字段

| 字段 | 含义 |
|---|---|
| `prompt_id` | 主键 |
| `environment` | development / staging / production |
| `version` | 整数版本 |
| `prompt_info.prompt_type` | 类型 |
| `content` / dotprompt | 模板，含 `{{variables}}` |
| `model` | 试跑与保存时的模型 |
| `temperature` / `max_tokens` | 采样 |
| `created_by` | 创建者；`default_user_id` 不链到用户页 |

## 状态

loading：表骨架。empty：无 prompt 空表。forbidden：未登录；Viewer 无写按钮。校验：名称必填、变量未填齐不能试跑。试跑失败 toast + 可重试。删除中按钮 disabled。窄屏编辑器上下堆叠。

## 验收

Admin 能建/编/删/试跑；Viewer 能看列表与详情但不能写；环境筛选改 list；桌面与窄屏；不得 mock 表格。
