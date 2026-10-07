# gateway/family

不是管理模块的那些 OpenAI 形状家族：responses、files、batches、assistants，以及 `ingress` 没有单独处理的嵌入目录路径。

`family.Module` 把 POST `/v1/responses` 和 POST `/responses` 登记到 `Responses`，再以 responses 操作进入 `dataplane.Serve`。其他目录路径由 `ingress.go` 挂上，并回调这个包。

`resourceKind` 从路径读集合名：`files`、`batches`、`assistants`、`threads`、`fine_tuning`、`containers`、`vector_stores`、`videos`，以及 search、ocr、rag、mcp 前缀。认不出的路径是 `resources`。`idField` 是这种资源的行 id 字段名（`credential_name`、`guardrail_id`、`user_id` 等）。没有专门名字的种类用单数名加 `_id`。`aliasField` 是显示名字段（`guardrail_name`、`agent_name` 等）。没有专门约定的种类用单数名加 `_alias`，不是空串。

`ServeMixed` 处理被分成 mixed 的目录路径（`/v1/agents`、`/v1/skills`、`/v1/workflows` 以及同一类）。它仍然先要身份，然后拒绝。这些路径后面的通用存储是一个没有属主、没有团队列的键值命名空间，提供出来会让任何已登录成员和任何推理密钥读到别人的行。拒绝是故意的。

仍会存储的行（files、batches，以及不是 mixed 的种类）进 `RecordStore` 键值。`Freeze` 在缺省时补上 id、状态和时间戳，并删掉 `password`。`mergeCredentialPatch` 覆盖普通字段，不把 `password` 或 `credential_values` 整段替换掉。

## 这个包不做什么

它不实现七牛或火山的内容生成接口。那些是 `provider` 里的 bypass 匹配和 `dataplane.ServeBypass`，不会进入 `resourceKind`。端点类型 `video_generation` 是适配的 `/v1/videos`，那个会进入 `Serve`。

English notes are in `readme.md` in this directory.
