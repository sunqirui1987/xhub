import { useTeamMetadataSchema } from "@/app/(dashboard)/hooks/teams/useTeamMetadataSchema";
import * as networking from "@/components/networking";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { chooseSelectOption, renderWithProviders, testQueryClient } from "../../../tests/test-utils";
import { toast } from "@/lib/toast";
import type { EffectiveMcpServer } from "../mcp_server_management/effectiveMcpServers";
import type { MCPServer } from "../mcp_tools/types";
import TeamInfoView, {
  grantedMcpServerIds,
  type McpGrantInput,
  retainedMcpToolPermissions,
  standingToolPermissionServerIds,
  type TeamData,
} from "./TeamInfo";

const authState = vi.hoisted(() => ({ userRole: "Admin" }));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: () => ({
    token: "123",
    accessToken: "123",
    userId: "user-1",
    userEmail: "user@example.com",
    userRole: authState.userRole,
    premiumUser: false,
    disabledPersonalKeyCreation: null,
    showSSOBanner: false,
  }),
}));

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));

vi.mock("@/components/networking", () => ({
  serverRootPath: "",
  teamInfoCall: vi.fn(),
  teamMemberDeleteCall: vi.fn(),
  teamMemberAddCall: vi.fn(),
  teamMemberUpdateCall: vi.fn(),
  teamUpdateCall: vi.fn(),
  getGuardrailsList: vi.fn(),
  getPoliciesList: vi.fn(),
  getPolicyInfoWithGuardrails: vi.fn(),
  fetchMCPAccessGroups: vi.fn(),
  getTeamPermissionsCall: vi.fn(),
  organizationInfoCall: vi.fn(),
  getRouterSettingsCall: vi.fn().mockResolvedValue({ fields: [] }),
  getRouteTemplatesCall: vi.fn().mockResolvedValue([]),
  getRouteTemplateBindingCall: vi.fn().mockResolvedValue({
    route_template_id: "",
    effective: { scope_type: "platform" },
  }),
  fetchMCPServers: vi.fn().mockResolvedValue([]),
  fetchMCPToolsets: vi.fn().mockResolvedValue([]),
  listMCPTools: vi.fn().mockResolvedValue({ tools: [] }),
  vectorStoreListCall: vi.fn().mockResolvedValue({ data: [] }),
  getAgentsList: vi.fn().mockResolvedValue({ agents: [] }),
  getClaudeCodePluginsList: vi.fn().mockResolvedValue({ plugins: [], count: 0 }),
}));

const can = vi.fn();
vi.mock("@/app/(dashboard)/hooks/useCan", () => ({
  default: (...args: unknown[]) => can(...args),
}));

vi.mock("@/components/utils/dataUtils", () => ({
  copyToClipboard: vi.fn().mockResolvedValue(true),
  formatNumberWithCommas: vi.fn((value: number) => value.toLocaleString()),
}));

vi.mock("@/app/(dashboard)/hooks/teams/useTeamMetadataSchema", () => ({
  useTeamMetadataSchema: vi.fn(() => ({ data: [], isLoading: false })),
}));

vi.mock("@/app/(dashboard)/hooks/models/useModels", () => ({
  useAllProxyModels: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/teams/useTeams", () => ({
  useTeam: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/organizations/useOrganizations", () => ({
  organizationKeys: { all: ["organizations"] },
  useOrganization: vi.fn(),
  useOrganizations: vi.fn().mockReturnValue({ data: [], isLoading: false }),
}));

vi.mock("@/app/(dashboard)/hooks/users/useCurrentUser", () => ({
  useCurrentUser: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/mcpServers/useMCPServers", () => ({
  useMCPServers: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/mcpServers/useMCPToolsets", () => ({
  useMCPToolsets: vi.fn(),
}));

vi.mock("@/components/mcp_server_management/MCPServerSelector", () => ({
  default: ({
    value,
    onChange,
  }: {
    value?: { servers: string[]; accessGroups: string[]; toolsets?: string[] };
    onChange: (next: { servers: string[]; accessGroups: string[]; toolsets: string[] }) => void;
  }) => (
    <>
      <button
        type="button"
        onClick={() =>
          onChange({ servers: [], accessGroups: value?.accessGroups ?? [], toolsets: value?.toolsets ?? [] })
        }
      >
        deselect all mcp servers
      </button>
      <button type="button" onClick={() => onChange({ servers: value?.servers ?? [], accessGroups: [], toolsets: [] })}>
        remove all access groups
      </button>
    </>
  ),
}));

