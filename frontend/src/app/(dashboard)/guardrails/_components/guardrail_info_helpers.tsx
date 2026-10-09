import { t } from "@/i18n";
import { scrubBrand } from "@/i18n/brand";
import aimSecurityLogo from "../../../../../public/assets/logos/aim_security.jpeg";
import aktoLogo from "../../../../../public/assets/logos/akto.svg";
import aliceLogo from "../../../../../public/assets/logos/alice.svg";
import conductLogo from "../../../../../public/assets/logos/conduct.png";
import aporiaLogo from "../../../../../public/assets/logos/aporia.png";
import bedrockLogo from "../../../../../public/assets/logos/bedrock.svg";
import catoNetworksLogo from "../../../../../public/assets/logos/cato_networks.svg";
import ciscoLogo from "../../../../../public/assets/logos/cisco.png";
import deepkeepLogo from "../../../../../public/assets/logos/deepkeep.svg";
import enkryptAiLogo from "../../../../../public/assets/logos/enkrypt_ai.avif";
import googleLogo from "../../../../../public/assets/logos/google.svg";
import guardrailsAiLogo from "../../../../../public/assets/logos/guardrails_ai.jpeg";
import javelinLogo from "../../../../../public/assets/logos/javelin.png";
import lakeraAiLogo from "../../../../../public/assets/logos/lakeraai.jpeg";
import lassoLogo from "../../../../../public/assets/logos/lasso.png";
import litellmLogo from "../../../../../public/assets/logos/litellm_logo.jpg";
import microsoftAzureLogo from "../../../../../public/assets/logos/microsoft_azure.svg";
import nomaSecurityLogo from "../../../../../public/assets/logos/noma_security.png";
import openaiSmallLogo from "../../../../../public/assets/logos/openai_small.svg";
import paloAltoNetworksLogo from "../../../../../public/assets/logos/palo_alto_networks.jpeg";
import pangeaLogo from "../../../../../public/assets/logos/pangea.png";
import pillarLogo from "../../../../../public/assets/logos/pillar.jpeg";
import promptSecurityLogo from "../../../../../public/assets/logos/prompt_security.png";
import promptguardLogo from "../../../../../public/assets/logos/promptguard.svg";
import qohashLogo from "../../../../../public/assets/logos/qohash.jpg";
import repelloAiLogo from "../../../../../public/assets/logos/repelloai.png";
import straikerLogo from "../../../../../public/assets/logos/straiker.svg";
import xecguardLogo from "../../../../../public/assets/logos/xecguard.svg";
import zscalerLogo from "../../../../../public/assets/logos/zscaler.svg";

// Legacy enum - keeping for backward compatibility
export enum GuardrailProviders {
  PresidioPII = "Presidio PII",
  Bedrock = "Bedrock Guardrail",
  Lakera = "Lakera",
}

// Dynamic guardrail providers object - populated from API response
export let DynamicGuardrailProviders: Record<string, string> = {};

// Function to populate dynamic providers from API response
/**
 * 用途：从接口字段声明更新显示名称目录，跳过不含名称的元数据。
 * 参数：providerParamsResponse：提供商字段响应。
 * 返回：显示名称映射，同时更新模块目录。
 * 调用：提供商字段加载。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export const populateGuardrailProviders = (providerParamsResponse: Record<string, any>) => {
  const providers: Record<string, string> = {};

  // Legacy hardcoded providers for backward compatibility
  providers.PresidioPII = "Presidio PII";
  providers.Bedrock = "Bedrock Guardrail";
  providers.Lakera = "Lakera";
  providers.LlmAsAJudge = "XHub LLM as a Judge";

  // Add dynamic providers from API response
  Object.entries(providerParamsResponse).forEach(([key, value]) => {
    if (value && typeof value === "object" && "ui_friendly_name" in value) {
      // Create a key from the provider name (camelCase)
      const providerKey = key
        .split("_")
        .map((word, index) =>
          index === 0 ? word.charAt(0).toUpperCase() + word.slice(1) : word.charAt(0).toUpperCase() + word.slice(1),
        )
        .join("");

      providers[providerKey] = scrubBrand(String(value.ui_friendly_name));
    }
  });

  DynamicGuardrailProviders = providers;
  return providers;
};

// Function to get current guardrail providers (dynamic or fallback to legacy)
/**
 * 用途：返回动态提供商目录；尚未加载时使用历史目录。
 * 参数：无；读取模块内 DynamicGuardrailProviders。
 * 返回：Record<string, string>，统一键索引类型供未知提供商查询。
 * 调用：护栏配置、展示名称和字段选择辅助方法。
 * 测试：guardrail_info_helpers.test.tsx。
 */
