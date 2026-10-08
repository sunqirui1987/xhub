import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import RouteTemplatesPanel from "./RouteTemplatesPanel";
import { createRouteTemplateCall, getRouterSettingsCall, updateRouteTemplateCall } from "@/components/networking";

const platform = { routing_strategy: "least-busy", timeout: 45, num_retries: 2, custom_setting: { keep: true } };
vi.mock("@/components/networking", () => ({
  getRouteTemplatesCall: vi.fn(async () => [
    { id: "first", name: "Fast", body: { ...platform, timeout: 20 }, writable: true, used_by: 1 },
    { id: "second", name: "Shared", body: platform, writable: false, used_by: 0 },
  ]),
  getRouterSettingsCall: vi.fn(async () => ({ router_settings: platform })),
  modelInfoCall: vi.fn(async () => ({ data: [] })),
  userAvailableModelsCall: vi.fn(async () => ({ data: [] })),
  createRouteTemplateCall: vi.fn(async () => ({})),
  updateRouteTemplateCall: vi.fn(async () => ({})),
  getRouteTemplateUsageCall: vi.fn(async () => ({ data: [] })),
  deleteRouteTemplateCall: vi.fn(async () => ({})),
  setCallbacksCall: vi.fn(async () => ({})),
}));

function setup() {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <RouteTemplatesPanel accessToken="fixture" />
    </QueryClientProvider>,
  );
  return user;
}

async function openCreate(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText("Fast");
  await user.click(screen.getByRole("button", { name: "New template" }));
  return screen.getByRole("region", { name: "New template" });
}

describe("route template workflow", () => {
  beforeEach(() => vi.clearAllMocks());

  it("creates from the platform document, trims the name and preserves extra fields", async () => {
    const user = setup();
    const editor = await openCreate(user);
    expect(within(editor).getByRole("button", { name: "New template" })).toBeDisabled();
    expect(within(editor).getByLabelText("Timeout (seconds)")).toHaveValue(45);
    await user.type(within(editor).getByLabelText("Name"), "  Custom  ");
    fireEvent.change(within(editor).getByLabelText("Timeout (seconds)"), { target: { value: "30" } });
    await user.click(within(editor).getByRole("button", { name: "New template" }));
    await waitFor(() =>
      expect(createRouteTemplateCall).toHaveBeenCalledWith(
        "fixture",
        expect.objectContaining({
          name: "Custom",
          body: expect.objectContaining({ timeout: 30, num_retries: 2, custom_setting: { keep: true } }),
        }),
      ),
    );
  });

  it("keeps copying as a draft and allows cancellation without creating anything", async () => {
    const user = setup();
    await screen.findByText("Fast");
    await user.click(screen.getByRole("button", { name: "Actions for Fast" }));
    await user.click(await screen.findByRole("menuitem", { name: "Copy" }));
    const editor = screen.getByRole("region", { name: "New template" });
    expect(within(editor).getByLabelText("Name")).toHaveValue("Fast copy");
    expect(within(editor).getByLabelText("Timeout (seconds)")).toHaveValue(20);
    await user.click(within(editor).getByRole("button", { name: "Cancel" }));
    expect(createRouteTemplateCall).not.toHaveBeenCalled();
    expect(updateRouteTemplateCall).not.toHaveBeenCalled();
  });

  it("blocks invalid JSON and resumes saving only after fixing it", async () => {
    const user = setup();
    const editor = await openCreate(user);
    await user.type(within(editor).getByLabelText("Name"), "JSON template");
    await user.click(within(editor).getByRole("tab", { name: "JSON" }));
    fireEvent.change(within(editor).getByRole("textbox", { name: "JSON" }), {
      target: { value: "{invalid" },
    });
    expect(within(editor).getByRole("alert")).toHaveTextContent("This is not a JSON object");
    expect(within(editor).getByRole("tab", { name: "Basic settings" })).toHaveAttribute("aria-disabled", "true");
    expect(within(editor).getByRole("button", { name: "New template" })).toBeDisabled();
    fireEvent.change(within(editor).getByRole("textbox", { name: "JSON" }), {
      target: { value: JSON.stringify({ timeout: 90, custom_setting: "retained" }) },
    });
    await user.click(within(editor).getByRole("tab", { name: "Basic settings" }));
    expect(within(editor).getByLabelText("Timeout (seconds)")).toHaveValue(90);
    await user.click(within(editor).getByRole("button", { name: "New template" }));
    await waitFor(() =>
      expect(createRouteTemplateCall).toHaveBeenCalledWith(
        "fixture",
        expect.objectContaining({ body: expect.objectContaining({ timeout: 90, custom_setting: "retained" }) }),
      ),
    );
  });

  it("uses the page for editing and opens full instructions in a dialog", async () => {
    const user = setup();
    const editor = await openCreate(user);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(editor).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Open the complete configuration guide" }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Close" }));
    expect(within(editor).getByLabelText("Name")).toHaveValue("");
  });

  it("renames and edits an existing template together", async () => {
    const user = setup();
    await screen.findByText("Fast");
    await user.click(within(screen.getByRole("row", { name: /Fast/ })).getByRole("button", { name: "Edit" }));
    const editor = screen.getByRole("region", { name: "Edit" });
    await user.clear(within(editor).getByLabelText("Name"));
    await user.type(within(editor).getByLabelText("Name"), "Renamed");
    await user.click(within(editor).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(updateRouteTemplateCall).toHaveBeenCalledWith(
        "fixture",
        "first",
        expect.objectContaining({ name: "Renamed", body: expect.objectContaining({ timeout: 20 }) }),
      ),
    );
  });

  it("filters named templates and marks shared templates as read-only", async () => {
    const user = setup();
    await screen.findByText("Shared");
    const row = screen.getByRole("row", { name: /Shared/ });
    expect(within(row).getByRole("button", { name: "Edit" })).toBeDisabled();
    await user.type(screen.getByRole("textbox", { name: "Search template names" }), " FAST ");
    expect(screen.getByText("Fast")).toBeInTheDocument();
    expect(screen.queryByText("Shared")).not.toBeInTheDocument();
  });

  it("offers a retry and blocks creation if the platform configuration fails to load", async () => {
    vi.mocked(getRouterSettingsCall).mockRejectedValueOnce(new Error("unavailable"));
    const user = setup();
    await screen.findByText("Failed to load the templates");
    expect(screen.getByRole("button", { name: "New template" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Reload" }));
    await screen.findByText("Fast");
    expect(screen.getByRole("button", { name: "New template" })).toBeEnabled();
  });
});
