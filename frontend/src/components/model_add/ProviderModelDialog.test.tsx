import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ProviderModelDialog } from "./ProviderModelDialog";
import { upsertPriceModel } from "@/components/networking";

const modelCreateCall = vi.fn();
const apiPost = vi.fn();
vi.mock("@/components/networking", () => ({
  apiClient: { post: (...args: unknown[]) => apiPost(...args) },
  upsertPriceModel: vi.fn(),
  modelCreateCall: (...args: unknown[]) => modelCreateCall(...args),
}));

const supplier = {
  credential_name: "fenno-work",
  credential_values: { api_key: "secret" },
  credential_info: { custom_llm_provider: "openai", builtin: "fennoai" },
};

describe("ProviderModelDialog catalog import", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiPost.mockResolvedValue({
      models: [
        { id: "gpt-priced", category: "llm", input_price: 2, output_price: 8 },
        { id: "image-unpriced", category: "image", input_price: 5, output_price: 9 },
      ],
    });
    vi.mocked(upsertPriceModel).mockResolvedValue({} as never);
  });

  it("imports selected rows through /price/model without creating a deployment", async () => {
    const user = userEvent.setup();
    const onAdded = vi.fn();
    render(
      <ProviderModelDialog
        provider="fennoai"
        initialCredentialName="fenno-work"
        credentials={[supplier]}
        accessToken="token"
        onClose={vi.fn()}
        onAdded={onAdded}
      />,
    );

    await user.click(await screen.findByRole("checkbox", { name: "gpt-priced" }));
    await user.click(screen.getByRole("checkbox", { name: "image-unpriced" }));
    await user.click(screen.getByRole("button", { name: "导入所选模型及价格" }));

    expect(apiPost).toHaveBeenCalledWith(
      "/model/builtin/models",
      expect.objectContaining({
        accessToken: "token",
        body: { provider: "fennoai", credential_name: "fenno-work" },
      }),
    );
    await waitFor(() => expect(upsertPriceModel).toHaveBeenCalledTimes(2));
    expect(upsertPriceModel).toHaveBeenNthCalledWith(1, "token", {
      id: "fenno-work/gpt-priced",
      upstream_model: "gpt-priced",
      supplier_name: "fenno-work",
      litellm_provider: "openai",
      display_name: "gpt-priced",
      source: "provider-import",
      endpoint_id: "chat",
      input_cost_per_token: 0.000002,
      output_cost_per_token: 0.000008,
    });
    expect(upsertPriceModel).toHaveBeenNthCalledWith(2, "token", {
      id: "fenno-work/image-unpriced",
      upstream_model: "image-unpriced",
      supplier_name: "fenno-work",
      litellm_provider: "openai",
      display_name: "image-unpriced",
      source: "provider-import",
    });
    expect(modelCreateCall).not.toHaveBeenCalled();
    expect(onAdded).toHaveBeenCalledOnce();
  });
});
