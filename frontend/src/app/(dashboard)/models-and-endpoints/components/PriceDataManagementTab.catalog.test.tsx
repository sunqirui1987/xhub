/* @vitest-environment jsdom */
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import {
  cancelModelCostMapReload,
  getModelCostMapReloadStatus,
  getModelCostMapSource,
  modelCostMap,
  reloadModelCostMap,
  scheduleModelCostMapReload,
} from "@/components/networking";
import { priceCatalogRows } from "./priceCatalogRows";
import PriceDataManagementTab from "./PriceDataManagementTab";
import { PriceCatalog } from "./PriceCatalog";

vi.mock("@/components/networking", () => ({
  modelCostMap: vi.fn(),
  cancelModelCostMapReload: vi.fn(),
  getModelCostMapReloadStatus: vi.fn(),
  getModelCostMapSource: vi.fn(),
  reloadModelCostMap: vi.fn(),
  scheduleModelCostMapReload: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: () => ({ accessToken: "sk-test" }) }));

const chatIds = Array.from({ length: 51 }, (_, index) => `chat-${String(index).padStart(2, "0")}`);
const priceMap: Record<string, unknown> = {
  sample_spec: { input_cost_per_token: 0, output_cost_per_token: 0, litellm_provider: "docs" },
  note: "not a model",
  "embed-one": { mode: "embedding", input_cost_per_token: 0.0000025, output_cost_per_token: 0.000008 },
  "blank-one": { mode: "chat" },
  "zero-rate": { mode: "chat", input_cost_per_token: 0, output_cost_per_token: 0 },
  "no-mode-one": { input_cost_per_token: 0.00000125, output_cost_per_token: 0.00001 },
};
for (const id of chatIds) {
  priceMap[id] = { mode: "chat", input_cost_per_token: 0.00000125, output_cost_per_token: 0.00001 };
}
const modelIds = [...chatIds, "embed-one", "blank-one", "zero-rate", "no-mode-one"];

function renderScreen() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <PriceDataManagementTab />
    </QueryClientProvider>,
  );
}

