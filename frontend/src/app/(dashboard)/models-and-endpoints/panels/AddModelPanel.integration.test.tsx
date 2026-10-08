import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AddModelPanel from "./AddModelPanel";

const modelEditor = vi.fn();

vi.mock("@/components/add_model/ModelEditor", () => ({
  default: (props: { initialCatalogId?: string; onSaved?: () => void; onCancel?: () => void }) => {
    modelEditor(props);
    return (
      <section aria-label="shared model editor">
        <label htmlFor="supplier-first">模型提供商 *</label>
        <select id="supplier-first" defaultValue="">
          <option value="">请选择模型提供商</option>
          <option value="provider-a">provider-a</option>
        </select>
        <button type="button" onClick={props.onSaved}>
          保存
        </button>
        <button type="button" onClick={props.onCancel}>
          取消
        </button>
      </section>
    );
  },
}));

describe("AddModelPanel current flow wiring", () => {
  beforeEach(() => vi.clearAllMocks());

  it("delegates the provider-first add flow to the shared ModelEditor", () => {
    render(<AddModelPanel initialCatalogId="catalog/gpt" />);

    expect(screen.getByRole("region", { name: "shared model editor" })).toBeInTheDocument();
    expect(screen.getByLabelText("模型提供商 *")).toHaveValue("");
    expect(modelEditor).toHaveBeenCalledWith(expect.objectContaining({ initialCatalogId: "catalog/gpt" }));
  });

  it("forwards save and cancel callbacks through the shared editor", async () => {
    const user = userEvent.setup();
    const onSaved = vi.fn();
    const onCancel = vi.fn();
    render(<AddModelPanel onSaved={onSaved} onCancel={onCancel} />);

    await user.click(screen.getByRole("button", { name: "保存" }));
    await user.click(screen.getByRole("button", { name: "取消" }));

    expect(onSaved).toHaveBeenCalledOnce();
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it("does not restore legacy team, passthrough, or More Tools controls", () => {
    render(<AddModelPanel />);

    expect(screen.queryByText(/team|团队/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/pass.?through|透传/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/more tools/i)).not.toBeInTheDocument();
  });
});
