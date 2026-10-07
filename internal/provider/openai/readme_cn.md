# provider/openai

这个目录登记的是适配端点，不是官方内容生成。`init` 只调用 `RegisterType`，不登记供应商，也不登记带价格的模型行。旧部署的 `model_info.mode` 为空时，网关仍把它当成 `chat`。

每一种类型的种类都是 `adapted`，模型字段都是 `model`。`dataplane.Serve` 按 `Operation` 选编码，不走 `ServeBypass`。

| id | 标签 | Operation | 公开路径 | 上游路径 |
| --- | --- | --- | --- | --- |
| `chat` | Chat - /chat/completions | `chat` | POST `/v1/chat/completions` | `/chat/completions` |
| `completion` | Completion - /completions | `completion` | POST `/v1/completions` | `/completions` |
| `embedding` | Embedding - /embeddings | `embedding` | POST `/v1/embeddings` | `/embeddings` |
| `image_generation` | Image Generation - /images/generations | `image` | POST `/v1/images/generations` | `/images/generations` |
| `audio_speech` | Audio Speech - /audio/speech | `audio_speech` | POST `/v1/audio/speech` | `/audio/speech` |
| `rerank` | Rerank - /rerank | `rerank` | POST `/v1/rerank` | `/rerank` |
| `video_generation` | Video Generation - /videos | `videos` | POST `/v1/videos` | `/videos` |

`video_generation` 仍是适配循环里的 `/v1/videos`。火山和七牛的内容生成不在这张表里，它们是 `provider/volcengine` 和 `provider/qiniu` 的 bypass。

添加模型时这些 id 出现在端点类型多选里（`model_info.endpoint_types`）。一个模型可以同时选多种。没选时回退到 `mode`，再没有就是 `chat`。见 `provider.SelectedTypes`。

`internal/provider/all` 空白导入这个包。单独测试某个供应商包时，如果没导入 `all` 或本包，这些类型不会出现在 `provider.Types()` 里。
