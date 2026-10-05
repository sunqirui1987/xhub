import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "../../tests/test-utils";
import Sidebar, { menuGroups, getBreadcrumb } from "./leftnav";
import { t } from "@/i18n";
import { setActiveLocale } from "@/i18n/runtime";

// The sidebar reads what the gateway says the session may do, not a role label
// in the token. Each test sets the identity it wants the menu to be built from.
const identity = vi.hoisted(() => ({
  current: {
    user_id: "test-user-id",
    user_role: "admin",
    kind: "session",
    capabilities: ["platform.admin", "teams.read", "teams.manage", "teams.platform", "audit.read"],
    teams: [] as { team_id: string; role: "team_admin" | "member" }[],
  },
}));

vi.mock("@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity")>();
  return {
    ...actual,
    useSessionIdentity: () => ({ data: identity.current, isLoading: false }),
  };
});

/** setIdentity points the menu at one caller. */
const setIdentity = (caps: string[], teams: { team_id: string; role: "team_admin" | "member" }[] = []) => {
  identity.current = {
    user_id: "test-user-id",
    user_role: caps.includes("platform.admin") ? "admin" : "user",
    kind: "session",
    capabilities: caps,
    teams,
  };
};

const PLATFORM_ADMIN_CAPS = [
  "profile.self",
  "keys.personal",
  "teams.read",
  "teams.manage",
  "teams.platform",
  "members.manage",
  "projects.manage",
  "access_groups.manage",
  "usage.global",
  "audit.read",
  "platform.admin",
  "users.manage",
  "orgs.manage",
];
const MEMBER_CAPS = ["profile.self", "keys.personal", "usage.self", "logs.self", "infer"];

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
  const defaultProps = { collapsed: false };

  const labels = (): string[] =>
    Array.from(document.querySelectorAll("[data-slot='sidebar-menu-button'], a, button"))
      .map((el) => el.textContent ?? "")
      .filter(Boolean);

  afterEach(() => {
    identity.current = {
      user_id: "test-user-id",
      user_role: "admin",
      kind: "session",
      capabilities: PLATFORM_ADMIN_CAPS,
      teams: [],
    };
    navState.pathname = "/ui/api-keys";
    setActiveLocale("en");
  });

  describe("layering by identity", () => {
    it("gives a platform administrator every group", () => {
      setIdentity(PLATFORM_ADMIN_CAPS);
      renderWithProviders(<Sidebar {...defaultProps} />);
      const text = labels().join(" | ");
      expect(text).toContain("Virtual Keys");
      expect(text).toContain("Usage");
      expect(text).toContain("Permissions");
      expect(text).toContain("Organizations");
      expect(text).toContain("Models + Endpoints");
      expect(text).toContain("Router Settings");
    });

    it("keeps platform administration away from a plain member", () => {
      setIdentity(MEMBER_CAPS, [{ team_id: "t1", role: "member" }]);
      renderWithProviders(<Sidebar {...defaultProps} />);
      const text = labels().join(" | ");
      // The deployment's own configuration is not a team's business, and the
      // server refuses every one of these routes to this caller.
      expect(text).not.toContain("Organizations");
      expect(text).not.toContain("Internal Users");
      expect(text).not.toContain("Models + Endpoints");
      expect(text).not.toContain("Router Settings");
      expect(text).not.toContain("Price data");
    });

    it("still gives a plain member their own pages and their team's", () => {
      setIdentity(MEMBER_CAPS, [{ team_id: "t1", role: "member" }]);
      renderWithProviders(<Sidebar {...defaultProps} />);
      const text = labels().join(" | ");
      expect(text).toContain("Virtual Keys");
      expect(text).toContain("Usage");
      expect(text).toContain("Logs");
      expect(text).toContain("Teams");
      expect(text).toContain("Permissions");
    });

    it("gives a caller who belongs to no team no team group", () => {
      // A group with nothing in it is dropped rather than rendered as an empty
      // heading, because an empty heading reads as a page that failed to load.
      setIdentity(MEMBER_CAPS, []);
      renderWithProviders(<Sidebar {...defaultProps} />);
      const text = labels().join(" | ");
      expect(text).not.toContain("Teams");
      expect(text).not.toContain("Permissions");
    });

    it("shows a team administrator the team pages but not platform administration", () => {
      setIdentity(["profile.self", "keys.personal", "teams.read", "teams.manage", "projects.manage"], [
        { team_id: "t1", role: "team_admin" },
      ]);
      renderWithProviders(<Sidebar {...defaultProps} />);
      const text = labels().join(" | ");
      expect(text).toContain("Teams");
      expect(text).toContain("Permissions");
      expect(text).not.toContain("Organizations");
      expect(text).not.toContain("Models + Endpoints");
    });
  });

  describe("menu structure", () => {
    it("keeps the team group together and in one place", () => {
      // The team's scope is one menu, not scattered across the sidebar: a team
      // administrator manages the same pages a member reads.
      for (const page of ["teams", "projects", "permissions"]) {
        expect(placementsOf(page)).toEqual(["nav.groups.team"]);
      }
      // The removed features must not linger in the menu: a link to a page
      // whose directory is gone is a 404 behind a click.
      for (const page of ["access-groups", "mcp-servers", "skills"]) {
        expect(placementsOf(page)).toEqual([]);
      }
    });

    it("keeps every platform page in the platform group", () => {
      for (const page of ["organizations", "users", "models", "price-data", "guardrails"]) {
        expect(placementsOf(page)).toEqual(["nav.groups.platform"]);
      }
    });

    it("has no duplicate keys among all menu items", () => {
      const keys = collectNavKeys();
      const duplicates = keys.filter((key, i) => keys.indexOf(key) !== i);
      expect(duplicates).toEqual([]);
    });

    it("marks the groups only a platform administrator sees", () => {
      const gated = menuGroups.filter((g) => g.requiresPlatformAdmin).map((g) => g.groupLabel);
      expect(gated).toEqual(["nav.groups.platform", "nav.groups.settings"]);
    });

    it("renders a group with no children as a plain link, not a toggle", () => {
      // Every group in the new structure lists leaves directly. A group rendered
      // as a toggle with nothing under it would open onto nothing, so the
      // sidebar must offer it as a link.
      setIdentity(PLATFORM_ADMIN_CAPS);
      renderWithProviders(<Sidebar {...defaultProps} />);
      expect(screen.getByRole("link", { name: t("nav.organizations") })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: t("nav.groups.platform") })).not.toBeInTheDocument();
    });
  });

  describe("i18n", () => {
    it("renders the group headings in Simplified Chinese", () => {
      setIdentity(PLATFORM_ADMIN_CAPS);
      setActiveLocale("zh-CN");
      renderWithProviders(<Sidebar {...defaultProps} />);
      // Group headings are rendered as labels above their items, so the test
      // asserts on the heading text the deployment ships, not on an item.
      expect(document.body.textContent).toContain("平台管理");
      expect(document.body.textContent).toContain("团队");
    });
  });

  describe("breadcrumbs", () => {
    it("names the section and page for a leaf", () => {
      const crumb = getBreadcrumb("/ui/api-keys");
      expect(crumb.section).toBe(t("nav.groups.mine"));
    });

    it("falls back to a prettified route for a page not in the menu", () => {
      const crumb = getBreadcrumb("/ui/some-unknown-page");
      expect(crumb.title).toBe("Some Unknown Page");
    });
  });
});
