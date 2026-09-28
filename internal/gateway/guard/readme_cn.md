# gateway/guard

## 这个模块做什么

`guard` 在请求发给上游之前执行内容规则。命中拦截时，数据面不再联系供应商。控制台也可以试跑一条规则，而不消耗供应商额度。

## HTTP 和 Go 入口

- `PreCall` 检查一份 JSON 正文。必须停下来时返回 true 和一条说明。
- `Apply` 是 `Module` 挂上的 HTTP 试跑入口。
- 进程在数据面使用上游 HTTP 客户端之前调用服务器上的 `GuardrailBlocks`，它再委托到这里。

## 调用方怎么用结果

```go
if blocked, message := guard.PreCall(host, body); blocked {
    httpx.WriteTypedError(w, r.URL.Path, 400, "guardrail_violation", message)
    return
}
```

规则是代理上保存的护栏行。标成默认开启的规则即使请求没有点名也会生效。请求里额外点名的护栏会再加进去。

## 这个包不做什么

除非保存的规则要求，否则它不会调用模型给文本分类。默认匹配器是护栏行上的词表。
