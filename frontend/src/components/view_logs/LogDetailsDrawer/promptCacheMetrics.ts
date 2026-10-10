import { extractPromptCacheTokens } from "@/utils/promptCacheUsage";

/** 从日志读取提示词缓存统计；参数为日志顶层字段和元数据，返回读取/写入token或undefined。
 * 由详情指标调用，原始上游统计优先，其次兼容账单及旧版字段；仅接受非负有限数，缺失保持未知，无副作用。 */
export function logPromptCacheTokens(log: object, metadata: Record<string, any>): { read?: number; creation?: number } {
  const raw = metadata.upstream_response?.usage;
  const legacy = metadata.additional_usage_values;
  const extracted = extractPromptCacheTokens(raw ?? legacy);
  const read = [raw?.cache_read_input_tokens, raw?.prompt_tokens_details?.cached_tokens, raw?.input_tokens_details?.cached_tokens,
    raw?.cachedContentTokenCount, extracted.cacheReadTokens, legacy?.cache_read_input_tokens, (log as { cache_read_input_tokens?: unknown }).cache_read_input_tokens, metadata.cached_tokens]
    .find(validCacheCount);
  const creation = [raw?.cache_creation_input_tokens, extracted.cacheCreationTokens, legacy?.cache_creation_input_tokens].find(validCacheCount);
  return { read, creation };
}

/** 判断缓存数值是否合法；参数为任意供应商字段，返回类型守卫。由指标提取调用，拒绝负数、无穷和字符串，无副作用。 */
function validCacheCount(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}
