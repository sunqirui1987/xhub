import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "../../tests/test-utils";
import Sidebar, { menuGroups, getBreadcrumb } from "./leftnav";
import { t } from "@/i18n";
import { setActiveLocale } from "@/i18n/runtime";

vi.mock("../utils/roles", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../utils/roles")>();
  return {
    ...actual,
    all_admin_roles: ["admin", "admin_viewer"],
    old_admin_roles: ["admin", "admin_viewer"],
    internalUserRoles: ["internal"],
    rolesWithWriteAccess: ["admin", "internal"],
    rolesAllowedToViewWriteScopedPages: ["admin", "internal", "admin_viewer"],
    isAdminRole: (role: string) => role === "admin" || role === "admin_viewer",
    isUserTeamAdminForAnyTeam: () => false,
  };
});

const navState = vi.hoisted(() => ({ pathname: "/ui/api-keys" }));

vi.mock("next/navigation", () => ({
  usePathname: () => navState.pathname,
}));

const { mockUseAuthorized, mockUseOrganizations } = vi.hoisted(() => {
  const mockUseAuthorized = vi.fn(() => ({
    userId: "test-user-id",
    accessToken: "test-access-token",
    userRole: "admin",
    isViewOnly: false,
    token: "test-token",
    userEmail: "test@example.com",
    premiumUser: false,
    disabledPersonalKeyCreation: false,
    showSSOBanner: false,
  }));

  const mockUseOrganizations = vi.fn(() => ({
    data: [],
    isLoading: false,
    error: null,
  }));

  return { mockUseAuthorized, mockUseOrganizations };
});

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: mockUseAuthorized,
}));

vi.mock("@/app/(dashboard)/hooks/organizations/useOrganizations", () => ({
  useOrganizations: mockUseOrganizations,
}));

vi.mock("@/app/(dashboard)/hooks/teams/useTeams", () => ({
  useTeams: () => ({ data: [], isLoading: false, error: null }),
}));

vi.mock("@/app/(dashboard)/hooks/uiConfig/useUIConfig", () => {
  return {
    useUIConfig: () => ({
      data: { admin_ui_disabled: false },
      isLoading: false,
    }),
  };
});

// The redesigned sidebar reads the custom logo from ThemeContext; the test tree
// has no ThemeProvider, so stub the hook.
const unbrandedTheme = () => ({
  logoUrl: null as string | null,
  logoUrlDark: null as string | null,
  faviconUrl: null as string | null,
  setLogoUrl: vi.fn(),
  setLogoUrlDark: vi.fn(),
  setFaviconUrl: vi.fn(),
});
let mockUseThemeImpl = unbrandedTheme;
vi.mock("@/contexts/ThemeContext", () => ({
  useTheme: () => mockUseThemeImpl(),
}));

// Version tag + logout target come from network hooks; keep them inert in unit tests.
vi.mock("@/app/(dashboard)/hooks/healthReadiness/useHealthReadinessDetails", () => ({
  useHealthReadinessDetails: () => ({ data: undefined }),
}));
vi.mock("@/app/(dashboard)/hooks/useLogout", () => ({
  useLogout: () => vi.fn(),
}));

const collectNavKeys = (): string[] =>
  menuGroups.flatMap((group) => group.items.flatMap((item) => [item.key, ...(item.children ?? []).map((c) => c.key)]));

// Every place a page id appears in the nav, as "GROUP" for a top-level item or
// "GROUP > parentKey" for a child.
const placementsOf = (page: string): string[] =>
  menuGroups.flatMap((group) => [
    ...group.items.filter((item) => item.page === page).map(() => group.groupLabel),
    ...group.items.flatMap((item) =>
      (item.children ?? []).filter((child) => child.page === page).map(() => `${group.groupLabel} > ${item.key}`),
    ),
  ]);

