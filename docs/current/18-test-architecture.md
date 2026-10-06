# 18 测试架构

状态：按当前代码和测试写。本文说明四层测试各自证明什么、怎么跑、依赖什么。和 `docs/design/13-test-and-acceptance-plan.md` 不同——那篇是**验收目标**（要新增哪些门槛），本文是**现在的测试到底是什么**。

## 四层

| 层 | 位置 | 证明什么 | 依赖 |
| --- | --- | --- | --- |
| Go 单元 | 各包内 `*_test.go`，不需要 DB 的那些 | 一个函数或一个小模块的行为 | 无 |
| Go 逻辑/集成 | `internal/gateway/visibility_chain_test.go`、`permission_test.go`、`internal/authz/authz_test.go` | 真实 DB 上的权限与可见范围，走真实 HTTP 路由 | PostgreSQL |
| 前端单元/组件/集成 | `frontend/src/**/*.test.ts(x)`、`*.integration.test.tsx` | 组件与前端逻辑，只 stub 网络边界 | 无 |
| 浏览器 E2E | `frontend/e2e/*.spec.ts` | 真实浏览器 + 真实网关 + 假上游的端到端行为 | PostgreSQL、网关、假上游 |

**每一层都不能代替上一层。** Go 逻辑测试跑绿，不能说明按钮藏对了；浏览器测试跑绿，不能说明并发下账目正确——它只跑一次。反过来，单元测试不能证明接线对。

## Go 层

### 怎么跑

```bash
make test                 # go test ./...
go test ./internal/gateway/ -run TestVisibilityChain -v   # 只跑可见范围链
```

`make test` 在本机有 `xhub-postgres`（映射到 `5433`）时执行全部；**连不上数据库的测试会 `t.Skip` 而不是失败**。所以它绿不代表权限逻辑真的跑过。要真跑，先起容器：

```bash
docker start xhub-postgres
# 或指向别的库
XHUB_TEST_DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=disable go test ./...
```

判断一次运行是否真的覆盖了 DB：`go test ./internal/gateway/ -v 2>&1 | grep -c SKIP`。数字不为零就说明有测试被跳过了。

### 每个测试怎么隔离

`internal/testsupport.Postgres(t, prefix)` 建一个一次性 schema，把 DSN 的 `search_path` 指过去，注册 `t.Cleanup` 删掉它。

这一步不是洁癖。这些测试会写真实账号、真实密钥和真实用量行，**而它们启动的网关读的是同一批表**。不加隔离的话，一个测试会看见另一个测试的行，还会把开发机上的真实账号留在库里。

调用者传一个 `prefix`（`gateway`、`iam`、`authz`），于是被 kill 掉的测试跑完剩下的 schema 能从名字追回是哪个包留下的。

这个包被排除在 `logx` 的「每个服务端文件都要有分级日志」规则之外，因为它只被 `_test.go` 引用。这条豁免由 `internal/logx/files_test.go` 的 `assertTestSupportIsTestOnly` 看守：一旦有生产文件 import 它，测试就失败。

### 可见范围链

`internal/gateway/visibility_chain_test.go` 是这一层里最该仿照的文件。它建一个从上到下的真实租户——平台管理员、两个组织、每个组织一个管理员、下面的团队和各自的团队管理员、普通成员——然后**每个账号都问同样的问题**。

它断言的不是「某条路由有守卫」，而是「每一层的答案形状一致，且形状就是产品承诺的那一个」：

```
平台管理员    看见全部
组织管理员    看见本组织，且只有本组织
团队管理员    看见自己管的团队，只有那些
普通成员      看见自己，看不到别人的任何东西
```

两个不同组织的账号被特意放在一起，所以放宽作用域的 bug 会表现为**错误组织的行跑出来了**，而不是表现为缺一个守卫。每一层都同时用正向和反向读取探测：返回空列表和一个返回全部列表的 bug 一样糟。

表驱动的用例用 `actor.label()` 渲染标签，失败时能直接看出是谁。

### 权限矩阵

`internal/authz/authz_test.go` 在**决策层**验证同一件事（`decide` 的真值表），`permission_test.go` 在**handler 边界**验证。两者都要有：决策层对、handler 问错问题，是实际发生过的 bug——`TestMasterCannotEnumerateTenants` 就是它的回归测试，主密钥没有 user ID，而列表从那个 ID 推作用域，空 ID 变成了 store 的「不过滤」，于是返回了全部署的组织和项目。决策核心有测试，handler 没有。

### 有只做守卫的测试

`internal/gateway/removed_test.go` 和 `console_split_test.go` 断言某些路由**没有**被挂上（LiteLLM 遗留的列、控制台不该由网关托管）。这类测试容易显得多余，但它们防止的是「有人顺手把一个已删除的接口加回来」。

### 端到端剧本

`cmd/seed` 装载一套可选验证租户（见 [17 验证数据](17-demo-data.md)），`-verify` 用各账号登录并核对列表范围。它是**手动**的，不在 `go test` 里，因为它需要一个已经在监听的网关。

```bash
go run ./cmd/seed -config configs/config.yaml
go run ./cmd/seed -config configs/config.yaml -verify -gateway http://127.0.0.1:4000
```

## 前端单元/组件/集成层

三个 vitest project，按标准定义分层，命名决定归属：

