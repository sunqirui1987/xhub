#!/usr/bin/env python3
"""Write unique frontend page specs (not generic stubs)."""
from __future__ import annotations

from pathlib import Path

OUT = Path(__file__).resolve().parents[1] / "frontend" / "pages"

NAV_DIR = {
    "壳": "auth",
    "AUTH": "auth",
    "AI GATEWAY": "gateway",
    "OBSERVABILITY": "observability",
    "ACCESS CONTROL": "access",
    "DEVELOPER TOOLS": "developer",
    "SETTINGS": "settings",
    "CHAT": "chat",
    "PUBLIC": "public",
}

# slug, route, title, nav, purpose, layout, actions[(label, method path, ui)], fields[(name, meaning)]
PAGES = [
    (
        "index", "/", "控制台根", "壳",
        "登录后进入控制台；HOME 重定向到 `/api-keys`。",
        "无业务表。顶栏用户菜单 + 侧栏。若未登录跳 `/login`。",
        [("打开控制台", "GET /health/readiness/details", "侧栏显示版本与就绪"), ("未登录", "GET /login", "跳转登录")],
        [("redirect_to", "固定 `/api-keys`"), ("session", "未登录则无")],
    ),
    (
        "login", "/login", "登录", "AUTH",
        "用 master key、用户名密码或 SSO 进入控制台。",
        "居中卡片：邮箱/用户名、密码、SSO 按钮（Google/Microsoft/SAML）、CLI 登录说明。",
        [("密码登录", "POST /login 或 POST /v2/login 或 POST /v3/login", "写入 session，跳 /api-keys"),
         ("SSO", "GET /sso/key/generate 然后 /sso/callback", "建 session"),
         ("就绪", "GET /health/readiness/details", "展示版本")],
        [("username", "登录名"), ("password", "密码"), ("sso_provider", "SSO 提供商")],
    ),
    (
        "onboarding", "/onboarding", "开通邀请", "AUTH",
        "受邀用户用 invitation token 设密码并激活。",
        "单页表单：email 只读、password、confirm。过期邀请展示错误。",
        [("读取邀请", "GET /onboarding/get_token", "填 email"),
         ("认领", "POST /onboarding/claim_token", "登录并跳转")],
        [("invitation_id", "邀请 id"), ("user_email", "邮箱"), ("password", "新密码")],
    ),
    (
        "connect", "/connect", "连接引导", "AUTH",
        "首次把 SDK base_url 指到本网关。",
        "步骤条：复制 base_url、创建 Key 链接、curl/OpenAI SDK 示例。",
        [("发现端点", "GET /public/endpoints", "展示 base_url"),
         ("模型列表", "GET /v1/models", "示例 model 名")],
        [("base_url", "网关地址"), ("api_key", "虚拟 Key")],
    ),
    (
        "api-keys", "/api-keys", "虚拟密钥", "AI GATEWAY",
        "默认首页。创建、筛选、轮换、拉黑虚拟 Key。",
        "顶栏 Create Key。筛选：Team、User、key_alias、状态。表列：key_alias、token 前缀、team_id、org_id、user、models、spend、expires、last_active。行菜单：Info、Edit、Regenerate、Block/Unblock、Delete。创建成功模态只显示一次 sk-。",
        [("列出", "GET /key/list", "填表"),
         ("创建", "POST /key/generate", "模态展示明文 key"),
         ("更新", "POST /key/update", "关闭抽屉并刷新"),
         ("删除", "POST /key/delete", "行消失"),
         ("轮换", "POST /key/regenerate", "再展示一次明文"),
         ("拉黑", "POST /key/block", "状态变为 blocked")],
        [("key_alias", "显示名"), ("token", "哈希/前缀，非明文"), ("team_id", "所属团队"), ("models", "允许模型"), ("spend", "已花费 USD"), ("max_budget", "预算"), ("expires", "过期"), ("key_type", "llm_api/management/read_only/default")],
    ),
    (
        "playground", "/playground", "调试台", "AI GATEWAY",
        "管理员用真实数据面调试协议。",
        "左：模型下拉、接口 Tab（chat / completions / responses / embeddings / messages）、temperature、max_tokens、stream、tools JSON、图片。中：消息列表与 SSE 输出。右：usage、x-litellm-response-cost、事件。控件按 GET /model/info 能力显示。",
        [("拉模型", "GET /v1/models 与 GET /model/info", "填充下拉"),
         ("发送 Chat", "POST /v1/chat/completions", "流式打字"),
         ("Completions", "POST /v1/completions", "文本输出"),
         ("Responses", "POST /v1/responses", "output 块"),
         ("Embeddings", "POST /v1/embeddings", "向量预览"),
         ("Messages", "POST /v1/messages", "Anthropic 块"),
         ("取消", "中止 fetch", "服务端取消上游")],
        [("model", "别名"), ("messages", "对话"), ("temperature", "采样"), ("max_tokens", "上限"), ("stream", "SSE"), ("tools", "工具 JSON")],
    ),
    (
        "models-and-endpoints", "/models-and-endpoints", "模型与端点", "AI GATEWAY",
        "登记 deployment、凭证、健康与 fallback。",
        "Tab：Models、Credentials、Health。Add Model 向导：provider → litellm_params.model/api_base/api_key → rpm/tpm/timeout → 测试连接。表：model_name、provider、api_base、mode、健康。",
        [("列出", "GET /v2/model/info", "表"),
         ("新建", "POST /model/new", "出现在表中且 Chat 可用"),
         ("更新", "POST /model/update", "刷新"),
         ("删除", "POST /model/delete", "行消失"),
         ("测连接", "POST /health/test_connection", "健康徽章"),
         ("凭证", "GET /credentials", "凭证抽屉")],
        [("model_name", "客户端别名"), ("litellm_params.model", "上游模型"), ("api_base", "上游地址"), ("api_key", "环境变量或凭证引用"), ("rpm", "每分钟请求"), ("tpm", "每分钟 token"), ("timeout", "秒")],
    ),
    (
        "agents", "/agents", "Agents", "AI GATEWAY",
        "创建 Agent、发现 Agent Card、绑定虚拟 Key。",
        "卡片/表：name、description、version、is_public。Add Agent 表单。详情：Agent Card JSON、Virtual Keys 子表、成本。",
        [("列出", "GET /v1/agents", "卡片"),
         ("创建", "POST /v1/agents", "新卡片"),
         ("更新", "PATCH /v1/agents/{agent_id}", "保存"),
         ("删除", "DELETE /v1/agents/{agent_id}", "移除"),
         ("公开", "POST /v1/agents/{agent_id}/make_public", "is_public")],
        [("agent_id", "id"), ("agent_name", "名称"), ("litellm_params", "模型参数"), ("is_public", "是否公开")],
    ),
    (
        "workflows", "/workflows", "工作流运行", "AI GATEWAY",
        "查看 durable workflow 运行。",
        "表：run_id、status、started_at。详情 Tab：Events、Messages。",
        [("列出", "GET /v1/workflows/runs", "表"),
         ("事件", "GET /v1/workflows/runs/{run_id}/events", "时间线"),
         ("消息", "GET /v1/workflows/runs/{run_id}/messages", "对话"),
         ("更新状态", "PATCH /v1/workflows/runs/{run_id}", "status 变化")],
        [("run_id", "运行 id"), ("status", "running/completed/failed"), ("started_at", "开始时间")],
    ),
    (
        "memory", "/memory", "Memory", "AI GATEWAY",
        "检索与删除 agent memory。",
        "搜索框 + 表：key、value 预览、agent_id。行删除需确认。",
        [("列出", "GET /v1/memory", "表"),
         ("删除", "DELETE /v1/memory/{key}", "行消失")],
        [("key", "记忆键"), ("value", "内容"), ("agent_id", "所属 agent")],
    ),
    (
        "mcp-servers", "/mcp-servers", "MCP 服务器", "AI GATEWAY",
        "注册 MCP、OAuth、列出 tools。",
        "Tab：Servers、Toolsets、Access Groups。表：server_name、url、transport、auth_type、status。Add Server 表单含 URL/transport。行：Health、OAuth、Tools。",
        [("列出", "GET /server", "表"),
         ("创建", "POST /server", "新行"),
         ("更新", "PUT /server/{server_id}", "保存"),
         ("删除", "DELETE /server/{server_id}", "移除"),
         ("健康", "GET /server/health", "徽章"),
         ("OAuth", "GET /server/oauth/{server_id}/authorize", "浏览器授权")],
        [("server_id", "id"), ("server_name", "名称"), ("url", "MCP URL"), ("transport", "sse/stdio/streamable-http"), ("auth_type", "none/oauth/bearer")],
    ),
    (
        "skills", "/skills", "Skills", "AI GATEWAY",
        "网关 Skills 与 Claude Code 插件。",
        "Tab：Skills、Plugins。Plugin 表：name、version、category、enabled。",
        [("列出 skills", "GET /v1/skills", "表"),
         ("创建", "POST /v1/skills", "新行"),
         ("插件列表", "GET /claude-code/plugins", "插件表"),
         ("启用插件", "POST /claude-code/plugins/{plugin_name}/enable", "enabled=true")],
        [("skill_id", "id"), ("name", "名称"), ("source", "来源"), ("enabled", "是否启用"), ("version", "版本")],
    ),
    (
        "guardrails", "/guardrails", "Guardrails", "AI GATEWAY",
        "内容策略 CRUD、测试、审批。",
        "表：guardrail_name、provider、mode、default_on。Add 向导含 provider specific params。详情：Test、Info、Submissions。",
        [("列出", "GET /guardrails/list", "表"),
         ("创建", "POST /guardrails", "新行"),
         ("更新", "PATCH /guardrails/{guardrail_id}", "保存"),
         ("删除", "DELETE /guardrails/{guardrail_id}", "移除"),
         ("测试", "POST /apply_guardrail", "展示 allow/block/redact")],
        [("guardrail_name", "名称"), ("litellm_params.guardrail", "提供商"), ("mode", "pre_call/post_call"), ("default_on", "默认开启")],
    ),
    (
        "policies", "/policies", "Policies", "AI GATEWAY",
        "版本化策略与绑定。",
        "表：policy_name、version、status。详情 Tab：Versions、Attachments、Test pipeline。",
        [("列出", "GET /policies/list", "表"),
         ("创建", "POST /policies", "新策略"),
         ("改状态", "PUT /policies/{policy_id}/status", "active/disabled"),
         ("绑定", "POST /policies/attachments", "附件表更新")],
        [("policy_name", "名称"), ("version", "版本"), ("status", "状态"), ("attachment_id", "绑定 id")],
    ),
    (
        "search-tools", "/search-tools", "Search Tools", "AI GATEWAY",
        "搜索工具登记与测连接。",
        "表 + Add：provider 下拉（来自 available_providers）、api_key。行：Test connection。",
        [("列出", "GET /search_tools/list", "表"),
         ("提供商", "GET /search_tools/ui/available_providers", "下拉"),
         ("创建", "POST /search_tools", "新行"),
         ("测连接", "POST /search_tools/test_connection", "成功/失败")],
        [("search_tool_name", "名称"), ("search_provider", "tavily/serper 等"), ("api_key", "上游密钥引用")],
    ),
    (
        "vector-stores", "/vector-stores", "Vector Stores", "AI GATEWAY",
        "管理端向量库登记。独立实现，不复制企业源码。",
        "表：name、provider、file_counts。详情：Files、Search 试探。",
        [("列出", "GET /vector_store/list", "表"),
         ("创建", "POST /vector_store/new", "新行"),
         ("更新", "POST /vector_store/update", "保存"),
         ("删除", "POST /vector_store/delete", "移除"),
         ("搜索试探", "POST /v1/vector_stores/{id}/search", "结果列表")],
        [("vector_store_name", "名称"), ("vector_store_id", "id"), ("provider", "后端")],
    ),
    (
        "tool-policies", "/tool-policies", "Tool Policies", "AI GATEWAY",
        "哪些主体能调哪些工具。",
        "左工具列表，右 allow/block 与主体绑定。",
        [("列出工具", "GET /v1/tool/list", "左栏"),
         ("策略选项", "GET /v1/tool/policy/options", "右栏"),
         ("覆盖", "DELETE /v1/tool/{tool_name}/overrides", "恢复默认")],
        [("tool_name", "工具名"), ("allowed", "是否允许"), ("blocked_tools", "黑名单")],
    ),
    (
        "usage", "/usage", "用量", "OBSERVABILITY",
        "按日/模型/团队聚合的 spend 与 token。只读。",
        "时间范围、Team、模型筛选。指标卡：请求数、spend、token。图 + 明细表。",
        [("全局 spend", "GET /global/spend", "卡片"),
         ("活跃度", "GET /global/activity", "趋势图"),
         ("用户日活", "GET /user/daily/activity", "表")],
        [("start_date", "起"), ("end_date", "止"), ("spend", "USD"), ("prompt_tokens", "输入 token"), ("completion_tokens", "输出 token")],
    ),
    (
        "cost-optimization", "/cost-optimization", "成本优化", "OBSERVABILITY",
        "prompt 压缩、缓存命中、自动路由、shadow eval。",
        "卡片：compression savings、cache hit。Tab：Auto Router benchmarks、Shadow eval 任务表。",
        [("benchmarks", "GET /auto_router/benchmarks", "表"),
         ("启动 shadow eval", "POST /auto_router/shadow_eval/start", "job 行"),
         ("停止", "POST /auto_router/shadow_eval/{job_id}/stop", "status=stopped")],
        [("job_id", "任务"), ("routing_strategy", "策略"), ("savings", "节省")],
    ),
    (
        "logs", "/logs", "请求日志", "OBSERVABILITY",
        "单请求明细与详情抽屉。",
        "筛选：request_id、model、status、时间。表：request_id、model、status、spend、latency。抽屉 Tab：Request、Response（脱敏）、Routing、Cost、Guardrails。",
        [("列表", "GET /spend/logs", "表"),
         ("详情", "GET /spend/logs/ui/{request_id}", "抽屉")],
        [("request_id", "调用 id"), ("model", "别名"), ("spend", "USD"), ("prompt_tokens", "输入"), ("completion_tokens", "输出"), ("status", "成功/失败")],
    ),
    (
        "guardrails-monitor", "/guardrails-monitor", "Guardrails 监控", "OBSERVABILITY",
        "命中次数与动作分布。",
        "概览卡 + 日志表 + 单条 detail。",
        [("概览", "GET /guardrails/usage/overview", "卡片"),
         ("日志", "GET /guardrails/usage/logs", "表"),
         ("详情", "GET /guardrails/usage/detail/{guardrail_id}", "抽屉")],
        [("guardrail_id", "id"), ("action", "allow/block/redact"), ("count", "次数")],
    ),
    (
        "old-usage", "/old-usage", "旧用量视图", "DEVELOPER TOOLS",
        "旧仪表盘布局，数据仍走 spend API。",
        "旧图表组件：按 key/team 柱状图。",
        [("spend", "GET /global/spend", "图")],
        [("start_date", "起"), ("end_date", "止"), ("api_key", "哈希")],
    ),
    (
        "teams", "/teams", "团队", "ACCESS CONTROL",
        "团队、成员、模型白名单、预算。",
        "表：team_alias、organization_id、members、models、spend/max_budget、blocked。详情 Tab：Members、Models、Budget、Callbacks。",
        [("列出", "GET /team/list", "表"),
         ("创建", "POST /team/new", "新行"),
         ("加成员", "POST /team/member_add", "成员表更新"),
         ("更新", "POST /team/update", "保存"),
         ("拉黑", "POST /team/block", "blocked")],
        [("team_alias", "名称"), ("organization_id", "组织"), ("members_with_roles", "成员与角色"), ("models", "模型白名单"), ("max_budget", "预算")],
    ),
    (
        "projects", "/projects", "项目", "ACCESS CONTROL",
        "团队下项目。enableProjectsUI 关闭则隐藏。企业能力独立实现。",
        "表：project_id、project_alias、team、blocked、created_at。详情含 Project Keys 子表（key_alias、last_active）。",
        [("列出", "GET /project/list", "表"),
         ("创建", "POST /project/new", "新行"),
         ("更新", "POST /project/update", "保存"),
         ("删除", "POST /project/delete", "移除")],
        [("project_alias", "名称"), ("team_id", "团队"), ("max_budget", "预算"), ("blocked", "是否停用")],
    ),
    (
        "users", "/users", "内部用户", "ACCESS CONTROL",
        "内部用户账号与角色。",
        "表列：user_id、user_email、user_role、spend、max_budget、SSO id。行菜单：Edit、Reset password、Copy id、Delete。Bulk edit。Default user settings 抽屉。",
        [("列出", "GET /user/list", "表"),
         ("创建", "POST /user/new", "新行"),
         ("更新", "POST /user/update", "保存"),
         ("删除", "POST /user/delete", "移除"),
         ("批量", "POST /user/bulk_update", "多行更新")],
        [("user_email", "邮箱"), ("user_role", "proxy_admin/internal_user 等"), ("user_alias", "显示名"), ("max_budget", "预算"), ("spend", "已花费")],
    ),
    (
        "organizations", "/organizations", "组织", "ACCESS CONTROL",
        "组织与成员。",
        "表：organization_alias、spend、models。详情：Members。",
        [("列出", "GET /organization/list", "表"),
         ("创建", "POST /organization/new", "新行"),
         ("加成员", "POST /organization/member_add", "成员更新"),
         ("更新", "PATCH /organization/update", "保存")],
        [("organization_alias", "名称"), ("budget_id", "预算对象"), ("models", "模型")],
    ),
    (
        "access-groups", "/access-groups", "访问组", "ACCESS CONTROL",
        "模型访问组。",
        "表 + Create：models 多选、budget。详情含 budget 子资源。",
        [("列出", "GET /access_group/list", "表"),
         ("创建", "POST /access_group/new", "新组"),
         ("更新", "PUT /access_group/{id}/update", "保存"),
         ("删除", "DELETE /access_group/{id}/delete", "移除")],
        [("access_group_id", "id"), ("models", "模型集合"), ("budget_id", "预算")],
    ),
    (
        "budgets", "/budgets", "预算", "ACCESS CONTROL",
        "可复用预算对象，挂到 org/team/key。",
        "表：budget_id、max_budget、tpm_limit、rpm_limit、budget_duration。创建表单含 soft_budget。",
        [("列出", "GET /budget/list", "表"),
         ("创建", "POST /budget/new", "新行"),
         ("更新", "POST /budget/update", "保存"),
         ("删除", "POST /budget/delete", "移除")],
        [("max_budget", "USD 上限"), ("soft_budget", "告警阈值"), ("tpm_limit", "TPM"), ("rpm_limit", "RPM"), ("budget_duration", "周期如 30d")],
    ),
    (
        "api-reference", "/api-reference", "API 参考", "DEVELOPER TOOLS",
        "展示本网关真实 method+path。",
        "可搜索路径列表，代码示例用当前 base_url。",
        [("可用路由", "GET /utils/available_routes", "列表"),
         ("OpenAI 参数", "GET /utils/supported_openai_params", "参数表")],
        [("method", "HTTP 方法"), ("path", "路径"), ("model", "示例模型")],
    ),
    (
        "model-hub-table", "/model-hub-table", "AI Hub", "DEVELOPER TOOLS",
        "已配置/公开模型目录。",
        "表：model_name、provider、mode、public。",
        [("模型目录", "GET /model_hub", "表"),
         ("公开目录", "GET /public/model_hub", "只读")],
        [("model_name", "别名"), ("provider", "供应商"), ("mode", "chat/embedding 等")],
    ),
    (
        "caching", "/caching", "响应缓存", "DEVELOPER TOOLS",
        "缓存后端与 TTL。",
        "表单：type、host、port、ttl、namespace。按钮 Ping、Flush all。Coordination Redis 子表。",
        [("读取设置", "GET /cache/settings", "填表"),
         ("保存", "POST /cache/settings", "成功 toast"),
         ("Ping", "GET /ping", "延迟"),
         ("清空", "POST /flushall", "确认后清空")],
        [("type", "redis/memory"), ("ttl", "秒"), ("namespace", "键前缀"), ("host", "Redis 主机")],
    ),
    (
        "prompts", "/prompts", "Prompts", "DEVELOPER TOOLS",
        "提示词版本与测试。",
        "表：prompt_id、version。编辑器 + Test 面板（选模型调用）。",
        [("列出", "GET /prompts/list", "表"),
         ("创建", "POST /prompts", "新版本"),
         ("测试", "POST /prompts/test", "输出预览")],
        [("prompt_id", "id"), ("prompt_name", "名称"), ("content", "模板"), ("version", "版本")],
    ),
    (
        "transform-request", "/transform-request", "请求转换调试", "DEVELOPER TOOLS",
        "查看转换后发给上游的请求。",
        "左：原始 messages/model。右：转换 JSON。",
        [("支持参数", "GET /utils/supported_openai_params", "参数列表"),
         ("转换", "POST /utils/transform_request", "右侧 JSON")],
        [("model", "模型"), ("messages", "消息"), ("call_type", "chat 等")],
    ),
    (
        "tag-management", "/tag-management", "标签", "DEVELOPER TOOLS",
        "请求归因标签。",
        "表：name、spend。日活 DAU/WAU/MAU 图。",
        [("列出", "GET /tag/list", "表"),
         ("创建", "POST /tag/new", "新标签"),
         ("日活", "GET /tag/daily/activity", "图")],
        [("name", "标签名"), ("description", "说明"), ("spend", "花费")],
    ),
    (
        "router-settings", "/router-settings", "路由设置", "SETTINGS",
        "策略、重试、fallback 图。",
        "表单：routing_strategy 下拉（simple-shuffle/least-busy/lowest-cost/…）、num_retries、timeout、allowed_fails。Fallback 编辑器。",
        [("读取", "GET /router/settings", "填表"),
         ("字段元数据", "GET /router/fields", "动态表单"),
         ("fallback", "POST /fallback", "保存图")],
        [("routing_strategy", "策略名"), ("num_retries", "重试次数"), ("timeout", "秒"), ("allowed_fails", "冷却阈值")],
    ),
    (
        "logging-and-alerts", "/logging-and-alerts", "日志与告警", "SETTINGS",
        "success/failure callback 与 Slack 告警。",
        "Callbacks 表：name、enabled。Alerting：threshold、webhook。",
        [("回调列表", "GET /callbacks/list", "表"),
         ("告警设置", "GET /alerting/settings", "表单"),
         ("保存告警", "PATCH alerting 配置", "toast")],
        [("callback_name", "langfuse 等"), ("enabled", "开关"), ("slack_webhook", "Webhook"), ("alerting_threshold", "阈值")],
    ),
    (
        "admin-panel", "/admin-panel", "管理员设置", "SETTINGS",
        "SSO、内部用户默认设置、允许 IP、可见页面。",
        "分区：SSO、Default user settings、Allowed IPs、UI pages、SCIM 说明。",
        [("SSO", "GET /get/sso_settings", "表单"),
         ("保存 SSO", "PATCH /update/sso_settings", "保存"),
         ("IP 列表", "GET /get/allowed_ips", "列表"),
         ("加 IP", "POST /add/allowed_ip", "新行"),
         ("UI 设置", "GET /get/ui_settings", "页面开关")],
        [("sso_enabled", "SSO 开关"), ("allowed_ip", "CIDR"), ("enabled_pages", "内部用户可见页")],
    ),
    (
        "cost-tracking", "/cost-tracking", "成本跟踪", "SETTINGS",
        "折扣、加价、CloudZero/Vantage 导出。",
        "表单 discount/margin。导出按钮与 dry-run。",
        [("CloudZero 设置", "GET /cloudzero/settings", "表单"),
         ("导出", "POST /cloudzero/export", "任务状态"),
         ("Vantage", "GET /vantage/settings", "表单")],
        [("discount", "折扣"), ("margin", "加价"), ("api_key", "导出凭证")],
    ),
    (
        "ui-theme", "/ui-theme", "界面主题", "SETTINGS",
        "Logo 与主色。",
        "上传 logo、颜色选择器、预览。",
        [("读取", "GET /get/ui_theme_settings", "填表"),
         ("保存", "PATCH 主题设置", "即时预览"),
         ("上传", "POST /upload/logo", "logo_url 更新")],
        [("logo_url", "Logo"), ("primary_color", "主色")],
    ),
    (
        "chat", "/chat", "Chat 终端壳", "CHAT",
        "给终端用户的对话 UI，不是管理员 Playground。",
        "左模型列表，中气泡对话，输入框发送。无管理侧栏的模型编辑。",
        [("模型", "GET /v1/models", "左栏"),
         ("发送", "POST /v1/chat/completions", "助手气泡 SSE")],
        [("model", "模型"), ("messages", "历史"), ("stream", "流式")],
    ),
    (
        "chat-api-keys", "/chat/api-keys", "Chat · 密钥", "CHAT",
        "终端用户只看自己的 Key。",
        "简化表：key_alias、spend、max_budget。无全局用户筛选。",
        [("列出自己的 Key", "GET /key/list", "表"),
         ("详情", "GET /key/info", "抽屉")],
        [("key_alias", "名称"), ("spend", "已花费"), ("max_budget", "预算"), ("expires", "过期")],
    ),
    (
        "chat-credentials", "/chat/credentials", "Chat · 凭证", "CHAT",
        "终端用户可见的命名凭证（无明文）。",
        "表：credential_name、provider。",
        [("列出", "GET /credentials", "表"),
         ("按名查询", "GET /credentials/by_name/{credential_name}", "详情")],
        [("credential_name", "名称"), ("custom_llm_provider", "类型"), ("created_at", "创建时间")],
    ),
    (
        "chat-integrations", "/chat/integrations", "Chat · 集成", "CHAT",
        "当前用户可见的 logging callback。",
        "开关列表，无管理员 webhook 编辑。",
        [("列出", "GET /callbacks/list", "开关"),
         ("配置", "GET /callbacks/configs", "只读详情")],
        [("callback_name", "集成名"), ("enabled", "是否开启"), ("callback_vars", "脱敏配置")],
    ),
    (
        "chat-logs", "/chat/logs", "Chat · 日志", "CHAT",
        "当前用户请求日志。",
        "精简 Logs：无跨用户筛选。",
        [("列出", "GET /spend/logs", "表"),
         ("详情", "GET /spend/logs/ui/{request_id}", "抽屉")],
        [("request_id", "id"), ("model", "模型"), ("spend", "费用"), ("status", "成功/失败")],
    ),
    (
        "chat-usage", "/chat/usage", "Chat · 用量", "CHAT",
        "当前用户日聚合。",
        "简单折线 + 数字。",
        [("日活", "GET /user/daily/activity", "图")],
        [("date", "日期"), ("spend", "USD"), ("tokens", "token")],
    ),
    (
        "model_hub", "/model_hub", "公开模型目录", "PUBLIC",
        "未登录可浏览。",
        "卡片墙：model_name、provider、useful_links。无写按钮。",
        [("公开目录", "GET /public/model_hub", "卡片"),
         ("信息", "GET /public/model_hub/info", "详情")],
        [("model_name", "名称"), ("provider", "供应商"), ("useful_links", "文档链接")],
    ),
    (
        "model_hub_table", "/model_hub_table", "公开模型表", "PUBLIC",
        "公开 Hub 的表格形态。",
        "纯表，无 Create。",
        [("公开目录", "GET /public/model_hub", "表"),
         ("提供商字段", "GET /public/providers/fields", "列说明")],
        [("model_name", "名称"), ("provider", "供应商"), ("mode", "chat/embedding")],
    ),
    (
        "mcp-oauth-callback", "/mcp/oauth/callback", "MCP OAuth 回调", "AUTH",
        "OAuth 浏览器回到控制台。",
        "无表单：解析 code/state，换 token，跳回 /mcp-servers。",
        [("换票", "POST /v1/mcp/oauth/token", "成功则回 MCP 页")],
        [("code", "授权码"), ("state", "防 CSRF"), ("server_id", "MCP 服务器")],
    ),
]


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    for p in OUT.rglob("*.md"):
        if p.name != "README.md":
            p.unlink()
    for slug, route, title, nav, purpose, layout, actions, fields in PAGES:
        act_rows = "\n".join(f"| {a} | `{m}` | {u} |" for a, m, u in actions)
        field_rows = "\n".join(f"| `{n}` | {m} |" for n, m in fields)
        text = f"""# {title}

- Route: `{route}`
- Nav: {nav}
- Status: `specified`

## 目的

{purpose}

## 布局

{layout}

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
{act_rows}

## 字段

| 字段 | 含义 |
|---|---|
{field_rows}

## 状态

loading / empty / forbidden / 校验失败可改后重试 / 超时锁定原 payload。密钥明文只展示一次。

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
"""
        dest = OUT / NAV_DIR[nav] / f"{slug}.md"
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text(text, encoding="utf-8")
    print("wrote", len(PAGES), "pages")


if __name__ == "__main__":
    main()
