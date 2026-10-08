import { describe, expect, it } from "vitest";
import {
  bodyFromForm,
  documentFromSettingsResponse,
  emptyForm,
  formFromBody,
  nextCopyName,
  omitUntouchedRoutingGroups,
  parseRoutingGroups,
  prettyRoutingGroups,
  parseDocument,
  prefillFromPlatform,
  weightRowsForEditor,
} from "./templateForm";

describe("formFromBody", () => {
  it("keeps keys the form does not edit so a save does not drop them", () => {
    const form = formFromBody({
      routing_strategy: "least-busy",
      num_retries: 4,
      model_group_alias: { fast: "gpt-4o" },
      custom_flag: true,
      fallbacks: [{ "gpt-4o": ["claude"] }],
    });
    expect(form.routing_strategy).toBe("least-busy");
    expect(form.num_retries).toBe("4");
    expect(form.fallbacks).toEqual([{ primary: "gpt-4o", targets: "claude" }]);
    expect(JSON.parse(form.model_group_alias)).toEqual({ fast: "gpt-4o" });
    expect(JSON.parse(form.extra)).toEqual({ custom_flag: true });
  });

  it("treats a missing cause list as all three, which is what the request path does", () => {
    expect(formFromBody({}).fallback_causes).toEqual(["no_response", "status", "ambiguous"]);
  });

  it("keeps a cause list that dropped the ambiguous timeout case", () => {
    expect(formFromBody({ fallback_causes: ["no_response", "status"] }).fallback_causes).toEqual([
      "no_response",
      "status",
    ]);
  });
});

describe("bodyFromForm", () => {
  it("round-trips a chain and the keys it did not edit", () => {
    const form = formFromBody({
      routing_strategy: "weighted-split",
      num_retries: 2,
      timeout: 90,
      fallbacks: [{ "gpt-4o": ["claude", "gemini"] }],
      retry_policy: { RateLimitErrorRetries: 1 },
    });
    form.fallbacks = [{ primary: "gpt-4o", targets: "claude, gemini" }];
    const written = bodyFromForm(form);
    expect(written.ok).toBe(true);
    if (!written.ok) return;
    expect(written.body.routing_strategy).toBe("weighted-split");
    expect(written.body.timeout).toBe(90);
    expect(written.body.fallbacks).toEqual([{ "gpt-4o": ["claude", "gemini"] }]);
    expect(JSON.parse(form.retry_policy)).toEqual({ RateLimitErrorRetries: 1 });
    expect(written.body.retry_policy).toEqual({ RateLimitErrorRetries: 1 });
  });

  it("omits a number the operator cleared, and refuses one that is not a number", () => {
    const cleared = emptyForm();
    cleared.timeout = "";
    const written = bodyFromForm(cleared);
    expect(written.ok).toBe(true);
    if (!written.ok) return;
    expect(written.body).not.toHaveProperty("timeout");

    const broken = emptyForm();
    broken.num_retries = "abc";
    const refused = bodyFromForm(broken);
    expect(refused).toEqual({ ok: false, field: "num_retries" });
  });
});

describe("prefillFromPlatform", () => {
  it("copies the routing numbers and leaves routing groups behind", () => {
    const form = prefillFromPlatform({
      routing_strategy: "cost-based-routing",
      num_retries: 3,
      timeout: 30,
      routing_groups: [{ name: "fast" }],
      fallbacks: [{ a: ["b"] }],
    });
    expect(form.routing_strategy).toBe("cost-based-routing");
    expect(form.num_retries).toBe("3");
    expect(form.fallbacks).toEqual([{ primary: "a", targets: "b" }]);
    expect(form.routing_groups).toContain("fast");
  });
});

describe("weights", () => {
  it("round-trips shares inside the same document and keeps other strategy args", () => {
    const form = formFromBody({
      routing_strategy: "weighted-split",
      routing_strategy_args: {
        ttl: 60,
        weights: [{ model_name: "gpt-4o", api_base: "https://a", model: "gpt-4o", weight: 70 }],
      },
    });
    expect(form.weights).toEqual([{ model_name: "gpt-4o", api_base: "https://a", model: "gpt-4o", weight: "70" }]);
    expect(form.ttl).toBe("60");
    expect(form.extra).toBe("");
    const written = bodyFromForm(form);
    expect(written.ok).toBe(true);
    if (!written.ok) return;
    expect(written.body.routing_strategy_args).toEqual({
      ttl: 60,
      weights: [{ model_name: "gpt-4o", api_base: "https://a", model: "gpt-4o", weight: 70 }],
    });
  });

  it("offers a row per live deployment and keeps a share the catalog does not have yet", () => {
    const rows = weightRowsForEditor(
      [{ model_name: "old", api_base: "https://old", model: "old", weight: "4" }],
      [{ model_name: "gpt-4o", api_base: "https://a", model: "gpt-4o" }],
    );
    expect(rows.map((row) => row.model)).toEqual(["gpt-4o", "old"]);
    expect(rows[0].weight).toBe("1");
  });

  it("refuses a configuration file that is not one object", () => {
    expect(parseDocument("[]").ok).toBe(false);
    expect(parseDocument("{").ok).toBe(false);
    expect(parseDocument('{"routing_strategy":"weighted-split"}').ok).toBe(true);
  });
});

