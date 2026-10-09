# 回归测试夹具

本目录是上级 `internal/regression` 包的输入数据，没有生产 Go API 或 HTTP 路由。

| 文件 | 用途 |
| --- | --- |
| [config_provider.yaml](config_provider.yaml) | 多供应商确定性选路测试共用的供应商与部署结构。测试宿主将上游地址替换为独立环回服务器，并注入合成的命名凭据。 |

夹具由网关使用的配置解析器加载，不应包含真实 API 密钥或生产端点。回归断言检查解析后的部署身份、模板权重、重试与账务，而非仅检查 YAML 语法。运行 `go test ./internal/regression -count=1 -timeout=600s`；完整模式和前置条件见[回归方案](../../../docs/development/regression.md)。
