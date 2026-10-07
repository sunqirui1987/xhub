# 控制台（frontend）

这是 XHub 的管理界面，包名仍是 `litellm-dashboard`，跑的是这个仓库的页面，不是上游 LiteLLM 的模板应用，也不部署到 Vercel。

控制台和网关是两个进程：

| 进程 | 地址 | 命令 |
| --- | --- | --- |
| 控制台 | http://localhost:3000 | `npm run dev`（`next dev -p 3000`）或仓库根的 `make ui` |
| 网关 API | http://localhost:4000 | 仓库根的 `make run` |

登录页是 http://localhost:3000/login 。默认账号来自网关配置 `general_settings.admin_email` / `admin_password`，示例是 `admin@xhub.local` / `admin-pass-1234`。控制台自己的请求打到网关。构建前用 `NEXT_PUBLIC_BASE_URL` 指向别的网关；不设时开发默认连 `http://localhost:4000`。`/ui/...` 只属于 `:3000`。网关不提供这些页面。

## 页面

路由在 `src/app/(dashboard)/`。括号不出现在 URL 里。侧栏能打开的目录包括：

- `models-and-endpoints`、`mine-models`、`model-hub-table`、`price-data`：部署、我的模型、广场、价格覆盖。
- `api-keys`、`teams`、`projects`、`organizations`、`users`：密钥和身份。个人密钥固定在调用方所在的团队上，创建表单不提供「全部组织」搜索。
- `logs`、`usage`、`old-usage`、`cost-tracking`：请求日志和用量。日志按会话 id 分组，费用在界面上按万 token、两位小数显示。原始数由网关返回。
- `playground`：调用示例的 `base_url` 是网关 `:4000`，不是本端口。
- `guardrails`、`prompts`、`router-settings`、`caching`：护栏文档、提示词、路由策略、缓存开关。
- `api-reference`：对 `:4000` 的接口说明。

`removedDashboardPages` 和对应测试记下已经从产品里拿掉、仍可能留在目录数据里的路径。那些路径在网关上由 `ServeMixed` 拒绝，不在这里渲染成可写的共享表。

## 开发

Node.js `>=24.14.1`。在 `frontend/` 里：

```bash
npm install
npm run dev
npm test
npm run test:e2e
```

第一次端到端测试先 `npx playwright install chromium`。端到端会用本机 `xhub-postgres`（映射 `5433`）和已经在跑的网关。单元测试是 Vitest，不启动网关。

`npm run build` 然后 `npm start` 在 `:3000` 提供生产构建。Docker 镜像不在这里编译。根目录 `deploy/build.sh` 把构建结果放进 `bin/console`，控制台容器只运行那份产物。

## 这个目录不做什么

它不实现 `/v1/chat/completions`，不写 `usage_events`，也不匹配七牛或火山的内容生成路径。那些在网关进程里。