vi.mock("@/components/team/TeamMemberTab", () => ({
  default: vi.fn(({ setIsAddMemberModalVisible, setSelectedEditMember, setIsEditMemberModalVisible }) => (
    <div>
      <button onClick={() => setIsAddMemberModalVisible(true)}>Add Member</button>
      <button onClick={() => {
        setSelectedEditMember({ user_email: "edit@test.com", user_id: "edit-user", role: "user", max_budget_in_team: 400 });
        setIsEditMemberModalVisible(true);
      }}>Edit Member</button>
    </div>
  )),
}));

vi.mock("@/components/common_components/user_search_modal", () => ({
  default: vi.fn(({ isVisible, onCancel, onSubmit }) =>
    isVisible ? (
      <div>
        <button onClick={onCancel}>Cancel</button>
        <button onClick={() => onSubmit({ user_email: "new@test.com", user_id: "new-user", role: "user" })}>
          Submit
        </button>
      </div>
    ) : null,
  ),
}));

vi.mock("@/components/common_components/DeleteResourceModal", () => ({
  default: vi.fn(({ isOpen, onCancel, onOk }) =>
    isOpen ? (
      <div>
        <button onClick={onCancel}>Cancel</button>
        <button onClick={onOk}>Confirm Delete</button>
      </div>
    ) : null,
  ),
}));

vi.mock("@/components/team/member_permissions", () => ({
  default: vi.fn(() => <div>Member Permissions</div>),
}));

vi.mock("@/components/common_components/ModelAliasManager", () => ({
  default: vi.fn(({ initialModelAliases, onAliasUpdate }) => (
    <div>
      <div data-testid="alias-editor-initial">{JSON.stringify(initialModelAliases)}</div>
      <button type="button" onClick={() => onAliasUpdate({ "gpt-4o": "gpt-4" })}>
        Set Alias
      </button>
      <button type="button" onClick={() => onAliasUpdate({})}>
        Clear Aliases
      </button>
    </div>
  )),
}));

vi.mock("@/app/(dashboard)/hooks/accessGroups/useAccessGroups", () => ({
  useAccessGroups: vi.fn().mockReturnValue({
    data: [
      { access_group_id: "ag-1", access_group_name: "Group 1", access_mcp_server_ids: [] },
      { access_group_id: "ag-2", access_group_name: "Group 2", access_mcp_server_ids: [] },
    ],
    isLoading: false,
    isError: false,
  }),
}));

vi.mock("@/components/common_components/AccessGroupSelector", () => ({
  default: ({ value, onChange }: { value?: string[]; onChange?: (next: string[]) => void }) => (
    <button type="button" onClick={() => onChange?.((value ?? []).slice(1))}>
      remove first unified access group
    </button>
  ),
}));

vi.mock("@/app/(dashboard)/hooks/keys/useKeys", () => ({
  useKeys: vi.fn().mockReturnValue({
    data: { keys: [], total_count: 0, current_page: 1, total_pages: 1 },
    isPending: false,
    isFetching: false,
    refetch: vi.fn(),
  }),
}));

vi.mock("../key_team_helpers/filter_helpers", () => ({
  fetchTeamFilterOptions: vi.fn().mockResolvedValue({
    keyAliases: [],
    organizationIds: [],
    userIds: [],
  }),
  fetchAllKeyAliases: vi.fn().mockResolvedValue([]),
  fetchAllOrganizations: vi.fn().mockResolvedValue([]),
}));

import { useAllProxyModels } from "@/app/(dashboard)/hooks/models/useModels";
import { useKeys } from "@/app/(dashboard)/hooks/keys/useKeys";
import { useOrganization } from "@/app/(dashboard)/hooks/organizations/useOrganizations";
import { useTeam } from "@/app/(dashboard)/hooks/teams/useTeams";
import { useCurrentUser } from "@/app/(dashboard)/hooks/users/useCurrentUser";
import { useMCPServers } from "@/app/(dashboard)/hooks/mcpServers/useMCPServers";
import { useMCPToolsets } from "@/app/(dashboard)/hooks/mcpServers/useMCPToolsets";
import { useAccessGroups } from "@/app/(dashboard)/hooks/accessGroups/useAccessGroups";

const mockUseAllProxyModels = vi.mocked(useAllProxyModels);
const mockUseKeys = vi.mocked(useKeys);
const mockUseTeam = vi.mocked(useTeam);
const mockUseOrganization = vi.mocked(useOrganization);
const mockUseCurrentUser = vi.mocked(useCurrentUser);
const mockUseMCPServers = vi.mocked(useMCPServers);
const mockUseMCPToolsets = vi.mocked(useMCPToolsets);
const mockUseAccessGroups = vi.mocked(useAccessGroups);

