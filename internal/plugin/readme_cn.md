# plugin

## 这个模块做什么

`plugin` 是调用上游之前的扩展点。扩展能看到操作、模型和路径，可以改响应头，也可以拒绝这次调用。拒绝之后，数据面不读缓存，也不联系上游。这不是加载 `.so` 的打开器。你按名字注册一个 Go 值。

## 功能

- `New` 返回空注册表。空表上的 `Run` 直接放行。
- `Register` 把扩展加到末尾。重名会失败，原有顺序不变。
- `Run` 按顺序调用全部扩展。第一个拒绝会停住后面的扩展，并保留已经写下的响应头。
- `Invoke` 按注册时的名字调用一个扩展。
- `Names` 按顺序返回名字的副本。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/plugin`。

```go
type refuseChat struct{}

func (refuseChat) Name() string { return "gate-refuse" }
func (refuseChat) BeforeUpstream(c plugin.Call) plugin.Decision {
    if c.Op == "chat" && c.Model == "blocked-model" {
        return plugin.Decision{Refuse: true, Status: 403, Code: "extension_refused", Message: "blocked"}
    }
    return plugin.Decision{}
}

reg := plugin.New()
if err := reg.Register(refuseChat{}); err != nil {
    log.Fatal(err)
}
decision := reg.Run(plugin.Call{Op: "chat", Model: "blocked-model", Path: "/v1/chat/completions"})
if decision.Refuse {
    httpx.WriteError(w, decision.Status, decision.Code, decision.Message)
    return
}
```

网关只保留一份注册表，用 `Extensions` 交出去。在开始接流量之前把扩展注册到这个值上。空注册表不改变聊天、花费、冷却，也不改变对未知供应商的跳过。

## 这个包不做什么

它不懂 HTTP，也不引用网关。数据面调用 `Run`，并遵守 `Refuse`。
