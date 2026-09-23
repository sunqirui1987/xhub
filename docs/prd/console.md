# 控制台怎么转

## 五组导航

侧栏分组标签必须是：

1. **AI GATEWAY** — Virtual Keys `/api-keys`、Playground、Models + Endpoints、Agents、Workflows、Memory、MCP、Skills、Guardrails、Policies、Search Tools、Vector Stores、Tool Policies
2. **OBSERVABILITY** — Usage、Cost Optimization、Logs、Guardrails Monitor
3. **ACCESS CONTROL** — Teams、Projects、Users、Organizations、Access Groups、Budgets
4. **DEVELOPER TOOLS** — API Reference、AI Hub、Cache、Prompts、Transform、Tags、Old Usage
5. **SETTINGS** — Router、Logging & Alerts、Admin、Cost Tracking、UI Theme

**默认落地 `/api-keys`。** 已登录访问 `/` 也渲染 Virtual Keys，不是空白 HOME。未登录把 return URL 存好后整页 `/login`。

## view-only

`proxy_admin_viewer` / `internal_user_viewer`（及 `isViewOnly`）**不渲染**突变 CTA：Create Key、Add Model、Delete、Regenerate、Block。菜单隐藏不能代替 API **403**。`enabled_ui_pages` / 内部用户可见页列表收窄侧栏。

## SecretOnce 与筛选

虚拟 Key 明文只在 Create / Regenerate 成功模态出现一次。筛选、排序、分页写入 **URL query**，刷新不丢。

## Chat 壳 vs Playground

| | Playground `/playground` | Chat 壳 `/chat` |
|---|---|---|
| 谁 | 有写权限的管理员/内部用户 | Chat 终端用户 |
| 侧栏 | 五组管理导航 | 无五组；对话壳 + 子页 `/chat/api-keys` 等 |
| 请求 | 真实数据面 `POST /v1/chat/completions` 等 | 同样走数据面，不是管理封装 |
| 失败 | view-only 隐藏入口；无模型空态 | 不能进 Admin Settings；无模型空态 |

公开目录 `/model_hub`、`/model_hub_table` 未登录可浏览，无写按钮。