describe("PriceDataManagementTab catalog", () => {
  beforeEach(() => {
    setActiveLocale("en");
    vi.clearAllMocks();
    vi.mocked(modelCostMap).mockResolvedValue(priceMap);
    const reloadStatus = {
      scheduled: false,
      interval_hours: null,
      last_run: null,
      next_run: null,
    };
    vi.mocked(getModelCostMapReloadStatus).mockResolvedValue(reloadStatus);
    vi.mocked(getModelCostMapSource).mockResolvedValue({
      source: "local",
      url: null,
      is_env_forced: false,
      fallback_reason: null,
      loaded_at: null,
      source_revision: null,
      etag: null,
      model_count: 3,
    } as never);
  });

  it("maps every model, keeps a zero rate, and skips sample_spec", () => {
    const rows = priceCatalogRows(priceMap);
    expect(rows.map((row) => row.id).sort()).toEqual([...modelIds].sort());
    expect(rows.find((row) => row.id === "chat-00")).toMatchObject({ category: "chat", input: 1.25, output: 10 });
    expect(rows.find((row) => row.id === "embed-one")).toMatchObject({ category: "embedding", input: 2.5, output: 8 });
    expect(rows.find((row) => row.id === "zero-rate")).toMatchObject({ input: 0, output: 0 });
    expect(rows.find((row) => row.id === "blank-one")).toMatchObject({ category: "chat", input: null, output: null });
    expect(rows.find((row) => row.id === "no-mode-one")).toMatchObject({ category: "other", input: 1.25, output: 10 });
    expect(rows.some((row) => row.id === "sample_spec")).toBe(false);
  });

  it("lists the full catalog and filters by category and name without reloading or rescheduling", async () => {
    const user = userEvent.setup();
    renderScreen();

    expect(await screen.findByText("chat-50")).toBeInTheDocument();
    for (const id of modelIds) {
      expect(screen.getByText(id)).toBeInTheDocument();
    }
    expect(within(screen.getByTestId("price-row-chat-00")).getByText("$1.25")).toBeInTheDocument();
    expect(within(screen.getByTestId("price-row-embed-one")).getByText("$2.5")).toBeInTheDocument();
    expect(within(screen.getByTestId("price-row-embed-one")).getByText("$8")).toBeInTheDocument();
    expect(within(screen.getByTestId("price-row-zero-rate")).getAllByText("$0")).toHaveLength(2);
    expect(screen.getAllByText(/Price unavailable/).length).toBeGreaterThan(0);
    expect(screen.queryByText("sample_spec")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Filters" }));
    await user.click(screen.getByRole("button", { name: "embedding" }));
    expect(screen.getByText("embed-one")).toBeInTheDocument();
    expect(screen.queryByText("chat-00")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "All" }));
    await user.type(screen.getByRole("textbox", { name: "Search" }), "embed-one");
    expect(screen.getByText("embed-one")).toBeInTheDocument();
    expect(screen.queryByText("chat-00")).not.toBeInTheDocument();
    expect(reloadModelCostMap).not.toHaveBeenCalled();
    expect(scheduleModelCostMapReload).not.toHaveBeenCalled();
    expect(cancelModelCostMapReload).not.toHaveBeenCalled();
  });

  it("combines provider, type, and search filters and clears an empty result", async () => {
    const user = userEvent.setup();
    render(
      <PriceCatalog
        costMap={{
          "alpha-chat": { mode: "chat", litellm_provider: "alpha" },
          "alpha-embed": { mode: "embedding", provider: "alpha" },
          "beta-chat": { mode: "chat", litellm_provider: "beta" },
        }}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Filters" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Provider" }), "alpha");
    await user.click(screen.getByRole("button", { name: "chat" }));
    expect(screen.getAllByRole("article")).toHaveLength(1);
    expect(screen.getByText("alpha-chat")).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Search" }), "missing");
    expect(screen.getByText("No matching models")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(screen.getAllByRole("article")).toHaveLength(3);
  });

  it("sorts zero, positive and unavailable prices correctly on either side", async () => {
    const user = userEvent.setup();
    render(
      <PriceCatalog
        costMap={{
          "a-missing": { mode: "chat" },
          "b-paid": { input_cost_per_token: 0.000001, output_cost_per_token: 0 },
          "c-free": { input_cost_per_token: 0, output_cost_per_token: 0.000002 },
        }}
      />,
    );
    const order = () => screen.getAllByRole("article").map((card) => within(card).getByRole("heading").textContent);
    await user.selectOptions(screen.getByRole("combobox", { name: "Sort models" }), "input");
    expect(order()).toEqual(["c-free", "b-paid", "a-missing"]);
    await user.selectOptions(screen.getByRole("combobox", { name: "Sort models" }), "output");
    expect(order()).toEqual(["b-paid", "c-free", "a-missing"]);
  });

  it("preserves small prices and the distinct units of extra billing items", async () => {
    const user = userEvent.setup();
    render(
      <PriceCatalog
        costMap={{
          tiny: {
            input_cost_per_token: 0.00000000001,
            cache_read_input_token_cost: 0.0000001,
            output_cost_per_image: 0.04,
            output_cost_per_second: 0.12,
            supports_vision: true,
          },
        }}
      />,
    );
    expect(screen.getByText("$0.00001")).toBeInTheDocument();
    expect(screen.getByText("Vision")).toBeInTheDocument();
    await user.click(screen.getByText("More pricing (3)"));
    const prices = screen.getAllByRole("definition");
    expect(prices[2]).toHaveTextContent("$0.1 / 1M tokens");
    expect(prices[3]).toHaveTextContent("$0.04 / image");
    expect(prices[4]).toHaveTextContent("$0.12 / second");
  });

  it("rejects invalid numeric prices without turning them into free prices", () => {
    const rows = priceCatalogRows({
      bad: { input_cost_per_token: -1, output_cost_per_token: Infinity },
      overflow: { input_cost_per_token: Number.MAX_VALUE, output_cost_per_token: "0" },
    });
    expect(rows.every((row) => row.input === null && row.output === null)).toBe(true);
  });

  it("loads the rest of a large catalog on demand", async () => {
    const user = userEvent.setup();
    render(
      <PriceCatalog
        costMap={Object.fromEntries(Array.from({ length: 61 }, (_, i) => ["model-" + i, { mode: "chat" }]))}
      />,
    );
    expect(screen.getAllByRole("article")).toHaveLength(60);
    await user.click(screen.getByRole("button", { name: "Show more models (1 remaining)" }));
    expect(screen.getAllByRole("article")).toHaveLength(61);
  });

  it("keeps reload management available and updates the catalog after reloading", async () => {
    const user = userEvent.setup();
    renderScreen();
    await screen.findByText("chat-50");
    expect(screen.queryByRole("button", { name: "Reload Price Data" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Price Data Management" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    vi.mocked(modelCostMap).mockResolvedValue({ "fresh-model": { input_cost_per_token: 0.000003 } });
    vi.mocked(reloadModelCostMap).mockResolvedValue({ status: "success", models_count: 1 });
    await user.click(screen.getByRole("button", { name: "Reload Price Data" }));
    await user.click(screen.getByRole("button", { name: "Yes" }));
    await waitFor(() => expect(reloadModelCostMap).toHaveBeenCalledWith("sk-test"));
    await user.keyboard("{Escape}");
    expect(await screen.findByText("fresh-model")).toBeInTheDocument();
    expect(screen.queryByText("chat-50")).not.toBeInTheDocument();
  });
});
