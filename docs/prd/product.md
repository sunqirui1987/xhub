# 产品定义

XHub 是自托管 AI 网关。调用方把 SDK 的 `base_url` 指到本网关、使用虚拟 Key，即可调用 100+ 模型供应商，并得到统一的认证、路由、限流、预算、转换、计费与日志。

运转细节（鉴权顺序、生命周期、计量失败）见 [怎么运转](runtime.md)，不要只靠本节菜单列表实现。

## 产品面（分母，不可缩小）

**管理控制台五组导航**

1. AI Gateway：虚拟密钥、调试台、模型与端点、Agents、工作流、Memory、MCP、Skills、Guardrails、Policies、Search Tools、Vector Stores、Tool Policies
2. Observability：用量、成本优化、请求日志、Guardrails 监控
3. Access Control：团队、项目、内部用户、组织、访问组、预算
4. Developer Tools：API 参考、AI Hub、响应缓存、Prompts、请求转换、标签
5. Settings：路由设置、日志与告警、管理员设置、成本跟踪、界面主题

**另外必须有的产品面**

- 终端用户 Chat 壳（`/chat` 及其子页：密钥、凭证、集成、日志、用量）
- 公开模型目录（`/model_hub`、`/model_hub_table`）
- 登录、开通邀请、连接引导、MCP OAuth 回调、控制台根路径

**数据面协议**

Chat Completions、Text Completions、Anthropic Messages、Responses、Embeddings、Images、Audio、Moderations、Rerank、Files、Batches、Assistants/Threads、Fine-tuning、Containers、Vector Stores、Videos、Realtime、Search/OCR/RAG、Skills/Tools/Memory、Evals、Workflows、Agents、MCP、A2A、Gemini generateContent、供应商透传、模型列表。

完整 method+path 见 [后端 API](../backend-api/README.md)。

## 不是什么

- 不是训练平台。
- 不是自选子集网关。分母以上述产品面为准。
- 实现分期见 [delivery.md](delivery.md)，分期不是砍需求。
