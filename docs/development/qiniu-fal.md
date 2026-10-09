# 七牛 Fal 视频扩展与验证

Fal 是传输协议，七牛兼容地址由用户配置，具体路径是模型标识，市场价格 ID 是计价来源。这四项分别登记，避免混用同名模型、不同协议或不同供应商价格。原生 qiniu_contents_generation 保留；Fal 使用独立 transport。XHub 不预置七牛凭据供应商，也不读取 `QINIU_API_KEY` 自动创建账户；管理员在凭据表单选择 Custom、Custom OpenAI 或 OpenAI，填写地址和密钥后，再为模型选择对应传输。

上游模型 `bytedance/doubao-seedance-2-0-260128` 对应 Ark Video；FAL Doubao 使用 `bytedance/seedance-2.0/text-to-video`，FAL Dreamina 使用 `byteplus/seedance-2.0/text-to-video`。菜单会显示 `FAL · Dreamina Seedance 2.0` 等明确协议名。对外模型名称支持 `/`、`:` 和中文 `：`，修改别名不会改变上游型号或协议。七牛连接可填写目录 ID `qiniu` 自动选择登记能力；普通中转商目录按实际上游列表发现，并允许显式选择精确匹配路径的 FAL 协议。

## 模型与队列

internal/provider/qiniu/fal.go 登记 100 个具体创建路径、9 个任务队列。模型 ID 为 qiniu/<创建路径去掉 /queue/>。下面列出代表模型；完整模式清单以登记文件为准。

| 系列 | transport | 代表创建路径（POST） | 查询路径前缀（GET） |
| --- | --- | --- | --- |
| Doubao Seedance 2.0 / Fast / Mini | qiniu_fal_doubao_20 | /queue/bytedance/seedance-2.0/mini/text-to-video | /queue/bytedance/seedance-2.0/requests/ |
| Doubao Seedance 2.5 | qiniu_fal_doubao_25 | /queue/bytedance/seedance-2.5/text-to-video | /queue/bytedance/seedance-2.5/requests/ |
| Dreamina Seedance 2.0 / Fast / Mini | qiniu_fal_dreamina_20 | /queue/byteplus/seedance-2.0/fast/text-to-video | /queue/byteplus/seedance-2.0/requests/ |
| Dreamina Seedance 2.5 | qiniu_fal_dreamina_25 | /queue/byteplus/seedance-2.5/text-to-video | /queue/byteplus/seedance-2.5/requests/ |
| Kling 2.5 Turbo / 2.6 / 3 / O3 / O1 / 3 Turbo | qiniu_fal_kling | /queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video | /queue/fal-ai/kling-video/requests/ |
| Vidu Q1 / Q2 / Q2 Pro / Q2 Turbo / Q3 / Q3 Pro / Q3 Turbo | qiniu_fal_vidu | /queue/fal-ai/vidu/q3/text-to-video/pro | /queue/fal-ai/vidu/requests/ |
| Veo 3.1 / Fast | qiniu_fal_veo31 | /queue/fal-ai/veo3.1/fast | /queue/fal-ai/veo3.1/requests/ |
| MiniMax H3 | qiniu_fal_minimax_h3 | /queue/minimax/h3/text-to-video | /queue/minimax/h3/requests/ |
| MiniMax H3 Max | qiniu_fal_minimax_h3_max | /queue/minimax/h3-max/text-to-video | /queue/minimax/h3-max/requests/ |

查询结果使用 <前缀>{request_id}，查询状态使用 <前缀>{request_id}/status。2.0 的 Fast/Mini 查询走版本公共队列，不能把创建路径直接加 /requests/。创建接口按文档列出具体路径，未开放任意路径转发；并非每个型号都有所有生成模式。

## 配置和调用

~~~yaml
model_list:
  - model_name: kling-25
    litellm_params:
      model: qiniu/fal-ai/kling-video/v2.5-turbo/pro/text-to-video
      custom_llm_provider: custom
      api_base: https://api.qnaigc.com
      api_key: os.environ/QINIU_API_KEY
    model_info:
      transport: qiniu_fal_kling
      endpoint_types: ["fal:queue"]
~~~

也可以在模型添加界面使用 Custom 凭据选择七牛的 Fal 模型；目录提供默认 transport 和市场价格绑定。生产环境应使用 litellm_credential_name 引用手工保存的 Custom 凭据。凭据名称、模型名称和 API hostname 都不会把普通凭据隐式升级为七牛账户。

客户端向网关发送原生 Fal JSON，例如：

~~~http
POST /queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video
Authorization: Bearer <网关调用密钥>
Content-Type: application/json

{"prompt":"A red apple on a white table.","duration":"5"}
~~~

不带 model 时，路径匹配已配置的公开模型别名。同一个路径有多个不同公开别名时，必须添加 "model":"kling-25" 选择；多个部署使用同一个别名时仍参与原有路由策略。这个可选字段仅用于网关选路，发给七牛前删除，不能借它切换成另一个路径模型。

网关向七牛使用 Authorization: Key <供应商密钥>。创建响应中的 status_url、response_url 改为网关相对路径；客户端应相对于网关 base URL 解析，并用创建时的同一网关身份查询。上游 URL 和不可信 Host 不决定查询目的地。fal_webhook 查询参数仍透传；Webhook 本身不会触发网关结算。取消未实现，cancel_url 置空。未验证完整 Fal SDK 自动订阅兼容性。

