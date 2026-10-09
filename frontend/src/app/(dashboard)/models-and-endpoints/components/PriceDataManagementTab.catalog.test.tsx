/* @vitest-environment jsdom */
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import { getPriceCatalog } from "@/components/networking";
import PriceDataManagementTab from "./PriceDataManagementTab";
import { PriceCatalog } from "./PriceCatalog";

vi.mock("@/components/networking", () => ({
  getPriceCatalog: vi.fn(),
  upsertPriceModel: vi.fn(),
  deletePriceModel: vi.fn(),
  resetPriceModel: vi.fn(),
  upsertPriceProvider: vi.fn(),
  deletePriceProvider: vi.fn(),
  cancelModelCostMapReload: vi.fn(),
  getModelCostMapReloadStatus: vi.fn(),
  getModelCostMapSource: vi.fn(),
  reloadModelCostMap: vi.fn(),
  scheduleModelCostMapReload: vi.fn(),
  apiClient: { get: vi.fn(), post: vi.fn() },
  getProxyBaseUrl: vi.fn(() => ""),
  modelInfoCall: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: () => ({ accessToken: "sk-test" }) }));

const chatIds = Array.from({ length: 61 }, (_, index) => `chat-${String(index).padStart(2, "0")}`);
const models: Record<string, unknown> = {
  "embed-one": { mode: "embedding", litellm_provider: "alpha", input_cost_per_token: 0.0000025 },
  "blank-one": { mode: "chat", baseline: true },
  "zero-rate": { mode: "chat", baseline: true, input_cost_per_token: 0, output_cost_per_token: 0 },
};
for (const id of chatIds) {
  models[id] = { mode: "chat", litellm_provider: "alpha", baseline: true, input_cost_per_token: 0.00000125 };
}

const catalog = {
  models,
  providers: [
    { litellm_provider: "alpha", provider_display_name: "Alpha", model_count: 62, baseline: true },
    { litellm_provider: "acme", provider_display_name: "Acme", model_count: 0, baseline: false },
  ],
  source: "https://api.modelink.ai/v1/market/models",
  generated_at: "2026-10-07T00:00:00Z",
};

/** 渲染独立目录页面；无参数，返回测试视图，查询不重试，DOM及缓存由测试清理。 */
function renderScreen() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <PriceDataManagementTab />
    </QueryClientProvider>,
  );
}

