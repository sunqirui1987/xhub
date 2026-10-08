# 正文护栏

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

guard.go 读取已配置的关键词护栏，执行 block 或 redact。Module 提供 /apply_guardrail 与 /guardrails/apply_guardrail；PreCall 由数据面在适用的 chat 请求发送前调用。Host 把配置读取和身份检查交给网关。
block 必须在上游拨号前拒绝；redact 修改即将发送的正文并保持可解释结果。配置存储读取失败拒绝执行，不能当作不存在护栏。直接护栏 HTTP 调用也需要身份与授权，不构成绕过推理入口的路径。
这里是本地关键词规则，不承诺远端安全分类器或所有多模态内容审核。官方 bypass 当前不使用普通聊天护栏。修改规则要验证阻断不产生上游请求、替换正文正确、失败不计成功及日志脱敏。

## 源码职责与入口

### codec.go

内部实现和协议边界见 [codec.go](codec.go)。

### guard.go

- [`func Apply(s Host, w http.ResponseWriter, r *http.Request)`](guard.go) — Apply is the management trial for a guardrail. It does not change storage.
- [`func PreCall(s Host, body map[string]any) (bool, string)`](guard.go) — PreCall runs guardrails before chat is sent. A block returns true and a reason, and the data plane does not call the upstream.
- [`func Evaluate(s Host, body map[string]any) (blocked bool, message string, findings []map[string]any)`](guard.go) — Evaluate runs every default-on pre-call rule and reports each one. The logs drawer reads these rows as guardrail monitoring. A block stops the scan: later rules did not run, so they are not reported as if they had.

### host.go

公开类型：`Host`.

内部实现和协议边界见 [host.go](host.go)。

### mount.go

- [`func Module(h Host) httpx.Module`](mount.go) — Module is the guardrail trial API. The data plane calls PreCall before the upstream and does not go through these two paths.

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
