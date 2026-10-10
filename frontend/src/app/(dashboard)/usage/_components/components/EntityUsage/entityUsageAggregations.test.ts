import { describe, expect, it } from "vitest";
import { getTopModels, getTopAPIKeys, getProviderSpend, type ExtendedDailyData } from "./entityUsageAggregations";

/** 构造确定性的有效日报；参数为费用、请求结果和标签，返回含零价供应商及模型/密钥明细的内存夹具，无清理。 */
function day(spend: number, successful: number, failed: number, tags = [{ tag: "test", usage: spend }]): ExtendedDailyData {
  const metrics = { spend, api_requests: successful + failed, successful_requests: successful, failed_requests: failed,
    prompt_tokens: 8, completion_tokens: 2, total_tokens: 10, cache_read_input_tokens: 0, cache_creation_input_tokens: 0 };
  const bucket = { metrics, metadata: {}, api_key_breakdown: {} };
  return { date: "2026-10-10", metrics, breakdown: { models: { model: bucket }, model_groups: { group: bucket },
    providers: { free: { ...bucket, metrics: { ...metrics, spend: 0 } } },
    api_keys: { key: { metrics, metadata: { key_alias: "test-key", team_id: "team", tags } } },
    entities: { team: bucket }, mcp_servers: {} } };
}

describe("实体明细对账", () => {
  /** 两日成功和失败调用；验证模型、公开模型、供应商请求和 Token 跨日总和，内存夹具无需清理。 */
  it("cross-day model and zero-cost provider totals reconcile", () => {
    const rows = [day(1, 1, 0), day(2, 0, 1)];
    expect(getTopModels(rows, "models", 5)).toEqual([{ key: "model", spend: 3, requests: 2, successful_requests: 1, failed_requests: 1, tokens: 20 }]);
    expect(getTopModels(rows, "model_groups", 1)[0].key).toBe("group");
    expect(getProviderSpend(rows)).toEqual([{ provider: "free", spend: 0, requests: 2, successful_requests: 1, failed_requests: 1, tokens: 20 }]);
  });
  /** 实体不是标签；验证仅累加显式标签，不修改输入，不依赖可选实体明细，内存夹具无需清理。 */
  it("key tags come only from explicit metadata and accumulate across days", () => {
    const rows = [day(1, 1, 0), day(2, 1, 0)];
    delete (rows[1].breakdown as Partial<ExtendedDailyData["breakdown"]>).entities;
    expect(getTopAPIKeys(rows, 5)).toEqual([{ api_key: "key", key_alias: "test-key", spend: 3, tags: [{ tag: "test", usage: 3 }] }]);
    expect(rows[0].breakdown.api_keys.key.metadata.tags![0].usage).toBe(1);
  });
  /** 空日报和空 Top 限额为边界输入；验证返回空行，无虚构统计或异常，无持久数据。 */
  it("empty and zero-limit results stay empty", () => {
    expect(getTopModels([], "models", 5)).toEqual([]);
    expect(getTopAPIKeys([], 5)).toEqual([]);
    expect(getProviderSpend([])).toEqual([]);
    expect(getTopModels([day(0, 0, 1)], "models", 0)).toEqual([]);
    expect(getTopAPIKeys([day(0, 0, 1, [])], 0)).toEqual([]);
  });
});
