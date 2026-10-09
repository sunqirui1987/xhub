# 七牛视频任务传输

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

seedance.go 登记 qiniu_contents_generation 协议，默认协议地址为 api.qnaigc.com。官方创建路径为 POST /v3/contents/generations/tasks，查询为 GET 同路径/{id}；没有 list 动作。XHub 不登记固定七牛凭据供应商；管理员必须选择 Custom 或 Custom OpenAI，填写地址和密钥，并在模型上显式选择该传输。
请求模型字段为 model，任务 ID 字段为 id，上游模型名去掉 qiniu 前缀。目录包含标准版、Fast、Mini（260615）和 2.5（260628）四条模型；价格显式绑定七牛 Modelink 市场目录，避免借用其他供应商的同名模型价。凭据名、模型名和 hostname 都不会触发协议推断。
实际发送、任务钉、权限、轮询与终态结算由 dataplane 管理。登记测试只证明路径和字段描述；真实任务测试使用 XHUB_QINIU_SEEDANCE_LIVE=1 和环境中的 QINIU_API_KEY，会创建一个付费视频。不得把成功创建等价于终态完成或准确计费。

## 源码职责与入口

### seedance.go

内部实现和协议边界见 [seedance.go](seedance.go)。

### fal.go

[fal.go](fal.go) 登记 Doubao/Dreamina Seedance、Kling、Vidu、Veo、MiniMax 的 100 个具体 Fal 创建路径和 9 个公共任务队列。上游使用 Key 鉴权和 request_id；创建路径与供应商价格 ID 分开登记。共享 provider.FalBilling 按实际完成 token 或输出秒数提取用量，并选择明确的价格档位。Vidu Q2 时长区间和 MiniMax 输入素材/免费图片规则尚未完整进入价格契约，保留实测用量并标记待核价。配置、计价边界和实测见 [七牛 Fal 扩展](../../../docs/development/qiniu-fal.md)。

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../../logx/readme_cn.md), [internal/provider](../readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [seedance_test.go](seedance_test.go) | `TestQiniuSeedanceKeepsTheBytedancePrefix` |
| [billing_test.go](billing_test.go) | `TestSeedanceMeasuredBands`, `TestSeedanceOnlySettlesExplicitSuccess`, `TestSeedanceSearchWithoutRateIsUnpriced` |
| [fal_test.go](fal_test.go) | 具体路径覆盖、供应商费率、终态用量、上下文缺失及不完整价格 |

```bash
go test ./internal/provider/qiniu -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。

## 异步用量与计价

原生 transport 使用共享的 provider.SeedanceBilling()。创建时保存模型、开始时刻、分辨率及是否带参考视频，不保存提示词或素材 URL。成功终态 succeeded 且无业务错误时，按上游 completion_tokens 计价；不以时长、total_tokens 或提示 token 替代。实际响应分辨率优先于请求值。Fal transport 使用 provider.FalBilling() 处理队列信封与独立结果。

480p/720p 按 wiv（有参考视频）或 woiv（无参考视频）选价，1080p/4K 使用对应限定档位。已测量档位没有价格时不选其他最低价；显式部署费率表不退到目录或扁平价。缺少创建上下文、未知分辨率或搜索次数缺价时保留用量和 pricing_status: unpriced，日志显示待核价。明确配置的统一输出单价仍可用于人工收费策略。

Redis 上下文与任务钉保存七天；无 Redis 时只在进程内保存。稳定结算身份经 Redis/PostgreSQL 去重，重复成功查询不重复扣费。当前没有后台补查、预算预占、自动补价或供应商对账；创建时未锁定价格版本，结算使用当时可用费率。

实测、复现命令与限制见 [七牛验证记录](../../../docs/development/seedance-qiniu-validation.md)。

独立的付费 Fal 测试是 internal/dataplane/qiniu_live_test.go 中的 TestQiniuFalLive，需要 XHUB_QINIU_FAL_LIVE=1 和环境中的 QINIU_API_KEY；每次运行会新建一个付费视频。Fal 创建遇到网络错误或 HTTP 5xx 不重放，因为上游可能已经受理任务。
