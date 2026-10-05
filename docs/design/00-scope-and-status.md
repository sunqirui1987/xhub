# 00 范围与现状

状态：审查结论与待实施设计。审查对象是 2026-10-02 的当前工作区，包含已有未提交改动；没有以干净主分支替换用户代码。结论描述可观察的代码行为，未运行真实付费供应商调用。

## 范围与交付边界

覆盖 Go 网关、身份和模型权限、数据平面、Redis/数据库支出链路、报表、扩展以及相关前端契约。优先回答“用户能不能调用”“请求有没有被记录”“钱有没有算对”“新协议能不能安全加入”。本次重建 docs 和索引引用，不交付上述业务修复。

之前提出的 SaaS 登录布局、我的/管理员导航、模型选择器需求仍是产品验收条件。当前工作区已有相关改动，不能仅凭截图或组件文件认定已完成。登录表单不应展示默认管理员密码来源、MASTER_KEY 或环境配置提示；错误反馈和实际 SSO 入口仍需保留。

## 证据与风险

| 编号 / 优先级 | 代码证据 | 当前判断 | 目标 |
| --- | --- | --- | --- |
| F01 / P0 | internal/gateway/models/access.go：membershipScopes、entitiesForIDs、effectiveScope | 部分实体读取错误被忽略，限制可能缺失；无 DB 时返回不受限 | 带错误的统一权限决策，关键读失败拒绝 |
| F02 / P0 | internal/gateway/session.go：lookupSession | 签名 JWT 验证成功即可形成会话，不强制检查服务端记录；内存会话也需撤销失效 | 服务端撤销记录与权限版本 |
| F03 / P0 | internal/gateway/limits.go：enforceIdentityLimits、withCredential、enforceRedisRateLimits；internal/live/redis.go：HotSpend | 密钥刷新失败保留旧值，凭据读错不传播，限流失败放行，热支出错误趋向零 | 无法确认身份、预算、凭据或强制限流时拒绝 |
| F04 / P0 | internal/gateway/spend.go：recordSpend、persistSpend | 多处写错被忽略，非 Redis 分别更新实体及日志；缺少统一事务 | 账本、终态、聚合一致提交 |
| F05 / P0 | internal/store/spend_batch.go：ApplySpendBatch | seen 只包含入库前已有 ID，循环未标记本批新 ID；同批重复可能重复累计或导致整批失败 | 唯一约束和事务冲突处理，不依赖进程 mutex |
| F06 / P0 | internal/gateway/spend.go：writeCacheHit | 原始 usage 再走普通 Cost 与 recordSpend，可重复收生成费 | 独立缓存命中用量及费用 |
| F07 / P1 | internal/gateway/models/available.go：Available | 有权限过滤、阻断与非模型过滤，但未检查适配器/凭据/有效部署/健康 | 统一 AvailabilityService |
| F08 / P1 | internal/gateway/spend.go：CallType 固定 chat；writeChatJSON 的 audio_speech 固定 8/2/10 | 操作类型与真实媒体用量不一致 | 按操作提取并标明未知或估算 |
| F09 / P1 | internal/gateway/usage/activity.go：loadActivity、selectActivity | 用当前 key 补历史归属，provider 从当前价格表推导；数据库错误被忽略 | 不可变快照、显式查询错误、数据库分页聚合 |
| F10 / P1 | internal/llm/build.go：Upstream；internal/dataplane/serve.go：HTTP POST | 可建 URL/header/body，但不足以表达通用方法和传输生命周期 | 协议与传输分离 |
| F11 / P1 | internal/plugin/registry.go：Extension | 只有 BeforeUpstream，无 context/超时/响应及结算钩子 | 受约束的版本化生命周期 |
| F12 / P1 | internal/gateway/ingress.go：realtimeContract、passthroughContract；internal/gateway/family/handlers.go：固定 usage、count_tokens | 部分处理只是契约占位；WebSocket 升级即关闭、passthrough 只计算地址 | 默认禁用占位，生产能力独立验收 |
| F13 / P1 | internal/catalog/publicdata/model_cost_map.json：seedance2 系列条目 | 有价格资料不能证明 Seedance 2.0 任务协议已经接入 | 核验供应商文档并跑异步闭环 |

P0 表示可能破坏权限或账务一致性，应阻断生产开放；P1 表示能力、统计或协议语义不完整，应限定功能范围。这里是设计风险分级，不声称每个风险都已在生产中发生。

## 现状分层

已存在：Go 路由与数据平面、模型配置、身份数据、凭据补全、JSON 构造、多种 provider 分组、缓存、Redis 热支出/日志队列、数据库支出报表、前端我的模型页面与扩展注册器。

部分实现：当前账号模型过滤、session 到推理身份转换、预算/限流、多模型重试、流式响应记录、用量报表、历史日志归属。

本方案提出、尚未实现：能力级授权、可撤销会话统一协议、不可变请求/尝试/结算事件、跨预算主体原子预占、统一多维 usage、通用传输、完整异步媒体状态机及 Seedance 2.0 验收。

## 可复现的诊断边界

用户报告 /model/available 加载失败和 builtin.go 的 bodyCredential 编译错误。历史错误提示不能证明当前错误原因。处理时应先构建当前代码，再以当前有效会话请求接口，记录 HTTP 状态、错误码和 request_id，分别判断会话解析、AllowLLM、授权依赖、路由/CORS和部署配置。不得在文档中保存用户粘贴的 session token，也不得通过换成主密钥“修复”普通用户权限。

## 验收与风险

每个风险必须在 [实施计划](12-migration-plan.md) 中有阶段，在 [测试计划](13-test-and-acceptance-plan.md) 中有场景。旧路由基线只验证表面积，不提供业务正确性保证。任何缺少真实响应、故障恢复或结算证据的协议，最高只能标记为实验。
