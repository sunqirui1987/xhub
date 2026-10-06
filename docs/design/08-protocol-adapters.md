# 08 协议适配器与传输

> **目标设计，不是当前行为。** 本文描述的是计划要实现的架构。现在实际运行的行为见 [docs/current/](../current/)。两份冲突时，以 `current/` 和代码为准。

状态：已有多供应商请求构造，通用协议生命周期尚未实现。internal/llm/build.go 的 Upstream 只有 URL、Header、Body；internal/dataplane/serve.go 使用固定 HTTP POST。internal/gateway/ingress.go 的 realtimeContract 升级后关闭连接，passthroughContract 只生成计划；internal/gateway/family/handlers.go 部分返回固定 usage。这些路径不能仅凭路由存在标记为生产支持。

## 目标与边界

将公共 API、供应商协议、传输和业务生命周期分开。公共 API 负责参数校验与权限；适配器负责供应商认证形式、请求转换、错误映射和用量提取；传输执行器负责发送、流处理、连接资源和取消；RequestService、TaskService、BillingService 负责持久化与账务。适配器不得自行扣款、跳过预占或改写身份。

新增协议不是新增一个 provider 名字和 URL。支持矩阵的粒度是 adapter_version × provider × operation × transport，能力含参数范围、用量口径、幂等、取消与恢复。状态分为 disabled / experimental / verified；只有 verified 项可发布为用户可用能力。历史兼容路由和占位处理默认 disabled。

## 建议接口

以下是目标接口草案，类型需在实施阶段定义，并非当前代码：

```go
type ProtocolAdapter interface {
    Name() string
    Supports(op Operation) bool
    BuildRequest(context.Context, Request) (UpstreamRequest, error)
    DecodeResponse(context.Context, UpstreamResponse) (Response, error)
    ExtractUsage(UpstreamResponse) (Usage, error)
}
```

Supports 只是静态能力，AvailabilityService 还需检查配置、发布、授权及部署状态。注册元数据提供版本、transport、参数 schema、凭据 schema、usage schema、重试/副作用策略与供应商文档版本。返回的不支持错误是 operation_not_supported，不能静默降级为 chat。

UpstreamRequest 显式包含 method、URL、headers、content_type、可关闭 body reader、内容长度上限、超时、稳定 execution key 和可重放性。流体、文件或 multipart body 默认不可重放，重试需工厂重新打开并校验摘要。凭据由受控解析器注入，不进入用户响应或事件快照。

UpstreamResponse 保存状态、受控响应头、body/事件读取器、供应商请求 ID；DecodeResponse 产出统一业务结果及 delivery 状态。ExtractUsage 必须支持最终快照或增量聚合，未取得完整证据返回 unknown/partial，规则见 [用量契约](schemas/usage.md)。

## 传输支持矩阵

| transport | 必须解决的问题 | 验收证据 |
| --- | --- | --- |
| json | 方法、认证、参数、错误 envelope、大小上限 | 成功和错误真实样本、usage 对照 |
| sse | 任意 chunk 分割、末帧、心跳、背压、断开 | 流中断与缺失 usage 测试 |
| multipart | 文件字段、MIME、上传大小、资源关闭 | 文件和参数一起传递、失败不泄漏句柄 |
| binary | 媒体类型、内容长度、下载权限、部分写出 | 二进制不经 JSON 重新编码 |
| websocket | 双向事件、握手、心跳、时段用量、关闭码 | 真正双向会话和断线结算 |
| async_task | Submit/Get/Cancel、回调、任务状态与输出 | [异步闭环](09-async-media-and-seedance.md) |

SSE 和 WebSocket 使用增量事件解码器；不能一次读完无限流。传输设置最大消息/帧长度、并发与缓冲上限，慢客户端触发明确取消。TTFT 与连接成功分开测量。HTTP 发出成功不证明业务成功；所有错误最终映射到 request/attempt/task 事实。

## 故障与保证

执行器不能自行决定付费重试，必须将发送阶段与可确定的副作用信息交给生命周期服务。只有确认未执行或供应商支持同键幂等才重试；未知结果进入对账。跨供应商自动降级只能在同一授权能力范围内，并有累计费用上界。

外部 URL 来自受控部署配置；用户资产 URL 和输出 URL 按独立规则验证，限制协议、重定向、内网地址及下载规模。允许自建内网部署通过明确管理员策略配置，不能让用户输入绕过地址策略。

最坏情况：供应商无幂等且发送结果丢失时，本地适配器无法证明外部至多执行一次；系统可保证禁止盲目重发、保留未知状态和本地至多一次结算。增加 URL 模板无法消除这个信息下界。

## 迁移与验收

先把现有 Build 包装成 json/sse 适配器，在固定样本上保持已验证行为；再显式扩展 method 和 body；按能力逐一迁移 multipart、binary、WebSocket。迁移前核对真实契约，占位响应不能作为兼容基线。异步协议由 TaskService 调用专用接口。旧路由保留与否由支持矩阵决定。

验收：不支持操作无上游发送；错误/超时/取消有事件；内容与用量不混淆；每种 transport 有契约和故障测试；新版本通过参数、认证、用量、预算及恢复测试后才能发布。风险是将旧协议误标 verified，因此发布需要可追溯证据。