describe("PriceDataManagementTab", () => {
  beforeEach(() => {
    setActiveLocale("en");
    vi.clearAllMocks();
    vi.mocked(getPriceCatalog).mockResolvedValue(catalog as never);
  });

  /** 前置旧本地目录；验证无市场声明的模型只留在本地，且供应商标签已移除；测试结束清理DOM。 */
  it("keeps legacy records locally without exposing them in the marketplace", async () => {
    const user = userEvent.setup();
    renderScreen();
    expect(await screen.findByText("No matching models")).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Providers" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "本地模型列表" }));
    expect(screen.getByRole("status")).toHaveTextContent("本地共 64 个模型");
    expect(screen.getByTestId("local-model-blank-one")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  /** 前置公开、私有、下架市场记录；验证目录仅展示在售公开模型及编辑入口；DOM自动清理。 */
  it("shows only public sale models and permits editing", async () => {
    vi.mocked(getPriceCatalog).mockResolvedValue({
      ...catalog,
      models: {
        public: { market_catalog: true, baseline: true, mode: "chat" },
        private: { market_catalog: true, private: true },
        down: { market_catalog: true, delisted: true },
      },
    } as never);
    renderScreen();
    const card = await screen.findByTestId("price-row-public");
    expect(screen.getAllByRole("article")).toHaveLength(1);
    expect(screen.getByTestId("price-add-model")).toBeInTheDocument();
    expect(within(card).getByRole("button", { name: "Edit" })).toBeInTheDocument();
    expect(within(card).getByRole("button", { name: "下架" })).toBeInTheDocument();
    expect(within(card).queryByRole("button", { name: "Restore the built-in price" })).not.toBeInTheDocument();
  });
});

describe("PriceCatalog filtering", () => {
  const rows = [
    { id: "alpha-chat", category: "chat", provider: "alpha", input: 1, output: 2 },
    { id: "alpha-embed", category: "embedding", provider: "alpha", input: 3, output: 4 },
    { id: "beta-chat", category: "chat", provider: "beta", input: 5, output: 6 },
    { id: "gamma-chat", category: "chat", provider: null, input: null, output: 2 },
  ].map((row) => ({
    ...row,
    displayName: null,
    maxInput: null,
    maxOutput: null,
    capabilities: [],
    extraPrices: [],
    rateGroups: [],
    windowed: false,
    baseline: true,
    overridden: false,
    removed: false,
    raw: {},
  }));

  beforeEach(() => setActiveLocale("en"));

  it("combines provider, category and search filters and clears an empty result", async () => {
    const user = userEvent.setup();
    render(<PriceCatalog rows={rows} />);
    await user.click(screen.getByRole("button", { name: "More filters" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Provider" }), "alpha");
    await user.click(screen.getByRole("button", { name: "chat" }));
    expect(screen.getAllByRole("article")).toHaveLength(1);
    expect(screen.getByText("alpha-chat")).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Search" }), "missing");
    expect(screen.getByText("No matching models")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(screen.getAllByRole("article")).toHaveLength(4);
  });

  it("sorts an unavailable rate last on either side", async () => {
    const user = userEvent.setup();
    render(<PriceCatalog rows={rows} />);
    const order = () => screen.getAllByRole("article").map((card) => within(card).getByRole("heading").textContent);
    await user.selectOptions(screen.getByRole("combobox", { name: "Sort models" }), "input");
    expect(order()).toEqual(["alpha-chat", "alpha-embed", "beta-chat", "gamma-chat"]);
    await user.selectOptions(screen.getByRole("combobox", { name: "Sort models" }), "output");
    expect(order()).toEqual(["alpha-chat", "gamma-chat", "alpha-embed", "beta-chat"]);
  });

  it("filters from the always-visible manufacturer, modality, and capability controls", async () => {
    const user = userEvent.setup();
    const filterRows = rows.map((row) =>
      row.id === "beta-chat"
        ? {
            ...row,
            maxInput: 200_000,
            capabilities: ["reasoning"],
            rateGroups: [{ measure: "token", unitSize: 1, rates: [], windows: [] }],
          }
        : row,
    );
    render(<PriceCatalog rows={filterRows} />);
    await user.click(screen.getByRole("button", { name: "beta" }));
    await user.click(within(screen.getByRole("group", { name: "Modalities" })).getByRole("button", { name: "Text" }));
    await user.click(
      within(screen.getByRole("group", { name: "Context & capabilities" })).getByRole("button", {
        name: "128K+ context",
      }),
    );
    expect(screen.getAllByRole("article")).toHaveLength(1);
    expect(screen.getByText("beta-chat")).toBeInTheDocument();
  });

  it("URL-encodes the catalog id in the deploy link", () => {
    render(<PriceCatalog rows={[{ ...rows[0], id: "supplier/model one" }]} />);
    expect(screen.getByRole("link", { name: "Deploy model" })).toHaveAttribute(
      "href",
      "/models-and-endpoints?catalog=supplier%2Fmodel%20one",
    );
  });

  it("labels normalized token rates per 1M without showing source unit_size", () => {
    const tokenRow = {
      ...rows[0],
      rateGroups: [
        {
          measure: "token",
          unitSize: 1000,
          windows: ["all"],
          rates: [
            {
              measure: "token",
              unitSize: 1000,
              side: "input",
              variant: "",
              window: "all",
              sourceKey: "input",
              label: "",
              usd: 0.000005,
            },
          ],
        },
      ],
    };
    render(<PriceCatalog rows={[tokenRow]} />);
    expect(screen.getByText("$5 /1M")).toBeInTheDocument();
    expect(screen.queryByText("per 1000")).not.toBeInTheDocument();
  });
});