const createMockTeamData = (overrides = {}) => ({
  team_id: "123",
  team_info: {
    team_alias: "Test Team",
    team_id: "123",
    organization_id: null,
    admins: ["admin@test.com"],
    members: ["user1@test.com"],
    members_with_roles: [
      {
        user_id: "user1@test.com",
        user_email: "user1@test.com",
        role: "member",
        spend: 0,
        budget_id: "budget1",
      },
    ],
    metadata: {},
    tpm_limit: null,
    rpm_limit: null,
    max_budget: null,
    budget_duration: null,
    models: [],
    blocked: false,
    spend: 0,
    max_parallel_requests: null,
    budget_reset_at: null,
    model_id: null,
    litellm_model_table: null,
    created_at: "2024-01-01T00:00:00Z",
    team_member_budget_table: null,
    guardrails: [],
    policies: [],
    object_permission: null,
    ...overrides,
  },
  keys: [],
  team_memberships: [],
});

const seedDefaultMocks = () => {
  mockUseAllProxyModels.mockReturnValue({
    data: { data: [] },
    isLoading: false,
  } as any);
  mockUseTeam.mockReturnValue({
    data: undefined,
    isLoading: false,
  } as any);
  mockUseOrganization.mockReturnValue({
    data: undefined,
    isLoading: false,
  } as any);
  mockUseCurrentUser.mockReturnValue({
    data: { models: [] },
    isLoading: false,
  } as any);
  mockUseMCPServers.mockReturnValue({ data: [], isLoading: false, isError: false } as any);
  mockUseMCPToolsets.mockReturnValue({ data: [], isLoading: false, isError: false } as any);
  mockUseAccessGroups.mockReturnValue({
    data: [
      { access_group_id: "ag-1", access_group_name: "Group 1", access_mcp_server_ids: [] },
      { access_group_id: "ag-2", access_group_name: "Group 2", access_mcp_server_ids: [] },
    ],
    isLoading: false,
    isError: false,
  } as any);
  mockUseKeys.mockReturnValue({
    data: { keys: [], total_count: 0, current_page: 1, total_pages: 1 },
    isPending: false,
    isFetching: false,
    refetch: vi.fn(),
  } as any);
  vi.mocked(useTeamMetadataSchema).mockReturnValue({ data: [], isLoading: false } as any);

  can.mockReturnValue(true);
  vi.mocked(networking.getGuardrailsList).mockResolvedValue({ guardrails: [] });
  vi.mocked(networking.getPoliciesList).mockResolvedValue({ policies: [] });
  vi.mocked(networking.fetchMCPAccessGroups).mockResolvedValue([]);
  vi.mocked(networking.getTeamPermissionsCall).mockResolvedValue({
    all_available_permissions: [],
    team_member_permissions: [],
  });
};

