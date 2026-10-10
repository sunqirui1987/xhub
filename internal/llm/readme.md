# Upstream construction and protocol adaptation

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

Dialogue conversion, credential hydration, request filtering, and passthrough helpers support the data plane. `dialogue.go` converts supported chat protocols through a common structure; `credential.go` fills deployment parameters; `allow.go` and `proxy_params.go` constrain requests; `passthrough.go` joins transport URLs. `call.go` only defines operation-name constants.
This package does not send HTTP, retry, query the database, or record spend. The data plane owns timeout, stream lifetime, and settlement after selecting a deployment. Base URL handling belongs to the selected provider implementation.
Proxy-only fields must not leak upstream. Usage, error, and stream semantics preserve zero and cache quantities. New adapters need successful and failing fixtures plus real inference evidence.

## Subdirectories and collaboration

| Directory | Responsibility |
| --- | --- |
| [estimate](estimate/readme.md) | Token and cost estimation |

## Source responsibilities and entry points

### allow.go

- [`func PathPrefixes() []string`](allow.go) — PathPrefixes returns the prefixes in the allowlist.py order so a test can compare them with the Python source.
- [`func Allow(path string, mount bool) bool`](allow.go) — Allow reports whether a path may stay on the gateway process. When mount is true only the mount table is checked, matching a FastAPI Mount. An ordinary route checks an exact path first, then a prefix. An empty path is not a data-plane path. The comparison uses the path from route registration, not a URL with a query string.
- [`func Filter(next http.Handler) http.Handler`](allow.go) — Filter drops management routes before the process serves them. The Python gateway trims the route table after startup, once registration is finished. Here the request has already reached a registeredhandler, and the same list decides whether to allow it or return 404. The effect matches: the data plane stays, and management routes such as /key/generate are not served by this filter.

### build.go

Exported type: `Upstream`, the URL, headers, and body prepared for an upstream call. Construction is owned by the selected provider implementation.

原生 Responses 的转发、usage 与 continuation 由 [provider/openai](../provider/openai/readme.md) 和 [dataplane](../dataplane/readme.md) 负责，不能用已经移除的 Responses-to-chat 转换函数作为验证入口。

### call.go

This file defines the `Op*` operation constants shared by routing and provider implementations. It does not choose a default API base, build headers, or encode provider requests.

### credential.go

- [`func Hydrate(params, credentialValues map[string]any) map[string]any`](credential.go) — Hydrate fills deployment parameters that are still empty from a named credential. LiteLLM load_credentials_from_list writes a value only when the key is entirely absent. Deployments in this gateway often store api_key as an empty string after a redacted response is read back, so an empty string also counts as unset. api_key is the exception. The dashboard masks a stored key as ***** and the update handlerkeeps the previous value, so a model can keep a stale key after the user points it at a credential. A non-blank api_key on the credential replaces that stale key. Every other credential fieldstill fills only a blank deployment field, and a non-empty value there still wins. os.environ/NAME inside a string is expanded on this call, not when the credential is saved. The same credential can fo

### dialogue.go

Exported types: `Turn`, `Dialogue`, `DialogueResult`.

- [`func ParseDialogue(protocol string, body map[string]any) (Dialogue, error)`](dialogue.go) — Converts a supported protocol request into the common dialogue structure and rejects unsupported protocols.
- [`func EncodeDialogue(d Dialogue, protocol, model string) (map[string]any, error)`](dialogue.go) — Converts the common dialogue into the selected upstream protocol and model.
- [`func ParseDialogueResult(protocol string, raw []byte) (DialogueResult, error)`](dialogue.go) — Parses a supported upstream response without discarding protocol usage facts.
- [`func EncodeDialogueResult(r DialogueResult, protocol, model string) ([]byte, error)`](dialogue.go) — Emits the selected public response protocol and model name.
- [`func NormalizeDialogueUsage(u map[string]any) map[string]any`](dialogue.go) — Preserves explicit zero and cache-token quantities while normalizing usage fields.

### error.go

- [`func ExceptionForStatus(status int) (name string, ok bool)`](error.go) — ExceptionForStatus maps an upstream HTTP status to a LiteLLM exception class name. Status 400 and 422 are BadRequestError. Status 408 and 504 are Timeout. A status without its own branch returns ok false so the caller treats it as an unknown upstream error.

### gemini.go

Internal implementation and protocol boundaries:  [gemini.go](gemini.go)。

供应商协议分类由 [provider/capability.go](../provider/capability.go) 的能力定义和 [build.go](build.go) 的分派处理，未知协议必须沿真实失败路径验证。

### message.go

- [`func IsWildcardModel(model string) bool`](message.go) — IsWildcardModel reports whether a model name is a wildcard route.  LiteLLM uses * in a name to mean a group of models, so openai/* can catch openai/gpt-4o. An exact name wins. The router looks at these patterns only when no exact deployment exists.
- [`func NormalizeStreamOptions(opts map[string]any) (includeObfuscation bool, ok bool)`](message.go) — NormalizeStreamOptions reads include_obfuscation from Responses stream options. The field must be a boolean. A missing field, a wrong type, or options that are not an object all mean the caller did not give a usable switch, so ok is false. Do not treat the default as false and write it back onto the request.
- [`func ShapeResponsesMessage(message map[string]any) map[string]any`](message.go) — ShapeResponsesMessage reshapes one chat message into a form the Responses API accepts. An assistant message stays as it is. A user message whose content is a list of blocks, with a type of text, rewrites those blocks to input_text. Responses does not accept the chat-completion text type. Without such a block the message is returned unchanged, so a message that is already input_text is not wrappedagain.

### passthrough.go

- [`func PassthroughURL(base, endpoint, provider string) string`](passthrough.go) — PassthroughURL is the outbound address produced by LiteLLM _join_url_paths. It first joins the subpath onto the api_base path and rejects "..". If an OpenAI result still has no /v1/, v1 is inserted after api.openai.com/.
- [`func PassthroughSubpath(base, subpath string, include bool) string`](passthrough.go) — PassthroughSubpath matches HttpPassThroughEndpointHelpers.construct_target_url_with_subpath. When include is false or the subpath is empty, base is returned unchanged. Otherwise the subpath is normalized and joined.

### proxy_params.go

- [`func StripProxyParams(body map[string]any)`](proxy_params.go) — StripProxyParams deletes LiteLLM proxy fields and leaves the body the provider understands.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [allow_test.go](allow_test.go) | `TestAllowWithoutPublicSchema`, `TestFilterRejectsPublicSchema` |
| [call_test.go](call_test.go) | `TestHydrateCredentialAPIKeyReplacesStaleDeploymentKey`, `TestHydrateKeepsDeploymentKeyWhenCredentialHasNone` |
| [dialogue_test.go](dialogue_test.go) | `TestDialogueProtocolMatrix`, `TestDialogueRejectsUnsupported`, `TestDialogueResultMatrix` |
| [dialogue_google_test.go](dialogue_google_test.go) | `TestGoogleDialogueBoundaries`, `TestGoogleResultBoundaries` |

```bash
go test ./internal/llm -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
