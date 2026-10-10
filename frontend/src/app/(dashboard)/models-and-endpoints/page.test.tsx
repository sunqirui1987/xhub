/* @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import ModelsAndEndpointsPage from "./page";

vi.mock("./panels/AllModelsPanel", () => ({ default: () => <div data-testid="panel-all-models" /> }));
vi.mock("./panels/AddModelPanel", () => ({ default: () => <div data-testid="panel-add" /> }));
vi.mock("./panels/LlmCredentialsPanel", () => ({ default: () => <div data-testid="panel-credentials" /> }));
vi.mock("next/navigation", () => ({ useSearchParams: () => new URLSearchParams() }));

const detailState = { modelId: null as string | null };
vi.mock("./detailNavigation", () => ({
  useModelDetailRouting: () => ({ ...detailState, close: vi.fn(), openModel: vi.fn() }),
}));

vi.mock("@/components/model_info_view", () => ({
  default: ({ modelId }: { modelId: string }) => <div data-testid="model-info">model:{modelId}</div>,
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
    setActiveLocale("en");
    detailState.modelId = null;
    mockUseAuthorized.mockReturnValue(ADMIN);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (global as any).ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
  });

  it("renders the model list as the only primary tab", () => {
    renderPage();
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["All Models"]);
    expect(screen.queryByRole("tab", { name: "LLM Credentials" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Add Model" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Health Status" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add Model" })).toBeInTheDocument();
    expect(screen.getByTestId("panel-all-models")).toBeInTheDocument();
  });

  /** 中文管理入口只呈现模型目录；供应商不再是同级页签。渲染由框架清理。 */
  it("keeps model providers out of the primary tabs in zh-CN", () => {
    setActiveLocale("zh-CN");
    renderPage();
    expect(screen.queryByRole("tab", { name: "模型提供商" })).not.toBeInTheDocument();
    expect(screen.getAllByRole("tab")).toHaveLength(1);
    expect(screen.queryByRole("tab", { name: "LLM 凭据" })).not.toBeInTheDocument();
    expect(screen.queryByText("LLM 凭据")).not.toBeInTheDocument();
    expect(screen.queryByText("LLM Credentials")).not.toBeInTheDocument();
  });

  it("renders the model detail overlay from the ?model drill-in and hides the tabs", () => {
    detailState.modelId = "abc-123";
    renderPage();
    expect(screen.getByTestId("model-info")).toHaveTextContent("model:abc-123");
    expect(screen.queryByRole("tab", { name: "All Models" })).not.toBeInTheDocument();
  });

  it("hides admin-only tools from a non-admin user", () => {
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
    expect(tabs).toEqual(["All Models"]);
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

  it("hides write tools from a view-only admin", () => {
    mockUseAuthorized.mockReturnValue(VIEW_ONLY_ADMIN);
    renderPage();
    expect(screen.getByRole("tab", { name: "All Models" })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Providers" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add Model" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "More tools" })).not.toBeInTheDocument();
  });

  // POST /model/new 403s a proxy_admin_viewer, so the add action must not render for one.
  it("hides Add Model for a view-only admin session", () => {
    mockUseAuthorized.mockReturnValue(VIEW_ONLY_ADMIN);
    renderPage();
    expect(screen.queryByRole("button", { name: "Add Model" })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "All Models" })).toBeInTheDocument();
  });
});
