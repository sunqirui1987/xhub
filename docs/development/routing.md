# 路由模板与统一执行架构

每份路由模板包含负载均衡、路由组、故障转移和可靠性参数。表单和 JSON 双向同步，非法 JSON 阻止保存和切回表单。新模板没有模板默认策略开关；未覆盖的模型使用模型管理中的默认端点权重及回退。

## 模型规则的独立权重

model_routes 每条包含 model、strategy 和可选 allocations。模型名唯一、非空；strategy 使用支持的策略枚举。

| 配置 | 权重语义 |
| --- | --- |
| 省略 allocations | traffic-split 实时继承模型管理默认权重 |
| allocations 为 [] | 当前模板所有兼容部署使用权重 1 |
| allocations 有明细 | 当前模板独立权重；未列出的部署权重 1，显式 0 排除 |

allocations 每项包含 deployment_id 和数值 weight。ID 必须属于该公开模型且唯一；权重有限非负，非空列表至少一个正值，总和必须有限。不要求合计 100。其他策略不能带 allocations。表单显示提供商、上游型号、公开模型、协议与稳定 ID；不展示凭据。

## 完整正文示例

以下是 JSON 正文；名称在管理接口另行提交。模型名和部署 ID 应换成真实目录值。

    {
      "model_routes": [{
        "model": "chat-main", "strategy": "traffic-split",
        "allocations": [
          {"deployment_id": "deployment-a", "weight": 3},
          {"deployment_id": "deployment-b", "weight": 7}
        ]
      }],
      "retry_policy": {
        "max_attempts": 2, "timeout_seconds": 60,
        "failure_threshold": 3, "cooldown_seconds": 60
      },
      "routing_groups": [{
        "group_name": "chat-pool", "models": ["chat-main", "chat-fast"],
        "routing_strategy": "cost-based-routing"
      }],
      "fallbacks": [{"chat-pool": ["chat-backup"]}],
      "context_window_fallbacks": [{"chat-main": ["chat-long"]}],
      "content_policy_fallbacks": []
    }

model_routes、retry_policy 必填，可靠性四字段必须完整。max_attempts 是正整数，每部署总尝试次数包含首次；timeout_seconds 为正数，单次调用超时。failure_threshold 为非负整数，0 禁用被动冷却；cooldown_seconds 为非负数，0 使用 60 秒。未知字段、字符串数值、重复模型和循环回退均拒绝。

## 策略与绑定

| 策略 | 实际依据 |
| --- | --- |
| simple-shuffle / random | 均匀随机，忽略权重 |
| traffic-split | 独立配置或继承的相对权重 |
| least-busy | 观测并发 |
| latency-based-routing | 观测延迟 |
| cost-based-routing | 实际输入 token 单价，不估算请求总价 |
| usage-based-routing | 统计 token 用量，不是 RPM/TPM 剩余额度 |

身份选模板：个人 API 密钥 → 团队 → 组织 → 内置默认。第一份有效绑定整份选用，不混合多层模板。账号和项目没有独立模板层。模型规则优先于成员组策略；组名调用展开组成员部署。旧 routing_strategy 仅为已有文档兼容读取，没有新建表单控件；旧模板缺少 routing_groups 时兼容历史全局组，新模板显式 [] 与历史组隔离。

有效会话固定部署优先，但必须兼容、可用且非零权重。模型权限与预算沿身份链独立检查；组名及实际成员都检查权限和预算，实际部署用于计费。模板不能扩大调用权限。

## 故障转移

fallbacks、context_window_fallbacks、content_policy_fallbacks 分别配置通用错误、上下文超限、内容策略错误链。每项是一条主模型到有序目标数组的映射，源与目标可为公开模型或当前模板组。禁止重复、自引用和跨类别循环；与模型默认合并后再次判环。

空映射表 [] 继承模型默认回退；[{"chat-main":[]}] 明确禁用该源对应类别的链；有值时覆盖对应源与类别。部署池耗尽后进入相应回退链；未覆盖源和类别继承模型管理默认配置。本地护栏拦截不作为供应商内容策略回退。流式内容发出后不重新开始响应；异步任务创建不能在发送结果不明确时盲目重复。

## 后端职责

| 边界 | 文件与职责 |
| --- | --- |
| 统一配置解析 | internal/router/settings.go：模板契约、模型策略、身份继承 |
| 统一编译 | internal/router/compile.go：模板与模型默认权重/回退合成请求快照 |
| 组与回退校验 | internal/router/template.go、groups.go、fallback.go：真实引用、权重归属和环检查 |
| 调度算法 | internal/router/policy.go、schedule.go：相对权重、候选排序 |
| 网关适配 | internal/gateway/template_routing.go：锁内复制模型目录，锁外校验；identity 保存及绑定 |
| 数据面执行 | internal/dataplane/fallback.go：请求私有回退队列和当前源；serve/official 执行转发 |

保存、预览和真实请求共用编译器；接口负责鉴权与持久化，解析器不发 HTTP 或写存储。prefs/route_settings.go 仅保留类型及调用兼容。说明性中文注释随实现维护。

## 验证边界

回归与 E2E 使用隔离 PostgreSQL schema、真实网关、真实浏览器及本地协议上游，验证独立权重、组与回退、身份绑定、账单、非法保存与数据清理。不依赖付费供应商凭据；真实供应商限流/故障、跨实例 Redis 观测与生产大流量分布仍需部署环境验证。

参考 LiteLLM 的 routing、load_balancing、reliability，以及本地 RoutingGroupModal、FallbackSelectionForm 交互。当前 JSON 是 xhub 契约，不能直接导入 LiteLLM router_settings 或 YAML；num_retries 与 max_attempts 计数不同。
