/** @vitest-environment jsdom */
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import { grantedModelCards } from "./grantedModelCards";
import { MyModels } from "./MyModels";

const cards = grantedModelCards(
  ["gpt-allowed", "eye-allowed", "unpriced", "fennoai", "gpt-allowed", " "],
  [
    {
      model_name: "gpt-allowed",
      model_info: { category: "llm", provider: "openai", input_price: 1.25, output_price: 10 },
    },
    {
      model_name: "eye-allowed",
      model_info: { category: "vision", provider: "qiniu", input_price: 2, output_price: 4 },
    },
    { model_name: "unpriced", model_info: { category: "llm" } },
    { model_name: "catalog-only-model", model_info: { category: "llm", input_price: 9, output_price: 9 } },
    { model_name: "fennoai", model_info: { role: "provider", category: "llm" } },
  ],
);

describe("MyModels", () => {
  beforeEach(() => setActiveLocale("en"));

  it("only lists granted model ids, removing duplicates and provider shells", () => {
    expect(cards.map((card) => card.id)).toEqual(["gpt-allowed", "eye-allowed", "unpriced"]);
    render(<MyModels models={cards} />);
    expect(screen.getByRole("heading", { name: "My models" })).toBeInTheDocument();
    expect(screen.getByText("$1.25")).toBeInTheDocument();
    expect(screen.getAllByText("Price unavailable")).toHaveLength(2);
    expect(screen.queryByText("catalog-only-model")).not.toBeInTheDocument();
    expect(screen.queryByText("fennoai")).not.toBeInTheDocument();
    expect(within(screen.getByTestId("my-model-gpt-allowed")).getByText("Available")).toBeInTheDocument();
    expect(
      within(screen.getByTestId("my-model-gpt-allowed")).getByRole("link", { name: /Use in Playground/ }),
    ).toHaveAttribute("href", "/ui/playground");
  });

  it("uses optional price metadata without granting catalog-only models", () => {
    const enriched = grantedModelCards(
      ["gpt-5.5"],
      [{ model_name: "gpt-5.5", model_info: { role: "model", input_price: 7 } }],
      {
        "gpt-5.5": {
          mode: "chat",
          litellm_provider: "openai",
          supports_function_calling: true,
          max_input_tokens: 128000,
          max_output_tokens: 16000,
          input_cost_per_token: 0.000005,
          output_cost_per_token: 0.00003,
          cache_read_input_token_cost: 0.0000005,
          cache_creation_input_token_cost: 0.00000625,
        },
        "catalog-only-model": { input_cost_per_token: 0.000009 },
      },
    );
    expect(enriched).toMatchObject([
      {
        id: "gpt-5.5",
        category: "chat",
        provider: "openai",
        capabilities: ["tools"],
        max_input_tokens: 128000,
        max_output_tokens: 16000,
        input_price: 7,
        output_price: 30,
        cache_read_price: 0.5,
        cache_write_price: 6.25,
      },
    ]);
    render(<MyModels models={enriched} />);
    expect(screen.getByText("$7")).toBeInTheDocument();
    expect(screen.getByText("$30")).toBeInTheDocument();
    expect(screen.getByText("Cache Read $0.5")).toBeInTheDocument();
    expect(screen.getByText("Cache Write $6.25")).toBeInTheDocument();
    expect(screen.getByText("128,000 tokens")).toBeInTheDocument();
    expect(screen.getByText("16,000 tokens")).toBeInTheDocument();
  });

  it("renders granted ids even when optional model info cannot be loaded", () => {
    expect(grantedModelCards(["still-available"], []).map((card) => card.id)).toEqual(["still-available"]);
  });

  it("searches, filters by type and provider, sorts and clears filters", async () => {
    const user = userEvent.setup();
    render(<MyModels models={cards} />);
    await user.type(screen.getByRole("textbox", { name: "Search" }), "eye");
    expect(screen.getByTestId("my-model-eye-allowed")).toBeInTheDocument();
    expect(screen.queryByTestId("my-model-gpt-allowed")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Clear filters" }));
    await user.click(screen.getByRole("button", { name: "Filters" }));
    await user.click(screen.getByRole("button", { name: "vision" }));
    expect(screen.queryByTestId("my-model-gpt-allowed")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Clear filters" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Provider" }), "openai");
    expect(screen.getByTestId("my-model-gpt-allowed")).toBeInTheDocument();
    expect(screen.queryByTestId("my-model-eye-allowed")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Clear filters" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Sort models" }), "input");
    expect(screen.getAllByTestId(/^my-model-/).map((node) => node.getAttribute("data-testid"))).toEqual([
      "my-model-gpt-allowed",
      "my-model-unpriced",
      "my-model-eye-allowed",
    ]);
  });

  it("shows loading, retryable error, empty account and no-match states", async () => {
    const { rerender } = render(<MyModels models={[]} isLoading />);
    expect(screen.getByText("Loading your models…")).toBeInTheDocument();
    const onRetry = vi.fn();
    rerender(<MyModels models={[]} isError onRetry={onRetry} />);
    await userEvent.setup().click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
    rerender(<MyModels models={[]} />);
    expect(screen.getByText("No models are available to this account")).toBeInTheDocument();
    rerender(<MyModels models={cards} />);
    await userEvent.setup().type(screen.getByRole("textbox", { name: "Search" }), "not-found");
    expect(screen.getByText("No matching models")).toBeInTheDocument();
  });
});
