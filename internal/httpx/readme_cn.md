# httpx

网关和数据面共用的一小套 HTTP 工具。这里没有路由。

`CallID` 读 16 个随机字节再编成十六进制。`SetCallID` 写响应头 `x-litellm-call-id`，不写正文。`WriteTypedError` 在这个头还空着时补上，在 `x-litellm-version` 还空着时写成 `xhub-dev`，并设置 `Content-Type: application/json`。状态 429 还会设置 `Retry-After: 1`。

JSON 形状看路径，先转成小写：

- 路径含 `/messages`，并且不含 `chat`、不含 `/threads` 时，用 Anthropic 包络（`{"type":"error","error":{"type","message"}}`）。
- 路径含 `generatecontent` 或 `streamgeneratecontent`，或者含 `counttokens` 且不含 `/messages` 时，用 Gemini 原生包络（`error.code`、`error.message`、`error.status` 为 Google RPC 名字）。
- 其他情况用 OpenAI 对象：`error.message`、`error.type`、`error.param` 为 null、`error.code` 是状态码的字符串。

`WriteError` 就是路径为空的 `WriteTypedError`，所以永远是 OpenAI 对象。推理路径应把真实路径传给 `WriteTypedError`。

`WriteJSON` 给管理接口的成功响应写状态码和 JSON 正文。

`Bind` / `Module` / `Registrar` 让网关子包不用导入 `gateway` 就能挂路由。`Mount` 在登记口或挂载函数是 nil 时什么都不做。`gateway/routes.go` 在安装模块时调用 `Name` 和 `Mount`。先登记的方法和路径赢；后面的模块不会替换它。

## 这个包不做什么

它不记用量，不选部署，也不做身份认证。只调用了 `SetCallID` 的处理函数还没有回答客户端。

English notes are in `readme.md` in this directory.
