# llm

## 这个模块做什么

`llm` 把一次逻辑调用编成供应商要的 HTTP 请求，再把供应商的字节解回对外的 JSON。它不拨号，也不读 PostgreSQL。数据面发送的是这个包造出来的 `Upstream`。

## 功能

- `Build` 为一个 `Request` 返回 URL、请求头和正文。OpenAI 兼容供应商用 OpenAI SDK 的形状。Gemini 和 Vertex 用 Gemini 的形状。Azure 用 OpenAI 的 JSON，但鉴权头是 `api-key`，URL 是部署路径。
- `Encode` 和 `Decode` 是路由适配器调用的较小一对。`Decode` 把调用方的模型别名写回响应。
- `ProtocolGroup` 在这个包里把供应商名字映射到报文分组。不认识的供应商由调用方跳过。
- `Hydrate` 把密钥库里的值填进部署参数。缺的字段保持原值。
- `StripProxyParams` 在发出前去掉只属于网关的字段，避免供应商因为未知参数拒绝请求。
- `ExceptionForStatus` 把上游 HTTP 状态映射成 LiteLLM 的异常名。没有单独分支的状态返回 `ok == false`。
- `Allow` 和 `Filter` 判断哪些 URL 前缀可以作为推理挂载。
- `PassthroughURL` 在正文已经是厂商协议时，把基址和端点拼起来。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/llm`。

```go
group, ok := llm.ProtocolGroup(provider)
if !ok || group == "" {
    // 跳过这个部署，不要假装它是 OpenAI 兼容
    continue
}
up, err := llm.Build(ctx, llm.Request{
    Op: "chat", Provider: provider, APIBase: apiBase, APIKey: apiKey,
    Model: realModel, Body: publicBody,
})
if err != nil {
    return err
}
req, _ := http.NewRequest(http.MethodPost, up.URL, bytes.NewReader(up.Body))
req.Header = up.Header
```

`APIBase` 和 `APIKey` 必须已经由凭证层填好。`Build` 不会把空地址改成厂商的官方域名。

## 这个包不做什么

它不挑选哪个部署胜出，也不记录花费。
