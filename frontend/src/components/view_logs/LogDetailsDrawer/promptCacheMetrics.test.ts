import { describe, expect, it } from "vitest";
import { logPromptCacheTokens } from "./promptCacheMetrics";

describe("logPromptCacheTokens", () => {
  /** 前置真实后台字段，验证旧日志、顶层和供应商嵌套统计均可读取，纯函数无需清理。 */
  it.each([
    [{}, { cached_tokens: 12 }],
    [{ cache_read_input_tokens: 12 }, {}],
    [{}, { upstream_response: { usage: { prompt_tokens_details: { cached_tokens: 12 } } } }],
    [{}, { upstream_response: { usage: { input_tokens_details: { cached_tokens: 12 } } } }],
    [{}, { upstream_response: { usage: { cachedContentTokenCount: 12 } } }],
    [{}, { additional_usage_values: { cache_read_input_tokens: 12 } }],
  ])("reads persisted cache data %j %j", (log, metadata) => {
    expect(logPromptCacheTokens(log, metadata).read).toBe(12);
  });
  /** 前置零值及冲突旧字段，验证原始零优先、写入统计正常，纯函数无需清理。 */
  it("retains explicit zero and cache creation", () => {
    expect(logPromptCacheTokens({}, { cached_tokens: 12, upstream_response: { usage: { cache_read_input_tokens: 0, cache_creation_input_tokens: 4 } } })).toEqual({ read: 0, creation: 4 });
  });
  /** 前置缺失及非法输入，验证不伪造零缓存，纯函数无需清理。 */
  it.each([undefined, null, -1, Infinity, NaN, "12"])("rejects invalid cache statistics %s", (value) => {
    expect(logPromptCacheTokens({ cache_read_input_tokens: value }, { cached_tokens: value })).toEqual({ read: undefined, creation: undefined });
  });
});
