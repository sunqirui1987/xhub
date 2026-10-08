# HTTP 错误与模块协议

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

httpx.go 提供协议错误写入、JSON 辅助和调用 ID；module.go 定义 Registrar、Module、Bind。Registrar.Handle 是模块唯一登记边界；Module.Name/Mount 使子模块通过宿主接口安装。
CallID 生成 32 位十六进制调用标识，SetCallID 写入响应头，用于连接响应、日志、费用和失败排查。WriteTypedError 按调用协议输出错误，429 带 Retry-After: 1；不要把 Anthropic/Gemini 的错误体机械写成 OpenAI 形式。
这里不识别用户身份或决定权限，只负责把已决定的错误和模块操作表达成 HTTP。响应已经写出后不能再替换状态码；流中断的业务结果要另外记日志。

## 源码职责与入口

### httpx.go

- [`func CallID() string`](httpx.go) — CallID returns a new 32-character hexadecimal call identifier.
- [`func SetCallID(w http.ResponseWriter, id string)`](httpx.go) — SetCallID writes x-litellm-call-id. If a value is already set, the caller decides whether to overwrite it.
- [`func WriteError(w http.ResponseWriter, status int, typ, msg string)`](httpx.go) — WriteError writes the gateway JSON error. Inference paths should call WriteTypedError so the provider envelope is used instead.
- [`func WriteTypedError(w http.ResponseWriter, path string, status int, typ, msg string)`](httpx.go) — WriteTypedError picks the error envelope from the request path. Status 429 also sets Retry-After to 1. A missing call ID is filled in.
- [`func WriteJSON(w http.ResponseWriter, status int, v any)`](httpx.go) — WriteJSON sets the status code and writes JSON. A later encoding failure does not change the status that was already sent.

### module.go

公开类型：`Registrar`, `Module`.

- [`func Bind(name string, mount func(Registrar)) Module`](module.go) — Bind builds a module from a name and a mount function. An empty name stays empty, and the process refuses to register it.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [httpx_test.go](httpx_test.go) | `TestWriteTypedErrorUsesProviderEnvelope`, `TestWriteJSONPreservesExistingHeaders` |

```bash
go test ./internal/httpx -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
