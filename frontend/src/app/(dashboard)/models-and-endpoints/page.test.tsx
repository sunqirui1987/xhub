/* @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import ModelsAndEndpointsPage from "./page";

vi.mock("./panels/AllModelsPanel", () => ({ default: () => <div data-testid="panel-all-models" /> }));
vi.mock("./panels/AddModelPanel", () => ({ default: () => <div data-testid="panel-add" /> }));
vi.mock("./panels/AutoRoutersTabPanel", () => ({ default: () => <div data-testid="panel-auto-routers" /> }));
vi.mock("./panels/LlmCredentialsPanel", () => ({ default: () => <div data-testid="panel-credentials" /> }));
vi.mock("./panels/PassThroughPanel", () => ({ default: () => <div data-testid="panel-pass-through" /> }));
vi.mock("./panels/HealthStatusPanel", () => ({ default: () => <div data-testid="panel-health" /> }));
vi.mock("./panels/ModelRetrySettingsPanel", () => ({ default: () => <div data-testid="panel-retry" /> }));
vi.mock("./panels/ModelGroupAliasPanel", () => ({ default: () => <div data-testid="panel-alias" /> }));
vi.mock("./panels/PriceDataPanel", () => ({ default: () => <div data-testid="panel-price" /> }));
vi.mock("./panels/AccessGroupBudgetsPanel", () => ({ default: () => <div data-testid="panel-budgets" /> }));

const detailState = { modelId: null as string | null, teamId: null as string | null };
vi.mock("./detailNavigation", () => ({
  useModelDetailRouting: () => ({ ...detailState, close: vi.fn(), openModel: vi.fn(), openTeam: vi.fn() }),
}));

vi.mock("@/components/molecules/cost_optimization_feedback_banner", () => ({ default: () => null }));
vi.mock("@/components/model_info_view", () => ({
  default: ({ modelId }: { modelId: string }) => <div data-testid="model-info">model:{modelId}</div>,
}));
vi.mock("@/components/team/TeamInfo", () => ({
  default: ({ teamId, is_team_admin }: { teamId: string; is_team_admin: boolean }) => (
    <div data-testid="team-info" data-team-admin={String(is_team_admin)}>
      team:{teamId}
    </div>
  ),
}));

const mockUseAuthorized = vi.fn();
vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: () => mockUseAuthorized() }));
vi.mock("@/app/(dashboard)/hooks/teams/useTeams", () => ({ useTeams: () => ({ data: [] }) }));
vi.mock("@/app/(dashboard)/hooks/uiSettings/useUISettings", () => ({
  useUISettings: () => ({ data: { values: {} } }),
}));
vi.mock("./useModelDashboardData", () => ({
  useModelDashboardData: () => ({ availableModelAccessGroups: [], allModelsOnProxy: [], availableModelGroups: [] }),
}));

const ADMIN = { accessToken: "at", token: "t", userRole: "Admin", userId: "u1", premiumUser: false, isViewOnly: false };
const NON_ADMIN = {
  accessToken: "at",
  token: "t",
  userRole: "Internal User",
  userId: "u1",
  premiumUser: false,
  isViewOnly: false,
};
// A proxy_admin_viewer session: effectiveSessionRole masquerades the role as "Admin".
const VIEW_ONLY_ADMIN = { ...ADMIN, isViewOnly: true };

const renderPage = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <ModelsAndEndpointsPage />
    </QueryClientProvider>,
  );
};

describe("ModelsAndEndpointsPage", () => {
  beforeEach(() => {
    detailState.modelId = null;
    detailState.teamId = null;
    mockUseAuthorized.mockReturnValue(ADMIN);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (global as any).ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
  });

  it("renders the model list and Providers as the only primary tabs", () => {
    renderPage();
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["All Models", "Providers"]);
    expect(screen.queryByRole("tab", { name: "LLM Credentials" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Add Model" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Health Status" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add Model" })).toBeInTheDocument();
    expect(screen.getByTestId("panel-all-models")).toBeInTheDocument();
  });

  it("titles the credential section 供应商 in zh-CN", () => {
    setActiveLocale("zh-CN");
    renderPage();
    expect(screen.getByRole("tab", { name: "供应商" })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "LLM 凭据" })).not.toBeInTheDocument();
    expect(screen.queryByText("LLM 凭据")).not.toBeInTheDocument();
    expect(screen.queryByText("LLM Credentials")).not.toBeInTheDocument();
  });

  it("opens a secondary tool without making it a sibling tab", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(screen.getByRole("button", { name: "More tools" }));
    await user.click(await screen.findByRole("menuitem", { name: "Health Status" }));
    expect(screen.getByTestId("panel-health")).toBeInTheDocument();
    expect(screen.queryByTestId("panel-all-models")).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Health Status" })).not.toBeInTheDocument();
  });

  it("renders the model detail overlay from the ?model drill-in and hides the tabs", () => {
    detailState.modelId = "abc-123";
    renderPage();
    expect(screen.getByTestId("model-info")).toHaveTextContent("model:abc-123");
    expect(screen.queryByRole("tab", { name: "All Models" })).not.toBeInTheDocument();
  });

  it("renders the team detail overlay from the ?team drill-in with admin edit rights", () => {
    detailState.teamId = "team-9";
    renderPage();
    expect(screen.getByTestId("team-info")).toHaveTextContent("team:team-9");
    expect(screen.getByTestId("team-info")).toHaveAttribute("data-team-admin", "true");
  });

  it("opens the ?team drill-in without edit rights for a view-only admin", () => {
    mockUseAuthorized.mockReturnValue(VIEW_ONLY_ADMIN);
    detailState.teamId = "team-9";
    renderPage();
    expect(screen.getByTestId("team-info")).toHaveTextContent("team:team-9");
    expect(screen.getByTestId("team-info")).toHaveAttribute("data-team-admin", "false");
  });

  it("hides admin-only tools from a non-admin user", async () => {
    mockUseAuthorized.mockReturnValue(NON_ADMIN);
    renderPage();
    expect(screen.queryByRole("tab", { name: "Providers" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "LLM Credentials" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Health Status" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "More tools" })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Your Models" })).toBeInTheDocument();
  });

  it("does not list the old ten labels as sibling tabs", () => {
    renderPage();
    const tabs = screen.getAllByRole("tab").map((tab) => tab.textContent);
    expect(tabs).toEqual(["All Models", "Providers"]);
    for (const label of [
      "Add Model",
      "Auto-Routers",
      "LLM Credentials",
      "Pass-Through Endpoints",
      "Health Status",
      "Model Retry Settings",
      "Model Group Alias",
      "Model Access Group Budgets",
      "Price Data Reload",
    ]) {
      expect(screen.queryByRole("tab", { name: label })).not.toBeInTheDocument();
    }
  });

  it("hides write tools from a view-only admin and keeps the read tools", async () => {
    const user = userEvent.setup();
    mockUseAuthorized.mockReturnValue(VIEW_ONLY_ADMIN);
    renderPage();
    expect(screen.getByRole("tab", { name: "All Models" })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Providers" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add Model" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "More tools" }));
    expect(await screen.findByRole("menuitem", { name: "Health Status" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /Auto-Routers/ })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Pass-Through Endpoints" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Model Retry Settings" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Model Group Alias" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: /Model Access Group Budgets/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Price Data Reload" })).not.toBeInTheDocument();
  });

  // POST /model/new 403s a proxy_admin_viewer, so the add action must not render for one.
  it("hides Add Model for a view-only admin session", () => {
    mockUseAuthorized.mockReturnValue(VIEW_ONLY_ADMIN);
    renderPage();
    expect(screen.queryByRole("button", { name: "Add Model" })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "All Models" })).toBeInTheDocument();
  });

  // Read parity: the Auto-Routers list stays reachable for a view-only admin; only the
  // create affordance inside it is withheld, which AutoRoutersTabPanel decides.
  it("keeps Auto-Routers reachable for a view-only admin session", async () => {
    const user = userEvent.setup();
    mockUseAuthorized.mockReturnValue(VIEW_ONLY_ADMIN);
    renderPage();
    await user.click(screen.getByRole("button", { name: "More tools" }));
    expect(await screen.findByRole("menuitem", { name: /Auto-Routers/ })).toBeInTheDocument();
  });

  // Auto-routers are excluded from the All Models table, so this tool is their home.
  describe("Auto-Routers tool", () => {
    it("is a secondary tool, not a primary tab, and keeps the Beta badge", async () => {
      const user = userEvent.setup();
      renderPage();
      expect(screen.queryByRole("tab", { name: /Auto-Routers/ })).not.toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: "More tools" }));
      const item = await screen.findByRole("menuitem", { name: /Auto-Routers/ });
      expect(item).toHaveTextContent("Beta");
    });

    it("renders its panel when selected", async () => {
      const user = userEvent.setup();
      renderPage();
      await user.click(screen.getByRole("button", { name: "More tools" }));
      await user.click(await screen.findByRole("menuitem", { name: /Auto-Routers/ }));
      expect(screen.getByTestId("panel-auto-routers")).toBeInTheDocument();
    });

    it("is hidden from non-admins, who cannot write models", () => {
      mockUseAuthorized.mockReturnValue(NON_ADMIN);
      renderPage();
      expect(screen.queryByRole("tab", { name: /Auto-Routers/ })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "More tools" })).not.toBeInTheDocument();
    });
  });
});
