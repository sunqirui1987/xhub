import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders, screen, waitFor } from "../../../tests/test-utils";
import SidebarAccountMenu from "./SidebarAccountMenu";

interface AuthMock {
  userId: string | null;
  userEmail: string | null;
  userRoleLabel: string;
  premiumUser: boolean;
  accessToken: string;
}

let mockOrgAdmin = false;
let mockTeamAdmin = false;

let mockUseAuthorizedImpl: () => AuthMock = () => ({
  userId: "test-user-id",
  userEmail: "test@example.com",
  userRoleLabel: "Admin",
  premiumUser: false,
  accessToken: "test-token",
});

let mockUseDisableShowPromptsImpl = () => false;
let mockUseDisableBouncingIconImpl = () => false;
let mockHealthDataImpl = (): { litellm_version?: string } | undefined => ({ litellm_version: "1.99.0" });

let mockGetLocalStorageItemImpl = (key: string): string | null => {
  if (key === "disableShowNewBadge") return null;
  if (key === "disableShowPrompts") return null;
  return null;
};

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: () => mockUseAuthorizedImpl(),
}));

vi.mock("@/app/(dashboard)/hooks/useIsOrgAdmin", () => ({
  default: () => mockOrgAdmin,
}));

vi.mock("@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity", () => ({
  useSessionIdentity: () => ({
    data: {
      teams: mockTeamAdmin ? [{ team_id: "team-1", role: "team_admin" }] : [],
    },
  }),
}));

vi.mock("@/app/(dashboard)/hooks/useDisableShowPrompts", () => ({
  useDisableShowPrompts: () => mockUseDisableShowPromptsImpl(),
}));

vi.mock("@/app/(dashboard)/hooks/useDisableBlogPosts", () => ({
  useDisableBlogPosts: () => false,
}));

vi.mock("@/app/(dashboard)/hooks/useDisableBouncingIcon", () => ({
  useDisableBouncingIcon: () => mockUseDisableBouncingIconImpl(),
}));

vi.mock("@/app/(dashboard)/hooks/healthReadiness/useHealthReadinessDetails", () => ({
  useHealthReadinessDetails: () => ({ data: mockHealthDataImpl() }),
}));

vi.mock("@/utils/localStorageUtils", () => ({
  LOCAL_STORAGE_EVENT: "local-storage-change",
  getLocalStorageItem: (key: string) => mockGetLocalStorageItemImpl(key),
  setLocalStorageItem: vi.fn(),
  removeLocalStorageItem: vi.fn(),
  emitLocalStorageChange: vi.fn(),
}));

