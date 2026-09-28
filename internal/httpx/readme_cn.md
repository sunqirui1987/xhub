# httpx

## 这个模块做什么

`httpx` 写出网关返回的 JSON，并在响应上盖调用 ID。处理函数不用自己设置 `Content-Type`，也不自己拼错误对象。推理路由可以用供应商那种错误形状。管理路由用网关自己的信封。

## 功能

- `CallID` 生成一个新标识。`SetCallID` 把它写到响应上。
- `WriteJSON` 写状态码和 JSON 正文。
- `WriteError` 写网关错误信封：`error.type` 和 `error.message`。
- `WriteTypedError` 按请求路径选信封。Anthropic messages 路径和 Gemini 原生路径用各自的形状，其它路径用网关信封。
- `Bind` 造出一个 `Module`。`Mount` 在 `Registrar` 上登记 `METHOD /path`。功能包依赖这个表面，不引用进程类型。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/httpx`。

```go
httpx.SetCallID(w, httpx.CallID())
if err != nil {
    httpx.WriteError(w, 400, "invalid_request", err.Error())
    return
}
httpx.WriteJSON(w, 200, map[string]any{"status": "ok"})
```

推理失败时把请求路径传进去，客户端才能看到它期望的形状：

```go
httpx.WriteTypedError(w, r.URL.Path, 429, "rate_limit_error", "slow down")
```

## 这个包不做什么

它不记日志，也不记花费。调用 ID 只是响应头上的值，方便调用方对照。
