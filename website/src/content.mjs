export const repository = "https://github.com/sunqirui1987/xhub";
export const quickstart = "git clone https://github.com/sunqirui1987/xhub.git\ncd xhub\nbash deploy/build.sh\ndocker compose up -d --no-build";
export const integration = `from openai import OpenAI

client = OpenAI(
    api_key="YOUR_XHUB_VIRTUAL_KEY",
    base_url="http://localhost:4000/v1",
)

response = client.chat.completions.create(
    model="YOUR_PUBLIC_MODEL_NAME",
    messages=[{"role": "user", "content": "Hello, XHub!"}],
)
print(response.choices[0].message.content)`;

export const copy = {
  zh: {
    language: "简体中文", switch: "EN", tag: "企业 AI 网关 · 自托管", features: "产品能力", useCases: "应用场景", docs: "帮助文档", start: "开始使用", github: "查看 GitHub", search: "搜索文档", menu: "打开导航", close: "关闭", copy: "复制代码", copied: "已复制", copyFailed: "复制失败，请手动选择代码",
    hero: "让企业 AI，", heroAccent: "有序生长。", intro: "从模型接入到业务落地，一个网关管理模型、权限、费用与安全。让平台团队掌握全局，让每个业务团队用好 AI。", heroNote: "Go 网关 / 中英文控制台 / Docker Compose", heroDoc: "阅读部署指南",
    strip: ["多协议接入", "组织与团队治理", "可编程护栏", "Agent 与多模态"],
    eyebrow: "BUILT FOR YOUR ORGANIZATION", featureTitle: "统一入口，更完整的企业能力。", featureIntro: "不止转发一次请求。将模型资源、使用者、策略与调用结果连接起来，覆盖企业 AI 的日常管理。",
    capabilities: [
      ["01", "多模型，多协议", "OpenAI Chat / Responses、Anthropic Messages、Gemini / Vertex。统一公开模型名，按能力适配协议或原生转发。", "model-endpoints.md", "接入方式"],
      ["02", "组织治理，团队自治", "组织、团队、项目与分级角色；个人和服务虚拟密钥。按模型范围、有效期和成员关系管理访问。", "permissions.md", "权限规则"],
      ["03", "费用有归属，计价有依据", "按用户、密钥、团队、项目查看用量。预算、RPM / TPM、缓存读写、时段费率和历史价格快照。", "pricing.md", "费用与计价"],
      ["04", "让路由服从业务", "权重、负载、延迟、费用与用量策略；组织继承模板，组合路由组、分类回退、重试和冷却。", "routing.md", "路由策略"],
      ["05", "把安全规则写进调用链", "内置文本规则、密钥检测与外部审核适配。用 XGo 组合 HTTP、JSON、LLM 检查，执行拦截、脱敏或告警。", "guardrails.md", "可编程护栏"],
      ["06", "面向 Agent 的连续调用", "多轮对话与工具调用结构，Responses 历史续接，会话与调用追踪。在 Playground 验证流式结果与模型对比。", "runtime.md", "会话与运行"],
      ["07", "从对话走向多模态", "按部署能力连接图片、音频、Embedding、Rerank 与视频任务；跟踪异步状态、结果、用量和结算。", "qiniu-fal.md", "媒体任务"],
      ["08", "看得见，才能管得好", "模型目录、接入诊断、调用与错误日志、费用明细、管理审计和健康检查，连接配置与实际运行。", "operations.md", "运行与排查"],
    ],
    differenceLabel: "THE XHUB DIFFERENCE", differenceTitle: "把特色放进真实业务。", differences: [
      ["规则可以编程", "用 XGo 将企业检查编排为调用前护栏，草稿调试与实际调用共用执行器。", "guardrails.md", "查看脚本与示例"],
      ["策略沿组织传递", "密钥、团队、组织逐层选择路由模板，清除绑定恢复继承；模板不会扩大模型权限。", "routing.md", "理解模板继承"],
      ["一次调用，一份费用依据", "记录当次价格快照，区分响应缓存与提示词缓存，支持多维计价和异步任务结算去重。", "pricing.md", "了解计价机制"],
    ],
    consoleLabel: "ONE WORKSPACE", consoleTitle: "平台团队与业务团队，各得其所。", consoleIntro: "管理员配置共享资源，成员发现可用模型。用同一套控制台完成接入、验证与追踪。", screenshot: "XHub 模型目录控制台", screenshotNote: "已配置实例的实际界面；新安装需先添加供应商和模型部署。",
    flowLabel: "FROM SETUP TO SCALE", flowTitle: "一条清晰的落地路径。", flow: [["接入模型", "添加供应商凭证与部署，发布模型名。"], ["建立边界", "配置团队、成员、模型权限与预算。"], ["接入业务", "生成虚拟密钥，Playground 验证后集成。"], ["持续治理", "核对用量与费用，调整策略并追踪问题。"]],
    casesTitle: "从一个应用，到整个组织。", cases: [["企业 AI 平台", "统一模型目录与部门预算，让共享资源有清晰的管理边界。"], ["业务助手与编码 Agent", "管理服务密钥、工具调用与多轮会话，追踪使用情况。"], ["RAG 与内容生产", "将检索、对话和媒体任务纳入同一入口与用量体系。"]],
    installLabel: "START ON YOUR INFRASTRUCTURE", installTitle: "在自己的环境，开始第一步。", installIntro: "克隆仓库，在主机上构建镜像，再通过 Docker Compose 启动网关、控制台与存储。", requirements: "需要 Git、Go 1.25、Node.js ≥24.14.1、npm ≥11.10.0、Docker Compose、Bash 和 /etc/ssl/cert.pem。Windows 请使用 WSL2。", installDocs: "完整安装与首次调用", console: "控制台", gateway: "网关 API",
    docsLabel: "DOCUMENTATION", docsTitle: "从了解产品，到用好每个能力。", docsIntro: "安装、管理、协议、权限、安全与运维。文档直接来自仓库，随版本同步更新。", docCards: [["快速开始", "环境准备、Docker 安装、源码运行与首次调用。", "getting-started.zh-CN.md"], ["使用与管理", "模型、组织、团队、密钥、路由与费用核对。", "user-guide.zh-CN.md"], ["接口与集成", "查看已登记端点、协议能力与请求入口。", "development/api-reference.md"]], allDocs: "浏览全部文档",
    footer: "企业 AI 接入与治理平台", status: "能力与限制", statusText: "具体协议和媒体操作以部署能力为准。护栏当前为调用前文本检查；预算与多实例运行边界见运行参考。", noResults: "没有找到相关文档", searchHint: "输入关键词搜索全文，例如：预算、路由、Docker", searchError: "搜索索引加载失败，请使用文档目录或稍后重试。", searching: "正在加载文档索引…", toc: "本页内容", source: "在 GitHub 查看原文", documentHome: "文档首页", moduleDocs: "模块参考", chineseReference: "本篇参考文档使用中文。",
  },
  en: {
    language: "English", switch: "中文", tag: "ENTERPRISE AI GATEWAY · SELF-HOSTED", features: "Capabilities", useCases: "Use cases", docs: "Documentation", start: "Get started", github: "View on GitHub", search: "Search docs", menu: "Open navigation", close: "Close", copy: "Copy code", copied: "Copied", copyFailed: "Copy failed. Please select the code manually.",
    hero: "Give enterprise AI", heroAccent: "room to grow.", intro: "One gateway for models, access, cost, and security. Give platform teams a shared view and business teams the tools to put AI to work.", heroNote: "Go gateway / Bilingual console / Docker Compose", heroDoc: "Read the setup guide",
    strip: ["Multiple protocols", "Organization governance", "Programmable guardrails", "Agents & multimodal"],
    eyebrow: "BUILT FOR YOUR ORGANIZATION", featureTitle: "One endpoint. A broader enterprise toolkit.", featureIntro: "Connect model resources, people, policies, and request outcomes for the everyday work of managing enterprise AI.",
    capabilities: [
      ["01", "Models & protocols", "OpenAI Chat / Responses, Anthropic Messages, and Gemini / Vertex. Public model names with supported conversion or native forwarding.", "model-endpoints.md", "Model access"],
      ["02", "Governance & autonomy", "Organizations, teams, projects, and scoped roles. Personal and service virtual keys with model permissions, expiration, and access revocation.", "permissions.md", "Permissions"],
      ["03", "Costs with clear ownership", "Usage by user, key, team, and project. Budgets, RPM / TPM, cache rates, time windows, and historical price snapshots.", "pricing.md", "Pricing & usage"],
      ["04", "Routing for each business", "Weight, load, latency, cost, and usage policies. Inherited templates, model groups, categorized fallbacks, retries, and cooldowns.", "routing.md", "Routing policies"],
      ["05", "Policies in the request path", "Text rules, secret detection, and moderation adapters. Combine HTTP, JSON, and LLM checks in XGo to block, redact, or flag.", "guardrails.md", "Programmable guardrails"],
      ["06", "Continuous agent calls", "Multi-turn messages and tool-call structures, Responses history continuation, and session tracing. Validate streams and compare models in Playground.", "runtime.md", "Sessions & runtime"],
      ["07", "Beyond conversation", "Deployment-specific images, audio, embeddings, reranking, and video tasks. Track asynchronous status, results, usage, and settlement.", "qiniu-fal.md", "Media tasks"],
      ["08", "Visibility for operations", "Model catalogs, configuration diagnostics, request and error logs, cost details, audit records, and health checks.", "operations.md", "Operations"],
    ],
    differenceLabel: "THE XHUB DIFFERENCE", differenceTitle: "Distinct capabilities. Practical outcomes.", differences: [
      ["Policies you can program", "Compose enterprise checks in XGo pre-call guardrails. Draft testing and real calls share the same executor.", "guardrails.md", "Explore scripts & examples"],
      ["Routing along your organization", "Select templates through key, team, and organization scopes. Clear a binding to inherit; routing never expands model permissions.", "routing.md", "Understand inheritance"],
      ["Pricing evidence for every call", "Keep applied price snapshots, distinguish response and prompt caches, and deduplicate asynchronous task settlement.", "pricing.md", "Understand pricing"],
    ],
    consoleLabel: "ONE WORKSPACE", consoleTitle: "Shared resources. Scoped responsibilities.", consoleIntro: "Administrators configure resources. Members discover available models. Use one console to connect, validate, and trace requests.", screenshot: "XHub model catalog console", screenshotNote: "Actual interface from a configured instance. Fresh installations need provider credentials and deployments.",
    flowLabel: "FROM SETUP TO SCALE", flowTitle: "A clear path to adoption.", flow: [["Connect models", "Add credentials and deployments. Publish model names."], ["Set boundaries", "Configure teams, members, access, and budgets."], ["Integrate apps", "Issue virtual keys. Validate in Playground."], ["Keep governing", "Review costs, adjust policies, and trace failures."]],
    casesTitle: "From one application to your organization.", cases: [["Enterprise AI platform", "Share a model catalog with departmental budgets and clear resource ownership."], ["Assistants & coding agents", "Manage service keys, tool-call structures, and multi-turn usage."], ["RAG & content production", "Bring retrieval, dialogue, and media tasks into a shared endpoint and usage system."]],
    installLabel: "START ON YOUR INFRASTRUCTURE", installTitle: "Start in your own environment.", installIntro: "Clone the repository, build images on your host, then start the gateway, console, and stores with Docker Compose.", requirements: "Requires Git, Go 1.25, Node.js ≥24.14.1, npm ≥11.10.0, Docker Compose, Bash, and /etc/ssl/cert.pem. Use WSL2 on Windows.", installDocs: "Complete setup & first request", console: "Console", gateway: "Gateway API",
    docsLabel: "DOCUMENTATION", docsTitle: "Understand the platform. Use its capabilities.", docsIntro: "Setup, administration, protocols, permissions, security, and operations. Built directly from repository documentation.", docCards: [["Quickstart", "Prerequisites, Docker, source installation, and your first call.", "getting-started.md"], ["Administration", "Models, organizations, teams, keys, routing, and costs.", "user-guide.md"], ["API & integration", "Registered endpoints, protocol capabilities, and request paths.", "development/api-reference.md"]], allDocs: "Browse all documentation",
    footer: "Enterprise AI access & governance", status: "Capabilities & limits", statusText: "Protocol and media operations depend on deployments. Guardrails currently check text before calls; see the runtime reference for budget and multi-instance boundaries.", noResults: "No matching documents", searchHint: "Search full documentation: budgets, routing, Docker…", searchError: "Could not load the search index. Use the document navigation or try again.", searching: "Loading the document index…", toc: "On this page", source: "View source on GitHub", documentHome: "Documentation", moduleDocs: "Module reference", chineseReference: "This reference is currently available in Chinese.",
  },
};

export const groups = [
  { zh: "开始使用", en: "Getting started", files: ["README.zh-CN.md", "getting-started.zh-CN.md", "user-guide.zh-CN.md"], english: ["README.md", "getting-started.md", "user-guide.md"] },
  { zh: "接入与集成", en: "Models & integration", files: ["development/model-endpoints.md", "development/provider-catalog.md", "development/provider-credential-forms.md", "development/model-marketplace.md", "development/api-reference.md"] },
  { zh: "企业治理", en: "Enterprise governance", files: ["development/permissions.md", "development/routing.md", "development/routing-groups.md", "development/pricing.md", "development/guardrails.md"] },
  { zh: "部署与运维", en: "Deployment & operations", files: ["development/configuration.md", "development/runtime.md", "development/operations.md", "development/qiniu-fal.md"] },
  { zh: "开发参考", en: "Development", files: ["development/README.md", "development/architecture.md", "development/implementation.md", "development/testing.md", "development/regression.md", "development/e2e-regression.md"] },
];
