# 真实数据验收：架构、运行方式与覆盖边界

本目录描述 `scripts/e2e-real-dataset.py` 驱动的真实数据验收。它用公开管理 API 建立一套隔离租户数据，通过两个真实模型供应商完成请求，再从浏览器、网关报告、上游观察记录和 PostgreSQL 四个角度核对结果。`coverage.json` 是按当前 `frontend/src/app/**/page.tsx` 审计得到的覆盖库存，不是“所有前端均已测试”的声明。

## 数据流与证据

```text
dataset.json（声明资源、预算、角色、供应商环境变量）
        │
        ▼
scripts/e2e-real-dataset.py
  ├─ 随机 e2e_real_<12hex> PostgreSQL schema
  ├─ 独立 redis:7-alpine 容器和随机回环端口
  ├─ 本次构建的 cmd/gateway + 私有 c.yaml
  ├─ e2e/real_dataset.py：管理 API 建数、数据面请求、契约断言
  ├─ Observer：网关与真实供应商之间的本地转发观察器
  ├─ e2e/real_dataset_browser.cjs：Next.js + Playwright 浏览器流程
  └─ psql：事件、日报和五级实体累计金额一致性
```

manifest 是 `dataset.json`。它声明 3 个组织、9 个团队、27 个成员、81 把个人密钥、9 个项目、两套真实供应商、路由模板、护栏、预算、RPM/TPM、模型白名单和角色预期。manifest 中的 `frontend_workflows` 只表示资源与验收意图；只有 `browser-report.json` 中的检查才算真实浏览器覆盖。当前浏览器实际访问组织、Playground、日志、护栏和团队页面，并用四个独立浏览器上下文验证角色。

runtime 由 runner 每次生成：随机端口、管理员密码、master key、schema 名、Redis 容器名和私有配置。网关只连接本次 schema 与 Redis。管理 API 创建资源；个人密钥调用 `/v1/chat/completions`；API 验证真实响应 token、固定验收费率、五级账单归属、预算/RPM/TPM 拒绝、拒绝时零外发与恢复、模型白名单、回退以及资源生命周期。

`Observer` 位于网关和供应商之间。正常路径把请求转发到 manifest 指定的 HTTPS 供应商，保留供应商真实响应与 `usage`；故障路径在转发前注入 429，用于验证回退；它同时记录供应商、模型、请求标记、是否转发、状态码和 usage 到 `upstream-observations.json`。它不模拟成功补全，也不证明供应商侧最终账单；金额验证使用 manifest 固定的输入、输出 token 单价。观察记录含请求消息，应把整个运行目录视为私密材料。

浏览器层启动独立 Next.js dev server，不拦截网关请求。它验证管理员登录、组织展示、Playground 真实回答与五级账单、日志可见、护栏创建/调试/保存/删除，以及 `platform_admin`、`organization_admin`、`team_admin`、`member` 四种身份的团队可见范围和编辑权限。API 与浏览器之后，SQL 读取隔离 schema：成功事件数、唯一 request ID 和总费用；`usage_daily` 总费用必须等于事件费用；organization、team、user、project、api_key 的 `spend` 必须分别等于对应事件维度的费用和。

## 运行

### 共享真实环境：先构建，后验收

`make testdata` 是共享真实数据入口。它会先验证 Fenno AI 和七牛凭据，然后删除并重建 PostgreSQL `xhub` 数据库、清空 Redis DB 1，再把 `dataset.json` 的 3 个组织、9 个团队、27 个成员、9 个项目和 81 把个人密钥写入 `public`。凭据验证先于清库，目标 URL 也有精确白名单，防止误清其他数据库或 Redis DB。

```bash
export FENNO_AI_API_KEY='…'
export QINIU_API_KEY='…'
make testdata
make e2e
```

固定目标是：

```text
postgres://xhub:xhub_dev_password@127.0.0.1:5433/xhub?sslmode=disable
redis://127.0.0.1:6379/1
```

`make e2e` 读取 `.e2e/real-acceptance-current` 中由 `make testdata` 保存的状态，使用同一份 `public` 数据完成 81 把密钥的真实请求、权限/预算/限流/模型/护栏/回退验收，随后执行真实 Playwright 页面流程、PostgreSQL 事件/日报/token/五级 spend 核对，以及 `bash scripts/regression.sh -v` 后台回归。过程直接输出到终端，主报告是 `.e2e/real-acceptance-current/report.json`，浏览器报告是同目录的 `browser-report.json`，网关和前端日志分别是 `gateway.log` 与 `console.log`。两个命令都会保留共享数据，下一次 `make testdata` 才会再次全部清空。

### 隔离的一次性入口

前置条件是本地 `xhub-postgres` 容器可通过 runner 内置连接访问、Docker 可启动 Redis、Go 与前端依赖已准备好，并配置两个真实供应商凭据：

