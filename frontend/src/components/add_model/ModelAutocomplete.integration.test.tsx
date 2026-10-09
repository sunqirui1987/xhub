import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import ModelAutocomplete from "./ModelAutocomplete";

function ModelList() {
  const [value, setValue] = useState("");
  const options = Array.from({ length: 153 }, (_, i) => ({ value: `model-${i}`, label: `model-${i}` }));
  options.push({ value: "fal-ai/kling-video/v3/pro/text-to-video", label: "Kling" }, { value: "fal-ai/vidu/q3/text-to-video/pro", label: "Vidu" });
  return <><label htmlFor="models">上游模型</label><ModelAutocomplete id="models" value={value} options={options} onChange={setValue} onSelect={setValue} /></>;
}

describe("complete model suggestions", () => {
  it("shows all 155 models and selects Kling and Vidu from the end of the catalog", async () => {
    const user = userEvent.setup();
    render(<ModelList />);
    const input = screen.getByRole("combobox", { name: "上游模型" });
    await user.click(input);
    expect(screen.getAllByRole("option")).toHaveLength(155);
    await user.click(screen.getByRole("option", { name: /kling-video/ }));
    expect(input).toHaveValue("fal-ai/kling-video/v3/pro/text-to-video");
    await user.clear(input);
    await user.type(input, "vidu");
    expect(screen.getAllByRole("option")).toHaveLength(1);
    await user.keyboard("{ArrowDown}{Enter}");
    expect(input).toHaveValue("fal-ai/vidu/q3/text-to-video/pro");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });
});
