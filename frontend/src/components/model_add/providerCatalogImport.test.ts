import { describe, expect, it } from "vitest";
import { providerCatalogEntry } from "./providerCatalogImport";

const supplier = {
  credential_name: "relay-a",
  credential_values: {},
  credential_info: { custom_llm_provider: "openai" },
};

describe("providerCatalogEntry", () => {
  it("names imported rows by supplier and converts explicit token prices per million", () => {
    expect(
      providerCatalogEntry({ id: "gpt-x", category: "llm", input_price: 0, output_price: 12 }, supplier),
    ).toMatchObject({
      id: "relay-a/gpt-x",
      upstream_model: "gpt-x",
      supplier_name: "relay-a",
      litellm_provider: "openai",
      input_cost_per_token: 0,
      output_cost_per_token: 0.000012,
    });
  });

  it("does not reinterpret image prices as token prices", () => {
    const row = providerCatalogEntry({ id: "image-x", category: "image", input_price: 2, output_price: 4 }, supplier);
    expect(row).not.toHaveProperty("input_cost_per_token");
    expect(row).not.toHaveProperty("output_cost_per_token");
  });
});
