import { t } from "@/i18n";

// 已移植服务的配置描述，与后端 externalProviders/validateExternal 协议标识对应。
// secret 字段仅控制输入框类型，服务器负责真正的遮蔽和凭据保存。
/** 配置字段：key 是后端参数名，label 是展示文案；numeric 在构建请求时转换数值。 */
export interface ExternalField {
  /** 后端参数名，与 validateExternal 和提供商适配器读取的字段一致。 */
  key: string;
  /** 展示名称，经 externalGuardrailText 翻译，保留外部服务品牌。 */
  label: string;
  /** 使用密码输入框；真正的掩码、更新保留和环境变量解析由后端负责。 */
  secret?: boolean;
  /** 必填字段，控制前端保存按钮；后端仍独立执行完整配置校验。 */
  required?: boolean;
  /** 新建规则的初始字符串；编辑时优先使用服务器保存的值。 */
  initial?: string;
  /** payload 转成 number；非整数、超出范围等条件交由后端拒绝。 */
  numeric?: boolean;
}
export const EXTERNAL_PROVIDERS: Record<
  string,
  { name: string; description: string; fields: ExternalField[]; redact?: boolean }
> = {
  openai_moderation: {
    name: "OpenAI Moderation",
    description: "调用 /moderations，flagged 时拦截。",
    fields: [
      { key: "api_base", label: "API Base", initial: "https://api.openai.com/v1" },
      { key: "api_key", label: "API Key", secret: true, required: true },
      { key: "model", label: "Moderation model", initial: "omni-moderation-latest" },
    ],
  },
  lakera_v2: {
    name: "Lakera Guard",
    description: "调用 Lakera v2/guard 检测提示词注入与不安全内容。",
    fields: [
      { key: "api_base", label: "API Base", initial: "https://api.lakera.ai" },
      { key: "api_key", label: "API Key", secret: true, required: true },
      { key: "project_id", label: "Project ID" },
    ],
  },
  "azure/text_moderations": {
    name: "Azure Content Safety",
    description: "检查 Hate、SelfHarm、Sexual、Violence，严重程度达到阈值时拦截。",
    fields: [
      { key: "api_base", label: "Azure endpoint", required: true },
      { key: "api_key", label: "API Key", secret: true, required: true },
      { key: "api_version", label: "API version", initial: "2024-09-01" },
      { key: "severity_threshold", label: "严重程度阈值（0–7）", initial: "4", numeric: true },
    ],
  },
  "azure/prompt_shield": {
    name: "Azure Prompt Shield",
    description: "在发送请求前检测 userPrompt 中的攻击。",
    fields: [
      { key: "api_base", label: "Azure endpoint", required: true },
      { key: "api_key", label: "API Key", secret: true, required: true },
      { key: "api_version", label: "API version", initial: "2024-09-01" },
    ],
  },
  bedrock: {
    name: "AWS Bedrock",
    description: "调用 AWS ApplyGuardrail，发生 GUARDRAIL_INTERVENED 时拦截；支持 AWS 默认凭据链。",
    fields: [
      { key: "guardrailIdentifier", label: "Guardrail identifier", required: true },
      { key: "guardrailVersion", label: "Guardrail version", required: true },
      { key: "aws_region_name", label: "AWS region", required: true, initial: "us-east-1" },
      { key: "aws_access_key_id", label: "AWS access key（可选）", secret: true },
      { key: "aws_secret_access_key", label: "AWS secret key（可选）", secret: true },
      { key: "aws_session_token", label: "AWS session token（可选）", secret: true },
    ],
  },
  presidio: {
    name: "Presidio PII",
    description: "调用 Analyzer 检测个人信息；脱敏需同时配置 Anonymizer。",
    redact: true,
    fields: [
      { key: "api_base", label: "Analyzer API Base", required: true },
      { key: "anonymizer_api_base", label: "Anonymizer API Base" },
      { key: "language", label: "Language", initial: "en" },
    ],
  },
  "hide-secrets": {
    name: "Secret detection",
    description: "本地检测 AWS、OpenAI、GitHub 密钥、JWT、私钥及敏感赋值；不等同于 detect-secrets 的全部插件。",
    redact: true,
    fields: [{ key: "replacement", label: "替换文本", initial: "[REDACTED]" }],
  },
  litellm_proxy: {
    name: "Remote LiteLLM",
    description: "连接已有 LiteLLM 的 apply_guardrail 接口，可使用远端已配置的服务。",
    fields: [
      { key: "api_base", label: "LiteLLM API Base", required: true },
      { key: "api_key", label: "LiteLLM API Key", secret: true, required: true },
      { key: "remote_guardrail_name", label: "远端护栏名称", required: true },
    ],
  },
};
// 花园卡片 ID 与实际执行协议的映射；未出现在此表的伙伴显示尚未移植。
export const CARD_PROVIDERS: Record<string, string> = {
  openai_moderation: "openai_moderation",
  lakera: "lakera_v2",
  presidio: "presidio",
  bedrock: "bedrock",
  azure_content_safety: "azure/text_moderations",
  azure_prompt_shield: "azure/prompt_shield",
  secret_detection: "hide-secrets",
  litellm_proxy: "litellm_proxy",
};

/**
 * 用途：翻译外部服务说明，同时保留真实的 LiteLLM 产品名称。
 * 参数：key：文案；vars：命名插值，例如伙伴服务名称。
 * 返回：本地化文本；先由 t 翻译其余内容，再填入外部品牌。
 * 调用：外部编辑器和护栏花园。全局品牌替换仍用于 XHub 自身页面。
 * 测试：guardrail_garden.integration.test.tsx 的远端配置流程。
 */
export function externalGuardrailText(key: string, vars?: Record<string, unknown>): string {
  return t(key.replaceAll("LiteLLM", "{externalProduct}"), vars).replaceAll("{externalProduct}", "LiteLLM");
}