describe("TeamInfoView", () => {
  const defaultProps = {
    teamId: "123",
    onUpdate: vi.fn(),
    onClose: vi.fn(),
    accessToken: "test-token",
    is_team_admin: true,
    is_proxy_admin: true,
    userModels: ["gpt-4", "gpt-3.5-turbo"],
    editTeam: false,
    premiumUser: false,
  };

  beforeEach(seedDefaultMocks);

  afterEach(() => {
    vi.clearAllMocks();
    authState.userRole = "Admin";
  });

  describe("display and rendering", () => {
    it("should render", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await waitFor(() => {
        const teamNameElements = screen.queryAllByText("Test Team");
        expect(teamNameElements.length).toBeGreaterThan(0);
      });
    });

    it("links direct and access-group model badges to the models page filtered to that group", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(
        createMockTeamData({
          models: ["gpt-4.1"],
          access_group_models: ["claude-sonnet-5"],
          access_group_details: [{ access_group_id: "ag-1", access_group_name: "prod", models: ["claude-sonnet-5"] }],
        }),
      );

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      expect(await screen.findByRole("link", { name: "gpt-4.1" })).toHaveAttribute(
        "href",
        expect.stringContaining("/models-and-endpoints?model_group=gpt-4.1"),
      );
      expect(screen.getByRole("link", { name: "claude-sonnet-5" })).toHaveAttribute(
        "href",
        expect.stringContaining("/models-and-endpoints?model_group=claude-sonnet-5"),
      );
    });

    it("does not show object permissions on the team page", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(
        createMockTeamData({
          object_permission: {
            mcp_servers: ["mcp-github-1234"],
            agents: ["agent-support-5678"],
          } as never,
        }),
      );

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      expect(await screen.findByRole("tab", { name: "Overview" })).toBeInTheDocument();
      expect(screen.queryByText("Object Permissions")).not.toBeInTheDocument();
      expect(screen.queryByRole("tab", { name: "Member Permissions" })).not.toBeInTheDocument();
      expect(screen.queryByRole("tab", { name: "My User" })).not.toBeInTheDocument();
    });

    it("keeps the all-proxy-models badge non-clickable", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData({ models: ["all-proxy-models"] }));

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      expect(await screen.findByText("All proxy models")).toBeInTheDocument();
      expect(screen.queryByRole("link", { name: "All proxy models" })).not.toBeInTheDocument();
    });

    it("should display loading state while fetching team data", () => {
      vi.mocked(networking.teamInfoCall).mockImplementation(() => new Promise(() => {}));

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      expect(screen.getByText("Loading...")).toBeInTheDocument();
    });

    it("should display error message when team is not found", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue({
        team_id: "123",
        team_info: null as any,
        keys: [],
        team_memberships: [],
      });

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await waitFor(() => {
        expect(screen.getByText("Team not found")).toBeInTheDocument();
      });
    });

    it("should display budget information in overview", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(
        createMockTeamData({
          max_budget: 1000,
          spend: 250.5,
          budget_duration: "30d",
        }),
      );

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await waitFor(() => {
        expect(screen.getByText("Budget Status")).toBeInTheDocument();
      });
      expect(screen.getByText("$250.50")).toBeInTheDocument();
      expect(screen.getByText("Budget cap: $1,000.00")).toBeInTheDocument();
    });

    it("should display virtual keys information", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue({
        ...createMockTeamData(),
        keys: [{ user_id: "user1", token: "key1" }, { token: "key2" }],
      });

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Virtual Keys" })).toBeInTheDocument();
      });
    });

    it("should display object permissions when present", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(
        createMockTeamData({
          object_permission: {
            object_permission_id: "perm-1",
            mcp_servers: ["server1"],
            vector_stores: ["store1"],
          },
        }),
      );

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await waitFor(() => {
        const teamNameElements = screen.queryAllByText("Test Team");
        expect(teamNameElements.length).toBeGreaterThan(0);
      });
    });

    it("should open Settings tab by default when editTeam is true and user can edit", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(<TeamInfoView {...defaultProps} editTeam={true} />);

      await waitFor(() => {
        const teamNameElements = screen.queryAllByText("Test Team");
        expect(teamNameElements.length).toBeGreaterThan(0);
      });

      expect(screen.getByText("Team Settings")).toBeInTheDocument();
    });

    it("should open Overview tab by default when editTeam is false", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(<TeamInfoView {...defaultProps} editTeam={false} />);

      await waitFor(() => {
        const teamNameElements = screen.queryAllByText("Test Team");
        expect(teamNameElements.length).toBeGreaterThan(0);
      });

      expect(screen.getByText("Budget Status")).toBeInTheDocument();
    });

    it("should open Overview tab by default when editTeam is true but user cannot edit", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(
        <TeamInfoView {...defaultProps} editTeam={true} is_team_admin={false} is_proxy_admin={false} />,
      );

      await waitFor(() => {
        const teamNameElements = screen.queryAllByText("Test Team");
        expect(teamNameElements.length).toBeGreaterThan(0);
      });

      expect(screen.getByText("Budget Status")).toBeInTheDocument();
    });
  });

  describe("tabs and navigation", () => {
    it("should show members tab when user can edit team", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Members" })).toBeInTheDocument();
      });
    });

    it("should not show members tab when user cannot edit team", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(<TeamInfoView {...defaultProps} is_team_admin={false} is_proxy_admin={false} />);

      await waitFor(() => {
        const teamNameElements = screen.queryAllByText("Test Team");
        expect(teamNameElements.length).toBeGreaterThan(0);
      });

      expect(screen.queryByRole("tab", { name: "Members" })).not.toBeInTheDocument();
    });

    it("should show settings tab when user can edit team", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Settings" })).toBeInTheDocument();
      });
    });

    it("shows edit tabs when the fetched team data marks the session user as team admin, even without the is_team_admin prop", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(
        createMockTeamData({
          members_with_roles: [
            {
              user_id: "user-1",
              user_email: "admin@test.com",
              role: "admin",
              spend: 0,
              budget_id: "budget1",
            },
          ],
        }),
      );

      renderWithProviders(<TeamInfoView {...defaultProps} is_team_admin={false} is_proxy_admin={false} />);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Settings" })).toBeInTheDocument();
      });
      expect(screen.getByRole("tab", { name: "Members" })).toBeInTheDocument();
    });

    it("should navigate to settings tab when clicked", async () => {
      const user = userEvent.setup({ delay: null });
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await waitFor(() => {
        const teamNameElements = screen.queryAllByText("Test Team");
        expect(teamNameElements.length).toBeGreaterThan(0);
      });

      const settingsTab = screen.getByRole("tab", { name: "Settings" });
      await user.click(settingsTab);

      await waitFor(() => {
        expect(screen.getByText("Team Settings")).toBeInTheDocument();
      });
    });

    it("should call onClose when back button is clicked", async () => {
      const user = userEvent.setup({ delay: null });
      const onClose = vi.fn();
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(<TeamInfoView {...defaultProps} onClose={onClose} />);

      await waitFor(() => {
        const teamNameElements = screen.queryAllByText("Test Team");
        expect(teamNameElements.length).toBeGreaterThan(0);
      });

      const backButton = screen.getByRole("button", { name: /back to teams/i });
      await user.click(backButton);

      expect(onClose).toHaveBeenCalled();
    });

    it("should copy team ID to clipboard when copy button is clicked", async () => {
      const user = userEvent.setup({ delay: null });
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await waitFor(() => {
        const teamNameElements = screen.queryAllByText("Test Team");
        expect(teamNameElements.length).toBeGreaterThan(0);
      });

      const copyButtons = screen.getAllByRole("button");
      const copyButton = copyButtons.find((btn) => btn.querySelector("svg"));
      expect(copyButton).toBeTruthy();

      if (copyButton) {
        await user.click(copyButton);
      }
    });

    it("should show Virtual Keys tab when user cannot edit team", async () => {
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(<TeamInfoView {...defaultProps} is_team_admin={false} is_proxy_admin={false} />);

      await waitFor(() => {
        expect(screen.getByRole("tab", { name: "Virtual Keys" })).toBeInTheDocument();
      });
    });

    it("should display X Members in Virtual Keys tab when navigated to", async () => {
      const user = userEvent.setup();
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());
      const fiveKeys = Array.from({ length: 5 }, (_, i) => ({
        token: `sk-${i}`,
        token_id: `key-${i}`,
        key_alias: `key_${i}`,
        key_name: `sk-...${i}`,
        user_id: `user-${i}`,
        organization_id: null,
        user: { user_id: `user-${i}`, user_email: `user${i}@test.com` },
        created_at: "2024-01-01T00:00:00Z",
        team_id: "123",
        spend: 0,
        max_budget: 100,
        models: ["gpt-4"],
      }));
      mockUseKeys.mockReturnValue({
        data: { keys: fiveKeys, total_count: 5, current_page: 1, total_pages: 1 },
        isPending: false,
        isFetching: false,
        refetch: vi.fn(),
      } as any);

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await waitFor(() => {
        const teamNameElements = screen.queryAllByText("Test Team");
        expect(teamNameElements.length).toBeGreaterThan(0);
      });

      const virtualKeysTab = screen.getByRole("tab", { name: "Virtual Keys" });
      await user.click(virtualKeysTab);

      await waitFor(() => {
        expect(screen.getByTestId("pagination-range")).toHaveTextContent("Showing 1-5 of 5");
      });
    });

    it("should show Filters and pagination controls in Virtual Keys tab", async () => {
      const user = userEvent.setup();
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());
      mockUseKeys.mockReturnValue({
        data: {
          keys: [
            {
              token: "sk-1",
              token_id: "key-1",
              key_alias: "key1",
              key_name: "sk-...1",
              user_id: "user-1",
              organization_id: null,
              user: { user_id: "user-1", user_email: "user1@test.com" },
              created_at: "2024-01-01T00:00:00Z",
              team_id: "123",
              spend: 0,
              max_budget: 100,
              models: ["gpt-4"],
            },
          ],
          total_count: 1,
          current_page: 1,
          total_pages: 1,
        },
        isPending: false,
        isFetching: false,
        refetch: vi.fn(),
      } as any);

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await waitFor(() => {
        const teamNameElements = screen.queryAllByText("Test Team");
        expect(teamNameElements.length).toBeGreaterThan(0);
      });

      const virtualKeysTab = screen.getByRole("tab", { name: "Virtual Keys" });
      await user.click(virtualKeysTab);

      await waitFor(() => {
        expect(screen.getByRole("button", { name: "Filters" })).toBeInTheDocument();
      });
      expect(screen.getByRole("button", { name: "Columns" })).toBeInTheDocument();
      expect(screen.getByTestId("pagination-range")).toHaveTextContent("Showing 1-1 of 1");
      expect(screen.getByTestId("pagination-prev")).toBeInTheDocument();
      expect(screen.getByTestId("pagination-next")).toBeInTheDocument();
    });
  });

  describe("settings and editing", () => {
    it("saves the name, description, models and budget for a platform administrator", async () => {
      const user = userEvent.setup({ delay: null });
      vi.mocked(networking.teamInfoCall).mockResolvedValue(
        createMockTeamData({ description: "hello", max_budget: 10, models: ["gpt-4"] }),
      );
      vi.mocked(networking.teamUpdateCall).mockResolvedValue({ data: {}, team_id: "123" } as any);

      renderWithProviders(<TeamInfoView {...defaultProps} />);

      await user.click(await screen.findByRole("tab", { name: "Settings" }));
      await user.click(await screen.findByRole("button", { name: /edit settings/i }));
      const name = await screen.findByLabelText("Team Name");
      await user.clear(name);
      await user.type(name, "Renamed");
      await user.click(screen.getByRole("button", { name: /save changes/i }));

      await waitFor(() => {
        expect(networking.teamUpdateCall).toHaveBeenCalledWith(
          "test-token",
          expect.objectContaining({
            team_id: "123",
            team_alias: "Renamed",
            team_description: "hello",
            models: ["gpt-4"],
            max_budget: 10,
            blocked: false,
          }),
        );
      });
      const payload = vi.mocked(networking.teamUpdateCall).mock.calls[0][1] as Record<string, unknown>;
      expect(payload).not.toHaveProperty("metadata");
      expect(payload).not.toHaveProperty("guardrails");
    });

    it("does not send models or a budget when the caller is only a team admin", async () => {
      const user = userEvent.setup({ delay: null });
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());
      vi.mocked(networking.teamUpdateCall).mockResolvedValue({ data: {}, team_id: "123" } as any);

      renderWithProviders(<TeamInfoView {...defaultProps} is_proxy_admin={false} />);

      await user.click(await screen.findByRole("tab", { name: "Settings" }));
      await user.click(await screen.findByRole("button", { name: /edit settings/i }));
      await user.click(screen.getByRole("button", { name: /save changes/i }));

      await waitFor(() => expect(networking.teamUpdateCall).toHaveBeenCalled());
      const payload = vi.mocked(networking.teamUpdateCall).mock.calls[0][1] as Record<string, unknown>;
      expect(payload).toMatchObject({ team_id: "123", team_alias: "Test Team", team_description: "" });
      expect(payload).not.toHaveProperty("models");
      expect(payload).not.toHaveProperty("max_budget");
      expect(payload).not.toHaveProperty("blocked");
    });

    it("blocks the save on an empty team name", async () => {
      const user = userEvent.setup({ delay: null });
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());

      renderWithProviders(<TeamInfoView {...defaultProps} />);
      await user.click(await screen.findByRole("tab", { name: "Settings" }));
      await user.click(await screen.findByRole("button", { name: /edit settings/i }));
      await user.clear(screen.getByLabelText("Team Name"));
      await user.click(screen.getByRole("button", { name: /save changes/i }));

      expect(await screen.findByText(/Please input a team name|请输入团队名称/)).toBeInTheDocument();
      expect(networking.teamUpdateCall).not.toHaveBeenCalled();
    });

    /** 验证成员超配失败后保留编辑输入并能重试；前置模拟额度接口失败后成功，确认保存结果与列表刷新，测试框架清理 DOM 与 mock。 */
    it("preserves the member budget on failure and allows a corrected retry", async () => {
      const user = userEvent.setup({ delay: null });
      const onUpdate = vi.fn();
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());
      vi.mocked(networking.teamMemberUpdateCall)
        .mockRejectedValueOnce(new Error("Quota allocation exceeds available budget"))
        .mockResolvedValueOnce({} as any);

      renderWithProviders(<TeamInfoView {...defaultProps} onUpdate={onUpdate} />);
      await user.click(await screen.findByRole("tab", { name: "Members" }));
      await user.click(screen.getByRole("button", { name: "Edit Member" }));
      const dialog = await screen.findByRole("dialog");
      const budget = within(dialog).getByLabelText("Team Member Budget (USD)");
      await user.clear(budget);
      await user.type(budget, "601");
      await user.click(within(dialog).getByRole("button", { name: "Save Changes" }));
      await waitFor(() => expect(networking.teamMemberUpdateCall).toHaveBeenCalledTimes(1));
      await waitFor(() => expect(within(dialog).getByRole("button", { name: "Save Changes" })).toBeEnabled());
      expect(dialog).toBeVisible();
      expect(budget).toHaveValue(601);
      expect(onUpdate).not.toHaveBeenCalled();

      await user.clear(budget);
      await user.type(budget, "401");
      await user.click(within(dialog).getByRole("button", { name: "Save Changes" }));
      await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
      expect(networking.teamMemberUpdateCall).toHaveBeenLastCalledWith(
        "test-token", "123", expect.objectContaining({ user_id: "edit-user", max_budget_in_team: "401" }),
      );
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    });

    it("should add team member when form is submitted", async () => {
      const user = userEvent.setup({ delay: null });
      const onUpdate = vi.fn();
      vi.mocked(networking.teamInfoCall).mockResolvedValue(createMockTeamData());
      vi.mocked(networking.teamMemberAddCall).mockResolvedValue({} as any);

      renderWithProviders(<TeamInfoView {...defaultProps} onUpdate={onUpdate} />);

      await user.click(await screen.findByRole("tab", { name: "Members" }));
      await user.click(await screen.findByRole("button", { name: /add member/i }));
      await user.click(await screen.findByRole("button", { name: "Submit" }));

      await waitFor(() => {
        expect(networking.teamMemberAddCall).toHaveBeenCalled();
      });
    });
  });

});


