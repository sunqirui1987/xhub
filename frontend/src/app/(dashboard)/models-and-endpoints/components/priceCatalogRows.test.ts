import { describe, expect, it } from "vitest";
import {
  parseRate,
  priceCatalogProviders,
  priceCatalogRows,
  priceModelPayload,
  rateGroupsOf,
  rateToInputValue,
  unitRateToInputValue,
} from "./priceCatalogRows";

const doc = (models: Record<string, unknown>, providers?: unknown) => ({ models, providers });

describe("priceCatalogRows", () => {
  it("converts per-token rates to per-million and keeps the category from mode", () => {
    const rows = priceCatalogRows(
      doc({
        "chat-00": { mode: "chat", input_cost_per_token: 0.00000125, output_cost_per_token: 0.00001 },
        "embed-one": { mode: "embedding", input_cost_per_token: 0.0000025, output_cost_per_token: 0.000008 },
      }),
    );
    expect(rows.map((row) => row.id)).toEqual(["chat-00", "embed-one"]);
    expect(rows[0]).toMatchObject({ category: "chat", input: 1.25, output: 10 });
    expect(rows[1]).toMatchObject({ category: "embedding", input: 2.5, output: 8 });
  });

  it("keeps a genuine zero rate and leaves a missing rate unavailable", () => {
    const rows = priceCatalogRows(doc({ "zero-rate": { mode: "chat", input_cost_per_token: 0 }, "blank-one": {} }));
    expect(rows.find((row) => row.id === "zero-rate")).toMatchObject({ input: 0, output: null });
    // $0 and "not priced" are different facts. Only the first one may render as $0.
    expect(rows.find((row) => row.id === "blank-one")).toMatchObject({ input: null, output: null });
  });

  it("rejects negative, infinite and overflowing rates instead of showing them as free", () => {
    const rows = priceCatalogRows(
      doc({
        bad: { input_cost_per_token: -1, output_cost_per_token: Infinity },
        overflow: { input_cost_per_token: Number.MAX_VALUE, output_cost_per_token: "0" },
      }),
    );
    expect(rows.every((row) => row.input === null && row.output === null)).toBe(true);
  });

  it("falls back to one category for a row with no mode", () => {
    const rows = priceCatalogRows(doc({ "no-mode-one": { input_cost_per_token: 0.000001 } }));
    expect(rows[0]).toMatchObject({ category: "other", input: 1 });
  });

  it("reads providers from litellm_provider, falling back to provider", () => {
    const rows = priceCatalogRows(
      doc({
        "alpha-chat": { mode: "chat", litellm_provider: "alpha", display_name: "Alpha Chat" },
        "beta-chat": { mode: "chat", provider: "beta" },
      }),
    );
    expect(rows.find((row) => row.id === "alpha-chat")).toMatchObject({
      provider: "alpha",
      displayName: "Alpha Chat",
    });
    expect(rows.find((row) => row.id === "beta-chat")).toMatchObject({ provider: "beta" });
  });

  it("accepts the models map as either an object or a list, and drops rows without an id", () => {
    const fromObject = priceCatalogRows(doc({ "a-one": { mode: "chat" }, "": { mode: "chat" } }));
    expect(fromObject.map((row) => row.id)).toEqual(["a-one"]);
    const fromList = priceCatalogRows({
      models: [{ id: "a-one", mode: "chat" }, { mode: "chat" }, "not-an-object"],
    });
    expect(fromList.map((row) => row.id)).toEqual(["a-one"]);
  });

  it("reports nothing for a malformed document rather than throwing", () => {
    expect(priceCatalogRows(null)).toEqual([]);
    expect(priceCatalogRows({ models: "nope" })).toEqual([]);
    expect(priceCatalogRows({ models: { x: "not-an-object" } })).toEqual([]);
  });

  it("separates extra prices by unit", () => {
    const rows = priceCatalogRows(
      doc({
        tiny: {
          input_cost_per_token: 0.00000000001,
          cache_read_input_token_cost: 0.0000001,
          output_cost_per_image: 0.04,
          output_cost_per_second: 0.12,
          supports_vision: true,
        },
      }),
    );
    expect(rows[0].input).toBeCloseTo(0.00001, 12);
    expect(rows[0].capabilities).toEqual(["vision"]);
    expect(rows[0].extraPrices).toEqual([
      { key: "cacheRead", value: 0.1, unit: "tokens" },
      { key: "imageOutput", value: 0.04, unit: "image" },
      { key: "secondOutput", value: 0.12, unit: "second" },
    ]);
  });

  it("carries the baseline and override flags the page edits by", () => {
    const rows = priceCatalogRows(
      doc({
        "from-file": { mode: "chat", baseline: true, overridden: false },
        edited: { mode: "chat", baseline: true, overridden: true },
        "removed-one": { baseline: true, overridden: false, removed: true },
        added: { mode: "chat", baseline: false, overridden: false },
      }),
    );
    expect(rows.find((row) => row.id === "from-file")).toMatchObject({ baseline: true, overridden: false });
    expect(rows.find((row) => row.id === "edited")).toMatchObject({ baseline: true, overridden: true });
    expect(rows.find((row) => row.id === "removed-one")).toMatchObject({ removed: true });
    expect(rows.find((row) => row.id === "added")).toMatchObject({ baseline: false });
  });

  it("keeps the raw row so the edit form can seed rates it does not show", () => {
    const rows = priceCatalogRows(
      doc({ model: { mode: "chat", input_cost_per_token: 0.000002, endpoint_type: "ark_contents_generation" } }),
    );
    expect(rows[0].raw.endpoint_type).toBe("ark_contents_generation");
  });
});

