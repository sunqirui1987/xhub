# HTTP 契约

数据面给 SDK，管理面给控制台。每族含：目的、HTTP、请求头/体、响应头/体、错误、副作用、控制台绑定、验收。

## 数据面

| 分组 | 目录 |
|---|---|
| 推理 | [data/inference](data/inference/) Chat / Completions / Messages / Responses / Embeddings / Moderations / Rerank / Gemini |
| 媒体 | [data/media](data/media/) Images / Audio / Videos / Realtime |
| 资源 | [data/resources](data/resources/) Files / Batches / Assistants / Fine-tune / Containers / Vector / Search |
| 平台 | [data/platform](data/platform/) Agents / MCP / A2A / Workflows / Skills / Hub |

## 管理面

| 分组 | 目录 |
|---|---|
| 身份 | [management/identity](management/identity/) Keys / Users / Teams / Orgs / Projects / Budgets |
| 目录 | [management/catalog](management/catalog/) Models / Credentials |
| 治理 | [management/governance](management/governance/) Guardrails / Policies / Prompts / Tags |
| 观测 | [management/observability](management/observability/) Spend / Logs |
| 平台 | [management/platform](management/platform/) MCP 管理、SSO、Health、Config… |
