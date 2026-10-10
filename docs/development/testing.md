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

`cmd/regression/testsupport/postgres.go` 中的 `Postgres` 为各测试创建独立 schema，用 search_path 隔离，并通过 `t.Cleanup` 清理。数据库测试只在测试文件中引用 testsupport。不要把生产数据库的数据关系用作 fixture。

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

修改后端接口后，若控制台使用该接口，必须核对实际请求和响应，并同步验证消费者。`npm run gen:api` 当前仍从历史 Python LiteLLM/FastAPI 应用生成 `schema.d.ts`，并非从本项目 Go 网关生成；本轮执行因缺少 `litellm.proxy.proxy_server` 失败，不能宣称已同步 Go 接口类型。不要手改生成文件，也不要用其它版本的 LiteLLM 输出覆盖本项目接口契约。Go 接口在当前阶段以后台 HTTP regression、前端消费者测试和真实浏览器链核对，生成器迁移仍是明确待办。

## 浏览器 E2E

首次安装 Chromium：

```bash
cd frontend
npx playwright install chromium
```

然后从仓库根目录运行：

```bash
make e2e-offline       # 本地假上游，免费
make e2e-model-endpoints # 隔离 Redis、随机 schema、本地供应商，免费
make e2e                # 共享验收基线，真实供应商，收费
make e2e-all            # 完整隔离验收，真实供应商，收费
```

`make e2e-offline` 使用 `scripts/e2e.sh`，不读取真实供应商密钥。`make e2e-model-endpoints` 使用隔离 Redis、随机浏览器 schema 和本地供应商。`make e2e` 使用共享 `xhub/public` 与 Redis DB 1 的验收基线，读取真实供应商密钥并产生费用；`make testdata` 会重建该共享基线但不调用模型。`make e2e-all` 使用独立 Redis、随机 schema 和真实供应商并产生费用。浏览器默认使用独立端口和 `.next-e2e`；完整场景见[浏览器回归](e2e-regression.md)。

权限界面重点看 `frontend/e2e/visibility-chain.spec.ts`；密钥与 Playground 看 `keys-playground.spec.ts`；创建和写操作看 wizards/writes 相关 spec。浏览器检查不代替数据库并发和故障测试。测试输出是临时产物，不加入正式文档。

## 隔离回归数据

`make regression` 为每个 Go 用例创建并清理独立 PostgreSQL schema；Redis 专项必须另行提供独立的 `XHUB_REGRESSION_REDIS_URL`。`make testdata` 使用共享 `xhub/public` 与 Redis DB 1，会清空并重建基线，不调用模型，也不是隔离 schema。E2E 报告位于 `.e2e/runs/<timestamp>-<pid>/`，并更新 `.e2e/latest`。

启动只建表、按配置创建尚不存在的初始管理员，并登记内置供应商凭据；不会自动创建演示组织、团队、项目或账号。

## 解释验证结果

`docs/testdata/catalog.json` 是路由表面积基线，被 gateway 与 E2E 读取，必须保留；登记的路径数量不是供应商业务正确性证据。模拟上游不验证真实供应商的费用或错误语义。进程重启、跨实例故障、长时间压力与供应商对账需要专门场景，不能由一次全绿结果推导。
