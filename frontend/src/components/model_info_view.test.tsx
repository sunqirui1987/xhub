import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ModelInfoView from "./model_info_view";
import { testConnectionRequest } from "./networking";

const useModelsInfo = vi.fn();
const modelEditor = vi.fn();
const modelStatusToggle = vi.fn();

vi.mock("@/app/(dashboard)/hooks/models/useModels", () => ({
  useModelsInfo: (...args: unknown[]) => useModelsInfo(...args),
}));
vi.mock("@/app/(dashboard)/hooks/models/useModelCostMap", () => ({
  useModelCostMap: () => ({ data: { "catalog/gpt": { input_cost_per_token: 0.000001 } } }),
}));
vi.mock("@/app/(dashboard)/hooks/teams/useTeams", () => ({ useTeams: () => ({ data: [] }) }));
vi.mock("@/app/(dashboard)/hooks/credentials/useCredentials", () => ({
  useCredentials: () => ({ data: { credentials: [] } }),
}));
vi.mock("./networking", () => ({ modelDeleteCall: vi.fn(), testConnectionRequest: vi.fn() }));
vi.mock("./add_model/ModelEditor", () => ({
  default: (props: Record<string, unknown>) => {
    modelEditor(props);
    return <button onClick={() => (props.onSaved as (v: unknown) => void)({ updated: true })}>shared editor</button>;
  },
}));
vi.mock("./add_model/ModelStatusToggle", () => ({
  default: ({ model }: { model: Record<string, unknown> }) => {
    modelStatusToggle(model);
    return <div data-testid="model-status-toggle" />;
  },
}));
vi.mock("./add_model/PricingTable", () => ({
  default: ({ row }: { row?: Record<string, unknown> }) => <div data-testid="pricing">{JSON.stringify(row)}</div>,
}));
vi.mock("./common_components/DeleteResourceModal", () => ({ default: () => null }));

const requested = {
  model_name: "requested-model",
  litellm_params: {
    model: "upstream-gpt",
    custom_llm_provider: "openai",
    litellm_credential_name: "supplier-a",
    input_cost_per_token: 0.000007,
  },
  model_info: { id: "model-123", db_model: true, endpoint_types: ["chat"], transport: "bypass_openai_chat", pricing_source: "catalog", base_model: "catalog/gpt" },
};
const other = { ...requested, model_name: "wrong-model", model_info: { ...requested.model_info, id: "other-id" } };
const props = {
  modelId: "model-123",
  onClose: vi.fn(),
  accessToken: "token",
  userID: "user-1",
  userRole: "Admin",
  isViewOnly: false,
  onModelUpdate: vi.fn(),
};
function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>;
}

describe("ModelInfoView reconstructed behavior", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useModelsInfo.mockReturnValue({
      data: { data: [other, requested] },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    vi.mocked(testConnectionRequest).mockResolvedValue({ status: "success" } as never);
  });

  it("queries by exact ID and renders only the exact matching deployment", () => {
    render(<ModelInfoView {...props} />, { wrapper });
    expect(useModelsInfo).toHaveBeenCalledWith(1, 1, undefined, "model-123");
    expect(screen.getByRole("heading", { name: "requested-model" })).toBeInTheDocument();
    expect(screen.queryByText("wrong-model")).not.toBeInTheDocument();
    expect(modelStatusToggle).toHaveBeenCalledWith(requested);
    expect(screen.getByTestId("model-status-toggle")).toBeInTheDocument();
    expect(screen.queryByText("所属团队")).not.toBeInTheDocument();
  });

  it("shows effective deployment pricing ahead of catalog pricing", () => {
    render(<ModelInfoView {...props} />, { wrapper });
    expect(screen.getByText("部署定价 · USD")).toBeInTheDocument();
    expect(screen.getByTestId("pricing")).toHaveTextContent('"input_cost_per_token":0.000007');
  });

  it("opens the shared editor with the exact model and forwards saves", async () => {
    const user = userEvent.setup();
    render(<ModelInfoView {...props} />, { wrapper });
    await user.click(screen.getByRole("button", { name: "编辑模型" }));
    expect(modelEditor).toHaveBeenLastCalledWith(expect.objectContaining({ model: requested }));
    await user.click(screen.getByRole("button", { name: "shared editor" }));
    await waitFor(() => expect(props.onModelUpdate).toHaveBeenCalledWith({ updated: true }));
  });

  it("hides connection and write actions for readonly sessions", () => {
    render(<ModelInfoView {...props} isViewOnly />, { wrapper });
    expect(screen.queryByRole("button", { name: "测试连接" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "编辑模型" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "删除" })).not.toBeInTheDocument();
  });

  it("tests the exact deployment connection", async () => {
    const user = userEvent.setup();
    render(<ModelInfoView {...props} />, { wrapper });
    await user.click(screen.getByRole("button", { name: "测试连接" }));
    await waitFor(() =>
      expect(testConnectionRequest).toHaveBeenCalledWith(
        "token",
        { model: "upstream-gpt", custom_llm_provider: "openai", litellm_credential_name: "supplier-a" },
        { id: "model-123", endpoint_types: ["chat"], transport: "bypass_openai_chat" },
        "chat",
      ),
    );
    expect(await screen.findByText("连接正常，模型已响应。")).toBeInTheDocument();
  });
});
