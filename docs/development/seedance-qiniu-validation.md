# 七牛 Seedance 实测与计价边界

2026-10-08 使用环境变量中的七牛凭据，通过 xhub 的 ServeBypass 创建并查询一个真实付费任务。凭据没有写入该文件或测试代码。
这项环境变量只供显式启用的付费测试读取，不参与应用启动和凭据创建。产品配置需要管理员保存 Custom 或 Custom OpenAI 凭据，并显式选择 qiniu_contents_generation；系统不按地址、名称或模型推断七牛账户。

| 项目 | 实测结果 |
| --- | --- |
| 模型 | qiniu/bytedance/doubao-seedance-2-0-mini-260615 |
| 任务 | qvideo-1383141937-1791471635726467449 |
| 请求 | 纯文本、4 秒、480p、16:9、generate_audio=false |
| 结果 | HTTP 200，succeeded，返回视频 URL |
| 完成时间 | 约 112 秒 |
| completion_tokens | 40,594 |
| 计价变体 | woiv，无参考视频 |
| 目录美元单价 | $0.00000333333 / token |
| 计算美元费用 | 40,594 × 0.00000333333 = $0.13531319802 |
| 目录人民币报价 | ¥0.023 / 1,000 tokens |
| 计算人民币费用 | 40,594 / 1,000 × 0.023 = ¥0.933662 |

两个金额分别按目录对应币种报价计算，美元目录涉及汇率换算及舍入。尚未查询账户账单，不能把计算金额当作已经核准的七牛实际扣款。供应商返回签名视频 URL，有有效期。

真实测试验证 provider 请求路径、模型重写、创建/查询、成功终态、实际用量及目录计价，未直接经过生产 SQL 网关。另用本地 PostgreSQL 网关回归验证等待/失败不扣费，成功结果重复查询三次只产生一条收费事件，并且五个身份支出范围只更新一次。Redis 测试验证服务器实例替换后仍能读取创建时上下文。

## 可复现入口

普通包检查：go test ./internal/provider/... ./internal/catalog ./internal/dataplane ./internal/gateway ./internal/gateway/usage -count=1

数据库验收：XHUB_REGRESSION_STRICT=1 go test ./internal/regression -run '^TestSeedance' -count=1 -v

显式付费测试：XHUB_QINIU_SEEDANCE_LIVE=1 go test ./internal/dataplane -run '^TestQiniuSeedanceLive$' -count=1 -v，需要环境中的 QINIU_API_KEY。

重复运行真实测试会创建新任务并再次消费额度。对既有任务排查应继续查询原 task ID。

## 尚未实现

- 没有后台自动查询补结算；客户端停止查询后可能漏记成功任务费用。
- 没有异步任务预算预占，不能保证并发任务不超过预算。
- 价格版本没有在创建时锁定；人工价格或目录变更可能影响稍后结算。
- 上下文无 Redis 时仅保存在进程内，Redis 状态丢失或超过七天也可能失去准确选档依据。
- 未定价用量会保存，但没有自动补价与调整分录。
- 供应商成本、客户售价没有独立账本，未实现账户扣款对账和退款分录。

资料来源：[七牛 Seedance API](https://docs.modelink.ai/api/video-doubao-seedance-20)、[按量计费说明](https://docs.modelink.ai/billing/usage-based-billing)、[Modelink 价格目录](https://api.modelink.ai/v1/market/models)。
