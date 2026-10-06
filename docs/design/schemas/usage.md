# 多维用量数据契约

> **目标设计，不是当前行为。** 见 [docs/current/](../../current/)。

状态：Proposed。schema_version=1。用量描述资源测量，不直接代表收费或请求成功；供应商成本和用户收费在结算中分别计算。

## 字段

| 字段 | 类型与要求 | 语义 |
| --- | --- | --- |
| usage_id / schema_version | ID / 整数，必填 | 不可变用量记录及结构版本 |
| execution_id / request_id | ID / ID，可空 | 对应执行和采集入口；预检查拒绝可以没有用量 |
| attempt_id / task_id | ID，可空 | 上游成本归属和异步工作关联 |
| measurement_key / version | 稳定字符串 / 单调整数 | 同一测量事实的不同完整修订版本 |
| scope_snapshot_id / model_snapshot_id | ID，必填 | 引用提交时归属和模型快照 |
| operation / provider / adapter_version | 字符串，必填 | 按实际执行确定，不固定为 chat |
| quality | 枚举 | measured、estimated、partial、unknown、invalid |
| source | 枚举 | provider_response、provider_query、provider_invoice、estimator、cache、legacy |
| source_event_id / evidence_digest | 字符串可空 / 摘要 | 供应商事件关联与去敏证据 |
| observed_at / recorded_at | UTC 时间 | 测量与落库时间 |
| estimator_version / assumptions | 字符串 / 受控结构，可空 | estimated 时必填估算方法、界限与假设 |
| supersedes_usage_id | ID，可空 | 新版完整测量替代的旧版本；不删除旧事实 |
| items | 计量项数组，必填 | 所有 quantity 都保留单位和维度 |

计量项字段：metric、quantity、unit、dimensions、semantics、inclusive_of、billable_rule_id。quantity 为非负精确十进制字符串或 null，禁止 NaN/Infinity；数量是否整数由 unit 约束。semantics=total 表示当前完整累计值，semantics=delta 表示有独立去重标识的增量；不能对重复 total 帧再次累加。

dimensions 用版本化白名单，包括 resolution、quality、fps、audio_track、voice、channel、cache_type 等；影响报价的维度必填。不将用户任意 JSON 原样作为维度，避免敏感泄漏和无限指标基数。inclusive_of 标明总量包含的子项；相交/包含的 metric 不允许盲目求和。

## 单位和缺失

| 操作 | 常用 metric / unit | 解释约束 |
| --- | --- | --- |
| chat、messages、responses | input/output/reasoning/cache_read_tokens / token | reasoning 和缓存可能是总 token 的子集 |
| embeddings | input_tokens / token | batch_count 独立，不能与 token 相加 |
| images | image_count / image | 分辨率和质量参与报价 |
| speech | characters / character 或 audio_seconds / second | 按实际供应商口径选择，不能猜换算 |
| transcription、video | audio/video_seconds / second | 测得时长、音轨、规格和输出数量 |
| rerank | document_count / document 或 input_tokens / token | 保留实际报价基数 |
| realtime | 分通道 tokens 或 audio_seconds | 文本/音频/缓存包含关系显式声明 |
| cache hit | cache_read_count / request 或读取字节 / byte | 无新上游生成用量；可引用原生成来源 |

0 必须有已证实零消耗的证据；未知为 null 和 quality=unknown，部分计量为 partial，错误测量为 invalid。缺失 usage、SSE 末帧、价格或时长不补固定 8/2/10。质量和 source 独立：供应商查询也可能只给 partial。

## 唯一约束和修订

usage_id 为主键；(measurement_key, version) 唯一；measurement_key 在 execution 内区分 attempt/task、计量窗口与证据类别，不能用 provider_request_id 单独充当全局键。有供应商事件 ID 时按 provider/account/外部对象/event_id 加去重约束；无 ID 时用适配器已验证的稳定测量键与摘要去重。相同唯一键不同内容视为冲突并告警。

实时增量保存为不可变去重事件，最终生成 total 版本并声明覆盖窗口；消费端只按指定规范化版本结算，不能把所有增量加上 final total。报表选用已接受的最新完整版本或明确 partial 投影，不累计 superseded 全量。账本已结算后新版用量只触发差额调整。

## 敏感字段、一致性和验收

证据默认只保留计量字段、事件标识与摘要，不保存输出正文、提示词、原始签名 URL 或凭据。只有受控留存开启时保存加密证据载荷。用户读取按提交时归属及当前读取能力授权。

数量单位不一致时拒绝金额计算并进入待核查；媒体操作由其适配器提取，不能用聊天 token 类型强制包装。估算收费必须有明确政策版本；估算不改变 quality 为 measured。验收覆盖任意 SSE chunk 拆分、重复累计帧、末帧丢失、供应商修订、计量包含关系和精确小数。

参见 [用量事件](../05-usage-events-and-logging.md)、[结算](settlement.md)。
