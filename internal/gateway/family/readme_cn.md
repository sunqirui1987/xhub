# gateway/family

处理非管理类目录入口。Responses 进入适配推理；通用资源要求具备推理权限的身份，以密钥凭证哈希或会话用户 ID 构造独立存储命名空间，不能跨调用方访问。

只支持本地 assistants、threads 元数据创建和更新。嵌套线程操作、真实文件上传、batch、微调、count-token/realtime 占位及其他供应商执行必须有真实实现，否则返回 501。列表、读取、删除只操作本调用方元数据；不伪造文件内容。旧的全局资源记录不通过新命名空间暴露。

非法 JSON 返回 400，资源不存在 404，存储错误 500，存储不可用 503。agents、skills、workflows 等 mixed 入口仍拒绝，因为通用存储没有对应权限模型。

七牛/火山官方任务走 provider 和 dataplane.ServeBypass。元数据 CRUD 不等于 OpenAI assistants 执行或完整异步状态机。

证据：resource_isolation_test.go、gateway/catalog_reads_test.go。
