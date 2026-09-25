import { screen, waitFor } from "@testing-library/react";
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
    getUISettings: vi.fn().mockResolvedValue({ values: { enable_projects_ui: false } }),
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
  "Internal Users",
  "Organizations",
  "Settings",
  "Cost Optimization",
  "Projects",
  "Access Groups",
  "Guardrails Monitor",
];

const removed = ["Agents", "MCP Servers", "Skills", "Policies", "Tools", "Developer Tools", "智能体", "Budgets"];

describe("ai-gateway admin shell", () => {
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
    expect(sidebar?.className).toMatch(/bg-\[#343a40\]/);
    expect(sidebar?.className).toMatch(/text-\[#c2c7d0\]/);
    expect(shell).toContainElement(header);
    expect(shell).toContainElement(sidebar as HTMLElement);
    expect(content).toHaveTextContent("dashboard body");
    expect(content.className).toMatch(/\bp-6\b/);
    expect(sidebar?.querySelector("a[aria-label='XHub home']")).not.toBeNull();

    await waitFor(() => {
      expect(sidebar?.querySelector('a[href*="projects"]')).not.toBeNull();
      expect(sidebar).toHaveTextContent("Projects");
    });
    for (const label of kept) {
      expect(sidebar).toHaveTextContent(label);
    }
    for (const label of removed) {
      expect(sidebar).not.toHaveTextContent(label);
    }
  });
});