describe("Sidebar (leftnav)", () => {
  const defaultProps = {
    collapsed: false,
  };

  afterEach(() => {
    mockUseAuthorized.mockReset();
    mockUseOrganizations.mockReset();
    mockUseThemeImpl = unbrandedTheme;
    navState.pathname = "/ui/api-keys";
  });

  it("should link the logo to the UI home route rather than the proxy origin", () => {
    renderWithProviders(<Sidebar {...defaultProps} />);

    expect(screen.getByRole("link", { name: /xhub/i })).toHaveAttribute("href", "/ui");
  });

  it("shows the product name instead of a default logo", () => {
    renderWithProviders(<Sidebar {...defaultProps} />);

    expect(screen.getByRole("link", { name: /xhub/i })).toHaveTextContent("XHub");
    expect(screen.getByRole("link", { name: /xhub/i }).querySelector("img")).toBeNull();
  });

  it("prefers a configured dark logo over the light one in dark mode", () => {
    mockUseThemeImpl = () => ({
      ...unbrandedTheme(),
      logoUrl: "https://cdn.example.com/logo.png",
      logoUrlDark: "https://cdn.example.com/logo-dark.png",
    });
    renderWithProviders(<Sidebar {...defaultProps} />);

    const [light, dark] = Array.from(screen.getByRole("link", { name: /xhub/i }).querySelectorAll("img"));

    expect(light).toHaveAttribute("src", "https://cdn.example.com/logo.png");
    expect(dark).toHaveAttribute("src", "https://cdn.example.com/logo-dark.png");
  });

  it("reuses the light custom logo in dark mode when no dark one is configured", () => {
    mockUseThemeImpl = () => ({ ...unbrandedTheme(), logoUrl: "https://cdn.example.com/logo.png" });
    renderWithProviders(<Sidebar {...defaultProps} />);

    const [light, dark] = Array.from(screen.getByRole("link", { name: /xhub/i }).querySelectorAll("img"));

    expect(light).toHaveAttribute("src", "https://cdn.example.com/logo.png");
    expect(dark).toHaveAttribute("src", "https://cdn.example.com/logo.png");
  });

  it("falls back to the light logo when a configured dark logo fails to load", () => {
    mockUseThemeImpl = () => ({
      ...unbrandedTheme(),
      logoUrl: "https://cdn.example.com/logo.png",
      logoUrlDark: "https://cdn.example.com/gone.png",
    });
    renderWithProviders(<Sidebar {...defaultProps} />);

    const [, dark] = Array.from(screen.getByRole("link", { name: /xhub/i }).querySelectorAll("img"));
    expect(dark).toHaveAttribute("src", "https://cdn.example.com/gone.png");

    fireEvent.error(dark);

    expect(dark).toHaveAttribute("src", "https://cdn.example.com/logo.png");
  });

  it("renders all top-level (non-nested) tabs for admin", () => {
    renderWithProviders(<Sidebar {...defaultProps} />);

    const topLevelLabels = [
      "Virtual Keys",
      "Playground",
      "Models + Endpoints",
      "Guardrails",
      "Usage",
      "Logs",
      "Guardrails Monitor",
      "Teams",
      "Internal Users",
      "Organizations",
      "Access Groups",
      "Settings",
    ];

    topLevelLabels.forEach((label) => {
      expect(screen.getByText(label)).toBeInTheDocument();
    });
  });

  it("expands Settings to reveal Router Settings", async () => {
    renderWithProviders(<Sidebar {...defaultProps} />);

    expect(screen.queryByText("Router Settings")).not.toBeInTheDocument();
    act(() => {
      fireEvent.click(screen.getByText("Settings"));
    });
    await waitFor(() => {
      expect(screen.getByText("Router Settings")).toBeInTheDocument();
    });
  });
  it("reports whether a nested tab is expanded", async () => {
    renderWithProviders(<Sidebar {...defaultProps} />);

    const toggle = screen.getByText("Settings").closest("button")!;
    expect(toggle).toHaveAttribute("aria-expanded", "false");

    act(() => {
      fireEvent.click(toggle);
    });
    await waitFor(() => {
      expect(toggle).toHaveAttribute("aria-expanded", "true");
    });
  });

  it("keeps Router Settings as a single Settings child", () => {
    // Router Settings is admin-only, so getAvailablePages() filters it out entirely and the
    // page_utils duplicate-key guard cannot see it. Walk menuGroups directly, otherwise a
    // stray duplicate placement ships silently.
    expect(placementsOf("router-settings")).toEqual(["nav.groups.settings > settings"]);
  });

  it("has no duplicate keys among all menu items and their children", () => {
    // React keys must be unique across the whole nav config, otherwise the
    // active-item highlight and group expansion collide.
    const keys = collectNavKeys();
    const duplicates = keys.filter((key, i) => keys.indexOf(key) !== i);
    expect(duplicates).toEqual([]);
  });

  describe("Admin Viewer parity", () => {
    // Admin Viewer follows a "read parity with Proxy Admin, no writes, no
    // cost-incurring actions" rule. The session hook presents the viewer as
    // an admin (`userRole: "admin"`) with `isViewOnly: true`; Playground
    // stays hidden (incurs LLM cost) via the isViewOnly flag, while every
    // admin page (Models + Endpoints, Agents, Logs, ...) is visible read-only.
    const adminViewerAuth = {
      userId: "admin-viewer-user-id",
      accessToken: "test-access-token",
      userRole: "admin",
      isViewOnly: true,
      token: "test-token",
      userEmail: "viewer@example.com",
      premiumUser: false,
      disabledPersonalKeyCreation: false,
      showSSOBanner: false,
    };

    it("hides Playground from Admin Viewer (cost-incurring action)", () => {
      mockUseAuthorized.mockReturnValue(adminViewerAuth);
      renderWithProviders(<Sidebar {...defaultProps} />);
      expect(screen.queryByText("Playground")).not.toBeInTheDocument();
    });

    it("shows Models + Endpoints to Admin Viewer (read-only)", () => {
      mockUseAuthorized.mockReturnValue(adminViewerAuth);
      renderWithProviders(<Sidebar {...defaultProps} />);
      expect(screen.getByText("Models + Endpoints")).toBeInTheDocument();
    });

    it("does not show Agents to Admin Viewer", () => {
      mockUseAuthorized.mockReturnValue(adminViewerAuth);
      renderWithProviders(<Sidebar {...defaultProps} />);
      expect(screen.queryByText("Agents")).not.toBeInTheDocument();
      expect(screen.queryByText("Agentic")).not.toBeInTheDocument();
    });

    it("shows Logs to Admin Viewer", () => {
      mockUseAuthorized.mockReturnValue(adminViewerAuth);
      renderWithProviders(<Sidebar {...defaultProps} />);
      expect(screen.getByText("Logs")).toBeInTheDocument();
    });
  });

  describe("capability-gated Tools children", () => {
    const internalAuth = {
      userId: "internal-user-id",
      accessToken: "test-access-token",
      userRole: "internal",
      isViewOnly: false,
      token: "test-token",
      userEmail: "internal@example.com",
      premiumUser: false,
      disabledPersonalKeyCreation: false,
      showSSOBanner: false,
    };

    afterEach(() => {
      mockUseAuthorized.mockReset();
    });

    it("does not show the removed Tools column to internal users", () => {
      mockUseAuthorized.mockReturnValue(internalAuth);
      renderWithProviders(<Sidebar {...defaultProps} />);
      expect(screen.queryByText("Tools")).not.toBeInTheDocument();
      expect(screen.queryByText("Search Tools")).not.toBeInTheDocument();
      expect(screen.getByText("Guardrails")).toBeInTheDocument();
    });

    it("does not show the removed Tools column to admins", () => {
      renderWithProviders(<Sidebar {...defaultProps} />);
      expect(screen.queryByText("Tools")).not.toBeInTheDocument();
      expect(screen.queryByText("Tool Policies")).not.toBeInTheDocument();
    });

    it("should hide the Policies entry from internal users while keeping Guardrails", () => {
      mockUseAuthorized.mockReturnValue(internalAuth);
      renderWithProviders(<Sidebar {...defaultProps} />);

      expect(screen.getByText("Guardrails")).toBeInTheDocument();
      expect(screen.queryByText("Policies")).not.toBeInTheDocument();
    });

    it("does not show Experimental or its children", () => {
      renderWithProviders(<Sidebar {...defaultProps} />);
      expect(screen.queryByText("Experimental")).not.toBeInTheDocument();
      expect(screen.queryByText("Prompts")).not.toBeInTheDocument();
      expect(screen.queryByText("Old Usage")).not.toBeInTheDocument();
    });
  });

  // Workflow Runs, Memory and Guardrails Monitor render a shell and then 401
  // for every non-proxy-admin role, because their page-load routes sit outside
  // internal_user_routes / self_managed_routes. Cost Optimization does not:
  // its primary call is /user/daily/activity, which every role may make, so
  // the entry stays and only its proxy-wide tabs are gated inside the page.
  describe("capability-gated pages whose data is proxy-admin-only", () => {
    const authFor = (userRole: string) => ({
      userId: "some-user-id",
      accessToken: "test-access-token",
      userRole,
      isViewOnly: false,
      token: "test-token",
      userEmail: "someone@example.com",
      premiumUser: false,
      disabledPersonalKeyCreation: false,
      showSSOBanner: false,
    });

    afterEach(() => {
      mockUseAuthorized.mockReset();
    });

    it("hides Workflow Runs and Memory from an internal user", () => {
      mockUseAuthorized.mockReturnValue(authFor("internal"));
      renderWithProviders(<Sidebar {...defaultProps} />);

      expect(screen.getByText("Logs")).toBeInTheDocument();
      expect(screen.queryByText("Workflow Runs")).not.toBeInTheDocument();
      expect(screen.queryByText("Memory")).not.toBeInTheDocument();
      expect(screen.queryByText("Agents")).not.toBeInTheDocument();
    });

    // An org admin's session role is "Org Admin", which no capability list
    // carries, and the proxy denies these routes to org admins too because
    // `_user_is_org_admin` needs an organization_id the page-load GET never sends.
    // Agents is already out of reach for this role, so gating the other two
    // empties the Agentic group entirely and the parent must go with it rather
    // than degrade into a leaf link to the non-route `?page=agentic`.
    it("drops the whole Agentic group for an org admin once its last child is gated", () => {
      mockUseAuthorized.mockReturnValue(authFor("org_admin"));
      renderWithProviders(<Sidebar {...defaultProps} />);

      // Liveness gate: Logs carries no role list, so it proves the sidebar rendered.
      expect(screen.getByText("Logs")).toBeInTheDocument();
      expect(screen.queryByText("Agentic")).not.toBeInTheDocument();
      expect(screen.queryByText("Workflow Runs")).not.toBeInTheDocument();
      expect(screen.queryByText("Memory")).not.toBeInTheDocument();
    });

    it("does not keep an Agentic group for an internal user", () => {
      mockUseAuthorized.mockReturnValue(authFor("internal"));
      renderWithProviders(<Sidebar {...defaultProps} />);

      expect(screen.queryByText("Agentic")).not.toBeInTheDocument();
      expect(screen.getByText("Guardrails")).toBeInTheDocument();
    });

    it("does not show Workflow Runs and Memory to admins", () => {
      renderWithProviders(<Sidebar {...defaultProps} />);

      expect(screen.queryByText("Workflow Runs")).not.toBeInTheDocument();
      expect(screen.queryByText("Memory")).not.toBeInTheDocument();
      expect(screen.getByText("Models + Endpoints")).toBeInTheDocument();
    });

    it("hides Guardrails Monitor from an internal user while keeping Usage and Cost Optimization", () => {
      mockUseAuthorized.mockReturnValue(authFor("internal"));
      renderWithProviders(<Sidebar {...defaultProps} />);

      expect(screen.queryByText("Guardrails Monitor")).not.toBeInTheDocument();
      expect(screen.getByText("Usage")).toBeInTheDocument();
      expect(screen.getByText("Cost Optimization")).toBeInTheDocument();
    });

    it("shows Guardrails Monitor to admins", () => {
      renderWithProviders(<Sidebar {...defaultProps} />);

      expect(screen.getByText("Guardrails Monitor")).toBeInTheDocument();
    });
  });

  it("should show Organizations tab for organization admins", () => {
    mockUseAuthorized.mockReturnValue({
      userId: "org-admin-user-id",
      accessToken: "test-access-token",
      userRole: "viewer",
      isViewOnly: false,
      token: "test-token",
      userEmail: "orgadmin@example.com",
      premiumUser: false,
      disabledPersonalKeyCreation: false,
      showSSOBanner: false,
    });

    mockUseOrganizations.mockReturnValue({
      data: [
        {
          organization_id: "org-1",
          organization_name: "Test Organization",
          spend: 0,
          max_budget: null,
          models: [],
          tpm_limit: null,
          rpm_limit: null,
          members: [
            {
              user_id: "org-admin-user-id",
              user_role: "org_admin",
            },
          ],
        },
      ],
      isLoading: false,
      error: null,
    } as any);

    renderWithProviders(<Sidebar {...defaultProps} />);

    expect(screen.getByText("Organizations")).toBeInTheDocument();
  });

  it("marks the nav item for the current route active", () => {
    navState.pathname = "/ui/logs";
    renderWithProviders(<Sidebar {...defaultProps} />);
    expect(screen.getByRole("link", { name: "Logs" })).toHaveAttribute("data-active", "true");
    expect(screen.getByRole("link", { name: "Virtual Keys" })).not.toHaveAttribute("data-active");
  });

  it("marks Virtual Keys active at the dashboard root", () => {
    navState.pathname = "/ui/";
    renderWithProviders(<Sidebar {...defaultProps} />);
    expect(screen.getByRole("link", { name: "Virtual Keys" })).toHaveAttribute("data-active", "true");
  });

  it("expands the parent group of the current nested route and marks the child active", () => {
    navState.pathname = "/ui/router-settings";
    renderWithProviders(<Sidebar {...defaultProps} />);
    expect(screen.getByRole("link", { name: "Router Settings" })).toHaveAttribute("data-active", "true");
    expect(screen.getByRole("button", { name: "Settings" })).toHaveAttribute("aria-expanded", "true");
  });

  it("links every leaf to its path route, including the ids that differ from their route", () => {
    renderWithProviders(<Sidebar {...defaultProps} />);

    const expectHref = (label: string, href: string) =>
      expect(screen.getByRole("link", { name: label })).toHaveAttribute("href", href);
    expectHref("Virtual Keys", "/ui/api-keys");
    expectHref("Playground", "/ui/playground");
    expectHref("Models + Endpoints", "/ui/models-and-endpoints");
    expectHref("Usage", "/ui/usage");
    expectHref("Guardrails", "/ui/guardrails");
    expectHref("Logs", "/ui/logs");
  });

  it("hides the removed columns and their nested pages", () => {
    renderWithProviders(<Sidebar {...defaultProps} enableProjectsUI />);
    const gone = [
      "Agents",
      "Workflow Runs",
      "Memory",
      "MCP Servers",
      "Skills",
      "Policies",
      "Tools",
      "Search Tools",
      "Vector Stores",
      "Tool Policies",
      "Developer Tools",
      "API Reference",
      "AI Hub",
      "Caching",
      "Experimental",
      "Prompts",
      "Transform Request",
      "Tag Management",
      "Old Usage",
      "Budgets",
    ];
    for (const label of gone) {
      expect(screen.queryByText(label), label).not.toBeInTheDocument();
    }
    const pages = menuGroups.flatMap((group) => group.items.flatMap((item) => [item.page, ...(item.children ?? []).map((child) => child.page)]));
    for (const id of ["agents", "workflows", "memory", "mcp-servers", "skills", "policies", "tools", "search-tools", "vector-stores", "tool-policies", "prompts", "tag-management", "transform-request", "caching", "api_ref", "model-hub-table", "usage", "budgets", "admin-panel", "ui-theme"]) {
      expect(pages, id).not.toContain(id);
    }
    for (const id of ["api-keys", "llm-playground", "models", "guardrails", "new_usage", "logs", "teams", "users", "organizations", "settings"]) {
      expect(pages, id).toContain(id);
    }
  });

  it("never links a leaf to the legacy ?page= switch", () => {
    renderWithProviders(<Sidebar {...defaultProps} enableProjectsUI />);
    for (const group of ["Settings"]) {
      act(() => {
        fireEvent.click(screen.getByText(group));
      });
    }
    const hrefs = screen.getAllByRole("link").map((link) => link.getAttribute("href") ?? "");
    expect(hrefs.filter((href) => href.includes("page="))).toHaveLength(0);
    expect(hrefs.filter((href) => href.startsWith("/ui/")).length).toBeGreaterThan(10);
  });

  it("hides labels but keeps items reachable (icon + link) when collapsed to the rail", () => {
    const { container } = renderWithProviders(<Sidebar {...defaultProps} collapsed />);
    expect(container.querySelector('[data-slot="sidebar"]')).toHaveAttribute("data-collapsed", "true");
    // The item stays navigable in the icon-only rail: its link still renders with
    // an icon (asserting the <a> + svg, not the text, so a removed icon would
    // fail here), while the label is present but CSS-hidden.
    const label = screen.getByText("Virtual Keys");
    const link = label.closest("a");
    expect(link).not.toBeNull();
    expect(link!.querySelector("svg")).not.toBeNull();
    expect(label).toHaveClass("group-data-[collapsed=true]/sidebar:hidden");
  });

  it("shows Cost Optimization without a Beta badge and no feature-flag gate", () => {
    const { container } = renderWithProviders(<Sidebar {...defaultProps} enableProjectsUI={false} />);

    const costOptimization = container.querySelector('a[href*="cost-optimization"]');
    expect(costOptimization).not.toBeNull();
    expect(costOptimization!).toHaveTextContent(/Cost Optimization/);
    expect(costOptimization!).not.toHaveTextContent(/Beta/);

    expect(container.querySelector('a[href*="projects"]')).toBeNull();
  });

  it("keeps a readable collapsed-rail tooltip for items whose label carries a badge", () => {
    const { container } = renderWithProviders(<Sidebar {...defaultProps} enableProjectsUI collapsed />);

    expect(container.querySelector('a[href*="cost-optimization"]')).toHaveAttribute("title", "Cost Optimization");
    expect(container.querySelector('a[href*="projects"]')).toHaveAttribute("title", "Projects");
  });

  it("translates group labels to Simplified Chinese", () => {
    setActiveLocale("zh-CN");
    renderWithProviders(<Sidebar {...defaultProps} />);
    expect(screen.getByText(t("nav.groups.gateway"))).toBeInTheDocument();
    expect(screen.getByText(t("nav.apiKeys"))).toBeInTheDocument();
    expect(screen.queryByText("AI GATEWAY")).not.toBeInTheDocument();
    expect(screen.queryByText("Virtual Keys")).not.toBeInTheDocument();
    setActiveLocale("en");
  });
});

