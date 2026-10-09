# Upstream construction and protocol adaptation

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

Build, Endpoint, Encode, Decode, and Hydrate construct requests and adapt protocol responses. Build/call/message/gemini handle structures; credential resolves keys; allow/proxy_params/groups constrain parameters; bypass and passthrough describe transport behavior.
This package does not send HTTP, retry, query the database, or record spend. The data plane owns timeout, stream lifetime, and settlement after selecting a deployment. Base URL handling must avoid duplicate or missing /v1 segments.
Proxy-only fields must not leak upstream. Usage, error, and stream semantics preserve zero and cache quantities. New adapters need successful and failing fixtures plus real inference evidence, not merely a nonempty constructed request.

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

Exported types: `Request`, `Upstream`.

- [`func Build(ctx context.Context, in Request) (Upstream, error)`](build.go) — Build builds the upstream request for the protocol group.  OpenAI and providers that only change the base URL and a Bearer key use github.com/openai/openai-go. Gemini and Vertex generateContent use google.golang.org/genai. Azure still uses the OpenAI SDK JSON, but the URL is the deployment path and the auth header is api-key. Anthropic, Cohere, and Bedrock each have their own path and body. They are not posted to /chat/completions.
- [`func ErrUnknownProvider() error`](build.go) — ErrUnknownProvider is errUnknownProvider exported, so a caller can tell "the gateway cannot encode for this provider at all" apart from a specific encoding failure. The two need different messages: the first is a wrong provider name, the second is a request the chosen provider cannot express.
- [`func ChatProbeUsesFixture(provider string) bool`](build.go) — ChatProbeUsesFixture reports that the standard test upstream can answer this chat. Those providers' chat URLs end in /chat/completions, /v1/messages, or :generateContent. Every other protocol group must be checked from the URL and body Build returns. Success cannot be judged from the same OpenAI path.
- [`func RealtimeClientSecretsURL(apiBase string) string`](build.go) — RealtimeClientSecretsURL matches LiteLLM OpenAIRealtimeHTTPConfig.get_complete_url. A custom api_base that ends in /v1 is trimmed first and then joined with /v1/realtime/client_secrets.

### bypass.go

- [`func ResponsesToChat(raw []byte, model string) []byte`](bypass.go) — ResponsesToChat turns one Responses JSON object into a chat completion. A body that is already a chat completion, or that is not a response object, is returned unchanged.
- [`func ResponsesSSEToChat(buf []byte, model string, flush bool) (emit, rest []byte)`](bypass.go) — ResponsesSSEToChat turns complete Responses SSE events into chat completion chunks. rest is the unfinished tail. flush parses that tail and ends the chat stream with [DONE].

### call.go

- [`func Headers(apiKey string) http.Header`](call.go) — Headers 组装访问上游的请求头。有密钥时写 Bearer，并带上调用方允许转发的头。
- [`func DefaultAPIBase(provider string) string`](call.go) — DefaultAPIBase is the official root used when a deployment does not set api_base. OpenAI chat joins /chat/completions onto https://api.openai.com/v1. An unknown provider returns an empty string.
- [`func Endpoint(op, provider, apiBase, model string) string`](call.go) — Endpoint returns the full URL for this operation. apiBase must already be resolved by the credential layer. This function does not replace an empty address with api.openai.com, or an unconfigured deployment would quietly call the official host. The caller should return an authentication error when the address or the key is empty. Path rules match each LiteLLM provider's get_complete_url, folded into one table: OpenAI-compatible providers share the default branch, and Azure, Anthropic, Gemini, and Vertex have their own branches.
- [`func Encode(op, provider string, body map[string]any, model string) ([]byte, error)`](call.go) — Encode turns the public request into the upstream body. The public model is the routing alias. The upstream wants the real model name from the deployment, so model is overwritten here. Other fields are kept as they are. Temperature, tools, and the stream flag come from the caller. This function does not fill defaults, except Anthropic Messages, which fills max_tokens with 256 when it is missing. That field is required by the Messages API, and the upstream returns 400 without it.
- [`func Decode(op, provider, alias string, raw []byte) []byte`](call.go) — Decode turns an upstream response into the public shape and sets model back to the alias the caller used. Audio bytes have no JSON model field and are returned unchanged. The model field on images, rerank, and transcriptions belongs to the result and is not overwritten with the alias. Other JSON objects set model to the alias. A Messages operation then turns a chat.completion into an Anthropic message object.

### credential.go

- [`func Hydrate(params, credentialValues map[string]any) map[string]any`](credential.go) — Hydrate fills deployment parameters that are still empty from a named credential. LiteLLM load_credentials_from_list writes a value only when the key is entirely absent. Deployments in this gateway often store api_key as an empty string after a redacted response is read back, so an empty string also counts as unset. api_key is the exception. The dashboard masks a stored key as ***** and the update handlerkeeps the previous value, so a model can keep a stale key after the user points it at a credential. A non-blank api_key on the credential replaces that stale key. Every other credential fieldstill fills only a blank deployment field, and a non-empty value there still wins. os.environ/NAME inside a string is expanded on this call, not when the credential is saved. The same credential can fo

### error.go

- [`func ExceptionForStatus(status int) (name string, ok bool)`](error.go) — ExceptionForStatus maps an upstream HTTP status to a LiteLLM exception class name. Status 400 and 422 are BadRequestError. Status 408 and 504 are Timeout. A status without its own branch returns ok false so the caller treats it as an unknown upstream error.

### gemini.go

Internal implementation and protocol boundaries:  [gemini.go](gemini.go)。

### groups.go

- [`func ProtocolGroup(provider string) (string, bool)`](groups.go) — ProtocolGroup returns the protocol group for a provider name from the LiteLLM 1.102.0 provider set. Groups follow wire format and authentication, not the old Python package layout. An unknown or empty name returns false.

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
| [call_test.go](call_test.go) | `TestHydrateCredentialAPIKeyReplacesStaleDeploymentKey`, `TestHydrateKeepsDeploymentKeyWhenCredentialHasNone`, `TestDefaultAPIBase` |

```bash
go test ./internal/llm -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