describe("priceCatalogProviders", () => {
  it("reads the slug, display name, base and model count", () => {
    const providers = priceCatalogProviders({
      providers: [
        {
          litellm_provider: "acme",
          provider_display_name: "Acme",
          default_api_base: "https://api.acme.test",
          model_count: 4,
          baseline: false,
        },
      ],
    });
    expect(providers).toEqual([
      { slug: "acme", name: "Acme", apiBase: "https://api.acme.test", modelCount: 4, baseline: false, overridden: false },
    ]);
  });

  it("drops an entry with no slug and tolerates a missing list", () => {
    expect(priceCatalogProviders({ providers: [{ provider: "no slug" }] })).toEqual([]);
    expect(priceCatalogProviders({})).toEqual([]);
    expect(priceCatalogProviders(null)).toEqual([]);
  });
});

describe("priceModelPayload", () => {
  it("divides token rates back to per-token and leaves unit rates alone", () => {
    const body = priceModelPayload({
      id: " acme-one ",
      provider: " acme ",
      displayName: "Acme One",
      mode: "chat",
      endpointType: "ark_contents_generation",
      tokenRates: { input_cost_per_token: "3", output_cost_per_token: "15" },
      unitRates: { output_cost_per_image: "0.04" },
    });
    expect(body).toMatchObject({
      id: "acme-one",
      litellm_provider: "acme",
      display_name: "Acme One",
      mode: "chat",
      endpoint_type: "ark_contents_generation",
      input_cost_per_token: 0.000003,
      output_cost_per_token: 0.000015,
      output_cost_per_image: 0.04,
    });
  });

  it("sends null for a blank rate so the backend clears it instead of billing zero", () => {
    const body = priceModelPayload({
      id: "acme-one",
      provider: "acme",
      tokenRates: { input_cost_per_token: "", output_cost_per_token: "0" },
      unitRates: { output_cost_per_second: "  " },
    });
    expect(body.input_cost_per_token).toBeNull();
    expect(body.output_cost_per_token).toBe(0);
    expect(body.output_cost_per_second).toBeNull();
  });

  it("omits a display name, mode and endpoint type that were left blank", () => {
    const body = priceModelPayload({
      id: "acme-one",
      provider: "acme",
      displayName: "",
      mode: "",
      endpointType: "",
      tokenRates: {},
      unitRates: {},
    });
    expect(body).not.toHaveProperty("display_name");
    expect(body).not.toHaveProperty("mode");
    expect(body).not.toHaveProperty("endpoint_type");
  });
});

describe("parseRate", () => {
  it("reads a decimal and rejects anything that is not a non-negative number", () => {
    expect(parseRate("1.25")).toBe(1.25);
    expect(parseRate("0")).toBe(0);
    expect(parseRate("")).toBeNull();
    expect(parseRate("   ")).toBeNull();
    expect(parseRate("-1")).toBeNull();
    expect(parseRate("abc")).toBeNull();
    expect(parseRate(null)).toBeNull();
  });
});