创建任务遇到网络错误或 HTTP 5xx 时不自动重试或切换部署：上游可能已受理付费任务，重放会生成第二个视频。HTTP 5xx 响应原样返回供调用方判断；HTTP 429 仍按路由重试策略处理。成功 HTTP 状态但带 error/detail 的业务失败不会建立任务钉。

## 用量与计价

创建时保存模型、时间、分辨率、音频/参考视频计价档位，排除提示词、素材 URL 和凭据。价格绑定 https://api.modelink.ai/v1/market/models 中明确的 seller 模型 ID，不借用其他供应商价格。

| 模型 | 计价依据与当前行为 |
| --- | --- |
| Doubao / Dreamina Seedance | 成功结果的 usage.completion_tokens，按分辨率及 wiv/woiv 选档；请求时长和 total_tokens 不替代完成 token |
| Kling | 最终 video.duration；根据模式、音频、声音 ID、参考视频选择价格变体 |
| Vidu Q1 / Q3 系列 | 最终 video.duration，按分辨率及 t2v/i2v/r2v 选档 |
| Vidu Q2 系列 | 转发可用、保留实际秒数；目录存在输出区间规则，当前扁平 rate 表不能完整表达，标记 vidu_duration_tiers 待核价 |
| Veo 3.1 | 最终 video.duration，选择音频及 4K 变体 |
| MiniMax H3 Max 文生/图生 | 优先 usage.output_seconds，缺失时使用最终 video.duration，按分辨率计输出秒数 |
| MiniMax H3 / H3 Max 参考生 | 保存输出秒、输入秒、参考图片数；输入视频和免费图片额度未完整进入 rate 表，标记 minimax_composite_usage 待核价 |

Vidu Q2 的 output_range 和 MiniMax 的 free_quota 不能丢弃后声称完整费用；MiniMax 文档的“通常前 5 张”与 H3 Max 市场的 free_quota=2 也不能互相替代。待核价任务保留用量和原因，费用快照标为 unpriced，不会把零金额表述成免费；统一人工秒价也不能掩盖这里缺少的计费维度。

只对成功结果且有有效视频 URL 的响应提取用量。IN_QUEUE、IN_PROGRESS，以及 COMPLETED 但存在 error/detail 的失败结果不结算。缺少实际用量时不按请求时长或 metrics.inference_time 猜测。测得的档位没有输出价时保留待核价，不使用其他最低价或输入秒价。

查询状态中的嵌套 result 与独立结果接口共用任务结算 ID。PostgreSQL 去重保证重复成功查询只有一条收费事件；任务按创建身份及队列 transport 隔离。上下文与部署钉在 Redis 中保存七天；没有 Redis 时仅保存在进程内。

当前仍由客户端查询触发结算，未实现后台补查、预算预占、创建时锁定价格版本、待核价自动补价、供应商成本/客户售价独立账本或供应商账户对账。因此本扩展能接入并可靠记录常规视频用量，还不是完整商业结算系统。

## 验证证据

2026-10-09 用环境变量中的七牛凭据，通过 ServeBypass 创建一个付费 Fal 任务，随后经改写的网关查询路径获取最终结果：

| 项目 | 实测 |
| --- | --- |
| 模型 | qiniu/bytedance/seedance-2.0/mini/text-to-video |
| request_id | qvideo-1383141937-1791498321813028599 |
| 参数 | 4 秒、480p、16:9、generate_audio=false |
| 状态 | HTTP 202 IN_PROGRESS → HTTP 200 COMPLETED；有视频 URL |
| 完成时间 | 约 102 秒 |
| completion_tokens | 40,594 |
| 输出价格档 | woiv；非 fallback |
| 目录计算费用 | 40,594 × $0.00000333333 = $0.13531319802 |

这里是目录估算，未查询七牛账户账单，不代表核准的供应商实际扣款。密钥与签名媒体 URL 没有写入验证文档。真实调用仅覆盖这一个 Fal 型号；其他注册型号由文档、假上游和目录计价测试验证，不宣称全部实测成功。

数据库回归通过真实 PostgreSQL 网关验证：请求 10 秒但实际生成 5.5 秒，按 5.5 × $0.07246377 计费；等待/业务失败不扣费；成功状态接口、结果接口重复查询三次只产生一条收费事件。

~~~bash
go test ./internal/provider/... ./internal/catalog ./internal/dataplane ./internal/gateway/usage -count=1
XHUB_REGRESSION_STRICT=1 go test ./internal/regression -run 'Test(FalSettlement|SeedanceSettlement)' -count=1 -v
# 明确付费，每次运行会创建一个新视频；需要环境中的 QINIU_API_KEY
XHUB_QINIU_FAL_LIVE=1 go test ./internal/dataplane -run '^TestQiniuFalLive$' -count=1 -v
~~~

## 新增其他模型

新增同协议型号时，在 fal.go 补具体创建路径、seller 价格 ID、协议默认档位和计价变体，优先复用其公共查询队列。新增不同队列时调用 registerFal 登记独立 transport；不同供应商注册自己的凭据与价格来源。用量结构不同则增加 TaskBilling 提取器，转发层无需增加模型分支。需要区间/免费额度的价格应先扩充目录 rate 契约并保留完整市场规则，不能依赖现有首条扁平费率。

新增后验证路径覆盖、请求鉴权、别名选路、终态/业务失败、准确用量、具体价格档、任务隔离和持久层扣费去重。资料入口：[Kling 2.5 Turbo](https://docs.modelink.ai/api/video-fal-kling-v25-turbo)、[Dreamina 2.5](https://docs.modelink.ai/api/video-fal-dreamina-seedance-25)、[价格目录](https://api.modelink.ai/v1/market/models)。
