import { describe, expect, it } from "vitest";
import {
  buildEditorModel,
  catalogFieldValue,
  catalogRows,
  editorDefaults,
  type EditorModel,
} from "./modelEditorPricing";

const baseValues = {
  supplier: "supplier-a",
  custom_llm_provider: "openai",
  model: "upstream",
  model_name: "public-name",
  mode: "chat",
  transport: "adapted",
  endpoint_types: ["chat"],
  pricing_source: "manual",
  pricing_format: "flat",
  billing_mode: "token",
};

describe("modelEditorPricing", () => {
  it("converts per-million prices, preserves zero, and leaves missing cache/peak prices null", () => {
    const result = buildEditorModel({
      ...baseValues,
      input_cost_per_token: "2.5",
      output_cost_per_token: "0",
      input_cost_per_token_peak: "",
      output_cost_per_token_peak: "",
      cache_read_input_token_cost: "",
      cache_creation_input_token_cost: "",
    });
    expect(result.litellm_params).toMatchObject({
      input_cost_per_token: 0.0000025,
      output_cost_per_token: 0,
      input_cost_per_token_peak: null,
      output_cost_per_token_peak: null,
      cache_read_input_token_cost: null,
      cache_creation_input_token_cost: null,
    });
  });

  it.each([
    ["image", "input_cost_per_image", "output_cost_per_image"],
    ["second", "input_cost_per_second", "output_cost_per_second"],
  ])("saves %s prices in native units", (billingMode, input, output) => {
    const result = buildEditorModel({ ...baseValues, billing_mode: billingMode, [input]: "0.12", [output]: "0" });
    expect(result.litellm_params[input]).toBe(0.12);
    expect(result.litellm_params[output]).toBe(0);
  });

  it("loads and stores an advanced mixed rates table with cache, peak, media, and variants intact", () => {
    const rates = [
      { measure: "token", side: "input", window: "offpeak", usd: 0.000002 },
      { measure: "token", side: "input", window: "peak", usd: 0.000004 },
      { measure: "token", side: "cache_read", window: "all", variant: "long-context", usd: 0 },
      { measure: "token", side: "cache_write", window: "all", usd: 0.000005 },
      { measure: "picture", side: "output", window: "all", variant: "1024x1024", usd: 0.08 },
      { measure: "second", side: "output", window: "all", variant: "720p", usd: 0.12 },
    ];
    const original: EditorModel = {
      model_name: "public-name",
      litellm_params: {
        model: "upstream",
        custom_llm_provider: "openai",
        litellm_credential_name: "supplier-a",
        rates,
      },
      model_info: { mode: "chat", pricing_source: "manual" },
    };
    const defaults = editorDefaults(original);
    expect(defaults).toMatchObject({ pricing_format: "rates", manual_rates: rates });
    expect(buildEditorModel(defaults, original).litellm_params.rates).toEqual(rates);
  });

  it("does not flatten qualified variants into a base price", () => {
    expect(
      catalogFieldValue(
        { rates: [{ measure: "token", side: "input", window: "all", variant: "long-context", usd: 0.00001 }] },
        "input_cost_per_token",
      ),
    ).toBe("");
  });

  it("clears embedded credentials when changing supplier", () => {
    const original: EditorModel = {
      model_name: "public-name",
      litellm_params: {
        model: "upstream",
        custom_llm_provider: "azure",
        litellm_credential_name: "old",
        api_key: "secret",
        api_base: "https://old.example",
        aws_region_name: "east",
        organization: "org",
      },
      model_info: { mode: "chat" },
    };
    const result = buildEditorModel({ ...baseValues, input_cost_per_token: "1" }, original);
    expect(result.litellm_params).toMatchObject({
      litellm_credential_name: "supplier-a",
      api_key: null,
      api_base: null,
      aws_region_name: null,
      organization: null,
    });
  });

  it("accepts catalog arrays and removes retired rows", () => {
    const rows = catalogRows([
      { id: "anthropic/claude", litellm_provider: "anthropic" },
      { id: "relay-a/private", litellm_provider: "openai", supplier_name: "relay-a" },
      { id: "relay-b/private", litellm_provider: "openai", supplier_name: "relay-b" },
      { id: "removed", removed: true },
    ]);
    expect(Object.keys(rows)).toEqual(["anthropic/claude", "relay-a/private", "relay-b/private"]);
    expect(rows).not.toHaveProperty("removed");
  });
});
