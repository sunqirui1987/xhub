import React, { useState } from "react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { chooseSelectOption, fireEvent, renderWithProviders, screen } from "../../../tests/test-utils";
import WeightedSplitFields from "./WeightedSplitFields";
import type { SplitDeployment, WeightRow } from "./templateForm";

const deployments: SplitDeployment[] = [
  {
    model_name: "chat",
    model: "openai/gpt-4o",
    api_base: "https://east.example",
    weight: "3",
    supplier: "Acme",
    provider: "azure",
  },
  { model_name: "chat", model: "openai/gpt-4o", api_base: "https://west.example" },
  { model_name: "embed", model: "openai/embed", api_base: "https://embed.example" },
];
const Harness = ({
  initial = [],
  catalog = deployments,
  modelName,
  onChange = () => {},
}: {
  initial?: WeightRow[];
  catalog?: SplitDeployment[];
  modelName?: string;
  onChange?: (rows: WeightRow[]) => void;
}) => {
  const [rows, setRows] = useState(initial);
  return (
    <WeightedSplitFields
      saved={rows}
      deployments={catalog}
      modelName={modelName}
      onChange={(next) => {
        setRows(next);
        onChange(next);
      }}
    />
  );
};

describe("WeightedSplitFields", () => {
  it("opens one public model and shows all effective candidates without auto-seeding overrides", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithProviders(<Harness onChange={onChange} />);
    expect(screen.queryByText("https://east.example")).not.toBeInTheDocument();
    await chooseSelectOption(user, screen.getByRole("combobox", { name: "Public model" }), "chat");
    expect(screen.getByText("https://east.example")).toBeInTheDocument();
    expect(screen.getByText("https://west.example")).toBeInTheDocument();
    expect(screen.getByText("Acme · azure")).toBeInTheDocument();
    expect(screen.getByText("Estimated 75%")).toBeInTheDocument();
    expect(screen.getByText("Estimated 25%")).toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("adds and resets only the selected deployment override", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithProviders(<Harness modelName="chat" onChange={onChange} />);
    const buttons = screen.getAllByRole("button", { name: "Override weight" });
    await user.click(buttons[1]);
    expect(onChange).toHaveBeenLastCalledWith([
      { model_name: "chat", model: "openai/gpt-4o", api_base: "https://west.example", weight: "1" },
    ]);
    await user.click(screen.getByRole("button", { name: "Reset override for openai/gpt-4o · https://west.example" }));
    expect(onChange).toHaveBeenLastCalledWith([]);
  });

  it("handles zero effective weights without rendering bars", () => {
    renderWithProviders(
      <Harness modelName="chat" catalog={deployments.slice(0, 2).map((row) => ({ ...row, weight: "0" }))} />,
    );
    expect(
      screen.getByText("All eligible deployments have weight 0, so no estimated split is available."),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Estimated 0%")).toHaveLength(2);
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });

  it("preserves an unknown saved row and excludes it from the estimate", () => {
    const onChange = vi.fn();
    const unknown: WeightRow = {
      model_name: "chat",
      model: "custom/retired",
      api_base: "https://retired.example",
      weight: "4",
    };
    renderWithProviders(<Harness modelName="chat" initial={[unknown]} onChange={onChange} />);
    expect(screen.getByText("Missing from deployment catalog")).toBeInTheDocument();
    expect(screen.getByText("Excluded from estimate")).toBeInTheDocument();
    expect(screen.getByText("Estimated 75%")).toBeInTheDocument();
    fireEvent.change(screen.getByRole("spinbutton", { name: "chat custom/retired Weight" }), {
      target: { value: "9" },
    });
    expect(onChange).toHaveBeenLastCalledWith([{ ...unknown, weight: "9" }]);
  });

  it("explains that a single deployment does not require a split", () => {
    renderWithProviders(<Harness modelName="embed" />);
    expect(screen.getByText("This model has one deployment, so no split is required.")).toBeInTheDocument();
  });
});