export const getGuardrailProviders = (): Record<string, string> => {
  return Object.keys(DynamicGuardrailProviders).length > 0 ? DynamicGuardrailProviders : GuardrailProviders;
};

export const guardrail_provider_map: Record<string, string> = {
  PresidioPII: "presidio",
  Bedrock: "bedrock",
  Lakera: "lakera_v2",
  LitellmContentFilter: "litellm_content_filter",
  ToolPermission: "tool_permission",
  BlockCodeExecution: "block_code_execution",
  Promptguard: "promptguard",
  LlmAsAJudge: "llm_as_a_judge",
  Xecguard: "xecguard",
  Deepkeep: "deepkeep",
  QostodianNexus: "qostodian_nexus",
  Repelloai: "repelloai",
  Alice: "alice",
  Conduct: "conduct",
};

// Function to populate provider map from API response - updates the original map
/**
 * 用途：从接口维护显示键到执行器 ID 的映射，保留历史映射。
 * 参数：providerParamsResponse：提供商字段响应。
 * 返回：void，更新 guardrail_provider_map。
 * 调用：提供商字段加载。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export const populateGuardrailProviderMap = (providerParamsResponse: Record<string, any>) => {
  // Add dynamic providers from API response directly to the main map
  Object.entries(providerParamsResponse).forEach(([key, value]) => {
    if (value && typeof value === "object" && "ui_friendly_name" in value) {
      // Create a key from the provider name (camelCase)
      const providerKey = key
        .split("_")
        .map((word, index) =>
          index === 0 ? word.charAt(0).toUpperCase() + word.slice(1) : word.charAt(0).toUpperCase() + word.slice(1),
        )
        .join("");

      guardrail_provider_map[providerKey] = key; // Add directly to the main map
    }
  });
};

// Normalizes a form "mode" value (string, string[], or empty) into a string array
/**
 * 用途：兼容历史字符串与数组格式，过滤非字符串项。
 * 参数：raw：未知模式值。
 * 返回：字符串数组，空或对象返回空数组。
 * 调用：模式表单及展示。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export const toModeArray = (raw: unknown): string[] => {
  if (Array.isArray(raw)) return raw.filter((m): m is string => typeof m === "string");
  if (typeof raw === "string") return [raw];
  return [];
};

/**
 * 用途：将历史单值、多值和标签模式整理成可读文本，仅负责展示。
 * 参数：raw：服务器模式值。
 * 返回：去重后的模式文本，未知格式返回空字符串。
 * 调用：详情、列表及删除确认。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export const formatGuardrailMode = (raw: unknown): string => {
  const flat: string[] = toModeArray(raw);
  if (flat.length > 0) return flat.join(", ");
  if (raw === null || typeof raw !== "object") return "";

  const { tags, default: fallback } = raw as { tags?: Record<string, unknown>; default?: unknown };
  const tagged: string[] = tags && typeof tags === "object" ? Object.values(tags).flatMap(toModeArray) : [];
  const modes: string[] = Array.from(new Set([...toModeArray(fallback), ...tagged]));
  return modes.length > 0 ? `${modes.join(", ")} (tag-based)` : "";
};

// Resolves the supported modes for the selected provider, falling back to the global list
/**
 * 用途：优先读取提供商阶段声明，缺失时使用全局声明。
 * 参数：settings：能力声明；selectedProvider：显示键或 null。
 * 返回：模式数组或 undefined；不等于执行器实际支持承诺。
 * 调用：旧版字段编辑器。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export const getSupportedModesForProvider = (
  settings: { supported_modes?: string[]; supported_modes_by_provider?: Record<string, string[]> } | null,
  selectedProvider: string | null,
): string[] | undefined => {
  const providerKey = selectedProvider ? guardrail_provider_map[selectedProvider]?.toLowerCase() : null;
  const perProvider =
    providerKey && settings?.supported_modes_by_provider
      ? settings.supported_modes_by_provider[providerKey]
      : undefined;
  return perProvider ?? settings?.supported_modes;
};

// Decides if we should render the PII config settings for a given provider
// For now we only support PII config settings for Presidio PII
/**
 * 用途：判断旧版表单是否展示 Presidio 字段。
 * 参数：provider：显示键或 null。
 * 返回：boolean，未知键返回 false。
 * 调用：历史 PII 配置表单。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export const shouldRenderPIIConfigSettings = (provider: string | null) => {
  if (!provider) {
    return false;
  }
  // Check both dynamic and legacy providers
  const currentProviders = getGuardrailProviders();
  const providerEnum = currentProviders[provider as keyof typeof currentProviders];
  return providerEnum === "Presidio PII";
};

// Decides if we should render the Azure Text Moderation config settings for a given provider
/**
 * 用途：判断旧版表单是否展示 Azure 审核字段。
 * 参数：provider：显示键或 null。
 * 返回：boolean。
 * 调用：历史 Azure 配置表单。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export const shouldRenderAzureTextModerationConfigSettings = (provider: string | null) => {
  if (!provider) {
    return false;
  }
  // Check both dynamic and legacy providers
  const currentProviders = getGuardrailProviders();
  const providerEnum = currentProviders[provider as keyof typeof currentProviders];
  return providerEnum === "Azure Content Safety Text Moderation";
};

// Decides if we should render the Content Filter config settings for a given provider
/**
 * 用途：按显示名称判断旧内容过滤器字段。
 * 参数：provider：显示键或 null。
 * 返回：boolean。
 * 调用：旧版内容过滤器表单。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export const shouldRenderContentFilterConfigSettings = (provider: string | null) => {
  if (!provider) {
    return false;
  }
  // Check both dynamic and legacy providers
  const currentProviders = getGuardrailProviders();
  const providerEnum = currentProviders[provider as keyof typeof currentProviders];
  return scrubBrand(String(providerEnum ?? "")) === "XHub Content Filter";
};

/**
 * 用途：按执行器 ID 判断旧版模型裁判字段。
 * 参数：provider：显示键或 null。
 * 返回：boolean。
 * 调用：历史提供商字段表单。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export const shouldRenderLLMJudgeFields = (provider: string | null) => {
  if (!provider) return false;
  return guardrail_provider_map[provider] === "llm_as_a_judge";
};

export const guardrailLogoMap = {
  "Zscaler AI Guard": zscalerLogo.src,
  "Presidio PII": microsoftAzureLogo.src,
  "Bedrock Guardrail": bedrockLogo.src,
  Lakera: lakeraAiLogo.src,
  "Azure Content Safety Prompt Shield": microsoftAzureLogo.src,
  "Azure Content Safety Text Moderation": microsoftAzureLogo.src,
  "Aporia AI": aporiaLogo.src,
  "PANW Prisma AIRS": paloAltoNetworksLogo.src,
  "Cisco AI Defense": ciscoLogo.src,
  "Noma Security": nomaSecurityLogo.src,
  "Javelin Guardrails": javelinLogo.src,
  "Pillar Guardrail": pillarLogo.src,
  "Google Cloud Model Armor": googleLogo.src,
  "Guardrails AI": guardrailsAiLogo.src,
  "Lasso Guardrail": lassoLogo.src,
  "Pangea Guardrail": pangeaLogo.src,
  "AIM Guardrail": aimSecurityLogo.src,
  "Cato Networks Guardrail": catoNetworksLogo.src,
  "OpenAI Moderation": openaiSmallLogo.src,
  EnkryptAI: enkryptAiLogo.src,
  "Prompt Security": promptSecurityLogo.src,
  PromptGuard: promptguardLogo.src,
  XecGuard: xecguardLogo.src,
  "LiteLLM Content Filter": litellmLogo.src,
  "XHub Content Filter": litellmLogo.src,
  "LiteLLM LLM as a Judge": litellmLogo.src,
  "XHub LLM as a Judge": litellmLogo.src,
  "Hide Secrets": litellmLogo.src,
  Akto: aktoLogo.src,
  "DeepKeep AI Firewall": deepkeepLogo.src,
  "Qostodian Nexus": qohashLogo.src,
  "RepelloAI Argus": repelloAiLogo.src,
  Straiker: straikerLogo.src,
  Alice: aliceLogo.src,
  "Conduct Guard": conductLogo.src,
} satisfies Record<string, string>;

/**
 * 用途：查找本地资源图标并兼容品牌名称映射。
 * 参数：displayName：展示名称。
 * 返回：图标资源路径或 undefined。
 * 调用：目录与详情。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export const getGuardrailLogo = (displayName: string): string | undefined => {
  const names = [displayName, scrubBrand(displayName)];
  for (const name of names) {
    if (Object.prototype.hasOwnProperty.call(guardrailLogoMap, name)) {
      return guardrailLogoMap[name as keyof typeof guardrailLogoMap];
    }
  }
  return undefined;
};

/**
 * 用途：统一解析本地、XGo 和历史提供商名称，未知提供商保留原值。
 * 参数：guardrailValue：执行器 ID。
 * 返回：logo 与 displayName 对象。
 * 调用：列表、详情及统计搜索。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export const getGuardrailLogoAndName = (guardrailValue: string): { logo: string; displayName: string } => {
  if (!guardrailValue) {
    return { logo: "", displayName: "-" };
  }

  if (guardrailValue === "custom_code") return { logo: "", displayName: "XGo" };
  if (["local", "blocked_words", "redact", "block", "always_block"].includes(guardrailValue))
    return { logo: "", displayName: t("本地关键词 / 正则") };

  // Find the enum key by matching guardrail_provider_map values
  const enumKey = Object.keys(guardrail_provider_map).find(
    (key) => guardrail_provider_map[key].toLowerCase() === guardrailValue.toLowerCase(),
  );

  if (!enumKey) {
    return { logo: "", displayName: guardrailValue };
  }

  // Get the display name from current GuardrailProviders and logo from map
  const currentProviders = getGuardrailProviders();
  const displayName = currentProviders[enumKey as keyof typeof currentProviders];
  const logo = getGuardrailLogo(displayName ?? "") ?? "";

  return { logo, displayName: displayName || guardrailValue };
};

/** Tri-state UI value for `litellm_params.skip_system_message_in_guardrail` (inherit = use global). */
export type SkipSystemMessageChoice = "inherit" | "yes" | "no";

