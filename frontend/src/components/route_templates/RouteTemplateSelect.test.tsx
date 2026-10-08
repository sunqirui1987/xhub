import { act, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getRouteTemplateBindingCall, getRouteTemplatesCall } from "@/components/networking";
import RouteTemplateSelect from "./RouteTemplateSelect";

vi.mock("@/components/networking", () => ({
  getRouteTemplatesCall: vi.fn(),
  getRouteTemplateBindingCall: vi.fn(),
}));

describe("RouteTemplateSelect", () => {
  it("never replaces a draft with a late saved binding", async () => {
    vi.mocked(getRouteTemplatesCall).mockResolvedValue([{ id: "a", name: "Saved", body: {} }, { id: "b", name: "Draft", body: {} }]);
    let resolve!: (value: { route_template_id: string; effective: { template_id: string; name: string; scope_type: "team" } }) => void;
    const pending = new Promise<{ route_template_id: string; effective: { template_id: string; name: string; scope_type: "team" } }>((done) => { resolve = done; });
    vi.mocked(getRouteTemplateBindingCall).mockReturnValue(pending);
    const onChange = vi.fn();
    const props = { accessToken: "token", scope: "team" as const, scopeId: "team", onChange };
    const view = render(<RouteTemplateSelect {...props} value="a" />);
    view.rerender(<RouteTemplateSelect {...props} value="b" />);
    await act(async () => { resolve({ route_template_id: "a", effective: { template_id: "a", name: "Saved", scope_type: "team" } }); });
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Route Templates")).toHaveTextContent("Draft");
    expect(screen.getByText(/Draft.*Team/)).toBeInTheDocument();
  });

  it("previews inheritance when a saved selection is cleared", async () => {
    vi.mocked(getRouteTemplatesCall).mockResolvedValue([{ id: "a", name: "Saved", body: {} }]);
    vi.mocked(getRouteTemplateBindingCall).mockResolvedValue({ route_template_id: "a", effective: { template_id: "a", name: "Saved", scope_type: "team" }, inherited: { template_id: "parent", name: "Parent", scope_type: "organization" } });
    const props = { accessToken: "token", scope: "team" as const, scopeId: "team", onChange: vi.fn() };
    const view = render(<RouteTemplateSelect {...props} value="a" />);
    await waitFor(() => expect(screen.getByLabelText("Route Templates")).toHaveTextContent("Saved"));
    view.rerender(<RouteTemplateSelect {...props} value="" />);
    await waitFor(() => expect(screen.getByText(/Parent.*Organization/)).toBeInTheDocument());
    expect(props.onChange).not.toHaveBeenCalled();
    view.rerender(<RouteTemplateSelect accessToken="token" value="" onChange={props.onChange} />);
    expect(screen.queryByText(/Parent.*Organization/)).not.toBeInTheDocument();
  });
  beforeEach(() => {
    vi.mocked(getRouteTemplatesCall).mockReset();
    vi.mocked(getRouteTemplateBindingCall).mockReset();
  });

  it("resolves a stored template ID to its readable name", async () => {
    vi.mocked(getRouteTemplatesCall).mockResolvedValue([
      { id: "0c5e5f2b17ce8cd650cff168751d1f8d", name: "低成本路由", body: {} },
    ]);

    render(
      <RouteTemplateSelect accessToken="test-token" value="0c5e5f2b17ce8cd650cff168751d1f8d" onChange={vi.fn()} />,
    );

    const trigger = screen.getByLabelText("Route Templates");
    await waitFor(() => expect(trigger).toHaveTextContent("低成本路由"));
    expect(trigger).not.toHaveTextContent("0c5e5f2b17ce8cd650cff168751d1f8d");
    expect(getRouteTemplatesCall).toHaveBeenCalledWith("test-token");
  });

  it("does not expose an internal ID while the template lookup is pending", () => {
    vi.mocked(getRouteTemplatesCall).mockReturnValue(new Promise(() => undefined));

    render(<RouteTemplateSelect accessToken="test-token" value="internal-template-id" onChange={vi.fn()} />);

    const trigger = screen.getByLabelText("Route Templates");
    expect(trigger).toHaveTextContent("Loading route template name…");
    expect(trigger).not.toHaveTextContent("internal-template-id");
  });
});
