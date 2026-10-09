import React, { useState } from "react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n";
import { chooseSelectOption, fireEvent, renderWithProviders, screen } from "../../../tests/test-utils";
import WeightedSplitFields from "./WeightedSplitFields";
import type { SplitDeployment, WeightRow } from "./templateForm";

setActiveLocale("en");

const deployments: SplitDeployment[] = [
  {
    deployment_id: "east",
    model_name: "chat",
    model: "openai/gpt-4o",
    api_base: "https://east.example",
    weight: "3",
    supplier: "Acme",
    provider: "azure",
  },
  { deployment_id: "west", model_name: "chat", model: "openai/gpt-4o", api_base: "https://west.example" },
  { deployment_id: "embed", model_name: "embed", model: "openai/embed", api_base: "https://embed.example" },
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
    expect(screen.getByText("Approx. 75%")).toBeInTheDocument();
    expect(screen.getByText("Approx. 25%")).toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("adds and resets only the selected deployment override", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithProviders(<Harness modelName="chat" onChange={onChange} />);
    const buttons = screen.getAllByRole("button", { name: "Set weight" });
    await user.click(buttons[1]);
    expect(onChange).toHaveBeenLastCalledWith([
      {
        ...(deployments[1].deployment_id ? { deployment_id: deployments[1].deployment_id } : {}),
        model_name: "chat",
        model: "openai/gpt-4o",
        api_base: "https://west.example",
        weight: "1",
      },
    ]);
    await user.click(screen.getByRole("button", { name: "Reset openai/gpt-4o · https://west.example" }));
    expect(onChange).toHaveBeenLastCalledWith([]);
  });

  it("preserves stable deployment identities when adding an override", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const catalog: SplitDeployment[] = [
      {
        model_name: "chat",
        model: "openai/gpt-4o",
        api_base: "https://stable.example",
        deployment_id: "dep-1",
        pricing_id: "price-1",
      },
    ];
    renderWithProviders(<Harness modelName="chat" catalog={catalog} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: "Set weight" }));
    expect(onChange).toHaveBeenLastCalledWith([
      {
        deployment_id: "dep-1",
        pricing_id: "price-1",
        model_name: "chat",
        model: "openai/gpt-4o",
        api_base: "https://stable.example",
        weight: "1",
      },
    ]);
  });

  it("matches stable IDs and leaves physical-only overrides unmatched", () => {
    const legacyCatalogRow: SplitDeployment = {
      deployment_id: "dep-legacy",
      model_name: "chat",
      model: "openai/gpt-4o",
      api_base: "https://legacy.example",
    };
    const catalog: SplitDeployment[] = [
      {
        deployment_id: "dep-east",
        model_name: "chat",
        model: "openai/gpt-4o",
        api_base: "https://east.example",
      },
      {
        pricing_id: "price-west",
        model_name: "chat",
        model: "openai/gpt-4o",
        api_base: "https://west.example",
      },
      legacyCatalogRow,
    ];
    const initial: WeightRow[] = [
      {
        deployment_id: "dep-east",
        model_name: "chat",
        model: "old/catalog-model",
        api_base: "https://old.example",
        weight: "6",
      },
      {
        pricing_id: "price-west",
        model_name: "chat",
        model: "old/west-model",
        api_base: "https://old-west.example",
        weight: "4",
      },
      {
        model_name: "chat",
        model: "openai/gpt-4o",
        api_base: "https://legacy.example",
        weight: "3",
      },
    ];
    renderWithProviders(<Harness modelName="chat" catalog={catalog} initial={initial} />);
    const inputs = screen.getAllByRole("spinbutton", { name: "chat openai/gpt-4o Weight" });
    expect(inputs[0]).toHaveValue(6);
    expect(inputs[1]).toHaveValue(4);
    expect(inputs[2]).toHaveValue(3);
    expect(screen.getByText("Not in current catalog")).toBeInTheDocument();
  });

  it("resolves a stable-ID-only root override through the catalog", () => {
    const catalog: SplitDeployment[] = [
      {
        deployment_id: "dep-only",
        model_name: "chat",
        model: "openai/gpt-4o",
        api_base: "https://only.example",
      },
    ];
    const initial: WeightRow[] = [{ deployment_id: "dep-only", model_name: "", model: "", api_base: "", weight: "7" }];
    renderWithProviders(<Harness catalog={catalog} initial={initial} />);
    expect(screen.getByText("Public model · chat")).toBeInTheDocument();
    expect(screen.getByRole("spinbutton", { name: "chat openai/gpt-4o Weight" })).toHaveValue(7);
    expect(screen.queryByText("Not in current catalog")).not.toBeInTheDocument();
  });

  it("handles zero effective weights without rendering bars", () => {
    renderWithProviders(
      <Harness modelName="chat" catalog={deployments.slice(0, 2).map((row) => ({ ...row, weight: "0" }))} />,
    );
    expect(screen.getByText("All effective weights are 0. This model has no enabled deployment.")).toBeInTheDocument();
    expect(screen.getAllByText("Approx. 0%")).toHaveLength(2);
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
    expect(screen.getByText("Not in current catalog")).toBeInTheDocument();
    expect(screen.getByText("No estimate")).toBeInTheDocument();
    expect(screen.getByText("Approx. 75%")).toBeInTheDocument();
    fireEvent.change(screen.getByRole("spinbutton", { name: "chat custom/retired Weight" }), {
      target: { value: "9" },
    });
    expect(onChange).toHaveBeenLastCalledWith([{ ...unknown, weight: "9" }]);
  });

  it("explains that a single deployment does not require a split", () => {
    renderWithProviders(<Harness modelName="embed" />);
    expect(
      screen.getByText("Only one deployment is available. No traffic split is needed; a weight of 0 disables it."),
    ).toBeInTheDocument();
    expect(screen.queryByText("Approx. 100%")).not.toBeInTheDocument();
  });

  it("shows a disabled estimate when the only deployment has zero weight", () => {
    renderWithProviders(<Harness modelName="embed" catalog={[{ ...deployments[2], weight: "0" }]} />);
    expect(screen.getByText("Approx. 0%")).toBeInTheDocument();
  });

  it("suppresses 100 percent when only one of several candidates is enabled", () => {
    renderWithProviders(
      <Harness
        modelName="chat"
        catalog={deployments.slice(0, 2).map((row, index) => ({ ...row, weight: index === 0 ? "3" : "0" }))}
      />,
    );
    expect(screen.queryByText("Approx. 100%")).not.toBeInTheDocument();
    expect(screen.getByText("Approx. 0%")).toBeInTheDocument();
  });

  it("inherits a root override across public aliases with the same deployment key", async () => {
    const user = userEvent.setup();
    const shared = { deployment_id: "shared", model: "openai/gpt-4o", api_base: "https://shared.example" };
    const saved: WeightRow = { ...shared, model_name: "alpha", weight: "7" };
    const catalog: SplitDeployment[] = [
      { ...shared, model_name: "alpha" },
      { ...shared, model_name: "beta", weight: "2" },
    ];
    renderWithProviders(<Harness initial={[saved]} catalog={catalog} />);
    await chooseSelectOption(user, screen.getByRole("combobox", { name: "Public model" }), "beta");
    expect(screen.getByRole("spinbutton", { name: "beta openai/gpt-4o Weight" })).toHaveValue(7);
  });

  it("keeps per-model rule weights isolated from aliases", () => {
    const shared = { deployment_id: "shared", model: "openai/gpt-4o", api_base: "https://shared.example" };
    renderWithProviders(
      <Harness
        modelName="beta"
        initial={[{ ...shared, model_name: "alpha", weight: "7" }]}
        catalog={[{ ...shared, model_name: "beta", weight: "2" }]}
      />,
    );
    expect(screen.queryByRole("spinbutton", { name: "beta openai/gpt-4o Weight" })).not.toBeInTheDocument();
    expect(screen.getByText("Weight 2")).toBeInTheDocument();
  });

  it("keeps and reports an incomplete raw weight draft", () => {
    const saved: WeightRow = {
      model_name: "chat",
      model: "openai/gpt-4o",
      api_base: "https://east.example",
      weight: "3",
    };
    const onChange = vi.fn();
    renderWithProviders(<Harness modelName="chat" initial={[saved]} onChange={onChange} />);
    const input = screen.getByRole("spinbutton", { name: "chat openai/gpt-4o Weight" });
    fireEvent.change(input, { target: { value: "" } });
    expect(input).toHaveValue(null);
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("alert")).toHaveTextContent("Check the value or JSON format of “Weight”.");
    expect(onChange).toHaveBeenLastCalledWith([{ ...saved, weight: "" }]);
    fireEvent.blur(input);
    expect(input).toHaveValue(null);
  });
});