describe("SidebarAccountMenu", () => {
  const mockOnLogout = vi.fn();

  const getAccountTrigger = () => screen.getByRole("button", { name: /account menu/i });

  const openMenu = async (user: ReturnType<typeof userEvent.setup>) => {
    await user.click(getAccountTrigger());
    await waitFor(() => {
      expect(screen.getByTestId("sidebar-account-menu-panel")).toBeInTheDocument();
    });
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockOrgAdmin = false;
    mockTeamAdmin = false;
    mockUseAuthorizedImpl = () => ({
      userId: "test-user-id",
      userEmail: "test@example.com",
      userRoleLabel: "Admin",
      premiumUser: false,
      accessToken: "test-token",
    });
    mockUseDisableShowPromptsImpl = () => false;
    mockUseDisableBouncingIconImpl = () => false;
    mockHealthDataImpl = () => ({ litellm_version: "1.99.0" });
    mockGetLocalStorageItemImpl = (key: string): string | null => {
      if (key === "disableShowNewBadge") return null;
      if (key === "disableShowPrompts") return null;
      return null;
    };
  });

  it("should render the account trigger with initials", () => {
    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);
    expect(getAccountTrigger()).toBeInTheDocument();
    expect(screen.getByText("TE")).toBeInTheDocument();
  });

  it("should render only the avatar (no name/role) when collapsed", () => {
    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} collapsed />);
    expect(getAccountTrigger()).toBeInTheDocument();
    expect(screen.getByText("TE")).toBeInTheDocument();
    expect(screen.queryByText("Admin")).not.toBeInTheDocument();
    expect(screen.queryByText("test@example.com")).not.toBeInTheDocument();
  });

  it("offers a password change for the signed-in account", async () => {
    const user = userEvent.setup();
    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);
    await openMenu(user);
    await user.click(screen.getByRole("button", { name: "Change account password" }));
    expect(screen.getByRole("dialog", { name: "Set a new password" })).toBeInTheDocument();
  });

  it("should show email, user ID, and role when the menu is opened", async () => {
    const user = userEvent.setup();
    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);

    await openMenu(user);

    expect(screen.getAllByText("test@example.com").length).toBeGreaterThan(0);
    expect(screen.getByText("test-user-id")).toBeInTheDocument();
    expect(screen.getAllByText("Platform administrator").length).toBeGreaterThan(0);
  });

  it("shows team management and organization administration when the account holds them", async () => {
    const user = userEvent.setup();
    mockUseAuthorizedImpl = () => ({
      userId: "test-user-id",
      userEmail: "111@qq.com",
      userRoleLabel: "Internal User",
      premiumUser: true,
      accessToken: "test-token",
    });
    mockOrgAdmin = true;
    mockTeamAdmin = true;
    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);
    await openMenu(user);
    expect(screen.getByRole("link", { name: "Organization administrator" })).toHaveAttribute("href", "/organizations");
    expect(screen.getByRole("link", { name: "Team management" })).toHaveAttribute("href", "/teams");
    expect(screen.queryByText("Internal User")).not.toBeInTheDocument();
  });

  it("does not show a tier or the new-feature toggle", async () => {
    const user = userEvent.setup();
    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);
    await openMenu(user);
    expect(screen.queryByText("Tier")).not.toBeInTheDocument();
    expect(screen.queryByText("Standard")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Toggle hide new feature indicators")).not.toBeInTheDocument();
  });

  it("should render the version as text without an outbound link", async () => {
    const user = userEvent.setup();
    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);

    await openMenu(user);

    expect(screen.getByText("v1.99.0")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /v1\.99\.0/ })).not.toBeInTheDocument();
    expect(screen.getByText("XHub")).toBeInTheDocument();
  });

  it("should not render the version badge when the version is unavailable", async () => {
    const user = userEvent.setup();
    mockHealthDataImpl = () => undefined;
    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);

    await openMenu(user);

    expect(screen.queryByText(/^v/)).not.toBeInTheDocument();
  });

  it("should hide the bouncing icon when Hide Bouncing Icon is enabled", async () => {
    const user = userEvent.setup();
    mockUseDisableBouncingIconImpl = () => true;
    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);
    await openMenu(user);
    expect(screen.queryByTitle("Thanks for using XHub!")).not.toBeInTheDocument();
  });

  it("wires the email row to the shared copy button", async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });

    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);
    await openMenu(user);

    const copyButton = screen.getByRole("button", { name: "Copy email" });
    await user.click(copyButton);

    expect(writeText).toHaveBeenCalledWith("test@example.com");
    await waitFor(() => expect(copyButton.querySelector(".lucide-check")).toBeInTheDocument());
  });

  it("should call onLogout when logout is clicked", async () => {
    const user = userEvent.setup();
    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);

    await openMenu(user);

    await user.click(screen.getByRole("button", { name: /logout/i }));

    expect(mockOnLogout).toHaveBeenCalledTimes(1);
  });



  it("should show Account in the trigger for the default placeholder user id", () => {
    mockUseAuthorizedImpl = () => ({
      userId: "default_user_id",
      userEmail: null,
      userRoleLabel: "Admin",
      premiumUser: false,
      accessToken: "test-token",
    });
    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);
    expect(screen.getByText("Account")).toBeInTheDocument();
  });

  it("should display a dash when email is unavailable", async () => {
    const user = userEvent.setup();
    mockUseAuthorizedImpl = () => ({
      userId: "test-user-id",
      userEmail: null,
      userRoleLabel: "Admin",
      premiumUser: false,
      accessToken: "test-token",
    });

    renderWithProviders(<SidebarAccountMenu onLogout={mockOnLogout} />);

    await openMenu(user);

    expect(screen.getByText("-")).toBeInTheDocument();
  });
});
