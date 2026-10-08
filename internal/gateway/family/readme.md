# gateway/family

Handlers for non-management catalog families. Responses enters adapted inference. Generic resources require an authorized inference principal; the key credential hash or session user ID defines a hashed storage namespace, preventing cross-caller access.

Only assistants and threads metadata can be created or updated locally. Nested thread operations, real files/uploads, batches, fine-tuning, count-token/realtime placeholders and other provider work must have a real implementation and return 501 when unsupported. Lists, reads and deletes operate only on caller-owned metadata; file content is not fabricated. Old global resource records are not exposed through the new namespace.

Malformed JSON returns 400. Missing records return 404; storage errors return 500, unavailable storage 503. Mixed catalog families such as agents, skills and workflows remain refused because the generic store does not supply their authorization model.

This package does not implement official Qiniu/Volcengine contents tasks, which use provider matching and dataplane.ServeBypass. Metadata storage does not implement OpenAI assistants execution or a complete async state machine.

Evidence: resource_isolation_test.go and gateway/catalog_reads_test.go. 中文说明见 readme_cn.md。
