# 测试指南

[开发指南](README.md) · [安装](../getting-started.zh-CN.md)

## 按修改选择验证

| 修改 | 优先验证 |
| --- | --- |
| 纯函数、解析、价格计算 | 对应包的 Go 用例或前端 unit 文件 |
| 权限或列表范围 | authz 决策用例、gateway HTTP 范围用例 |
| 请求/响应协议、usage、断流 | 固定上游样本、数据面失败用例、模拟上游回归 |
| 表单、选择器、国际化 | 受影响组件及集成测试 |
| 页面权限和跨服务流程 | Playwright 对应 spec |
| 文档 | 相对链接、代码示例、语言切换、旧路径引用与空白检查 |

## Go 与 PostgreSQL

```bash
make test
# 精确验证一个权限场景
go test ./internal/gateway -run TestVisibilityChain -v
```

数据库默认连接本地 Compose 的 `127.0.0.1:5433`。也可使用 `XHUB_TEST_DATABASE_URL`。普通测试在数据库不可达时可能跳过，检查输出中的 SKIP；通过不表示数据库行为已验证。

```bash
docker compose up -d postgres
XHUB_REGRESSION_STRICT=1 go test ./... -count=1
```

`internal/testsupport/postgres.go` 中的 `Postgres` 为各测试创建独立 schema，用 search_path 隔离，并通过 `t.Cleanup` 清理。数据库测试只在测试文件中引用 testsupport。不要把生产数据库的数据关系用作 fixture。

关键入口：`internal/authz/authz_test.go` 验证动作矩阵；`internal/gateway/permission_test.go` 验证 HTTP 边界；`visibility_chain_test.go` 用两个组织的不同角色检查正反向读取和列表范围。`internal/iam/usage_idempotency_test.go` 验证重复写入、并发结算和事务回滚；`internal/live/redis_test.go` 验证队列与去重。

## 模拟上游回归

```bash
make regression
# 只跑名称匹配的测试；脚本会检查是否匹配到用例
bash scripts/regression.sh -v Pricing
```

脚本使用真实网关、真实 PostgreSQL 和假上游，开启数据库严格模式。每个用例有独立 schema 和随机网关端口，覆盖权限、预算、计价、护栏、缓存及官方转发等接线。它不证明真实供应商协议全部正确。

`make regression-live` 另行调用真实供应商，消耗实际额度，需要 `XHUB_REGRESSION_<ID>_KEY` 等环境变量。只有需要真实上游验证且已获授权时才运行；凭据不写进仓库。

## 前端

从 `frontend` 目录定向运行：

```bash
npm run test:unit -- src/components/route_templates/templateForm.test.ts
npm run test:component -- src/components/route_templates/RouteTemplateSelect.test.tsx
npm run test:integration -- src/components/templates/key_edit_view.integration.test.tsx
npm run test:types
```

项目匹配由 `frontend/vitest.config.ts` 定义：unit 为 `.test.ts`，component 为 `.test.tsx`，integration 为 `.integration.test.tsx`，types 为 `.test-d.ts`。不要无路径执行整个 Vitest 套件；遵循 [frontend/CLAUDE.md](../../frontend/CLAUDE.md) 的范围与断言要求。

修改后端接口后，若控制台使用该接口，运行 `npm run gen:api` 同步生成类型，避免手改 schema.d.ts。

## 浏览器 E2E

首次安装 Chromium：

```bash
cd frontend
npx playwright install chromium
```

然后从仓库根目录运行：

```bash
make e2e
```

`make e2e` 运行 `scripts/e2e-all.sh`：浏览器流程之后执行含真实供应商的严格后端回归，需要可用的真实供应商凭据并会产生费用。只运行本地模拟浏览器测试可执行 `make e2e-offline`。浏览器脚本默认使用独立的 `3100/4100/4110` 端口及 `.next-e2e` 构建目录；端口占用时直接失败，可用 `E2E_UI_PORT`、`E2E_GW_PORT`、`E2E_UP_PORT` 覆盖。Playwright 的 webServer 启动假上游、网关和控制台；网关使用独立 e2e schema。完整场景矩阵见[浏览器回归](e2e-regression.md)，后端场景和真实模型条件见[全链路回归](regression.md)。

权限界面重点看 `frontend/e2e/visibility-chain.spec.ts`；密钥与 Playground 看 `keys-playground.spec.ts`；创建和写操作看 wizards/writes 相关 spec。浏览器检查不代替数据库并发和故障测试。测试输出是临时产物，不加入正式文档。

## 可选演示租户

这套数据不是启动默认。仅在开发或测试实例上执行：

```bash
go run ./cmd/seed -config configs/config.yaml
# 网关监听后再核对范围
go run ./cmd/seed -config configs/config.yaml -verify -gateway http://127.0.0.1:4000
```

启动只建表、按配置创建尚不存在的初始管理员，并登记内置供应商凭据；不会自动创建演示组织、团队、项目或这些账号。演示用户密码是 `demo-pass-1234`；平台管理员仍使用配置里原有密码。

| 演示账号 | 身份与预期范围 |
| --- | --- |
| `org-a-admin@xhub.local` | 组织甲管理员兼甲一组成员；看甲一组和甲二组 |
| `team-a1-admin@xhub.local` | 甲一组管理员；看本组成员与用量 |
| `member-a1@xhub.local` | 甲一组成员；只看自己的用量与日志 |
| `team-a2-admin@xhub.local` | 甲二组管理员 |
| `org-b-admin@xhub.local` | 组织乙管理员兼乙一组管理员 |
| `member-b1@xhub.local` | 乙一组成员 |
| `outsider@xhub.local` | 无团队；没有可用模型 |

平台管理员应看见两个组织和三个团队。组织甲用户不得看见乙的人员与日志；没有团队的用户只看自身账号。

## 解释验证结果

`docs/testdata/catalog.json` 是路由表面积基线，被 gateway 与 E2E 读取，必须保留；登记的路径数量不是供应商业务正确性证据。模拟上游不验证真实供应商的费用或错误语义。进程重启、跨实例故障、长时间压力与供应商对账需要专门场景，不能由一次全绿结果推导。