describe("ui and json", () => {
  it("round-trips a full LiteLLM router document through the form", () => {
    const document = {
      routing_strategy: "latency-based-routing",
      routing_strategy_args: { ttl: 3600, lowest_latency_buffer: 0.1 },
      routing_groups: [{ group_name: "hot", models: ["gpt-4o"], routing_strategy: "least-busy" }],
      num_retries: 3,
      timeout: 10,
      stream_timeout: 30,
      max_fallbacks: 2,
      fallbacks: [{ "zephyr-beta": ["gpt-3.5-turbo"] }],
      context_window_fallbacks: [{ "gpt-4": ["gpt-4-32k"] }],
      content_policy_fallbacks: [{ "claude-2": ["my-fallback-model"] }],
      default_fallbacks: ["gpt-3.5-turbo"],
      allowed_fails: 3,
      cooldown_time: 30,
      retry_after: 1,
      retry_policy: { RateLimitErrorRetries: 2 },
      model_group_alias: { cheap: "gpt-4o-mini" },
      enable_pre_call_checks: true,
      enable_tag_filtering: false,
      fallback_causes: ["no_response", "status", "ambiguous"],
    };
    const written = bodyFromForm(formFromBody(document));
    expect(written.ok).toBe(true);
    if (!written.ok) return;
    expect(written.body).toEqual(document);
  });
});

describe("nextCopyName", () => {
  it("preserves zero weights and rejects malformed shares", () => {
    const form = formFromBody({ routing_strategy_args: { weights: [{ api_base: "a", model: "m", weight: 0 }] } });
    const result = bodyFromForm(form);
    expect(result.ok && result.body.routing_strategy_args).toEqual({ weights: [{ model_name: "m", api_base: "a", model: "m", weight: 0 }] });
    for (const weight of ["-1", "invalid", "Infinity"]) {
      expect(bodyFromForm({ ...form, weights: [{ ...form.weights[0], weight }] })).toEqual({ ok: false, field: "weights" });
    }
  });
  it("numbers a second copy so the two names stay distinct", () => {
    const copyOf = (name: string) => `${name} 的副本`;
    expect(nextCopyName("便宜优先", [], copyOf)).toBe("便宜优先 的副本");
    expect(nextCopyName("便宜优先", ["便宜优先 的副本"], copyOf)).toBe("便宜优先 的副本 2");
  });
});

describe("documentFromSettingsResponse", () => {
  it("reads the stored document, then the current values", () => {
    expect(documentFromSettingsResponse({ router_settings: { timeout: 10 } })).toEqual({ timeout: 10 });
    expect(documentFromSettingsResponse({ current_values: { timeout: 4 } })).toEqual({ timeout: 4 });
    expect(documentFromSettingsResponse(null)).toEqual({});
  });
});

describe("omitUntouchedRoutingGroups", () => {
  it("leaves a routing group edit the operator did not touch out of the patch", () => {
    const body = { routing_strategy: "least-busy", routing_groups: [{ name: "old" }] };
    expect(omitUntouchedRoutingGroups(body, "[]", "[]")).toEqual({ routing_strategy: "least-busy" });
    expect(omitUntouchedRoutingGroups(body, '[{"name":"new"}]', "[]")).toEqual(body);
  });
});

describe("template routing groups", () => {
  it("round-trips groups used by the visual editor", () => {
    const groups = [
      { group_name: "fast", models: ["gpt-4o", "gpt-4o-mini"], routing_strategy: "least-busy" },
    ];
    expect(parseRoutingGroups(prettyRoutingGroups(groups))).toEqual({ ok: true, groups });
  });

  it("treats an empty field as no groups", () => {
    expect(parseRoutingGroups("")).toEqual({ ok: true, groups: [] });
    expect(prettyRoutingGroups([])).toBe("");
  });

  it("refuses malformed groups instead of losing them", () => {
    expect(parseRoutingGroups('{"group_name":"fast"}')).toEqual({ ok: false });
    expect(parseRoutingGroups('[{"group_name":"fast","models":"gpt-4o","routing_strategy":"least-busy"}]')).toEqual({ ok: false });
  });
});
