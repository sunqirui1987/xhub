# httpx

Small HTTP helpers shared by the gateway and the data plane. No routes live here.

`CallID` reads 16 random bytes and hex-encodes them. `SetCallID` writes the header `x-litellm-call-id` and does not write a body. `WriteTypedError` fills that header when it is still empty, sets `x-litellm-version` to `xhub-dev` when that header is empty, and sets `Content-Type: application/json`. Status 429 also sets `Retry-After: 1`.

The JSON shape depends on the path, lowercased:

- Anthropic envelope (`{"type":"error","error":{"type","message"}}`) when the path contains `/messages` and does not contain `chat` or `/threads`.
- Gemini native envelope (`error.code`, `error.message`, `error.status` as a Google RPC name) when the path contains `generatecontent` or `streamgeneratecontent`, or contains `counttokens` without `/messages`.
- Otherwise the OpenAI object: `error.message`, `error.type`, `error.param` null, `error.code` the status as a string.

`WriteError` is `WriteTypedError` with an empty path, so it always uses the OpenAI object. Inference paths should call `WriteTypedError` with the real path.

`WriteJSON` writes a status and a JSON body for successful management responses.

`Bind` / `Module` / `Registrar` are how gateway subpackages mount routes without importing `gateway`. `Mount` no-ops when the registrar or the mount function is nil. `gateway/routes.go` calls `Name` and `Mount` while installing modules. The first method and path wins; a later module does not replace it.

## What this package does not do

It does not record spend, choose a deployment, or authenticate. A handler that only calls `SetCallID` has not answered the client.

中文说明见同目录 `readme_cn.md`。
