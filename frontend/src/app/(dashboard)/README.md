# 控制台页面 `(dashboard)`

Next.js 的路由组。目录名上的括号不会出现在 URL 里。`layout.tsx` 是登录之后的壳：侧栏、当前团队、以及发往网关 `:4000` 的会话。

这里不是 LiteLLM UI 的贡献指南。页面按这个仓库的信息架构放，一个侧栏项一个目录，`page.tsx` 是路由入口，重的表格和表单放在同目录的 `components` 或 `_components`。`_components` 同样不进 URL。

## 数据从哪来

每个 `page.tsx` 通过 `src/components/networking` 或本目录的 `networking.ts` 调用网关，不调用 `:3000` 自己。`fetchTeams` 在角色不是平台管理员时带上当前用户 id，这样列表是这个人所属的团队，不是全部组织。

网关先按 `authz` 收窄，再返回行。前端不把 403 收成空列表再假装没有数据。调用方角色打不开的接口不应该发请求。管理接口返回的 `ApiError` 停在这一页的错误状态里，不冒泡成整站崩溃。

## 和网关路径的对应

| 目录 | 网关 |
| --- | --- |
| `api-keys` | `/key/generate`、`/key/list`、`/key/update` |
| `teams`、`projects`、`organizations`、`users` | `/team/*`、`/project/*`、`/organization/*`、`/user/*` |
| `models-and-endpoints` | `/model/new`、`/model/update`、`/v1/models`。端点类型多选对应 `model_info.endpoint_types` |
| `logs` | `/spend/logs/ui` 和 `/spend/logs/session/ui` |
| `usage` | `/user/daily/activity`、`/team/daily/activity`。查询参数 `timezone` 是浏览器偏移取反之前的 `getTimezoneOffset()` |
| `router-settings` | `/router/settings`、`/config/field/update` |
| `guardrails` | 键值种类 `guardrails`。推理时只有聊天会跑 `GuardrailBlocks` |
| `price-data` | `/price/model`、`/reload/model_cost_map` |

Playground 复制出的 Python 或 curl 使用 `base_url` `http://localhost:4000`（或 `NEXT_PUBLIC_BASE_URL`）。不要写成 `:3000`。

## 测试

同目录的 `*.test.tsx` 用 Vitest 渲染组件。`frontend/e2e` 用 Playwright 打开 `:3000` 并打到 `:4000`。改一条页面的数据来源时，同时看网关上读这批数据的处理函数，避免只改了一边。
