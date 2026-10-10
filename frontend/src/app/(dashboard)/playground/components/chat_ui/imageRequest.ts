import { t } from "@/i18n";
import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";

/** 判断绑定是否为 OpenAI Images 创建或编辑接口；参数为公开绑定，返回布尔值。
 * 调试台与模板调用；仅匹配声明协议和操作路径，避免给其他图片供应商添加不兼容字段。 */
export function isOpenAIImageEndpoint(endpoint: ModelEndpoint): boolean {
  return (
    endpoint.protocol === "openai-images" &&
    (endpoint.path.endsWith("/images/generations") || endpoint.path.endsWith("/images/edits"))
  );
}

/** 解析原生 JSON 草稿；参数 input 为编辑器正文，返回对象。
 * 表单与提交共同调用；语法错误或非对象抛错，不修改草稿或注入默认字段。 */
export function parseNativeRequest(input: string): Record<string, unknown> {
  const doc = JSON.parse(input);
  if (!doc || Array.isArray(doc) || typeof doc !== "object") throw new Error(t("myModels.invalidRequestObject"));
  return doc;
}

/** 校验 OpenAI 图片参数；参数 doc 为已解析草稿，返回 void。
 * 提交前调用；拒绝空提示词、缺失/非法尺寸、非正整数数量，避免空值被中转渠道拒绝。
 * 尺寸和质量支持因模型、渠道而异，允许自定义正整数尺寸及 auto，不猜测供应商能力。 */
export function validateImageRequest(doc: Record<string, unknown>): void {
  if (typeof doc.prompt !== "string" || !doc.prompt.trim()) throw new Error(t("myModels.imagePromptRequired"));
  if (typeof doc.size !== "string" || !/^(auto|[1-9][0-9]*x[1-9][0-9]*)$/.test(doc.size))
    throw new Error(t("myModels.imageSizeInvalid"));
  if (doc.n !== undefined && (typeof doc.n !== "number" || !Number.isSafeInteger(doc.n) || doc.n < 1))
    throw new Error(t("myModels.imageCountInvalid"));
}

/** 提取 OpenAI 图片响应的预览地址；参数 output 为原生响应、format 为请求输出格式，返回安全地址数组。
 * 调试结果调用；支持 URL 和 base64，忽略非法 JSON、非图片响应及非 HTTP(S) URL，不执行上游地址脚本。 */
export function imagePreviewSources(output: string, format: unknown = "png"): string[] {
  try {
    const doc = JSON.parse(output);
    if (!Array.isArray(doc?.data)) return [];
    const mime = format === "jpeg" ? "jpeg" : format === "webp" ? "webp" : "png";
    return doc.data.flatMap((item: unknown) => {
      if (!item || typeof item !== "object") return [];
      const image = item as { b64_json?: unknown; url?: unknown };
      if (typeof image.b64_json === "string" && /^[A-Za-z0-9+/]+={0,2}$/.test(image.b64_json))
        return ["data:image/" + mime + ";base64," + image.b64_json];
      if (typeof image.url === "string") {
        try {
          const url = new URL(image.url);
          if (url.protocol === "https:" || url.protocol === "http:") return [url.href];
        } catch {
          /* 非法地址不展示。 */
        }
      }
      return [];
    });
  } catch {
    return [];
  }
}
