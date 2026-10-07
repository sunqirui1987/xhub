import React from "react";
import { describe, it, expect } from "vitest";
import userEvent from "@testing-library/user-event";
import { renderWithProviders, screen } from "../../../tests/test-utils";
import { CostBreakdownViewer, CostBreakdown } from "./CostBreakdownViewer";

async function expandCostBreakdown() {
  const user = userEvent.setup();
  await user.click(screen.getByText("Cost Breakdown"));
}

describe("CostBreakdownViewer", () => {
  it("renders nothing when costBreakdown is null", () => {
    const { container } = renderWithProviders(<CostBreakdownViewer costBreakdown={null} totalSpend={0} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when costBreakdown is undefined", () => {
    const { container } = renderWithProviders(<CostBreakdownViewer costBreakdown={undefined} totalSpend={0} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("renders cost breakdown with input and output costs", async () => {
    renderWithProviders(
      <CostBreakdownViewer
        costBreakdown={{
          input_cost: 0.001,
          output_cost: 0.002,
          total_cost: 0.003,
          original_cost: 0.003,
        }}
        totalSpend={0.003}
        promptTokens={100}
        completionTokens={200}
      />,
    );

    expect(screen.getByText("Cost Breakdown")).toBeInTheDocument();
    await expandCostBreakdown();
    expect(screen.getByText("Input Cost:")).toBeInTheDocument();
    expect(screen.getByText("Output Cost:")).toBeInTheDocument();
    expect(screen.getByText("Final Calculated Cost:")).toBeInTheDocument();
  });

  it("shows token counts when the panel is expanded", async () => {
    renderWithProviders(
      <CostBreakdownViewer
        costBreakdown={{
          input_cost: 0.001,
          output_cost: 0.002,
        }}
        totalSpend={0.003}
        promptTokens={500}
        completionTokens={200}
      />,
    );

    await expandCostBreakdown();

    expect(screen.getByText("Input Cost:")).toBeInTheDocument();
    expect(screen.getByText("Output Cost:")).toBeInTheDocument();
    expect(screen.getByText(/500 prompt tokens/)).toBeInTheDocument();
    expect(screen.getByText(/200 completion tokens/)).toBeInTheDocument();
  });

  it("shows the token times rate formula in English", async () => {
    renderWithProviders(
      <CostBreakdownViewer
        costBreakdown={{
          input_cost: 0.027556,
          output_cost: 0.00018,
          original_cost: 0.027736,
          total_cost: 0.027736,
          input_cost_per_token: 0.000004,
          output_cost_per_token: 0.00002,
        }}
        totalSpend={0.027736}
        promptTokens={6889}
        completionTokens={9}
      />,
    );

    await expandCostBreakdown();
    expect(screen.getByText(/0.69万 prompt tokens × \$4.00\/1M/)).toBeInTheDocument();
    expect(screen.getByText(/9 completion tokens × \$20.00\/1M/)).toBeInTheDocument();
    expect(screen.getAllByText("$0.03").length).toBeGreaterThan(0);
    expect(screen.getByText("Original LLM Cost:")).toBeInTheDocument();
    expect(screen.getByText("Final Calculated Cost:")).toBeInTheDocument();
  });

  it("shows non-null, non-zero additional_costs", async () => {
    renderWithProviders(
      <CostBreakdownViewer
        costBreakdown={{
          input_cost: 0.001,
          output_cost: 0.002,
          total_cost: 0.00312,
          original_cost: 0.00312,
          additional_costs: {
            "Azure Model Router Flat Cost": 0.00012,
            "Routing Fee": 0.0005,
          },
        }}
        totalSpend={0.00312}
        promptTokens={100}
        completionTokens={200}
      />,
    );

    await expandCostBreakdown();
    expect(screen.getByText("Azure Model Router Flat Cost:")).toBeInTheDocument();
    expect(screen.getByText("Routing Fee:")).toBeInTheDocument();
  });

  it("filters out null and zero additional_costs", async () => {
    renderWithProviders(
      <CostBreakdownViewer
        costBreakdown={{
          input_cost: 0.001,
          output_cost: 0.002,
          total_cost: 0.00312,
          original_cost: 0.00312,
          additional_costs: {
            "Azure Model Router Flat Cost": 0.00012,
            "Zero Cost": 0,
            "Null Cost": null as unknown as number,
          },
        }}
        totalSpend={0.00312}
        promptTokens={100}
        completionTokens={200}
      />,
    );

    await expandCostBreakdown();
    expect(screen.getByText("Azure Model Router Flat Cost:")).toBeInTheDocument();
    expect(screen.queryByText("Zero Cost:")).not.toBeInTheDocument();
    expect(screen.queryByText("Null Cost:")).not.toBeInTheDocument();
  });

  it("renders when only additional_costs exist (no input/output costs)", async () => {
    const { container } = renderWithProviders(
      <CostBreakdownViewer
        costBreakdown={{
          total_cost: 0.00012,
          original_cost: 0.00012,
          additional_costs: {
            "Model Router Flat Cost": 0.00012,
          },
        }}
        totalSpend={0.00012}
      />,
    );

    expect(screen.getByText("Cost Breakdown")).toBeInTheDocument();
    await expandCostBreakdown();
    expect(screen.getByText("Model Router Flat Cost:")).toBeInTheDocument();
    expect(container).not.toBeEmptyDOMElement();
  });

  it("returns null when additional_costs are all null/zero", () => {
    const { container } = renderWithProviders(
      <CostBreakdownViewer
        costBreakdown={{
          additional_costs: {
            Zero: 0,
            Null: null as unknown as number,
          },
        }}
        totalSpend={0}
      />,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it("expands to show additional_costs on click", async () => {
    renderWithProviders(
      <CostBreakdownViewer
        costBreakdown={{
          input_cost: 0.001,
          output_cost: 0.002,
          total_cost: 0.00312,
          original_cost: 0.00312,
          additional_costs: {
            "Azure Model Router Flat Cost": 0.00012,
          },
        }}
        totalSpend={0.00312}
        promptTokens={100}
        completionTokens={200}
      />,
    );

    expect(screen.queryByText("Azure Model Router Flat Cost:")).not.toBeInTheDocument();
    await expandCostBreakdown();
    expect(screen.getByText("Azure Model Router Flat Cost:")).toBeInTheDocument();
  });

  it("labels provider prompt cache line items as Prompt Cache Read/Write Cost", async () => {
    renderWithProviders(
      <CostBreakdownViewer
        costBreakdown={{
          input_cost: 0.01,
          output_cost: 0.02,
          cache_read_cost: 0.001,
          cache_creation_cost: 0.002,
        }}
        totalSpend={0.03}
        cacheReadTokens={100}
        cacheCreationTokens={50}
      />,
    );

    await expandCostBreakdown();

    expect(screen.getByText("Prompt Cache Read Cost:")).toBeInTheDocument();
    expect(screen.getByText("Prompt Cache Write Cost:")).toBeInTheDocument();
  });

  it("shows '(Cached)' in the header when cacheHit is true", () => {
    const breakdown: CostBreakdown = {
      input_cost: 0.001,
      output_cost: 0.002,
      total_cost: 0.003,
    };

    renderWithProviders(<CostBreakdownViewer costBreakdown={breakdown} totalSpend={0} cacheHit="true" />);

    expect(screen.getByText(/\(Cached\)/)).toBeInTheDocument();
  });

  it("shows '(Cached)' when cacheHit is the boolean true", () => {
    const breakdown: CostBreakdown = {
      input_cost: 0.001,
      output_cost: 0.002,
      total_cost: 0.003,
    };

    renderWithProviders(<CostBreakdownViewer costBreakdown={breakdown} totalSpend={0} cacheHit={true} />);

    expect(screen.getByText(/\(Cached\)/)).toBeInTheDocument();
  });

  it("renders when cacheHit is the boolean false", () => {
    const { container } = renderWithProviders(
      <CostBreakdownViewer costBreakdown={null} totalSpend={0} cacheHit={false} />,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it("shows discount label with percentage when panel is expanded", async () => {
    const breakdown: CostBreakdown = {
      input_cost: 0.01,
      output_cost: 0.02,
      discount_percent: 0.1,
      discount_amount: 0.003,
    };

    renderWithProviders(<CostBreakdownViewer costBreakdown={breakdown} totalSpend={0.027} />);

    await expandCostBreakdown();

    expect(screen.getByText(/Discount \(10\.00%\)/)).toBeInTheDocument();
  });

  it("shows margin label with percentage when panel is expanded", async () => {
    const breakdown: CostBreakdown = {
      input_cost: 0.01,
      output_cost: 0.02,
      margin_percent: 0.15,
      margin_total_amount: 0.005,
    };

    renderWithProviders(<CostBreakdownViewer costBreakdown={breakdown} totalSpend={0.035} />);

    await expandCostBreakdown();

    expect(screen.getByText(/Margin \(15\.00%\)/)).toBeInTheDocument();
    expect(screen.getByText("Final Calculated Cost:")).toBeInTheDocument();
  });

  it("marks a peak-hour call so the doubled rate is explainable", async () => {
    const breakdown: CostBreakdown = {
      input_cost: 0.002,
      output_cost: 0.004,
      total_cost: 0.006,
      window: "peak",
    };

    renderWithProviders(<CostBreakdownViewer costBreakdown={breakdown} totalSpend={0.006} />);

    expect(screen.getByText("Peak hours")).toBeInTheDocument();
  });

  it("marks an off-peak call distinctly from a peak one", async () => {
    const peak: CostBreakdown = { input_cost: 0.002, output_cost: 0.004, total_cost: 0.006, window: "peak" };
    const { unmount } = renderWithProviders(<CostBreakdownViewer costBreakdown={peak} totalSpend={0.006} />);
    expect(screen.getByText("Peak hours")).toBeInTheDocument();
    unmount();

    const offpeak: CostBreakdown = {
      input_cost: 0.001,
      output_cost: 0.002,
      total_cost: 0.003,
      window: "offpeak",
    };
    renderWithProviders(<CostBreakdownViewer costBreakdown={offpeak} totalSpend={0.003} />);
    expect(screen.queryByText("Peak hours")).not.toBeInTheDocument();
  });

  it("labels a recalculated row as recalculated instead of as the original record", async () => {
    const breakdown: CostBreakdown = {
      input_cost: 0.01,
      output_cost: 0.02,
      total_cost: 0.03,
      source: "recomputed",
    };

    renderWithProviders(<CostBreakdownViewer costBreakdown={breakdown} totalSpend={0.03} />);

    await expandCostBreakdown();

    expect(screen.getByText(/predates stored rates/)).toBeInTheDocument();
  });

  it("does not label a stored snapshot as recalculated", async () => {
    const breakdown: CostBreakdown = {
      input_cost: 0.01,
      output_cost: 0.02,
      total_cost: 0.03,
      source: "snapshot",
    };

    renderWithProviders(<CostBreakdownViewer costBreakdown={breakdown} totalSpend={0.03} />);

    await expandCostBreakdown();

    expect(screen.queryByText(/predates stored rates/)).not.toBeInTheDocument();
  });

  it("lists the rates that were applied, each with its own unit", async () => {
    const breakdown: CostBreakdown = {
      input_cost: 0.00084,
      cache_read_cost: 0.00024,
      output_cost: 0.0003,
      total_cost: 0.00114,
      source: "snapshot",
      applied: [
        { measure: "token", side: "cache_read", usd: 0.0000003, quantity: 800, cost: 0.00024 },
        { measure: "token", side: "input", variant: "uncached", usd: 0.000003, quantity: 200, cost: 0.0006 },
        { measure: "token", side: "output", usd: 0.000006, quantity: 50, cost: 0.0003 },
      ],
    };

    renderWithProviders(<CostBreakdownViewer costBreakdown={breakdown} totalSpend={0.00114} />);

    await expandCostBreakdown();

    expect(screen.getByText("Rates applied")).toBeInTheDocument();
    expect(screen.getByText("Cache read")).toBeInTheDocument();
    expect(screen.getByText("Input · Uncached")).toBeInTheDocument();
    // 每个数量都带自己的单位，不假设都是 token。
    expect(screen.getByText("800 tokens")).toBeInTheDocument();
    expect(screen.getByText("50 tokens")).toBeInTheDocument();
    // token 单价按百万显示。
    // 单价按百万显示，用的是有效数字而不是固定两位，所以 0.000003 是 $3 /1M。
    expect(screen.getByText("$3 /1M")).toBeInTheDocument();
  });

  it("shows a per-second rate in its own unit rather than as a token rate", async () => {
    const breakdown: CostBreakdown = {
      output_cost: 0.4,
      total_cost: 0.4,
      source: "snapshot",
      applied: [{ measure: "second", side: "output", usd: 0.05, quantity: 8, cost: 0.4 }],
    };

    renderWithProviders(<CostBreakdownViewer costBreakdown={breakdown} totalSpend={0.4} />);

    await expandCostBreakdown();

    expect(screen.getByText("8 seconds")).toBeInTheDocument();
    expect(screen.getByText("$0.05")).toBeInTheDocument();
    expect(screen.queryByText(/\/1M/)).not.toBeInTheDocument();
  });

  it("still renders a breakdown when only the cache side was charged", async () => {
    const breakdown: CostBreakdown = {
      input_cost: 0.00024,
      cache_read_cost: 0.00024,
      total_cost: 0.00024,
      source: "snapshot",
    };

    renderWithProviders(<CostBreakdownViewer costBreakdown={breakdown} totalSpend={0.00024} />);

    await expandCostBreakdown();

    expect(screen.getByText("Prompt Cache Read Cost:")).toBeInTheDocument();
    expect(screen.getByText("Final Calculated Cost:")).toBeInTheDocument();
  });

  it("never shows a negative input cost when the cache side outweighs the input side", async () => {
    // 缓存那部分比输入侧还大：输入侧只算了未命中的一小段，缓存侧单独一行。
    // 抽屉把缓存那两行减掉就是为了避免重复计，减完会变成负数——那是两处口径
    // 对不上，不能让运维看到"输入成本 -$0.0002"。
    const breakdown: CostBreakdown = {
      input_cost: 0.00006,
      cache_read_cost: 0.00024,
      total_cost: 0.0003,
      source: "snapshot",
    };

    renderWithProviders(<CostBreakdownViewer costBreakdown={breakdown} totalSpend={0.00024} />);

    await expandCostBreakdown();

    // 整段提示都命中缓存时未命中那一部分是零。要断言的正是它没有变成负数。
    expect(document.body.textContent).not.toMatch(/\$-\d/);
    expect(document.body.textContent).toContain("$0.00");
  });
});

