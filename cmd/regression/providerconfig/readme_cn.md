# 真实供应商测试配置解析

[English](readme.md) · [回归测试方案](../../../docs/development/regression.md) · [多供应商场景](../../../docs/development/multi-supplier-regression.md)

## 职责与数据边界

本目录解析 `config_provider.yaml` 的非秘密元数据，为真实模型回归和确定性多供应商场景提供统一的结构。它不读取环境变量、不加载凭据、不拨号、不启动网关。密钥只以 `key_env` 变量名引用；调用方在运行时另行注入密钥值。解析错误不回显原始 YAML，以免误粘的凭据进入测试日志。

`Config` 包含版本、供应商和加权场景；`Provider` 包含稳定 ID、凭据名、环境变量名、API 根地址、协议及模型集合；`Scenario` 包含对外模型名、请求次数和部署列表；`Deployment` 定义稳定部署 ID、供应商、上游模型和非负权重。测试以部署 ID 核对实际分流，不用易变的根地址或模型名充当权重身份。

## 实现与接口

- [providerconfig.go](providerconfig.go) 的 `Load(io.Reader) (Config, error)` 只接受一份 YAML 文档，并启用未知字段拒绝；随后运行跨字段验证。nil 输入、语法错误、第二份文档和不合法字段均返回错误。
- `validate` 检查版本为 1、供应商 ID 与环境变量名格式、供应商 ID 唯一、凭据名非空、HTTP(S) 根地址无内嵌凭据/查询/片段、协议为 OpenAI 或 Anthropic、模型列表非空，以及加权场景引用的供应商和模型确实存在。
- 每个加权场景至少两个部署；部署 ID 全局唯一，权重非负且总量为正。请求次数范围为 1–100，必须覆盖整数个约简权重周期。例如权重 3:7 的场景要以 10 的倍数请求，测试才能精确核对周期；`gcd` 负责约简，求和时检查整数溢出。
- `validHTTPURL` 对 API 根地址做结构校验。它允许 URL 路径，拒绝用户名密码、查询串和片段。

本包没有直接登记 HTTP 路由。运行时调用方负责选择供应商、读取 `key_env` 对应凭据、创建部署、执行模型调用及账单验证。配置合法只表示场景可执行，不证明上游连接可用。

## 验证

[providerconfig_test.go](providerconfig_test.go) 覆盖同模型多供应商与同供应商多部署、字段和引用错误、权重周期/溢出、未知 YAML 字段、多文档输入以及错误消息不回显输入。完整业务链在 [cmd/regression](../readme_cn.md) 的多供应商和真实权重测试中执行。

```bash
go test ./cmd/regression/providerconfig -count=1
bash scripts/regression.sh
```
