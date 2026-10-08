import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getRouteTemplatesCall } from "@/components/networking";
import RouteTemplateSelect from "./RouteTemplateSelect";

vi.mock("@/components/networking", () => ({
  getRouteTemplatesCall: vi.fn(),
  getRouteTemplateBindingCall: vi.fn(),
}));

describe("RouteTemplateSelect", () => {
  beforeEach(() => {
    vi.mocked(getRouteTemplatesCall).mockReset();
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
