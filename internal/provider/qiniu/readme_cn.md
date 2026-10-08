# 七牛官方视频任务传输

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

seedance.go 登记 qiniu_contents_generation，默认供应商地址 api.qnaigc.com。官方创建路径为 POST /v3/contents/generations/tasks，查询为 GET 同路径/{id}；没有 list 动作。
请求模型字段为 model，任务 ID 字段为 id，上游模型名去掉 qiniu 前缀。内置贡献三条 Seedance 模型，未提供可用价格的条目不能承诺有准确费用。部署可以覆盖默认地址和命名凭据。
实际发送、任务钉、权限、轮询与终态结算由 dataplane 管理。登记测试只证明路径和字段描述；真实任务测试需 BYPASS_MODEL/BASE/ENDPOINT，并可能消耗视频额度。不得把成功创建等价于终态完成或准确计费。

## 源码职责与入口

### seedance.go

内部实现和协议边界见 [seedance.go](seedance.go)。

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../../logx/readme_cn.md), [internal/provider](../readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [seedance_test.go](seedance_test.go) | `TestQiniuSeedanceKeepsTheBytedancePrefix` |

```bash
go test ./internal/provider/qiniu -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
