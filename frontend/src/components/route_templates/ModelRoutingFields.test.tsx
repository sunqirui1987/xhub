import React, { useState } from "react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { chooseSelectOption, renderWithProviders, screen } from "../../../tests/test-utils";
import ModelRoutingFields from "./ModelRoutingFields";
import type { ModelRoutingRule, SplitDeployment } from "./templateForm";

const deployments: SplitDeployment[] = [
  { model_name: "chat", model: "openai/gpt-4o", api_base: "https://east", weight: "2" },
  { model_name: "chat", model: "openai/gpt-4o", api_base: "https://west" },
  { model_name: "embed", model: "openai/embed", api_base: "https://embed" },
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
    await chooseSelectOption(user, screen.getByRole("combobox", { name: "Public model" }), "chat");
    await user.click(screen.getByRole("button", { name: "Add rule" }));
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
    await user.click(screen.getAllByRole("button", { name: "Override weight" })[0]);
    const changed = onChange.mock.calls.at(-1)?.[0][0];
    expect(changed.audit_tag).toBe("keep");
    expect(changed.routing_strategy_args.custom).toBe("keep");
    expect(changed.routing_strategy_args.weights).toEqual([
      { model_name: "chat", api_base: "https://east", model: "openai/gpt-4o", weight: 2 },
    ]);
    expect(changed).not.toHaveProperty("num_retries");
    expect(changed.routing_strategy_args).not.toHaveProperty("ttl");
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
