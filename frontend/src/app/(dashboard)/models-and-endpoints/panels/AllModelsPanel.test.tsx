import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AllModelsPanel from "./AllModelsPanel";

const modelStatusToggle = vi.fn();
const model = {
  model_name: "global-model",
  litellm_params: { model: "upstream-model", custom_llm_provider: "openai" },
  model_info: {
    id: "model-123",
    db_model: true,
    disabled: true,
    mode: "chat",
    pricing_source: "catalog",
    base_model: "catalog/gpt",
  },
};

vi.mock("@/app/(dashboard)/hooks/models/useModels", () => ({
  useModelsInfo: () => ({
    data: { data: [model], total_count: 1, total_pages: 1 },
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }),
}));
vi.mock("@/app/(dashboard)/hooks/models/useModelCostMap", () => ({
  useModelCostMap: () => ({ data: { "catalog/gpt": { input_cost_per_token: 0.000001 } } }),
}));
vi.mock("../detailNavigation", () => ({ useModelDetailRouting: () => ({ openModel: vi.fn() }) }));
vi.mock("@/components/add_model/ModelStatusToggle", () => ({
  default: ({ model: received }: { model: typeof model }) => {
    modelStatusToggle(received);
    return <div data-testid="list-model-status-toggle" />;
  },
}));

describe("AllModelsPanel", () => {
  beforeEach(() => vi.clearAllMocks());

  it("renders the shared status toggle for a global model without team ownership", () => {
    render(<AllModelsPanel />);

    expect(screen.getByRole("button", { name: "global-model" })).toBeInTheDocument();
    expect(screen.getByTestId("list-model-status-toggle")).toBeInTheDocument();
    expect(modelStatusToggle).toHaveBeenCalledWith(model);
    expect(screen.queryByText(/所属团队|team/i)).not.toBeInTheDocument();
  });
});
