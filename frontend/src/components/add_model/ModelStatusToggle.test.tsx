import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ModelStatusToggle from "./ModelStatusToggle";
import type { EditorModel } from "./modelEditorPricing";

const mocks = vi.hoisted(() => ({
  authorized: { accessToken: "token", userRole: "Admin", isViewOnly: false },
  patch: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: () => mocks.authorized }));
vi.mock("@/components/networking", () => ({
  modelPatchUpdateCall: (...args: unknown[]) => mocks.patch(...args),
}));
vi.mock("@/lib/toast", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const pricedModel: EditorModel = {
  model_name: "priced-model",
  litellm_params: {
    model: "upstream-model",
    input_cost_per_token: 0.000002,
    output_cost_per_token: 0.000006,
    rates: [{ measure: "token", side: "cache_read", window: "peak", usd: 0.000001 }],
  },
  model_info: {
    id: "model-123",
    db_model: true,
    disabled: false,
    pricing_source: "manual",
    base_model: "catalog/reference",
  },
};

function renderToggle(model: EditorModel = pricedModel) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <ModelStatusToggle model={model} />
    </QueryClientProvider>,
  );
}

describe("ModelStatusToggle", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.authorized.accessToken = "token";
    mocks.authorized.userRole = "Admin";
    mocks.authorized.isViewOnly = false;
    mocks.patch.mockResolvedValue({});
  });

  it("disables an admin-owned DB model with a status-only patch and leaves pricing intact", async () => {
    const user = userEvent.setup();
    renderToggle();

    await user.click(screen.getByRole("switch", { name: "priced-model 启用模型" }));

    await waitFor(() =>
      expect(mocks.patch).toHaveBeenCalledWith("token", { model_info: { disabled: true } }, "model-123"),
    );
    expect(pricedModel.model_info).toMatchObject({
      disabled: false,
      pricing_source: "manual",
      base_model: "catalog/reference",
    });
    expect(pricedModel.litellm_params).toMatchObject({
      input_cost_per_token: 0.000002,
      output_cost_per_token: 0.000006,
    });
  });

  it.each([
    ["a non-admin session", { userRole: "Internal User", isViewOnly: false, dbModel: true }],
    ["a readonly admin session", { userRole: "Admin", isViewOnly: true, dbModel: true }],
    ["a config model", { userRole: "Admin", isViewOnly: false, dbModel: false }],
  ])("does not allow %s to change status", async (_label, state) => {
    const user = userEvent.setup();
    mocks.authorized.userRole = state.userRole;
    mocks.authorized.isViewOnly = state.isViewOnly;
    renderToggle({
      ...pricedModel,
      model_info: { ...pricedModel.model_info, db_model: state.dbModel },
    });

    const toggle = screen.getByRole("switch", { name: "priced-model 启用模型" });
    expect(toggle).toHaveAttribute("aria-disabled", "true");
    await user.click(toggle);
    expect(mocks.patch).not.toHaveBeenCalled();
  });
});
