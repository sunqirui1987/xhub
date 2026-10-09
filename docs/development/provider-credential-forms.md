# 模型提供商认证表单

模型提供商弹窗先搜索并选择供应商，再填写该类型的连接与认证字段。新建不预选 OpenAI；切换保留名称并清除旧认证参数。默认地址进入实际提交值，保存等待接口完成，失败保留输入。

## 目录来源与维护

认证定义独立于模型价格目录。internal/catalog/publicdata/provider_fields.json 是本地 LiteLLM 1.102.0 的 litellm/proxy/public_endpoints/provider_create_fields.json 快照，包含 118 个条目。公开接口 /public/providers/fields 合并 XHub 注册的本地供应商；相同协议的市场发行方不再重复生成通用表单。快照中的多个独立表单可以共用一个协议。

DeepSeek 参考 https://docs.litellm.com.cn/docs/providers/deepseek ，增加可编辑 API Base，默认 https://api.deepseek.com。OpenAI、Azure、Vertex AI、Google AI Studio、Bedrock、Ollama 等沿用各自字段。更新快照时需复查认证方式，并执行下列验证。

credential_info.provider_id 保存表单唯一标识，custom_llm_provider 保留原协议契约。例如 OpenAI 兼容端点保存 provider_id=OpenAI_Compatible 和 custom_llm_provider=openai，编辑恢复同一表单。旧凭据缺少标识时兼容协议 slug 或显示名。编辑固定类型：后台凭据更新采用字段合并，不适合跨类型替换认证；如需更换类型，创建新的模型提供商。

## 支持边界

目录表示认证配置方式。字段齐全不等于所有 LiteLLM 执行适配器、AWS 签名、Azure AD、Vertex 服务账号或第三方账户均已在 XHub 移植。无普通认证字段的条目（例如 ChatGPT Subscription）提示单独配置并禁止保存。此次未实现 OAuth 或云平台原生认证链，也未调用付费外部供应商。

## 验证方式

- 前端：在 frontend 运行 npx vitest run --project component src/components/model_add/CredentialModal.test.tsx src/components/model_add/CredentialsPanel.test.tsx src/components/add_model/provider_specific_fields.test.tsx。
- 目录：go test ./internal/catalog -count=1 -json。
- 浏览器：bash scripts/e2e.sh provider-forms.spec.ts model-credential-protocol.spec.ts model-discovery.spec.ts。


报告保存到 .e2e/provider-forms/（单元 JSON、目录 JSON、后台日志、浏览器日志与截图）。浏览器机器报告为 .e2e/current/results.json、.e2e/current/junit.xml，HTML 为 frontend/playwright-report/index.html。

## 供应商能力清理

正式供应商目录仅来自 LiteLLM 字段快照与注册的官方适配器。已删除中转专用适配器、启动种子凭据、环境变量密钥回退、地址推断和目录型号注入。用户保存的普通 OpenAI 兼容连接继续按显式地址与协议使用，名称或旧 builtin 元数据不再升级协议。

官方 Ark 使用 volcengine、ark_contents_generation 及 /api/v3/contents/generations/tasks。移除的传输无法创建新部署；不会删除用户已有凭据或模型。

嵌入价格保留现有通用模型费率，不代表已重新审核为厂商官方价格。删除中转别名与元数据；实际账号价格可人工覆盖。刷新只读取显式 XHUB_PRICE_FEED_URL，缺失时返回错误，定时计划拒绝启用。
