# 正文护栏

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

guard.go 合并默认启用与请求指定的 pre_call 护栏，按优先级执行，遇到 block 立即停止。rules.go 执行本地关键词/RE2 检查；custom.go 编译 XGo 脚本并通过 iXGo 执行。Module 提供 /apply_guardrail 与 /guardrails/apply_guardrail；数据面通过宿主调用 Evaluate，在适用的 chat/responses 请求发送前执行。Host 把配置读取和身份检查交给网关。
block 必须在上游拨号前拒绝；redact 修改即将发送的正文并保持可解释结果。配置存储读取失败拒绝执行，不能当作不存在护栏。直接护栏 HTTP 调用也需要身份与授权，不构成绕过推理入口的路径。
支持本地关键词/RE2、XGo 及 OpenAI/Lakera/Azure/Bedrock/Presidio/远端 LiteLLM/本地密钥检测。脚本通过 xhub/guardrail 使用 HTTP、JSON、LLMChat 和 Allow/Block/Modify/Flag，调试与真实请求共用执行器。CRUD、YAML、接口合约及执行限制见[配置与 XGo 使用指南](../../../docs/development/guardrails.md)。不承诺远端安全分类器或所有多模态内容审核。官方 bypass 当前不使用普通聊天护栏。修改规则要验证阻断不产生上游请求、替换正文正确、失败不计成功及日志脱敏。

## 源码职责与入口

### codec.go

内部实现和协议边界见 [codec.go](codec.go)。

### guard.go

- [`func Apply(s Host, w http.ResponseWriter, r *http.Request)`](guard.go) — 管理员调试入口，执行已保存或临时规则，不修改存储。
- [`func PreCall(s Host, body map[string]any) (bool, string)`](guard.go) — 兼容的请求前检查入口；拦截返回 true 与原因，阻止上游调用。
- [`func Evaluate(s Host, body map[string]any) (blocked bool, message string, findings []map[string]any)`](guard.go) — 按优先级执行默认与显式规则的并集，记录每条实际结果供日志监控查看；拦截后停止，不报告未执行规则。

### host.go

公开类型：`Host`.

内部实现和协议边界见 [host.go](host.go)。

### mount.go

- [`func Module(h Host) httpx.Module`](mount.go) — 注册管理调试接口；正式数据面通过宿主调用 Evaluate，不经过这两个 HTTP 路径。

## 对外 HTTP 边界

入口由下表所列注册文件安装。别名共享处理器；路径存在不替代权限和业务断言，授权说明见模块契约及接口参考。

| Method / path | 注册文件 |
| --- | --- |
| `POST /apply_guardrail` | [mount.go](mount.go) |
| `POST /guardrails/apply_guardrail` | [mount.go](mount.go) |

## 依赖关系

[internal/auth](../../auth/readme_cn.md), [internal/httpx](../../httpx/readme_cn.md), [internal/logx](../../logx/readme_cn.md), [internal/store](../../store/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [guard_regression_test.go](guard_regression_test.go) | `TestGuardrailLookupFailureBlocks`, `TestGuardrailRedactsEveryTextLeafWithoutChangingImageURL` |
| [guard_test.go](guard_test.go) | `TestGuardrailTextScansAllProtocolFields`, `TestGuardrailTextJoinsMessageStringsAndSkipsOtherShapes`, `TestExtraWordsAcceptsTheShapesASavedRuleUses`, `TestBoolOfReadsTheFormsJSONProduces`, `TestMatchGuardrailBlocksRedactsAndAllows`, `TestEvaluateReportsEachDefaultRule`, `TestPreCallOnlyStopsOnADefaultPreCallBlock`, `TestPreCallNamesAnUnnamedRule`, `TestEvalNamedRunsTheAskedRuleAndOtherwiseTheDefault`, `TestApplyTrialsOneRuleWithoutWritingIt` |

```bash
go test ./internal/gateway/guard -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。

## 扩展实现

external.go 适配外部协议；primitives.go 提供 HTTP、JSON 和每次解释器独立的网络上下文；secrets.go 检测本地密钥特征；capabilities.go 遮蔽管理响应中的凭据。纯本地脚本预算 100ms，网络脚本及外部规则预算 10 秒。详细支持矩阵、函数签名与边界见[使用指南](../../../docs/development/guardrails.md)。