describe("TeamInfo MCP permission retention", () => {
  beforeEach(seedDefaultMocks);

  afterEach(() => {
    vi.clearAllMocks();
  });

  const server = (serverId: string, serverName: string, alias?: string): MCPServer =>
    ({
      server_id: serverId,
      server_name: serverName,
      alias,
      mcp_access_groups: [],
      created_at: "",
      created_by: "",
      updated_at: "",
      updated_by: "",
    }) as MCPServer;

  const effective = (serverId: string, kind: EffectiveMcpServer["source"]["kind"]): EffectiveMcpServer =>
    ({
      server: server(serverId, serverId),
      permissionKey: serverId,
      supersededKeys: [],
      ambiguousKeys: [],
      keyedTools: undefined,
      toolsetTools: undefined,
      allowedTools: undefined,
      source: kind === "accessGroup" ? { kind, name: "ops_readonly" } : { kind },
    }) as EffectiveMcpServer;

  const UNIFIED_SERVER = server("unified-server", "wiki");
  const UNIFIED_GROUPS = [
    { access_group_id: "ag-1", access_group_name: "Group 1", access_mcp_server_ids: ["unified-server"] },
    { access_group_id: "ag-2", access_group_name: "Group 2", access_mcp_server_ids: [] },
  ];

  const unifiedTeam = (toolPermissions: Record<string, string[]>, serverIds: string[] = []) => {
    const teamData = {
      models: ["gpt-4"],
      access_group_ids: ["ag-1", "ag-2"],
      access_group_mcp_server_ids: serverIds,
      object_permission: {
        mcp_servers: [],
        mcp_access_groups: [],
        mcp_toolsets: [],
        mcp_tool_permissions: toolPermissions,
      },
    };
    return createMockTeamData(teamData);
  };

  const renderMcpEditor = async (
    user: ReturnType<typeof userEvent.setup>,
    {
      initialTeam = unifiedTeam({ wiki: ["read_page"] }, ["unified-server"]),
      freshTeam = initialTeam,
      accessGroups = [],
    }: {
      initialTeam?: ReturnType<typeof createMockTeamData>;
      freshTeam?: ReturnType<typeof createMockTeamData>;
      accessGroups?: typeof UNIFIED_GROUPS;
    } = {},
  ) => {
    mockUseMCPServers.mockReturnValue({ data: [UNIFIED_SERVER], isLoading: false, isError: false } as any);
    mockUseMCPToolsets.mockReturnValue({ data: [], isLoading: false, isError: false } as any);
    mockUseAccessGroups.mockReturnValue({ data: accessGroups, isLoading: false, isError: false } as any);
    vi.mocked(networking.teamInfoCall).mockResolvedValueOnce(initialTeam).mockResolvedValue(freshTeam);
    vi.mocked(networking.teamUpdateCall).mockResolvedValue({ data: {}, team_id: "123" } as any);

    renderWithProviders(
      <TeamInfoView
        teamId="123"
        onUpdate={vi.fn()}
        onClose={vi.fn()}
        accessToken="test-token"
        is_team_admin
        is_proxy_admin
        userModels={["gpt-4"]}
        editTeam={false}
      />,
    );
    await waitFor(() => expect(screen.queryAllByText("Test Team").length).toBeGreaterThan(0));
    await user.click(screen.getByRole("tab", { name: "Settings" }));
    await user.click(await screen.findByRole("button", { name: /edit settings/i }));
    await screen.findByLabelText("Team Name");
  };

  const saveMcpEditor = async (user: ReturnType<typeof userEvent.setup>) => {
    await user.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() => expect(networking.teamUpdateCall).toHaveBeenCalled());
    const calls = vi.mocked(networking.teamUpdateCall).mock.calls;
    const [, payload] = calls[calls.length - 1];
    return payload.object_permission.mcp_tool_permissions;
  };

  const refuseMcpSave = async (user: ReturnType<typeof userEvent.setup>, reason: RegExp) => {
    const errorToast = vi.spyOn(toast, "fromError").mockImplementation(() => {});
    await user.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() => expect(errorToast).toHaveBeenCalledWith(expect.stringMatching(reason)));
    expect(networking.teamUpdateCall).not.toHaveBeenCalled();
    errorToast.mockRestore();
  };

  const resolveGrants = (overrides: Partial<McpGrantInput>) => {
    const input: McpGrantInput = {
      effectiveServers: [effective("server-1", "direct")],
      selectedAccessGroupIds: ["ag-1"],
      accessGroups: [],
      standingServerIds: new Set(),
      loadTeamGroups: vi.fn(),
      ...overrides,
    };
    return grantedMcpServerIds(input);
  };

  it("retains permissions for directly and indirectly granted servers", async () => {
    const resolution = await resolveGrants({
      effectiveServers: [effective("direct", "direct"), effective("inherited", "toolPermission")],
      accessGroups: [{ access_group_id: "ag-1", access_mcp_server_ids: ["inherited"] }],
    });
    expect(resolution.kind).toBe("resolved");
    if (resolution.kind !== "resolved") return;

    expect(
      retainedMcpToolPermissions(
        { direct: ["create_issue"], inherited: ["list_issues"], removed: ["delete_repo"] },
        resolution.serverIds,
        [server("direct", "deploy_tracker"), server("inherited", "issue_tracker"), server("removed", "old_tracker")],
      ),
    ).toEqual({
      direct: ["create_issue"],
      inherited: ["list_issues"],
    });
  });

  it("matches name and alias permission keys to granted server ids", () => {
    const catalog = [server("server-1", "issue_tracker", "issues")];

    expect(
      retainedMcpToolPermissions(
        { issue_tracker: ["list_issues"], issues: ["create_issue"] },
        new Set(["server-1"]),
        catalog,
      ),
    ).toEqual({
      issue_tracker: ["list_issues"],
      issues: ["create_issue"],
    });
  });

  it("does not reload the team when the access group list covers the selection", async () => {
    const loadTeamGroups = vi.fn();
    await expect(
      resolveGrants({
        accessGroups: [{ access_group_id: "ag-1", access_mcp_server_ids: ["group-server"] }],
        loadTeamGroups,
      }),
    ).resolves.toEqual({
      kind: "resolved",
      serverIds: new Set(["server-1", "group-server"]),
    });
    expect(loadTeamGroups).not.toHaveBeenCalled();
  });

  it("falls back to the team's loaded access group servers when the list is unavailable and the selection is unchanged", async () => {
    expect(
      await resolveGrants({
        loadTeamGroups: vi.fn().mockResolvedValue({ ids: ["ag-1"], serverIds: ["group-server"] }),
      }),
    ).toEqual({
      kind: "resolved",
      serverIds: new Set(["server-1", "group-server"]),
    });
  });

  it("adds standing tool-permission grants to the reloaded team grants", async () => {
    expect(
      await resolveGrants({
        effectiveServers: [effective("server-1", "direct"), effective("standing", "toolPermission")],
        standingServerIds: new Set(["standing"]),
        loadTeamGroups: vi.fn().mockResolvedValue({ ids: ["ag-1"], serverIds: ["group-server"] }),
      }),
    ).toEqual({
      kind: "resolved",
      serverIds: new Set(["server-1", "group-server", "standing"]),
    });
  });

  it("is unresolvable when the list is unavailable and the selection changed", async () => {
    expect(
      await resolveGrants({
        loadTeamGroups: vi.fn().mockResolvedValue({ ids: ["ag-1", "ag-2"], serverIds: ["group-server"] }),
      }),
    ).toEqual({
      kind: "unresolvable",
      reason: expect.stringMatching(/access groups could not be loaded/),
    });
  });

  it("is unresolvable when the team reload fails", async () => {
    expect(await resolveGrants({ loadTeamGroups: vi.fn().mockRejectedValue(new Error("boom")) })).toEqual({
      kind: "unresolvable",
      reason: expect.stringMatching(/access groups could not be reloaded/),
    });
  });

  it("refuses an unresolved selected access group", async () => {
    expect(
      await resolveGrants({
        selectedAccessGroupIds: ["missing"],
        loadTeamGroups: vi.fn().mockResolvedValue({ ids: [], serverIds: [] }),
      }),
    ).toEqual({
      kind: "unresolvable",
      reason: expect.stringMatching(/access groups could not be loaded/),
    });
  });

  it("retains an unknown permission key so an inventory refresh cannot erase it", () => {
    expect(retainedMcpToolPermissions({ "not-yet-loaded": ["read"] }, new Set(), [])).toEqual({
      "not-yet-loaded": ["read"],
    });
  });

  it("splits an ambiguous permission across the matching granted server", () => {
    const catalog = [server("server-1", "shared"), server("server-2", "shared")];

    expect(
      retainedMcpToolPermissions({ shared: ["read"], "server-1": ["write"] }, new Set(["server-1"]), catalog),
    ).toEqual({
      "server-1": ["write", "read"],
    });
  });

  it("keeps an ambiguous permission when all matching servers remain granted", () => {
    const catalog = [server("server-1", "shared"), server("server-2", "shared")];

    expect(retainedMcpToolPermissions({ shared: ["read"] }, new Set(["server-1", "server-2"]), catalog)).toEqual({
      shared: ["read"],
    });
  });

});
