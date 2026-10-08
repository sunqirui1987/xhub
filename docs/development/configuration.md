# 配置字段、运行状态与数据存储

[功能实现](implementation.md) · [路由](routing.md) · [计价](pricing.md) · [接口](api-reference.md)

## 配置生命周期

配置分为启动 YAML、框架数据库覆盖、IAM 资源、请求级模板及 Redis 热状态。修改某一层不会自动改写其它层。启动配置由 internal/config/config.go 解析；平台读写由 gateway/prefs 完成；身份资源由 IAM 持久化。密钥、组织、团队选择模板存的是引用，模板正文另有一份完整文档。

| 数据 | 来源/持久化 | 生效位置 | 修改注意 |
| --- | --- | --- | --- |
| 数据库和监听等启动参数 | YAML / 环境引用 | 进程初始化 | 并非任意字段都有热加载 |
| 模型部署 | YAML + Store 数据库 | 模型发现与数据面候选 | 数据库覆盖 YAML；停用不等于删除 |
| 命名凭据 | Store credentials | llm 发送前解析 | 不返回原始秘密，不记日志 |
| general_settings | YAML + Store | 对应读取方 | 未实现字段仅保存，不承诺行为 |
| router_settings | 代码/YAML + Store | 平台默认与新模板播种 | DB 顶层明确值覆盖，不丢 null/0/false |
| RouteTemplate | IAM | 每次请求选一整份 | 创建播种一次，更新完整替换 |
| 组织/团队/key 模板选择 | IAM 资源引用 | key→team→org→platform | 空引用表示继承；项目/用户无独立层 |
| 密钥、成员、账号状态 | IAM 新鲜读取 | 认证/授权/限制 | 不用框架缓存替代身份读取 |
| 冷却/RPM/TPM/热费用/队列 | Redis | 运行时 | 可靠性依赖真实 Redis 和部署持久化 |
| 响应缓存/在途计数 | 进程内 | 当前实例 | 重启丢失，无跨实例失效保证 |
| usage 与价格快照 | IAM PostgreSQL | 历史账务/报表 | 不从当前配置重新赋予历史归属 |

os.environ/NAME 只在环境取值，不应把解析后的密钥写回文档。数据库 URL 只接受 PostgreSQL。测试使用 XHUB_TEST_DATABASE_URL，真实供应商测试使用 XHUB_REGRESSION_<ID>_*；测试环境变量不属于生产模型配置字段。

## ModelEntry 字段契约

model_name 是客户端模型名。litellm_params.model 是供应商模型标识，两者均必填。同一公开模型多个部署参与选路。params 包括上游 api_base、协议 custom_llm_provider、命名凭据 litellm_credential_name 和兼容内联连接字段。默认供应商地址只有在部署未覆盖时使用。

model_info.endpoint_types 表示入口能力；历史 mode 仍被识别，显式未知能力拒绝。model_info.disabled 仅布尔 true 表示暂停；字符串不是正式布尔状态。暂停部署不出现在可调用发现和实际路由池，不能由已有粘性恢复。

pricing_source 支持 catalog/manual。catalog 需要真实存在、有价格的 base_model。manual 需要至少一个明确费率/价格字段，非有限数和负价拒绝，明确 0 合法。rates 每个元素必须有效，不能只跳过坏行后保存剩下几行。支持 token/picture/second/query；窗口 all/offpeak/peak；输入输出、缓存读写及 batch 侧。实际使用维度取决于协议能提供的 usage。

pricing_id / deployment_id 和命名凭据参与运行状态隔离。没有稳定 ID 的相同端点/模型/凭据可能合并状态。模板权重仍按 api_base|model，目前不能分别覆盖同端点同模型的不同凭据份额。详见 [router](../../internal/router/readme_cn.md)。

## 模板执行字段

| 字段 | 类型 | 未提供时 | 当前执行语义 |
| --- | --- | --- | --- |
| routing_strategy | string | simple-shuffle | 同公开模型部署排序，不扩大权限 |
| num_retries | number | 1 | 每条部署总尝试次数，非正转 1 |
| timeout | number 秒 | 60 | 每次上游尝试时限，非正转 60 |
| allowed_fails | number | 3 | 非正关闭冷却失败累计 |
| cooldown_time | number 秒 | reader 返回 0 | 失败记录将非正转为 60 秒 |
| routing_strategy_args.weights | list/map | 部署原始权重 | 只 split 使用，0 排除，无效负数忽略 |

这些是 RouteSettings 对已选文档的读取默认；Config.Load 给平台默认预填重试 2，不应混淆。数字字符串不作为正式数字接受；零/false/null 在保存时不应被 UI 默认值吞掉。

权重列表形状：

```json
{"routing_strategy":"weighted-split","routing_strategy_args":{"weights":[{"api_base":"https://provider.example/v1","model":"upstream-model","weight":70}]}}
```

权重 map 形状：

```json
{"routing_strategy_args":{"weights":{"https://provider.example/v1|upstream-model":70}}}
```

列表未匹配的部署使用自身权重。创建模板补齐平台基线，后续平台修改不改变旧模板；更新接口完整替换正文，缺失字段由运行 reader 默认决定。不是每字段与父级合并。

## 保存但尚未完整执行的字段

fallbacks、context_window_fallbacks、content_policy_fallbacks、default_fallbacks、max_fallbacks、fallback_causes、retry_policy、model_group_retry_policy、routing_groups、model_group_alias、retry_after、stream_timeout、tag/pre-call 与 latency 高级窗口参数当前不能作为完整运行能力承诺。

回归可以证明字段往返不丢失；要证明执行还需观察上游序列、时限、结果或共享状态。现有同模型多部署回退与跨公开模型名回退有不同边界，后者未完整实现。

## 两种合并规则

OverlayDB 按顶层 key 把 DB 值覆盖 YAML，明确 null 和数组均保留。MergePatch 对 map 递归合并，对数组替换。这是平台设置更新机制。

Resolve 不是合并：按 key、team、organization 顺序找第一个已选模板，返回整文档。选中 ID 已不存在回平台；读取失败通过 gateway lookup 传播错误。控制台 binding/effective 和数据面必须同序。

## 持久化与历史边界

Store 保存框架配置、部署和通用 KV；IAM schema 保存账号、成员、key、模板、审计、usage/spend log/日汇总。不要把两层数据库接口名字相似理解成相同权限或缓存契约。

RecordUsage 的 request_id 是幂等身份。事件保存调用时 user/key/team/project/org、provider、模型、费率快照，后续身份迁移和目录修改不得改写。五个累计视图不是五笔独立费用。

Redis 队列先 Peek 再事务 commit 后 Ack，重复交付由数据库去重。响应头费用不代表已完成 durable commit。运行指标应发现落库失败、队列积压和未确认热支出；不能依赖成功 HTTP 响应推导账务完整。

## 配置变更验收

1. 读取当前字段与配置，保存明确值，再读取确认 0/false/null/空列表未改变。
2. 使用特定作用域调用，读取 effective 模板及实际模型/拨号顺序，不只观察表单。
3. 改模板后再次调用，确认动态字段不需重启生效且未选作用域不受影响。
4. 暂停模型验证发现和拨号均排除；改价验证新快照变化、历史快照保留。
5. 对读取故障、不可见模板和错误凭据验证无部分写入、无绕过、无秘密泄露。

对应测试在 prefs route_settings/weight、router template/disabled、gateway template_selection/retired_preferences、regression route_template_config/model/pricing。完整步骤见[回归方案](regression.md)。
