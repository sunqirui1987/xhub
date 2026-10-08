# 内置供应商装配

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

all.go 通过空白导入 openai、qiniu、volcengine 触发需要的 init 登记。生产启动只需导入此聚合包，避免不同入口漏掉供应商 transport 或内置模型。
目录没有独立导出函数和 HTTP 路由，行为来自被导入包的初始化。openai 当前占位无 init，实际适配在 llm；qiniu/volcengine 登记官方任务 transport 和目录模型。
新增供应商实现后还要加入这里并核对启动方导入。测试分别落在供应商和 provider 注册用例；聚合包本身没有直接测试，不应把导入成功理解为真实供应商认证成功。

## 源码职责与入口

### all.go

内部实现和协议边界见 [all.go](all.go)。

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../../logx/readme_cn.md), [internal/provider/openai](../openai/readme_cn.md), [internal/provider/qiniu](../qiniu/readme_cn.md), [internal/provider/volcengine](../volcengine/readme_cn.md).

## 验证与维护入口

当前目录没有直接测试文件；上层集成测试仅证明被执行的链路，不代表所有内部失败分支均已覆盖。

```bash
go test ./internal/provider/all -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
