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

  it("renders the models from the catalog API", async () => {
    const user = userEvent.setup();
    renderScreen();
    // A row with no token rate must read as unavailable, never as $0.
    expect(await screen.findByTestId("price-row-blank-one")).toBeInTheDocument();
    expect(within(screen.getByTestId("price-row-blank-one")).getAllByText("Price unavailable")).toHaveLength(2);
    // A genuine zero is kept and shown.
    await user.type(screen.getByRole("textbox", { name: "Search" }), "zero-rate");
    expect(within(screen.getByTestId("price-row-zero-rate")).getAllByText("$0")).toHaveLength(2);
    // A per-token rate is displayed per 1M tokens, without float noise.
    await user.clear(screen.getByRole("textbox", { name: "Search" }));
    await user.type(screen.getByRole("textbox", { name: "Search" }), "embed-one");
    expect(within(screen.getByTestId("price-row-embed-one")).getByText("$2.5")).toBeInTheDocument();
    expect(
      within(screen.getByTestId("price-row-embed-one")).getByRole("link", { name: "Deploy model" }),
    ).toHaveAttribute("href", "/models-and-endpoints?catalog=embed-one");
  });

  it("hides the models past the first page until asked", async () => {
    const user = userEvent.setup();
    renderScreen();
    await screen.findByText("chat-00");
    expect(screen.getAllByRole("article")).toHaveLength(60);
    await user.click(screen.getByRole("button", { name: /Show more models/ }));
    expect(screen.getAllByRole("article")).toHaveLength(64);
  });

  it("offers add and edit only on the models the catalog owns", async () => {
    const user = userEvent.setup();
    renderScreen();
    await screen.findByText("chat-00");
    expect(screen.getByTestId("price-add-model")).toBeInTheDocument();
    expect(within(screen.getByTestId("price-row-chat-00")).getByRole("button", { name: "Edit" })).toBeInTheDocument();
    // Reset restores the generated row, so it is only offered once a row is overridden.
    expect(
      within(screen.getByTestId("price-row-chat-00")).queryByRole("button", { name: "Restore the built-in price" }),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Providers" }));
    expect(screen.getByText("Acme")).toBeInTheDocument();
    expect(screen.getByTestId("price-add-provider")).toBeInTheDocument();
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
