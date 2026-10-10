# 真实供应商回归：如何执行

[回归方案](regression.md) · [浏览器回归](e2e-regression.md)

这条入口跑 `cmd/regression` 全套。网关、身份、PostgreSQL、预算、护栏和入账都是真实的。`XHUB_REGRESSION_LIVE=1` 时，预算、名单、计量、计费、路由模板和加权分流里的 live 用例会打到 Fenno 与七牛。精确的 500/429 回退次数仍由同一条命令里的假上游用例验证，因为真实供应商不能稳定地按指定状态码失败。

密钥只放在环境变量里。不要写进 YAML、日志、README 或 git。

## 1. 准备

- Docker 里的 PostgreSQL 已在 `127.0.0.1:5433` 监听。没有就执行 `docker compose up -d postgres`。
- 本机可以拉 `redis:7-alpine`。脚本会起一个只给这次使用的 Redis，并在开跑前清空。
- 已设置下面四个变量。地址可以省略，脚本使用这里的默认值。

```bash
export FENNO_AI_API_BASE=https://api.fenno.ai
export FENNO_AI_API_KEY=sk-...
export QINIU_API_BASE=https://api.modelink.ai
export QINIU_API_KEY=sk-...
```

默认模型是 Fenno 的 `gpt-5.5` 和七牛的 `claude-4.5-haiku`。这两个名字在 2026-10-09 的目录探测里能返回 usage，并且价目表能计价。`qwen-turbo` 和 `deepseek-v3` 当时返回 502，所以没有用作默认。要换模型：

```bash
export FENNO_LIVE_MODEL=gpt-5.6-sol
export QINIU_LIVE_MODEL=claude-4.5-haiku
```

模型必须出现在对应的 `GET /v1/models` 里。脚本开跑前会检查，不在目录里就直接退出，不会进入付费对话。

## 2. 一条命令

在仓库根目录执行：

```bash
bash scripts/regression-live-log.sh
```

脚本会：

1. 检查两个密钥存在，并确认所选模型在供应商目录中。
2. 使用独立库 `xhub_regression_live`。每条用例再建自己的 schema，结束时删除。不使用共享库 `xhub`。
3. 启动或复用容器 `xhub-regression-live-log`，清空里面的 Redis。
4. 在运行目录写一份不含密钥的 `providers.yaml`，交给 `scripts/with-live-vendors.py` 注入 `XHUB_REGRESSION_FENNO_*` 和 `XHUB_REGRESSION_QINIU_*`。
5. 执行 `go test ./cmd/regression/ -count=1 -timeout=3600s -json`。总超时可用 `XHUB_REGRESSION_TIMEOUT` 改，例如 `export XHUB_REGRESSION_TIMEOUT=5400s`。

这会花钱。预算链、路由模板、计量和加权分流都会对真实模型发出短提示。默认每次 `max_tokens` 很小，但供应商仍按真实 usage 计费。

## 3. 实时日志

终端不会等整包结束才出结果。每个顶层用例一开始就打印 `RUN`，结束就打印 `PASS`、`FAIL` 或 `SKIP`。子测试结束时同样立刻打印。供应商调用期间即使 `go test` 没有任何新输出，跟随程序仍每 20 秒打印当前用例和已等待秒数。间隔可用 `XHUB_REGRESSION_LOG_HEARTBEAT` 改，单位是秒。

同一份内容追加写入运行目录的 `live.log`。另开一个终端跟着看：

```bash
tail -f .e2e/runs/regression-live-*/live.log
```

运行开始时脚本会打印这次的具体目录。文件含义：

| 文件 | 内容 |
| --- | --- |
| `live.log` | 给人看的实时结果，含 RUN / PASS / FAIL / SKIP 和 20 秒心跳 |
| `test.jsonl` | `go test -json` 原始事件，含网关调试日志 |
| `test.stderr` | 编译或启动错误 |
| `providers.yaml` | 供应商地址和模型，没有密钥 |
| `report.md` | 结束后的通过、失败、跳过计数和失败摘录 |
| `evidence/weighted-*.json` | 加权分流每次调用的部署、token 和金额，不含密钥 |

日志里如果出现 `sk-` 开头的长字符串，跟随程序会改成 `sk-***`。不要把 `test.jsonl` 提交进 git。`.e2e/` 已被忽略。

## 4. 这次实际覆盖什么

| 关注点 | 真实供应商 | 同一次命令里的确定性用例 |
| --- | --- | --- |
| 权限 | 预算链、密钥停用、过期、项目删除会打到 Fenno 的第一个模型 | `permission_test.go` 用真实网关和真实库验证跨组织、成员越权、轮换和匿名拒绝 |
| 计费 | 响应费用、五层累计、日志金额和费率快照一致 | 假上游核对精确金额和失败不计费 |
| 计量 | 真实 `usage` 的数量 × 价目表费率 = 金额；流式同样核对 | 假上游核对 OpenAI / Anthropic 两种用量形状 |
| 路由 | 模板按 key、团队、组织、平台切换后真实推理并入账；两个供应商按权重分流，零权重不到达 | 假上游核对策略顺序、重试次数和精确比例 |
| 回退 | 真实供应商不能按要求返回 500/429，所以不用它证明“失败了几次” | `fallback_test.go` 注入 500、429、冷却和禁用，核对换部署且成功只计一笔 |

同一供应商的 OpenAI 与 Anthropic 双协议比价，需要同一模型、同一地址的两套协议配置。当前入口只登记 OpenAI。没有这组配置时，`TestLiveSameVendorViaEitherProtocolIsBilledAlike` 会跳过，并在实时日志里写明原因。媒体任务创建需要额外的 `XHUB_REGRESSION_<ID>_BYPASS_MODEL`，未设置时对应用例跳过。

加权场景固定为 2 次请求：一次 1:1，一次七牛权重 1、Fenno 权重 0。它证明归属、零权重排除和五层入账。它不把 2 次随机结果当成严格的 50% 比例；精确比例由确定性用例负责。

## 5. 怎么读结果

- 退出码 0 且最后一行附近有 `包结果 PASS`：全套通过。跳过要打开看原因，跳过不是通过。
- 退出码非 0：先看 `live.log` 里的 `FAIL` 行和紧跟的断言。供应商 502、超时要和产品断言分开看。
- 跑到一半中断：`report.md` 不会把已经成功的调用算成整套通过。实时日志停在最后一条 `RUN` 或心跳上。

## 6. 清理

用例 schema 会自己删。独立库和 Redis 容器默认留着，方便看证据。

```bash
docker exec xhub-postgres psql -U xhub -d postgres -c 'DROP DATABASE xhub_regression_live;'
docker rm -f xhub-regression-live-log
```

不要删除别人正在使用的 `xhub-postgres` 容器，也不要 `DROP DATABASE xhub`。
