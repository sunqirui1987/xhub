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
  it("reads the same keys the request path reads", () => {
    const summary = summarizeTemplate(
      { routing_strategy: "lowest-cost", num_retries: 5, fallbacks: [{ "gpt-4o": ["claude"] }] },
      labels,
    );
    expect(summary).toBe("strategy:lowest-cost · 5 retries · 60s · 1 fallbacks");
  });

  it("reads an explicit timeout instead of the 60 second default", () => {
    expect(summarizeTemplate({ timeout: 90 }, labels)).toContain("90s");
    expect(summarizeTemplate({ timeout: 0 }, labels)).toContain("60s");
  });

  it("falls back to the defaults a template need not spell out", () => {
    // A template seeded from the platform default has no reason to name the
    // strategy, so a blank summary would be wrong about what it does.
    expect(summarizeTemplate({}, labels)).toBe("strategy:simple-shuffle · 1 retries · 60s · no fallbacks");
  });

  it("reports no fallbacks when the list is absent or empty", () => {
    expect(summarizeTemplate({ fallbacks: [] }, labels)).toContain("no fallbacks");
    expect(summarizeTemplate({}, labels)).toContain("no fallbacks");
  });

  it("does not claim a fallback count from something that is not a list", () => {
    expect(summarizeTemplate({ fallbacks: "broken" }, labels)).toContain("no fallbacks");
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