```bash
export FENNO_AI_API_KEY='…'
export QINIU_API_KEY='…'
# 可选；必须是无用户信息、无 Markdown 字符的 HTTPS URL
export FENNO_AI_API_BASE='https://…'
export QINIU_API_BASE='https://…'
python3 scripts/e2e-real-dataset.py
```

默认运行目录为 `.e2e/real-dataset-YYYYMMDD-HHMMSS`，权限设为 `0700`。`access.json`、`c.yaml` 含访问材料；不要提交、复制到公开位置或在日志中打印。`report.json` 是主报告，`browser-report.json` 是浏览器检查，`upstream-observations.json` 是上游观察证据，`gateway.log` 和 `console.log` 用于诊断，PNG 是浏览器截图。

常用模式：

```bash
# 仅运行 API、真实供应商和 SQL；报告会明确 browser=not-run
python3 scripts/e2e-real-dataset.py --api-only

# 保留 PostgreSQL schema 和私有检查点，供诊断或续跑
python3 scripts/e2e-real-dataset.py --keep-data --directory .e2e/real-dataset-case

# 从同一目录恢复；resume 强制要求 keep-data
python3 scripts/e2e-real-dataset.py --resume --keep-data --directory .e2e/real-dataset-case
```

默认 cleanup 会终止 gateway、UI 和 observer，停止并删除本次 Redis 容器，删除本次 `e2e_real_*` schema，但保留报告目录并写入 `cleanup.txt`。`--keep-data` 会保留 schema，在 observer 退出前把主、备用部署的 `api_base` 恢复到供应商 HTTPS 地址，停止 Redis，并写入 `retained.txt`；再次使用配置前仍需启动可用 Redis 并重新导出供应商凭据。

`--resume` 只接受 runner 命名的 `e2e_real_<12hex>` schema，并要求旧报告已经有通过的 `81-personal-keys-real-calls-and-billing` 证据。它从 `c.yaml`、`schema.txt`、`access.json` 和 `report.json` 恢复状态，启动新的隔离 Redis，清理可能中断的临时回退部署，恢复团队模型白名单，将部署临时指向新的 observer，再补跑权限、限制、模型/护栏生命周期、浏览器和 SQL。续跑复用先前真实请求证据，因此报告必须和同一私有目录一起解释。

## schema reset 边界

当前仓库代码中没有 `oldpublic` 或 `backedup` schema 的创建、重命名或恢复逻辑。它们若存在，是人工操作或旧工具留下的 `public` 备份，不属于本 runner 的状态机，也不能作为本验收成功或可恢复的证据。不要为了运行验收重命名、清空或替换 `public`，也不要让 `--resume` 指向这些 schema。

本 runner 的隔离策略是新建随机 `e2e_real_*` schema，并通过 PostgreSQL DSN 的 `search_path` 让网关只读写该 schema。默认清理只执行 `DROP SCHEMA <本次 e2e_real_* > CASCADE`；续跑还会校验命名格式。这个边界避免触碰 `public`、`oldpublic`、`backedup` 或其他任务的 schema。若人工 reset 曾把 `public` 改名为 `oldpublic`/`backedup`，恢复与删除必须由该 reset 的拥有者按其备份流程处理，真实验收不会自动判断哪份是权威数据。

## 失败语义与能力边界

- 缺少任一供应商 key、供应商 URL 非 HTTPS、服务启动失败、API 契约失败、页面异常、上游尝试超过 manifest 上限或 SQL 不一致都会使命令失败。失败时尽可能写入 `report.json`，随后仍执行 cleanup；诊断先看命令输出给出的私有目录。
- `--api-only` 明确不产生浏览器覆盖。普通运行当前也只浏览 `coverage.json` 标出的页面；manifest 中有资源不等于对应页面被打开。
- Observer 只能证明网关尝试、阻断或转发以及收到的供应商响应，不能证明供应商结算、跨地域网络或生产部署配置。
- Redis 默认不保留，`--keep-data` 也只保留 PostgreSQL schema；依赖 Redis 瞬时状态的限流窗口不能在续跑后原样重现。
- 前端库存包括 dashboard、Chat、公共模型中心、连接、登录和 OAuth callback。Chat/MCP/OAuth 与若干管理页属于独立套件、外部流程或当前缺口，不能由本数据集推断为已覆盖。
- `coverage.json` 中 `declared_dataset` 表示存在 manifest 资源或本套 API/SQL/browser 证据；请继续查看每项的 `evidence`。`missing` 表示后端可能支持但本套没有对应页面验收，`unsupported` 表示当前 gateway 没有该页面所需完整能力，`external` 表示结果依赖外部授权方，`separate_suite` 表示重定向、兼容页或另一条产品流程。

优先可行动缺口是：给 users、projects、api-keys、models、route-templates、usage 和 guardrails-monitor 增加真实浏览器流程；给平台管理页补最小读写/权限流程；为 Chat/Responses/MCP 建独立真实验收；删除或明确隐藏没有完整后端能力的 vector-stores 页面。具体页面、关键后端路由、角色和逐项原因见 `coverage.json`。
