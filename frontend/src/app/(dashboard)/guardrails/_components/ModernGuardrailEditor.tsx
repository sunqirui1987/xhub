import type { Guardrail } from "@/components/guardrails/types";
import { LocalGuardrailEditor } from "./LocalGuardrailEditor";
import ExternalGuardrailEditor from "./ExternalGuardrailEditor";
import CustomCodeModal from "./custom_code/CustomCodeModal";
import { EXTERNAL_PROVIDERS } from "./externalProviders";

/**
 * 用途：判断当前护栏是否使用与花园一致的新版编辑器。
 * 参数：engine：后端提供商标识；空值沿用本地规则兼容逻辑。
 * 返回：本地、XGo 或已适配外部提供商返回 true，历史配置返回 false。
 * 调用：GuardrailInfoView 详情分派。
 * 测试：guardrail_info.integration.test.tsx 覆盖新版外部、本地和旧版内容过滤分支。
 */
export function supportsModernGuardrail(engine: string): boolean {
  return (
    ["local", "blocked_words", "redact", "block", "always_block", "", "custom_code"].includes(engine) ||
    Object.hasOwn(EXTERNAL_PROVIDERS, engine)
  );
}

interface Props {
  rule: Guardrail;
  accessToken: string | null;
  isAdmin: boolean;
  onClose: () => void;
}

/**
 * 用途：为详情选择本地、XGo 或外部配置表单，保持创建与编辑的布局和权限一致。
 * 参数：rule：已加载配置；accessToken：请求凭据；isAdmin：写权限；onClose：返回回调。
 * 返回：对应的表单；未知提供商返回 null，由上层使用历史详情。
 * 调用：GuardrailInfoView；仅在 supportsModernGuardrail 为真时调用。
 * 测试：guardrail_info.integration.test.tsx；具体表单还有各自的集成测试。
 */
export function ModernGuardrailEditor({ rule, accessToken, isAdmin, onClose }: Props) {
  const engine = rule.litellm_params?.guardrail ?? "";
  // 已有编辑器负责本地保存状态，无需在详情层重复刷新或触发列表加载。
  const props = { accessToken, rule, readOnly: !isAdmin, onClose, onSuccess: () => {} };
  if (engine === "custom_code") {
    return (
      <CustomCodeModal
        {...props}
        visible
        readOnly={!isAdmin || rule.guardrail_definition_location === "config"}
        editData={{
          guardrail_id: rule.guardrail_id,
          guardrail_name: rule.guardrail_name ?? "",
          litellm_params: { ...rule.litellm_params, mode: "pre_call" },
        }}
      />
    );
  }
  if (Object.hasOwn(EXTERNAL_PROVIDERS, engine)) return <ExternalGuardrailEditor {...props} provider={engine} />;
  if (supportsModernGuardrail(engine)) return <LocalGuardrailEditor {...props} />;
  return null;
}
