# 计量

## 预扣再结算

调用前（生命周期第 4 步）按主体预扣：Key / User / Team / Organization / End User / tag：

- `max_budget`（USD float）
- `rpm_limit` / `tpm_limit`（TPM = 本次输入估计 + 输出上限，**不要**把所有 fallback 部署的上下文窗口加总扣一次）
- `max_parallel_requests`

成功：按实际 `usage` 结算。失败或 4xx 未打上游：释放预扣。超限：**429** + `Retry-After`，信封 `budget_exceeded` 或限流 type。

## 计价

`usage` × 价格表 × `cost_discount_config` / `cost_margin_config`。分项写入：

- `x-litellm-response-cost`
- `x-litellm-response-cost-original`
- `-input` / `-output` / `-cache-read` / `-cache-creation` / `-reasoning` / `-tool-usage`

**未知价格不得记 0。** 缺 usage 或价格表行：记 pending / 拒绝对账配置项，不能当成功成本 0 写入 SpendLogs，否则 Usage 图会系统性偏低。

## SpendLogs

每请求一行，最小字段：`request_id`、`call_type`、`model`、`api_key`（哈希，不是明文）、`prompt_tokens`、`completion_tokens`、`spend`、`startTime`、`endTime`、`cache_hit`。可选 `team_id`、`metadata`、guardrail。

热状态（Redis）立刻可见，避免超卖。批量 flush 约 60s 写入 SpendLogs 与 Daily* 聚合。Daily 必须能从明细重建。控制台 Usage 读聚合；Logs 读明细。
