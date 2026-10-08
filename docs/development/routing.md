# 路由规则

[开发指南](README.md) · [客户使用与管理](../user-guide.zh-CN.md)

本文描述当前选择、执行和保存规则。

证据：`internal/iam/route_template.go`、`internal/gateway/identity/route_template.go`、`internal/gateway/templateauth/selection.go`、`internal/gateway/route_settings.go`、`internal/gateway/prefs/route_settings.go`、`internal/dataplane/serve.go`、`internal/dataplane/official.go`、`internal/router/router.go`。

## 选择与继承

一次请求使用最内层选中的模板：**密钥 > 团队 > 组织 > 平台**。账号和项目没有独立模板层。没有选择、或主动清空选择，表示继承父级。选中模板后整份替换，不逐字段与父级合并。

平台默认不是固定的 `route_templates` 行，而是 YAML、代码默认和数据库 `router_settings` 合成的全局文档。新模板由平台文档起步；已有模板不会随着平台默认的修改逐字段更新。

预算和模型权限仍沿身份链收窄。选择更宽松的路由模板不会扩大模型权限，也不会绕过预算。模板在预算链确定来源后按请求读取，不使用进程级模板缓存。模板读取失败返回 503，禁止静默改走默认上游。已选模板被并发删除时当前实现回到平台默认；管理 API 拒绝删除仍被选用的模板。

## 权限

平台模板对已认证调用方可见；组织模板对组织成员可见；团队模板对该团队及上级组织管理员可见。写模板需要对应范围的管理权限。选择范围的权限和读取模板的权限分别检查。

组织、团队、密钥普通保存、服务密钥创建、密钥批量更新和专用 binding API 都在修改前读取模板归属并检查可见性。仅知道另一团队的模板 ID 不能绑定；不可见或不存在返回 404，存储错误返回内部错误。清空选择恢复继承。

## 实际执行的设置

| 字段 | 当前行为 |
| --- | --- |
| `routing_strategy` | 同一个公开模型名下的部署排序；支持 simple-shuffle、least-busy、latency、cost、usage 与 weighted-split 等策略别名 |
| `num_retries` | **每条部署的总尝试次数**，包含第一次；缺省或小于 1 为 1。平台加载和新建表单可能预填 2 |
| `timeout` | 每次上游尝试的截止时间，缺省或不是正数为 60 秒；不是整个请求或整个部署池的总时限 |
| `allowed_fails` | 失败累计阈值，缺省 3；不大于 0 关闭冷却计数 |
| `cooldown_time` | 冷却秒数；不大于 0 使用一分钟 |
| `routing_strategy_args.weights` | 仅 weighted-split 使用；支持列表或映射，按 `api_base|model` 匹配 |

权重 0 在 split 策略下排除部署，负数和非有限数不作为候选。普通策略存在开放候选时排除冷却部署；全部冷却时仍会尝试，以避免全池不可用。split 没有开放且正权重候选时返回空池。simple-shuffle 当前偏向最高部署权重，不能理解为严格等概率轮询。

成本策略比较当前时段的每输入 token 费用，使用与结算相同的费率表优先级和高峰/非高峰规则；显式零价可以参与比较，缺失或非法价格排在后面。它不是对本次输出 token、缓存命中和非 token 消耗的完整总费用预测。

忙碌、延迟、用量、冷却和粘性使用 `CooldownID`，命名凭据不同的部署可以区分。配置提供 `pricing_id` 或 `deployment_id` 时进一步区分相同端点、模型和凭据名的部署；没有稳定 ID 的重复配置仍可能合并。权重覆盖继续按物理端点和模型匹配，因此目前无法为同端点的不同凭据单独填写覆盖权重。

## 请求和重试

普通数据面在排序前过滤暂停和不匹配的端点能力，粘性只把**已经入选**的候选移到首位，不能恢复禁用、零权重或被过滤的部署。每次重试前重新核验身份及限制。429、5xx 和部分传输失败会尝试同一模型的下一次或另一部署；其它 4xx 不自动更换部署。

流式响应已经向客户端写出字节后中断，记录失败并停止，禁止再次发送另一个响应，也不写入成功粘性或响应缓存。客户端可能已经看到 HTTP 200 和部分正文；日志的失败状态反映流中断，不能事后修改已发出的 HTTP 头。

官方任务创建和查询也读取所选模板。创建遇到 429/5xx 可以换候选；发送或读取结果不明时返回 502，不盲目重建任务。查询使用调用方和传输范围内的任务钉；不会把另一用户的同名任务转发出去。官方 bypass 当前没有普通聊天护栏或响应缓存。

## 仅保存的高级字段

`fallbacks`、`context_window_fallbacks`、`content_policy_fallbacks`、`default_fallbacks`、`max_fallbacks`、`fallback_causes`、`retry_policy`、`model_group_retry_policy`、`routing_groups`、`model_group_alias`、`retry_after`、`stream_timeout`、tag/pre-call 开关以及 latency 的高级窗口参数目前不构成完整的数据面执行能力。

同名模型部署失败后换另一部署已经实现；跨公开模型名回退尚未实现。编辑器保留高级字段以便往返，不代表保存后已生效。前端相关提示必须注明“仅保存”。

## 缓存与粘性

完整响应缓存与供应商 prompt cache 是不同机制。前者命中不再次收费；后者仍调用供应商，按返回的缓存 token 计费。响应键包括调用方、操作、公开模型、正文，以及部署配置、路由文档、会话和查询串的摘要。修改暂停状态或价格配置会改变键。缓存没有 TTL 或跨进程失效保证，不能证明所有业务结果始终新鲜。

会话钉和 previous_response_id 钉以模型、调用方和标识的 JSON 元组计算 SHA256，避免字段分隔碰撞与跨租户复用。只在成功完成后提交，TTL 一小时。任务钉另有七天期限；Redis 故障时的本地回退在进程重启后丢失。粘性保证优先尝试原部署，不保证供应商侧一定命中 prompt cache。

## 控制台

创建密钥的可选配置保持预算、重置预算、路由模板和 Lifecycle。路由策略在模板页集中编辑。选择器是受控值：异步加载名称不能覆盖用户草稿；清空选择表示继承；只读状态不能修改。

验证用例包括 `template_selection_regression_test.go`、`route_settings_test.go`、`weight_regression_test.go`、`template_regression_test.go`、`official_template_test.go`、`stream_failure_regression_test.go`、`cache_scope_test.go` 及前端 RouteTemplateSelect/templateForm/routeTemplatePayload 测试。缓存、幂等及协议边界见[运行行为与限制](runtime.md)。
