import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { DashboardShell } from "./layout";
import { renderWithProviders } from "../../../tests/test-utils";

vi.mock("next/navigation", () => ({
  usePathname: () => "/ui/api-keys",
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ accessToken: "test-access-token", authLoading: false }),
}));

/** 管理壳使用服务端能力而非 token 角色；固定平台管理员身份，无外部请求，夹具随测试卸载。 */
vi.mock("@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity")>(),
  useSessionIdentity: () => ({
    data: { user_id: "test-user-id", kind: "session", capabilities: ["platform.admin", "teams.read", "teams.manage", "teams.platform", "audit.read"], teams: [] },
    isLoading: false,
  }),
}));

vi.mock("@/contexts/PluginModeContext", () => ({
  usePluginMode: () => ({ mode: "ai-gateway", activePlugin: null, plugins: [], setMode: vi.fn() }),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: () => ({
    userId: "test-user-id",
    accessToken: "test-access-token",
    userRole: "Admin",
    isViewOnly: false,
    token: "test-token",
    userEmail: "test@example.com",
    premiumUser: false,
    disabledPersonalKeyCreation: false,
    showSSOBanner: false,
  }),
}));

vi.mock("@/app/(dashboard)/hooks/teams/useTeams", () => ({
  useTeams: () => ({ data: [], isLoading: false, error: null }),
}));

vi.mock("@/app/(dashboard)/hooks/organizations/useOrganizations", () => ({
  useOrganizations: () => ({ data: [], isLoading: false, error: null }),
}));

vi.mock("@/app/(dashboard)/hooks/useIsOrgAdmin", () => ({
  default: () => false,
}));

vi.mock("@/contexts/ThemeContext", () => ({
  useTheme: () => ({
    logoUrl: null,
    logoUrlDark: null,
    faviconUrl: null,
    setLogoUrl: vi.fn(),
    setLogoUrlDark: vi.fn(),
    setFaviconUrl: vi.fn(),
  }),
}));

vi.mock("@/app/(dashboard)/hooks/healthReadiness/useHealthReadinessDetails", () => ({
  useHealthReadinessDetails: () => ({ data: undefined }),
}));

vi.mock("@/app/(dashboard)/hooks/useLogout", () => ({
  useLogout: () => vi.fn(),
}));

vi.mock("@/hooks/useWorker", () => ({
  useWorker: () => ({ isControlPlane: false, selectedWorker: null }),
}));

vi.mock("@/components/networking", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/networking")>();
  return {
    ...actual,
    getUISettings: vi.fn().mockResolvedValue({ values: { enable_projects_ui: true } }),
    getProxyBaseUrl: vi.fn().mockReturnValue("http://localhost:4000"),
  };
});

const kept = [
  "Virtual Keys",
  "Playground",
  "Models + Endpoints",
  "Guardrails",
  "Usage",
  "Logs",
  "Teams",
  "Users",
  "Organizations",
  "Admin Settings",
  "Projects",
  "Routing & Load Balancing",
  "Guardrails Monitor",
];

const removed = ["Agents", "MCP Servers", "Skills", "Policies", "Tools", "Developer Tools", "智能体", "Budgets", "Cost Optimization", "成本优化"];

describe("ai-gateway admin shell", () => {
  // 目的：项目入口开启时验证管理壳完整导航；前置为管理员和 enable_projects_ui，结束由渲染/查询夹具清理。
  it("renders a header, a vertical sidebar, and a padded content region", async () => {
    renderWithProviders(
      <DashboardShell>
        <div>dashboard body</div>
      </DashboardShell>,
    );

    const header = screen.getByTestId("admin-header");
    const shell = screen.getByTestId("admin-shell");
    const content = screen.getByTestId("admin-content");
    const sidebar = document.querySelector("[data-slot='sidebar']");

    expect(header).toBeInTheDocument();
    expect(screen.queryByTestId("header-brand")).not.toBeInTheDocument();
    expect(sidebar).not.toBeNull();
    expect(sidebar?.tagName).toBe("ASIDE");
    expect(sidebar?.className).toMatch(/\bbg-sidebar\b/);
    expect(sidebar?.className).toMatch(/\btext-sidebar-foreground\b/);
    expect(sidebar?.className).not.toMatch(/bg-\[#343a40\]/);
    expect(shell).toContainElement(header);
    expect(shell).toContainElement(sidebar as HTMLElement);
    expect(content).toHaveTextContent("dashboard body");
    expect(content.className).toMatch(/\bp-6\b/);
    expect(screen.getByRole("link", { name: "XHub home" })).toBeInTheDocument();

    expect(await screen.findByRole("link", { name: "Projects" })).toBeInTheDocument();
    for (const label of kept) {
      expect(sidebar).toHaveTextContent(label);
    }
    for (const label of removed) {
      expect(sidebar).not.toHaveTextContent(label);
    }
  });
});
