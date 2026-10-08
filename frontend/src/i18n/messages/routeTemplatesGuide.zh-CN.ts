export const routeTemplatesGuideZhCN = {
  tab: "配置说明",
  title: "路由模板 JSON 完整说明",
  intro:
    "一份模板就是一个完整的路由设置对象。普通配置可以直接用前面的表单；需要批量导入或使用高级字段时，再到 JSON 页编辑。",
  ruleTitle: "模板整份生效",
  ruleBody:
    "组织、团队或密钥选中模板后，使用的是这一整份文档，不会和上一级模板按字段合并。新建模板会从平台默认复制，以免漏掉关键值。",
  currentStatusTitle: "先确认当前网关真正支持哪些字段",
  currentStatusBody:
    "当前请求路径已经执行路由策略、weighted-split、num_retries、timeout、allowed_fails 和 cooldown_time。回退链、按错误类型重试、路由组、模型别名、调用前检查、标签过滤、流式超时和重试等待目前只会保存并原样导出，尚未参与实际请求。下面仍完整说明它们的 JSON 格式，便于配置迁移和后续启用。",
  flowTitle: "一次请求如何执行",
  flowHint: "理解这四步，就能区分负载均衡、重试、冷静期和跨模型回退。",
  flow1Title: "选择当前模型的部署",
  flow1Body:
    "routing_strategy 在同一个公开模型名下选择一条部署。weighted-split 用 weights 分流；路由组可以为一组模型单独指定策略。",
  flow2Title: "在当前部署上重试",
  flow2Body:
    "当前由 num_retries 控制同一条部署的尝试次数，timeout 是每次上游调用的时限。retry_policy 和 retry_after 是兼容配置，当前请求路径尚未执行。",
  flow3Title: "标记失败部署",
  flow3Body: "一条部署累计达到 allowed_fails 后进入 cooldown_time 冷静期，在此期间路由器不会再选择它。",
  flow4Title: "换到回退模型",
  flow4Body:
    "设计顺序是在当前模型的部署和重试都耗尽后读取对应回退链，每换一个模型计数一次。当前网关尚未执行这一步，配置会被完整保存。",
  fallbackRecipes: "回退配置范例",
  normalFallbackTitle: "普通错误：按顺序换模型",
  normalFallbackBody:
    "目标行为是连接失败、429、5xx 和允许参与回退的超时，先从 gpt-main 换到 claude-main，再换到 gemini-main；当前版本仅保存此配置。",
  contextFallbackTitle: "上下文超限：使用长上下文模型",
  contextFallbackBody:
    "目标行为是开启调用前检查后，上下文超限只走 context_window_fallbacks，不落入普通 fallbacks；当前版本仅保存此配置。",
  safeFallbackTitle: "避免超时后的重复请求",
  safeFallbackBody: "回退功能启用后，移除 ambiguous 可阻止超时或请求发出后的断线自动换模型，从而降低重复请求风险。",
  referenceTitle: "字段参考",
  referenceHint: "默认值指模板未明确填写时的基线行为；平台默认表示取当前部署的平台配置。",
  field: "字段",
  type: "类型",
  defaultValue: "默认",
  support: "当前状态",
  active: "已生效",
  stored: "仅保存",
  purpose: "作用与注意事项",
  example: "示例",
  platformDefault: "平台默认",
  balanceFields: "负载均衡与路由组",
  balanceFieldsHint: "决定请求先落到哪个部署，尚未跨模型回退。",
  retryFields: "重试与超时",
  retryFieldsHint: "控制当前部署内部的再次尝试，发生在跨模型回退之前。",
  fallbackFields: "跨模型回退",
  fallbackFieldsHint: "当前模型无法完成请求时，按失败类型选择另一个模型公开名。",
  healthFields: "部署健康与名称映射",
  healthFieldsHint: "控制故障部署何时暂停参与路由，以及调用名称如何映射。",
  fieldRoutingStrategy:
    "同名部署之间的选择策略。可用值包括 simple-shuffle、least-busy、latency-based-routing、cost-based-routing、usage-based-routing-v2 和 weighted-split。",
  fieldRoutingStrategyArgs:
    "当前请求路径会读取 weighted-split 的 weights；其它策略参数会保存，但当前请求路径尚未读取。",
  fieldRoutingGroups:
    "命名路由组列表。每项包含 group_name、models、routing_strategy，可选 routing_strategy_args；建议在“路由组”页签维护。",
  fieldTagFiltering: "计划用于只选择标签与请求匹配的部署；当前版本仅保存。",
  fieldNumRetries: "当前部署的总尝试次数。当前网关把 1 视为尝试一次，并已执行此字段。",
  fieldRetryPolicy:
    "按错误类型设置重试次数的兼容对象，可填写 BadRequestErrorRetries、AuthenticationErrorRetries、TimeoutErrorRetries、RateLimitErrorRetries、ContentPolicyViolationErrorRetries、InternalServerErrorRetries；当前版本仅保存。",
  fieldGroupRetryPolicy:
    "按模型公开名覆盖 retry_policy。第一层键是模型名，第二层仍是错误类型和次数。当前没有专用表单，可在“其它字段”或 JSON 页设置。",
  fieldRetryAfter: "计划用于控制失败后再次尝试前的最短等待秒数；当前版本仅保存。",
  fieldTimeout: "单次上游调用的超时秒数。每次重试都会重新开始一次调用。",
  fieldStreamTimeout: "计划用于单独控制流式响应的超时秒数；当前版本仅保存，实际仍使用 timeout。",
  fieldFallbacks:
    "普通回退链。数组中每个对象的键是主模型，值是按顺序尝试的模型名数组；当前版本仅保存，尚不会切换模型。",
  fieldContextFallbacks: "上下文窗口超限的专用链。它不和普通 fallbacks 串联；通常同时开启 enable_pre_call_checks。",
  fieldContentFallbacks: "内容策略拒绝的专用链。它不和普通 fallbacks 串联。",
  fieldDefaultFallbacks: "当前模型在普通 fallbacks 中没有专属链时使用的通用模型列表，按数组顺序尝试。",
  fieldFallbackCauses:
    "允许触发普通回退的失败类别：no_response 表示未收到响应，status 表示 429/5xx，ambiguous 表示超时或无法确认上游是否执行。移除 ambiguous 可降低重复请求风险。",
  fieldMaxFallbacks: "一次请求最多跨到多少个其它模型。每换一个模型计数一次。",
  fieldPreCallChecks: "在发请求前检查上下文窗口；上下文专用回退依赖此检查。",
  fieldAllowedFails: "一条部署累计多少次失败后进入冷静期。小于 1 会关闭失败记录和冷静机制。",
  fieldCooldown: "部署进入冷静期后暂停参与路由的秒数。0 按当前约定表示 60 秒。",
  fieldAlias: "计划把调用方使用的别名映射到已有模型公开名；当前版本仅保存。键是别名，值是目标模型名。",
  fullExample: "完整 JSON 示例",
  fullExampleHint:
    "示例同时展示了负载均衡、路由组、重试、三种回退、冷静期和别名。替换其中的模型名与地址后，可粘贴到 JSON 页继续编辑。",
  copy: "复制示例",
  copied: "已复制",
} as const;
