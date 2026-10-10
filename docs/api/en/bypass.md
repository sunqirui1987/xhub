# Native protocol Bypass

Choose an endpoint to read authentication, parameters, examples and responses, then open its online runner. Each endpoint has its own Markdown source. Only registered paths and permitted deployments can be called.

## Native endpoints

| Method | Endpoint | Path |
| --- | --- | --- |
| POST | [Bypass Anthropic Messages](native-anthropic) | /bypass/anthropic/v1/messages |
| POST | [Bypass Vertex generateContent](native-vertex) | /bypass/vertex/v1/models/YOUR_MODEL_NAME:generateContent |
| POST | [Bypass Gemini generateContent](native-gemini) | /bypass/gemini/v1beta/models/YOUR_MODEL_NAME:generateContent |
| POST | [Bypass OpenAI image generation](native-images) | /bypass/openai/v1/images/generations |
| POST | [Bypass OpenAI image edits](native-image-edits) | /bypass/openai/v1/images/edits |
| POST | [Bypass OpenAI Responses](native-responses) | /bypass/openai/v1/responses |
| POST | [Bypass OpenAI Chat Completions](native-chat) | /bypass/openai/v1/chat/completions |

## Endpoint and authentication

Use an XHub API key: Authorization: Bearer <XHUB_API_KEY>. Do not use upstream provider credentials as gateway keys.

Use the gateway root Base URL shown in the playground, without appending /v1; send requests to the full endpoint path above.

## Request parameters

This is a protocol guide. Use the corresponding endpoint article for its parameters.

## Request example

See the executable example in the corresponding endpoint article.

## Success response

Responses follow the selected native protocol and depend on the endpoint and upstream model.

## Errors and troubleshooting

| HTTP | Meaning |
| --- | --- |
| 400 | Invalid request. Check JSON, parameters and query. |
| 401 | Missing or invalid XHub credential. |
| 403 | Permission denied. Check model scope or policy. |
| 429 | Limit exceeded. Inspect response details and Retry-After. |

## Capabilities and billing boundaries

Parameter support depends on the deployed model and conversion or Bypass path; upstream extension fields are not universally supported. Use the implemented protocol adapter and upstream response as the contract. Usage details are subsets of input/output totals and must not be added twice; reconcile costs against the billing snapshot in gateway logs. Example responses illustrate fields only.

## Capability and protocol selection

Bypass is a transport mechanism, not a model family. Select a deployment supporting the registered protocol.

| Capability | Guide |
| --- | --- |
| Chat | [OpenAI](chat-completions), [Anthropic](messages), [Gemini](gemini) |
| Images | [Generation](images), [Edits](image-edits) |
| Video | [OpenAI](videos), [Content generation](ark), [Seedance Fal](fal-seedance) |