describe("rate formatting for the edit form", () => {
  it("scales per-token rates to what the input shows", () => {
    expect(rateToInputValue(0.000003)).toBe("3");
    // A per-token rate that a person typed as 1e-11 must not come back as "1e-5".
    expect(rateToInputValue(0.00000000001)).toBe("0.00001");
    expect(rateToInputValue(undefined)).toBe("");
  });

  it("shows a per-unit rate unchanged", () => {
    expect(unitRateToInputValue(0.04)).toBe("0.04");
    expect(unitRateToInputValue(0)).toBe("0");
    expect(unitRateToInputValue(null)).toBe("");
  });
});

describe("rateGroupsOf", () => {
  it("groups rates by the measure they are counted in", () => {
    const groups = rateGroupsOf([
      { measure: "token", unit_size: 1, side: "input", window: "all", source_key: "input", usd: 0.000003 },
      { measure: "picture", unit_size: 1, side: "output", window: "all", source_key: "ti_quantity", usd: 0.04 },
    ]);
    expect(groups.map((group) => group.measure)).toEqual(["token", "picture"]);
  });

  it("keeps both the off-peak and peak rate of one side, off-peak first", () => {
    const groups = rateGroupsOf([
      { measure: "token", unit_size: 1, side: "output", window: "peak", source_key: "output_peak", usd: 0.0000039 },
      { measure: "token", unit_size: 1, side: "output", window: "offpeak", source_key: "output_offpeak", usd: 0.00000195 },
    ]);
    expect(groups).toHaveLength(1);
    expect(groups[0].rates.map((rate) => rate.window)).toEqual(["offpeak", "peak"]);
    expect(groups[0].windows).toEqual(["offpeak", "peak"]);
  });

  it("does not merge the two sides of one measure into a single rate", () => {
    const groups = rateGroupsOf([
      { measure: "token", unit_size: 1, side: "input", window: "all", source_key: "input", usd: 0.000003 },
      { measure: "token", unit_size: 1, side: "output", window: "all", source_key: "output", usd: 0.000015 },
    ]);
    expect(new Set(groups[0].rates.map((rate) => rate.side)).size).toBe(2);
  });

  it("keeps the unit size, which is not 1 for token rates", () => {
    const groups = rateGroupsOf([
      { measure: "token", unit_size: 1000, side: "input", window: "all", source_key: "wiv_v_output", usd: 0.0000033 },
    ]);
    expect(groups[0].unitSize).toBe(1000);
  });

  it("drops malformed entries without losing the rest of the model's rates", () => {
    const groups = rateGroupsOf([
      { measure: "token", side: "input", window: "all", source_key: "input", usd: 0.000003 },
      { side: "output", window: "all", usd: 0.000015 },
      { measure: "token", side: "output", window: "all", usd: null },
      "not an object",
      null,
    ]);
    expect(groups).toHaveLength(1);
    expect(groups[0].rates).toHaveLength(1);
    // A missing unit size must not become 0, which would divide the price away.
    expect(groups[0].unitSize).toBe(1);
  });

  it("returns nothing for a value that is not a list rather than throwing", () => {
    expect(rateGroupsOf(undefined)).toEqual([]);
    expect(rateGroupsOf(null)).toEqual([]);
    expect(rateGroupsOf({ key: "value" })).toEqual([]);
  });

  it("still shows a measure it does not recognize", () => {
    const groups = rateGroupsOf([
      { measure: "megabyte", unit_size: 1, side: "output", window: "all", source_key: "mb_out", usd: 0.01 },
    ]);
    expect(groups).toHaveLength(1);
    expect(groups[0].measure).toBe("megabyte");
  });

  it("marks a model as time-of-day priced when any rate carries a window", () => {
    const windowed = priceCatalogRows(
      doc({ "ds/v4": { rates: [{ measure: "token", side: "input", window: "peak", usd: 0.0000013 }] } }),
    );
    expect(windowed[0].windowed).toBe(true);

    const flat = priceCatalogRows(
      doc({ "plain": { rates: [{ measure: "token", side: "input", window: "all", usd: 0.000003 }] } }),
    );
    expect(flat[0].windowed).toBe(false);
  });

  it("reports no rate groups for a row whose rates are missing", () => {
    const rows = priceCatalogRows(doc({ "no-rates": { input_cost_per_token: 0.000003 } }));
    expect(rows[0].rateGroups).toEqual([]);
    expect(rows[0].windowed).toBe(false);
  });
});
