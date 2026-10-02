# 03 模型目录与可用模型

状态：/model/available 已解析身份、检查 AllowLLM、过滤 nonModelEntry / blocked、调用 AllowsModel 并按别名去重；还未证明有效部署、协议、凭据和健康条件。证据：internal/gateway/models/available.go、access.go、builtin.go。当前 capabilities 部分来自静态价格资料，不能作为实际协议能力证明。

## 三类数据

| 数据 | 用途 | 谁看 | 是否证明可以调用 |
| --- | --- | --- | --- |
| 模型广场 / catalog | 介绍已发布模型、规格和价格 | 按目录读取策略 | 否 |
| 我的可用模型 / available | 当前账号当前作用域可选择的模型与操作 | 登录用户 | 证明快照时符合本地调用资格 |
| 部署与供应商配置 | 上游地址、模型映射、凭据、路由 | 管理员 | 用于路由，不能直接展示给用户 |

qiniu、fennoai 是供应商/凭据来源，不能因为被加入配置表或成本表就显示为模型。禁止只按两个名称硬编码排除：领域数据需有 kind=model / provider / credential，模型记录必须关联真实 upstream_model 和 deployment。旧数据以类型及配置结构归类，歧义项隔离供管理员修正；供应商名前缀下的真实模型可以存在。

## 可用性定义

对主体 p、作用域 s、别名 m、操作 o：

available(p,s,m,o) = authorized(p,s,m,o) AND published(m,s) AND 存在部署 d，满足 d 属于 m、未阻断、路由启用、adapter.Supports(o)、操作所需配置有效、credential 可解析且允许该作用域使用、健康策略允许选择。

凭据“有效”在本地指引用存在、未撤销、未过期、必需字段齐全；不能保证供应商下一次一定接受。支持匿名访问的适配器不强制 API key。健康状态用 healthy / degraded / unhealthy / unknown，默认排除 unhealthy；unknown 是否可探测由版本化路由策略决定，不能默认全部可用。

价格不可确认的付费操作不进入可调用集合；明确零价用 price_state=free，未知用 unknown。预算余额、瞬时限流和并发是调用时条件，列表不锁定余额；页面只展示可安全公开的账户限制提示，不能承诺列表返回后调用必定成功。

## 统一服务与 API

AvailabilityService 同时供 /model/available、调试台、推理路由使用。实现先批量获取完整权限和部署快照，再以操作计算资格，最后按别名聚合可用 operations。一个失效部署不屏蔽同别名另一个有效部署；去重必须在逐部署检查之后进行。

目标保留 GET /model/available，可添加 operation、scope_id、category、cursor、limit 参数，服务端校验作用域，返回 object、data、next_cursor、snapshot_version、evaluated_at。数据行包含 id、display_name、provider_display、category、operations、capabilities、limits、prices、availability_state；价格必须附 currency、unit、quantity_basis、price_version 和 estimated，不以模糊的 input_price 统一表示视频秒价与每百万 token 价。

capabilities 从适配器与该部署配置的交集产生；规格可引用目录但必须标来源。用户响应不包含 api_base、credential_id、密钥、内部 deployment_id 和供应商报错原文。同别名多供应商时用准确的展示列表，不随意取第一项作为事实。

| 结果 | HTTP / code | 页面表现 |
| --- | --- | --- |
| 未登录、失效会话 | 401 / invalid_session | 引导重新登录 |
| 无 model.available.read | 403 / permission_denied | 无权限提示 |
| 成功但无可用模型 | 200 / data=[] | 空态和申请管理员开通入口 |
| 权限/配置依赖不可用 | 503 / availability_unavailable | 重试及 request_id |
| 作用域不属于账号 | 403 / scope_denied | 不暴露目标作用域信息 |

不得将依赖错误缓存成空结果；缓存键包含主体、作用域、permission_version、catalog_version、deployment_version、operation。涉及凭据/健康的新状态必须失效或重新验证，不能只用用户 ID 缓存。

## 页面与导航

/mine-models 使用模型广场相似的卡片、搜索、分类与规格展示，但数据源只取 available。调试台共享该源，不把供应商项混入下拉列表。分类与搜索操作不得把广场数据回填为可用数据。

左栏顶层为“我的”和“管理员”。我的包含我的模型、个人虚拟密钥、调试台、个人用量与日志；管理员包含模型/部署、供应商与凭据、价格、组织团队、策略及全局用量日志。管理员入口按能力显示，接口单独授权。登录采用桌面左侧产品介绍、右侧表单，小屏表单优先；删除默认凭据、MASTER_KEY、SSO 环境设置说明，保留有效 SSO 按钮、错误反馈和无障碍标签。

## 故障、迁移与验收

先将混合配置归类，再引入统一服务和 API 版本字段，最后替换前端数据源。关键依赖读失败拒绝整个快照，避免把无法确认的部署展示为可调用；明确无效配置可排除并给管理员诊断。

充分性：逐操作筛出满足所有本地条件的部署后再聚合，任一返回操作都有至少一个见证部署。列表与调用之间存在状态变化，推理启动时必须重算条件；无法用一次列表查询保证未来外部健康。

验收：普通用户不会看见 qiniu/fennoai 配置条目、其他租户模型或内部凭据；同别名一好一坏保留一次；仅有静态成本记录、无适配器或无凭据的条目不可选；403、503 和空列表显示不同状态。针对用户报告的加载错误，先验证当前构建、会话和 HTTP 错误码，不复用文档中的旧 token。参见 [ADR-002](decisions/ADR-002-available-model-definition.md)。
