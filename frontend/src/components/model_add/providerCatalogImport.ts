import type { CredentialItem } from "@/components/networking";
export type ListedModel = { id: string; category?: string; input_price?: number | null; output_price?: number | null };
export function providerCatalogEntry(model: ListedModel, supplier: CredentialItem): Record<string, unknown> {
  const protocol =
    supplier.credential_info?.custom_llm_provider ?? supplier.credential_values?.custom_llm_provider ?? "openai";
  const row: Record<string, unknown> = {
    id: supplier.credential_name + "/" + model.id,
    upstream_model: model.id,
    supplier_name: supplier.credential_name,
    litellm_provider: protocol,
    display_name: model.id,
    source: "provider-import",
  };
  // This API's input/output_price fields are USD per million tokens. Do not
  // interpret prices from media categories as token prices.
  if (["llm", "chat", "completion", "vision", "embedding", "text"].includes(model.category ?? "")) {
    if (model.category === "embedding") row.endpoint_id = "embedding";
    else row.endpoint_id = "chat";
    for (const side of ["input", "output"] as const) {
      const price = model[(side + "_price") as "input_price" | "output_price"];
      if (typeof price === "number" && Number.isFinite(price) && price >= 0)
        row[side + "_cost_per_token"] = price / 1e6;
    }
  }
  return row;
}
