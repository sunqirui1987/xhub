import React, { useState } from "react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n";
import { chooseSelectOption, fireEvent, renderWithProviders, screen } from "../../../tests/test-utils";
import ModelRoutingFields from "./ModelRoutingFields";
import type { ModelRoutingRule, SplitDeployment } from "./templateForm";

setActiveLocale("en");

const deployments: SplitDeployment[] = [
  { deployment_id: "east", model_name: "chat", model: "openai/gpt-4o", api_base: "https://east", weight: "2" },
  { deployment_id: "west", model_name: "chat", model: "openai/gpt-4o", api_base: "https://west" },
  { deployment_id: "embed", model_name: "embed", model: "openai/embed", api_base: "https://embed" },
];
const Harness = ({
  initial = [],
  onChange = () => {},
}: {
  initial?: ModelRoutingRule[];
  onChange?: (rules: ModelRoutingRule[]) => void;
}) => {
  const [rules, setRules] = useState(initial);
  return (
    <ModelRoutingFields
      rules={rules}
      deployments={deployments}
      defaultStrategy="least-busy"
      onChange={(next) => {
        setRules(next);
        onChange(next);
      }}
    />
  );
};

describe("ModelRoutingFields", () => {
  it("adds one exact public-model rule using the default strategy", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithProviders(<Harness onChange={onChange} />);
    await chooseSelectOption(user, screen.getByRole("combobox", { name: "Public model for override" }), "chat");
    await user.click(screen.getByRole("button", { name: "Add model rule" }));
    expect(onChange).toHaveBeenLastCalledWith([{ model_name: "chat", routing_strategy: "least-busy" }]);
    expect(screen.getByText("chat")).toBeInTheDocument();
  });

  it("changes strategy while preserving custom rule fields and args", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const rule: ModelRoutingRule = {
      model_name: "chat",
      routing_strategy: "least-busy",
      routing_strategy_args: { custom: 7 },
      audit_tag: "keep",
    };
    renderWithProviders(<Harness initial={[rule]} onChange={onChange} />);
    await chooseSelectOption(user, screen.getByRole("combobox", { name: "Strategy for chat" }), "Weighted Split");
    expect(onChange).toHaveBeenLastCalledWith([{ ...rule, routing_strategy: "weighted-split" }]);
  });

  it("edits weighted args without injecting global reliability fields", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const rule: ModelRoutingRule = {
      model_name: "chat",
      routing_strategy: "weighted-split",
      routing_strategy_args: { custom: "keep" },
      audit_tag: "keep",
    };
    renderWithProviders(<Harness initial={[rule]} onChange={onChange} />);
    await user.click(screen.getAllByRole("button", { name: "Set weight" })[0]);
    const changed = onChange.mock.calls.at(-1)?.[0][0];
    expect(changed.audit_tag).toBe("keep");
    expect(changed.routing_strategy_args.custom).toBe("keep");
    expect(changed.routing_strategy_args.weights).toEqual([
      { deployment_id: "east", model_name: "chat", api_base: "https://east", model: "openai/gpt-4o", weight: 2 },
    ]);
    expect(changed).not.toHaveProperty("num_retries");
    expect(changed.routing_strategy_args).not.toHaveProperty("ttl");
  });

  it("renders stable model-routing weights and keeps them when edited", () => {
    const onChange = vi.fn();
    const rule: ModelRoutingRule = {
      model_name: "chat",
      routing_strategy: "weighted-split",
      routing_strategy_args: {
        weights: [
          { deployment_id: "east", model_name: "chat", api_base: "https://east", model: "openai/gpt-4o", weight: 7 },
          { deployment_id: "west", model_name: "chat", api_base: "https://west", model: "openai/gpt-4o", weight: 3 },
        ],
      },
    };
    renderWithProviders(<Harness initial={[rule]} onChange={onChange} />);
    const inputs = screen.getAllByRole("spinbutton", { name: "chat openai/gpt-4o Weight" });
    expect(inputs[0]).toHaveValue(7);
    expect(inputs[1]).toHaveValue(3);
    expect(screen.getByText("Approx. 70%")).toBeInTheDocument();
    expect(screen.getByText("Approx. 30%")).toBeInTheDocument();

    fireEvent.change(inputs[0], { target: { value: "8" } });
    expect(onChange.mock.calls.at(-1)?.[0][0].routing_strategy_args.weights).toEqual([
      { deployment_id: "east", model_name: "chat", api_base: "https://east", model: "openai/gpt-4o", weight: 8 },
      { deployment_id: "west", model_name: "chat", api_base: "https://west", model: "openai/gpt-4o", weight: 3 },
    ]);
  });

  it("applies a stable-ID-only weight inside its exact model rule", () => {
    const rule: ModelRoutingRule = {
      model_name: "chat",
      routing_strategy: "weighted-split",
      routing_strategy_args: { weights: [{ deployment_id: "east", weight: 7 }] },
    };
    renderWithProviders(<Harness initial={[rule]} />);
    expect(screen.getByRole("spinbutton", { name: "chat openai/gpt-4o Weight" })).toHaveValue(7);
    expect(screen.queryByText("Not in current catalog")).not.toBeInTheDocument();
  });

  it("retains invalid raw weight drafts and custom args, then normalizes a corrected value", () => {
    const onChange = vi.fn();
    const rule: ModelRoutingRule = {
      model_name: "chat",
      routing_strategy: "weighted-split",
      routing_strategy_args: {
        custom: "keep",
        weights: [{ deployment_id: "east", model_name: "chat", api_base: "https://east", model: "openai/gpt-4o", weight: 2 }],
      },
    };
    renderWithProviders(<Harness initial={[rule]} onChange={onChange} />);
    const input = screen.getByRole("spinbutton", { name: "chat openai/gpt-4o Weight" });

    fireEvent.change(input, { target: { value: "" } });
    expect(onChange.mock.calls.at(-1)?.[0][0].routing_strategy_args).toEqual({
      custom: "keep",
      weights: [{ deployment_id: "east", model_name: "chat", api_base: "https://east", model: "openai/gpt-4o", weight: "" }],
    });

    fireEvent.change(input, { target: { value: "-2" } });
    expect(onChange.mock.calls.at(-1)?.[0][0].routing_strategy_args.weights[0].weight).toBe("-2");

    fireEvent.change(input, { target: { value: "5" } });
    expect(onChange.mock.calls.at(-1)?.[0][0].routing_strategy_args).toEqual({
      custom: "keep",
      weights: [{ deployment_id: "east", model_name: "chat", api_base: "https://east", model: "openai/gpt-4o", weight: 5 }],
    });
  });

  it("removes only the requested rule", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const rules: ModelRoutingRule[] = [
      { model_name: "chat", routing_strategy: "least-busy" },
      { model_name: "embed", routing_strategy: "simple-shuffle" },
    ];
    renderWithProviders(<Harness initial={rules} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: "Remove rule for chat" }));
    expect(onChange).toHaveBeenLastCalledWith([rules[1]]);
  });
});
