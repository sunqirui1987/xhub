import { describe, expect, it } from "vitest";
import { typedPublicName, withPublicName } from "./public_model_name";

describe("typedPublicName", () => {
  it("keeps a public name the user typed over the provider model", () => {
    expect(
      typedPublicName({ public_name: "e2e-added-model", litellm_model: "custom" }),
    ).toBe("e2e-added-model");
    expect(
      withPublicName(
        { public_name: "e2e-added-model", litellm_model: "custom" },
        "openai/gpt-4o-mini",
        "openai/gpt-4o-mini",
      ),
    ).toEqual({ public_name: "e2e-added-model", litellm_model: "openai/gpt-4o-mini" });
  });

  it("replaces the custom placeholder and a name that still matches the provider model", () => {
    expect(typedPublicName({ public_name: "custom", litellm_model: "custom" })).toBeUndefined();
    expect(typedPublicName({ public_name: "openai/gpt-4o-mini", litellm_model: "openai/gpt-4o-mini" })).toBeUndefined();
    expect(
      withPublicName({ public_name: "custom", litellm_model: "custom" }, "openai/gpt-4o-mini", "openai/gpt-4o-mini"),
    ).toEqual({ public_name: "openai/gpt-4o-mini", litellm_model: "openai/gpt-4o-mini" });
  });
});
