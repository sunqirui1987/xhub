# 标签

- Route: `/tag-management`
- Nav: DEVELOPER TOOLS
- Status: `specified`

## 目的

请求归因与 tag routing：标签可限制哪些模型能处理带该 tag 的请求，并可挂预算。动态 spend tag（请求里临时传入、描述为动态 spend tag）只读，不能进详情/编辑/删除。

## 布局

登录后控制台壳。`mx-4` 全高。列表与详情互斥：未选 tag 显示表；选中显示 `TagInfoView`（整页，不是侧抽屉）。

### 顶栏

列表：左 `Tag Management`；右 Last Refreshed + 刷新。说明 + tag routing 文档外链。主按钮 `+ Create New Tag`。详情：返回、tag 名、Copy、Admin Edit/Save。

### 筛选

无筛选栏。表头可按 Tag Name、Created 排序（客户端）。

### 表

列：Tag Name（非动态 tag 可点）、Description、Allowed Models（空则徽章 `All Models`，否则 model 徽章）、Created、行 `⋯`（Edit / Delete；动态 tag disabled）。

### Tab

无。详情是整页表单/只读卡，不是 Tab。

### 抽屉

无。详情替换主区。

### 模态

- Create Tag：`tag_name`（必填）、description、allowed_llms 多选（选项来自 `GET /model/info`）、可展开 Budget（max_budget、budget_duration）。TPM/RPM 文案标明 tag 暂不支持。
- Delete Tag：`DeleteResourceModal`，展示 Tag Name。

### URL

`/tag-management`。选中 tag 不写入 URL。

## 交互

进入 `GET /tag/list` 填表。点名称进详情 `GET /tag/info`。Create 提交 `POST /tag/new`（name/description/models/max_budget/soft_budget/tpm_limit/rpm_limit/budget_duration），成功关模态刷新。Edit 进详情编辑态，Save `POST /tag/update`。Delete 确认后 `POST /tag/delete`。动态 spend tag 名称灰色不可点，菜单 Edit/Delete disabled，tooltip 解释。刷新更新 Last Refreshed。Admin 以外详情只读。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /tag/list` | 表 |
| 模型选项 | `GET /model/info`（`modelInfoCall`） | Create 多选 |
| 创建 | `POST /tag/new` | 关模态、新行 |
| 详情 | `GET /tag/info` | 详情页 |
| 更新 | `POST /tag/update` | 保存退出编辑 |
| 删除 | `POST /tag/delete` | 行消失 |

本列表页不画 DAU/WAU/MAU；那些接口在其他用量视图。

## 字段

| 字段 | 含义 |
|---|---|
| `name` | 标签名，请求里传 tags |
| `description` | 说明；动态 spend tag 有固定描述 |
| `models` | 允许的 model id；空 = 全部 |
| `model_info` | id → 显示名 |
| `max_budget` / `budget_duration` | 预算与重置周期 |
| `soft_budget` | 软阈值（创建可传） |
| `created_at` | 创建时间 |

## 状态

loading：表 `isLoadingTags`。empty：无 tag。forbidden：未登录。创建校验：tag_name 必填贴在字段旁。删除中按钮锁定。动态 tag 不可写。窄屏模型徽章折行。

## 验收

能建/编/删普通 tag；动态 spend tag 不能进详情或删除；预算折叠默认不提交；桌面与窄屏；不得 mock 表格。