describe("getBreadcrumb", () => {
  it("resolves a top-level route to its section + title", () => {
    expect(getBreadcrumb("/ui/api-keys")).toEqual({ section: "AI GATEWAY", title: "Virtual Keys" });
    expect(getBreadcrumb("/ui/logs")).toEqual({ section: "OBSERVABILITY", title: "Logs" });
  });

  it("resolves routes whose segment differs from the sidebar page id", () => {
    expect(getBreadcrumb("/ui/models-and-endpoints")).toEqual({ section: "AI GATEWAY", title: "Models + Endpoints" });
    expect(getBreadcrumb("/ui/usage")).toEqual({ section: "OBSERVABILITY", title: "Usage" });
  });

  it("titles the dashboard root as Virtual Keys", () => {
    expect(getBreadcrumb("/ui/")).toEqual({ section: "AI GATEWAY", title: "Virtual Keys" });
  });

  it("does not keep a section for a removed nested route", () => {
    expect(getBreadcrumb("/ui/search-tools/")).toEqual({ section: null, title: "Search Tools" });
  });

  it("resolves router-settings under the Settings section", () => {
    expect(getBreadcrumb("/ui/router-settings")).toEqual({ section: "SETTINGS", title: "Router Settings" });
  });

  it("falls back to a prettified title with no section for unknown routes", () => {
    expect(getBreadcrumb("/ui/some-unknown-page")).toEqual({ section: null, title: "Some Unknown Page" });
  });
});