| project | 匹配 | 要求 |
| --- | --- | --- |
| `unit` | `src/**/*.test.ts`、`tests/**/*.test.ts` | 一个模块，协作者换成替身，毫秒级 |
| `component` | `src/**/*.test.tsx` | 渲染组件树，jsdom |
| `integration` | `src/**/*.integration.test.tsx` | 真实组件树，只 stub 网络边界 |
| `types` | `src/**/*.test-d.ts` | 类型测试（`tsc`） |

```bash
cd frontend
npm run test:unit
npm run test:integration
npm run test:types
```

**不要跑整个套件**（`npx vitest run` 不带路径）：几百个文件、上千条用例，会把机器占满好几分钟，而 CI 反正会跑。只跑你改动涉及的文件，加上你认为可能被这次改动弄坏的文件。

有逻辑值得断言时，把逻辑抽出来单测，而不是靠渲染去驱动它。`frontend/CLAUDE.md` 里有完整的取舍说明和一堆具体的坑（哪些 eslint 自动修复会产生看起来通过、实际不再断言的测试）。

## 浏览器 E2E 层

```bash
make e2e
```

`make e2e` 做三件事：清掉 3000/4000/4010 上的残留监听、构建控制台、跑 Playwright。Playwright 自己再拉起两个 `webServer`：`e2e/start-gateway.sh`（假上游 + 网关）和 `next start`。

首次需要 `cd frontend && npx playwright install chromium`。

### 组成

| 文件 | 作用 |
| --- | --- |
| `e2e/fake_upstream.py` | OpenAI 兼容的假上游，支持 chat/embeddings/transcription/moderations/rerank 等 |
| `e2e/start-gateway.sh` | 建独立 `e2e` schema、写临时配置、起假上游和网关，等 health 通过 |
| `e2e/free_ports.py` | 停掉 3000/4000/4010 上的残留进程 |
| `e2e/livesweep/main.go` | 对**已在监听的**网关逐条请求 catalog.json，不经过进程内 handler |
| `frontend/playwright.config.ts` | `testDir: ./e2e`，单 worker、串行 |

网关连的是 `search_path=e2e` 的独立 schema，所以浏览器测试不会碰你的开发数据。

### 各类 spec

| spec | 覆盖 |
| --- | --- |
| `visibility-chain.spec.ts` | Go 那道链的**浏览器**版本：断言控制台渲染的服务端结果、该层看不见的页面和控件确实不存在。服务端对、UI 却全显示，仍然是泄漏 |
| `login.spec.ts` | 登录表单校验与登录成功 |
| `pages.spec.ts` | 侧栏分组、每个控制台页面在壳子里正常渲染 |
| `coverage.spec.ts` | 发现所有 `page.tsx` 路由并逐个访问；模型广场；OAuth 回调；建号后可登录；管理员重置密码后可用新密码登录；chat 返回假上游内容；livesweep |
| `keys-playground.spec.ts` | 虚拟密钥列表与创建、Playground |
| `view-only.spec.ts` | 只读角色看不到 Playground 和建密钥入口 |
| `wizards.spec.ts` | 各「添加」向导：创建后能在列表里看到 |
| `writes.spec.ts` / `column-writes.spec.ts` | 控制台写接口与列级写操作 |
| `sso.spec.ts` | SSO 按钮在配置后可见 |

`coverage.spec.ts` 和 `visibility-chain.spec.ts` 用 `frontend/e2e/report.ts` 把结果追加到 `frontend/.e2e-report/`（`pages.txt`、`chains.txt`、`routes.txt`），`global-teardown.ts` 再汇总。仓库根目录的 `e2e/e2e-report.txt` 是同一份产物的汇总输出，已经在 `.gitignore` 里，不是源文件。

## 什么时候该加哪一层

- **改了一个纯函数、一个解析器、一段算术** → Go 单元测试或前端 `unit`。
- **改了一条权限规则** → `internal/authz/authz_test.go` 的真值表**和** `visibility_chain_test.go` 的至少一层；决策层和数据层都要有，因为它们历史上各错过一次。
- **改了控制台某个页面能不能看见某个东西** → `visibility-chain.spec.ts` 或对应 spec。按钮藏没藏是只有浏览器能回答的问题。
- **改了一条 HTTP 路由的返回结构** → 对应 Go 测试；如果控制台读它，跑 `npm run gen:api` 并提交（CI 有 `Check UI API Types Sync` 把关）。

## 这套测试不证明什么

写清楚免得被误读：

- **并发**。浏览器 E2E 每件事只跑一次；Go 层也没有为账务加并发压力测试。设计文档里要求的崩溃点注入和并发预占验证**尚未实现**。
- **幂等**。`internal/iam/usage_idempotency_test.go` 覆盖了重复投递：同一 `request_id` 反复写入只落一行，一次批量里的重复行只记一次，两笔不同调用各自记账，以及批量中途失败整体回滚。仍然没有覆盖的是**真并发**下的重复投递——两个 flusher 同时提交同一批，靠的是数据库唯一约束而不是测试。
- **真实供应商**。假上游是 OpenAI 形状的占位响应，不验证任何具体供应商的协议、计费或错误语义。
- **无 DB 时的权限逻辑**。跳过就是没跑。
- **覆盖率数字**。`catalog.json` 的 779 条路由是表面积基线，不是业务正确性证据；没有真实响应、故障恢复或结算证据的协议最高只能算实验。
