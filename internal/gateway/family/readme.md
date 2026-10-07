# gateway/family

Handlers for the OpenAI-shaped families that are not the management modules: responses, files, batches, assistants, and the rest of the embedded catalog that `ingress` does not special-case.

`family.Module` registers POST `/v1/responses` and POST `/responses` onto `Responses`, which enters `dataplane.Serve` with the responses operation. Other catalog paths are mounted by `ingress.go` and call back into this package.

`resourceKind` reads the collection from the path: `files`, `batches`, `assistants`, `threads`, `fine_tuning`, `containers`, `vector_stores`, `videos`, and the search, ocr, rag, and mcp prefixes. An unrecognized path is `resources`. `idField` is the row id name for that kind (`credential_name`, `guardrail_id`, `user_id`, …). Kinds without a special case use the singular name plus `_id`. `aliasField` is the display-name field (`guardrail_name`, `agent_name`, …). A kind with no special case uses the singular name plus `_alias`, not an empty string.

`ServeMixed` is the handler for catalog paths classified as mixed (`/v1/agents`, `/v1/skills`, `/v1/workflows`, and the same class). It still requires an identity, then refuses. The generic store behind those paths is one key-value namespace with no owner and no team column, so serving them would let any signed-in member, and any inference key, read every other principal's rows. The refusal is intentional.

Rows that are still stored (files, batches, and the kinds that are not mixed) go to `RecordStore` key-value. `Freeze` fills id, status, and timestamps when absent and deletes `password`. `mergeCredentialPatch` overlays ordinary fields and does not replace `password` or `credential_values` as a whole.

## What this package does not do

It does not implement the Qiniu or Volcengine contents APIs. Those are bypass matches in `provider` and `dataplane.ServeBypass`, and they never enter `resourceKind`. A `video_generation` endpoint type is the adapted `/v1/videos` operation, which does enter `Serve`.

中文说明见同目录 `readme_cn.md`。
