# 计量与限额

调用前预扣：max_budget、RPM、TPM（本次尝试的输入估计 + 输出上限）、max_parallel_requests。成功按 usage 结算，失败释放。

计价：usage × 价格表 × discount/margin。未知价格不得记 0。

SpendLogs 最小字段：`request_id`、`call_type`、`model`、`api_key`（哈希）、`prompt_tokens`、`completion_tokens`、`spend`、`startTime`、`endTime`、`cache_hit`。

热状态立刻可见；批量 flush 写入 SpendLogs / Daily 聚合（约 60s）。Daily 必须能从明细重建。
