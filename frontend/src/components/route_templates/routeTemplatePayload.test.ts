import { describe, expect, it } from "vitest";
import { deleteRefusalText, summarizeTemplate, toTemplateRows } from "./routeTemplatePayload";

/**
 * The route template library's view logic.
 *
 * These are the parts worth asserting without rendering: what a template's row
 * says, and what a delete refusal means. Both turn a server answer into something
 * an operator reads, so a mistake here is a wrong statement rather than a broken
 * layout.
 */

const labels = {
  strategy: (value: string) => `strategy:${value}`,
  retries: (value: number) => `${value} retries`,
  timeout: (seconds: number) => `${seconds}s`,
  fallbacks: (value: number) => `${value} fallbacks`,
  none: "no fallbacks",
};

describe("toTemplateRows", () => {
  it("carries the usage count onto the row", () => {
    const rows = toTemplateRows([
      { id: "a", name: "A", body: {}, used_by: 3 },
      { id: "b", name: "B", body: {}, used_by: 0 },
    ]);
    expect(rows.map((row) => row.usedBy)).toEqual([3, 0]);
  });

  it("treats a missing count as none rather than as unknown", () => {
    // The list endpoint always sends it, but a row without it must not render as
    // "unknown" — the delete button reads this number to decide.
    const rows = toTemplateRows([{ id: "a", name: "A", body: {} }]);
    expect(rows[0].usedBy).toBe(0);
  });
});

describe("summarizeTemplate", () => {
  /** 前置带默认策略的正文；验证模型例外不掩盖默认行为及空规则边界，纯函数无清理副作用。 */
  it("shows template defaults and model exceptions", () => {
    expect(summarizeTemplate({ routing_strategy: "cost-based-routing", model_routes: [] }, labels)).toBe(
      "strategy:cost-based-routing · 1 retries · 60s",
    );
    expect(
      summarizeTemplate(
        { routing_strategy: "simple-shuffle", model_routes: [{ model: "a", strategy: "least-busy" }] },
        labels,
      ),
    ).toBe("strategy:simple-shuffle · 模型例外 1 条 · 1 retries · 60s");
  });
  /** 验证模型规则数量和失败策略摘要，输入为新文档，无外部清理。 */
  it("describes model rules and retry policy", () => {
    expect(summarizeTemplate({}, labels)).toBe("全部使用模型默认 · 1 retries · 60s");
    expect(
      summarizeTemplate(
        {
          model_routes: [
            { model: "a", strategy: "random" },
            { model: "b", strategy: "least-busy" },
          ],
          retry_policy: { max_attempts: 5, timeout_seconds: 90 },
        },
        labels,
      ),
    ).toBe("模型规则 2 条 · 5 retries · 90s");
    expect(
      summarizeTemplate({ model_routes: [], retry_policy: { max_attempts: 1, timeout_seconds: 60 } }, labels),
    ).toContain("全部使用模型默认");
  });
});

describe("deleteRefusalText", () => {
  it("names the scopes when the server refused for that reason", () => {
    const error = {
      body: {
        error: {
          code: "template_in_use",
          used_by: [
            { scope_type: "team", scope_id: "t1", name: "Core" },
            { scope_type: "organization", scope_id: "o1", name: "Acme" },
          ],
        },
      },
    };
    const got = deleteRefusalText(error, { inUse: (n) => `${n} still use it`, failed: "failed" });
    expect(got.message).toBe("2 still use it");
    expect(got.usage.map((u) => u.name)).toEqual(["Core", "Acme"]);
  });

  it("does not invent a cause when the failure was something else", () => {
    // A 500 or a network error must not be reported as "still in use": that
    // would send the operator looking for scopes that are not there.
    const got = deleteRefusalText(new Error("boom"), { inUse: (n) => `${n} in use`, failed: "failed" });
    expect(got.message).toBe("failed");
    expect(got.usage).toEqual([]);
  });

  it("treats a refusal with no list as a refusal with no names", () => {
    const got = deleteRefusalText(
      { body: { error: { code: "template_in_use" } } },
      { inUse: (n) => `${n} in use`, failed: "failed" },
    );
    expect(got.message).toBe("0 in use");
    expect(got.usage).toEqual([]);
  });

  it("survives a body that is not the shape it expects", () => {
    expect(deleteRefusalText({ body: "not json" }, { inUse: () => "in use", failed: "failed" }).message).toBe("failed");
    expect(deleteRefusalText(undefined, { inUse: () => "in use", failed: "failed" }).message).toBe("failed");
  });
});
