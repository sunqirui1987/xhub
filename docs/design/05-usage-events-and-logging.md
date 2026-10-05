# 05 用量事件与日志

状态：已有 SpendLog 与可选 promptExchange；当前 recordSpend 主要按 prompt/completion token 记录成功调用，CallType 固定 chat，部分媒体响应填固定 8/2/10。证据：internal/gateway/spend.go、internal/gateway/family/handlers.go。待实施统一 RequestEvent、Attempt 和 Usage，不再把“无可计费用量”理解为“不需要日志”。

## 三种事实

请求事件记录谁在何时执行什么以及结果；用量记录实际资源数量和测量质量；结算记录按何价格收取何金额。失败请求可能有用量，成功请求可能暂时无用量，缓存请求可能无生成费但有缓存读取量，三者不能合成一个 spend 字段。

[请求事件](schemas/request-event.md) 保存身份/租户、模型/供应商、操作/部署、状态/错误、时间、缓存、用量引用和结算状态的不可变快照。[Usage](schemas/usage.md) 是多条带单位的计量项，连接到 request、attempt 或 task。供应商请求 ID 作为关联证据，不作全局唯一请求键。

## 操作与计量

| 操作 | 首选计量项 | 必需维度 |
| --- | --- | --- |
| chat / messages / responses | input_tokens、output_tokens | cache read/write、reasoning 的计费包含关系 |
| embeddings | input_tokens | embedding 模型与批量条数 |
| images | image_count | resolution、quality、输出数量 |
| speech | characters 或 audio_seconds | voice、音频规格及供应商计量口径 |
| transcription | audio_seconds | 输入时长、渠道或模型 |
| video | video_seconds | resolution、fps、audio_track、实际输出时长 |
| rerank | documents 或 input_tokens | 查询数、文档数及定价基数 |
| realtime | input/output text/audio tokens 或音频秒 | 通道、缓存、会话时段 |

以适配器已核验的协议口径选择单位，不在所有供应商之间猜测统一换算。供应商总 token 与明细可能有包含关系，不能再次把 reasoning/cache tokens 累加到已含它们的 total。用量项注明 inclusive_of 和计费规则；仅在已知相同单位且互斥时校验总量相等。

## 提取与质量

优先使用已验证上游响应/最终 usage 事件，其次使用供应商查询或账单对账。需要估算时记录 source=estimator、estimated=true、estimator_version 和误差假设；未知用 quantity=null、quality=unknown。固定占位数字、响应字符数、空 usage 都不能冒充实测 token。

流式解析器必须跨网络 chunk 增量处理事件：UTF-8、JSON 和 SSE 边界可以任意拆分；供应商可能只在最后一帧给 usage。重复终帧按事件标识/状态去重，增量与累计数明确区分，不能两次相加。末帧缺失即 unknown/partial，按价格策略等待对账或明确估算，不补零。

用量证据保存精简字段、供应商事件 ID、摘要与采集时间；原始敏感内容只有在受控留存策略启用后保存。适配器输出的负数、超界、单位冲突或不合法 JSON 应拒绝该测量并发出 usage_invalid，保留错误证据。

## 日志权限与留存

默认记录元数据，不记录明文 API key、Authorization、Cookie、签名 URL、完整 prompt 或输出。用户输入/上游异常文本做长度限制和转义，避免日志注入。key_id 与不可逆 key 指纹可关联，但不能供用户恢复原密钥。

prompt 存储独立开关和保留期；启用后加密、租户隔离、细粒度读取能力及读取审计。账务元数据和内容载荷有不同保留策略：删除 prompt 不删除合法保留的账本事实；实体删除后保留必要历史 ID 和快照，并按适用的留存要求脱敏。

查询支持 scope、时间半开区间、operation、model_alias、status、request_id、cache_status，数据库分页返回。默认不允许跨租户聚合，即使客户端隐藏了过滤参数也必须加服务端作用域谓词。

## 故障、迁移与验收

元数据持久化失败不能静默吞掉：终态/结算事务重试并留待恢复；外部已成功时返回策略需记录 delivery_outcome，不能假称上游失败后自动重新生成。可选内容日志失败可降级并告警，不阻断已确认的财务事实。

先保留旧 SpendLog 的只读视图，同时写新用量事实；历史字段缺失设 unknown / legacy，不用现价重算或固定数值补全。之后报表切新事实，禁用旧路径重复累计。

验收：所有结果有事件；媒体 unit 正确；一份缓存/推理输出不重复计算 usage；流末帧缺失不显示实测零；同名 key 及删除 key 不改变归属；日志查询跨租户失败；内容开关关闭时载荷不落库。参见 [报表方案](11-observability-and-reporting.md)。
