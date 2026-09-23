# API 参考

- Route: `/api-reference`
- Nav: DEVELOPER TOOLS
- Status: `specified`

## 目的

给开发者展示「本网关兼容 OpenAI SDK」的接入方式：把 `base_url` 指到当前 XHub，用虚拟 Key 调 Chat Completions。页顶有弃用横幅（The API Reference tab）。本页是静态 SDK 示例，不是动态路由目录，也不是管理员 Playground。

## 布局

登录后控制台壳（顶栏用户菜单 + 五组侧栏）。内容区全宽约 `h-[80vh]`，内边距 `p-8`。

### 顶栏

左：标题 `OpenAI Compatible Proxy: API Reference`。右：`API Reference Docs` 外链按钮（默认 `https://docs.litellm.ai/docs/proxy/user_keys`，新标签打开）。标题下一段说明：API Key 走 OpenAI SDK，只需替换 `base_url`。

### 筛选

无筛选栏。示例代码里的 `base_url` 来自 `GET /sso/get/ui_settings` 的 `LITELLM_UI_API_DOC_BASE_URL`（优先）或 `PROXY_BASE_URL`；都空则占位 `<your_proxy_base_url>`。

### 表

无数据表。三个 Tab 各自一块只读 `CodeBlock`（Python）。

### Tab

线型 Tab，默认 `openai`：

| Tab | 内容 |
|---|---|
| OpenAI Python SDK | `openai.OpenAI(api_key, base_url)` + `chat.completions.create`，model 示例 `gpt-3.5-turbo` |
| LlamaIndex | AzureOpenAI / AzureOpenAIEmbedding，`azure_endpoint` = 当前 base_url |
| Langchain Py | `ChatOpenAI(openai_api_base=base_url)` |

Tab 内容 `keepMounted`，切换不卸载代码。

### 抽屉

无。

### 模态

无。

### URL

`/api-reference`。无 query。Tab 不写入 URL。

## 交互

进入页：鉴权后拉 proxy UI settings，把示例里的 base_url 换成当前网关。复制代码走 CodeBlock 自带复制。点 Docs 外链。无提交、无写接口。窄屏 Tab 横向滚动，代码块横向溢出滚动。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 读网关 base_url | `GET /sso/get/ui_settings` | 三个 SDK 示例里的 `base_url` / `azure_endpoint` / `openai_api_base` 填成当前代理地址 |
| 打开文档 | 外链 docs | 新标签 |

本页不调用 `GET /utils/available_routes` 或 `GET /utils/supported_openai_params`。

## 字段

| 字段 | 含义 |
|---|---|
| `PROXY_BASE_URL` | 网关对外地址，写入示例 |
| `LITELLM_UI_API_DOC_BASE_URL` | 可选覆盖示例里的 base_url |
| `api_key` | 示例占位 `your_api_key` / `sk-1234`，不是本页生成的密钥 |
| `model` | 示例模型名，需换成本网关已配置别名 |

## 状态

loading：等 proxy settings。forbidden：无登录不进控制台。无 empty 表。无表单校验。窄屏代码块横向滚动。

## 验收

桌面与窄屏能切换三个 SDK Tab，示例 `base_url` 等于当前网关而非占位（settings 有值时）；不得 mock 表格。