/**
 * 用途：把系统消息的覆盖值转换为三态选择。
 * 参数：v：boolean、null 或 undefined。
 * 返回：继承、跳过或检查选项。
 * 调用：旧版详情初始化。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export function skipSystemMessageToChoice(v: boolean | null | undefined): SkipSystemMessageChoice {
  if (v === true) return "yes";
  if (v === false) return "no";
  return "inherit";
}

/** Create flow: omit key when inheriting global default. */
/**
 * 用途：把系统消息选择转换成创建参数，继承不写覆盖。
 * 参数：choice：系统消息三态选择。
 * 返回：boolean 或 undefined。
 * 调用：旧版创建表单。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export function choiceToSkipSystemForCreate(choice: SkipSystemMessageChoice | undefined): boolean | undefined {
  if (choice === "yes") return true;
  if (choice === "no") return false;
  return undefined;
}

/** Tri-state UI value for `litellm_params.skip_tool_message_in_guardrail` (inherit = use global). */
export type SkipToolMessageChoice = "inherit" | "yes" | "no";

/**
 * 用途：把工具消息覆盖值转换为三态选择。
 * 参数：v：boolean、null 或 undefined。
 * 返回：工具消息三态选项。
 * 调用：旧版详情初始化。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export function skipToolMessageToChoice(v: boolean | null | undefined): SkipToolMessageChoice {
  if (v === true) return "yes";
  if (v === false) return "no";
  return "inherit";
}

/** Create flow: omit key when inheriting global default. */
/**
 * 用途：把工具消息选择转换成创建参数，继承不写覆盖。
 * 参数：choice：工具消息三态选择。
 * 返回：boolean 或 undefined。
 * 调用：旧版创建表单。
 * 测试：guardrail_info_helpers.test.tsx；展示连线由详情集成测试覆盖。
 */
export function choiceToSkipToolForCreate(choice: SkipToolMessageChoice | undefined): boolean | undefined {
  if (choice === "yes") return true;
  if (choice === "no") return false;
  return undefined;
}
