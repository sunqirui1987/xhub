# 运行排查与功能故障定位

[功能实现](implementation.md) · [配置](configuration.md) · [运行边界](runtime.md) · [回归](regression.md)

## 用证据定位请求

优先记录客户端返回的状态、协议错误码和 x-litellm-call-id。调用 ID 连接日志、费用明细和失败阶段。不要粘贴 Authorization、上游 api_key、数据库 URL 或完整用户正文到排查报告。

顺序检查进程 liveness、readiness/details、身份、资源归属、模型能力、预算/限流、模板 effective、可用候选、上游响应、usage/快照、队列/落库。按此顺序可以分清拒绝发生在拨号前还是供应商阶段；只修改重试次数无法解决身份或能力错误。

## 现象与定位

| 现象 | 首要检查 | 实现入口 | 可验证结果 |
| --- | --- | --- | --- |
| 服务存活但管理请求失败 | PostgreSQL/账号/会话版本 | health、auth、iam | 区分依赖故障与认证拒绝 |
| master 不能推理 | 凭据类型和明确动作集合 | authz/decide | 主密钥不具有普通推理能力 |
| 管理员 API key 不能管理模型 | 是否用户会话 | auth Principal.PlatformAdmin | 所有者角色不赋予 key 管理动作 |
| 模型不显示/不能调用 | disabled、模型名单、endpoint_types | models、limits、provider | 发现与调用使用相同过滤 |
| 保存模板但选路没改变 | binding/effective、scope、字段执行状态 | prefs/Resolve、route_settings | 来源和值与拨号序列相符 |
| 清空模板后参数不等于平台 | 父层仍选了模板 | prefs RequestChain | 逐层继承 |
| 请求多次超时 | timeout 是每尝试，num_retries 是总尝试数 | dataplane Serve | 不是整个池单一总超时 |
| 流 200 但内容截断 | 上游读取错误/客户端关闭 | dataplane stream | 日志 failure；无第二响应拼接 |
| 费用为 0 | 缓存、明确零价、缺价、失败/无 usage | catalog、spend | 分别核验，缺价不当免费 |
| 金额出现倍数误差 | unit_size 二次换算、缓存重复输入 | catalog rates/usage | quantity×基础单位 usd |
| 查询视频重复费用 | settlement identity 和去重 | official、IAM RecordUsage | 完成查询仅一次 durable 费用 |
| 页面费用暂高于数据库 | 已提交但 Redis Ack 失败 | live/dataplane Flush | 重放不再加费，热支出待确认 |
| 同 session 汇总缺记录 | 当前页/分页/调用方隔离 | usage reports | 页汇总不是会话完整总计 |
| 改价后旧账变化 | snapshot 缺失回退重算 | usage cost breakdown | source snapshot/recomputed 区分 |

## 数据库与 Redis

生产故障排查使用只读查询与指标，先确认 schema/实例。回归连接必须使用专用配置，testsupport 为每用例创建独立 schema。不要随意 DROP 公共 schema 或清空共享 Redis。

Redis 保存热费用、限流、冷却和队列；SQL 保存权威事件。处理队列故障先核验落库 commit，再核验 Ack。不能手工删队首来隐藏错误而丢结算证据，也不能给 queued 项重复加总。

Redis 缺失的降级行为与 SQL 身份故障不同：冷却查询故障可能返回空列表，身份/模板权限读取失败必须拒绝。恢复后共享状态、历史数据和供应商费用还需对账。

## 真实供应商异常

核对 BASE 的协议根、model 是否上游名、命名凭据协议和路径，避免把公开名发给供应商。OpenAI/Anthropic 的 usage 和认证头不同；真实返回错误不应用假 200 覆盖。

429/5xx 与普通 4xx 有不同重试规则。官方创建结果未知时不能自动再次创建。媒体 task 的创建和完成是两个阶段，首次 GET 不保证终态。未定价模型要明确补价，而不是认为免费。

## 历史数据与隐私

读取日志先验证范围，再判断正文保留。prompt logging 关闭时无正文是正常行为；特权读取失败可能来自审计写入失败。历史归属优先记录字段，当前资源删除不应让旧费用归到其它租户。

运行日志脱敏是辅助，不能替代不输出秘密。对外错误避免泄漏内部连接信息；报告只保留资源标识、用量和金额等必要证据。

## 恢复后验证

先跑对应定向场景，再跑业务链。验证成功、预期拒绝、费用快照、无重复结算和租户隔离。公共实现变化时运行 internal 全套和严格回归。命令、环境及跳过分类见[测试指南](testing.md)和[回归方案](regression.md)。
