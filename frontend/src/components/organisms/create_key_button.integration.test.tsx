import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders, screen, testQueryClient, waitFor } from "../../../tests/test-utils";
import type { Team } from "../key_team_helpers/key_list";
import { keyCreateCall, keyCreateServiceAccountCall, modelAvailableCall } from "../networking";
import { toast } from "@/lib/toast";
import CreateKey from "./create_key_button";

const state = vi.hoisted(() => ({
  authorized: {
    accessToken: "test-token",
    userId: "test-user-id",
    userRole: "Admin",
    premiumUser: false,
  },
  can: {} as Record<string, boolean>,
  uiSettings: {} as Record<string, unknown>,
  tags: {} as Record<string, { name: string }>,
  teams: [] as { team_id: string; team_alias: string; models: string[]; organization_id?: string }[],
  organizations: [] as { organization_id: string; organization_alias: string }[],
  accessGroups: [] as { access_group_id: string; access_group_name: string }[],
  projects: [] as { project_id: string; project_alias: string; team_id?: string; models?: string[] }[],
}));

vi.mock("@/lib/toast", () => ({
  toast: {
    success: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
    fromError: vi.fn(),
    dismiss: vi.fn(),
  },
}));
vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: () => state.authorized }));
vi.mock("@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity", () => ({
  useSessionIdentity: () => ({
    data: {
      teams: state.teams.map((team) => ({ team_id: team.team_id, role: "member" as const })),
    },
    isFetched: true,
  }),
}));
vi.mock("@/app/(dashboard)/hooks/useCan", () => ({
  default: (capability: string) => state.can[capability] ?? true,
}));
vi.mock("@/app/(dashboard)/hooks/organizations/useOrganizations", () => ({
  useOrganizations: () => ({ data: state.organizations, isLoading: false }),
}));
vi.mock("@/app/(dashboard)/hooks/projects/useProjects", () => ({
  useProjects: () => ({ data: state.projects, isLoading: false }),
}));
vi.mock("@/app/(dashboard)/hooks/uiSettings/useUISettings", () => ({
  useUISettings: () => ({ data: { values: state.uiSettings } }),
}));
vi.mock("@/app/(dashboard)/hooks/tags/useTags", () => ({
  useTags: () => ({ data: state.tags, isLoading: false }),
}));
vi.mock("@/app/(dashboard)/hooks/teams/useTeams", () => ({
  useInfiniteTeams: () => ({
    data: { pages: [{ teams: state.teams, total: state.teams.length, page: 1, page_size: 50, total_pages: 1 }] },
    fetchNextPage: vi.fn(),
    hasNextPage: false,
    isFetchingNextPage: false,
    isLoading: false,
  }),
}));
vi.mock("@/app/(dashboard)/hooks/accessGroups/useAccessGroups", () => ({
  useAccessGroups: () => ({ data: state.accessGroups, isLoading: false, isError: false }),
}));
vi.mock("@/app/(dashboard)/hooks/mcpServers/useMCPServers", () => ({
  useMCPServers: () => ({ data: [], isLoading: false }),
}));
vi.mock("@/app/(dashboard)/hooks/mcpServers/useMCPAccessGroups", () => ({
  useMCPAccessGroups: () => ({ data: [], isLoading: false }),
}));
vi.mock("@/app/(dashboard)/hooks/mcpServers/useMCPToolsets", () => ({
  useMCPToolsets: () => ({ data: [], isLoading: false }),
}));

vi.mock("../networking", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../networking")>();
  const emptyMcpTools = { tools: [], error: null, message: null, stack_trace: null };
  return {
    ...actual,
    keyCreateCall: vi.fn().mockResolvedValue({ key: "sk-created", soft_budget: null }),
    keyCreateServiceAccountCall: vi.fn().mockResolvedValue({ key: "sk-service-account", soft_budget: null }),
    modelAvailableCall: vi.fn().mockResolvedValue({ data: [{ id: "gpt-4" }] }),
    getGuardrailsList: vi.fn().mockResolvedValue({ guardrails: [] }),
    getPoliciesList: vi.fn().mockResolvedValue({ policies: [] }),
    getPromptsList: vi.fn().mockResolvedValue({ prompts: [] }),
    getPossibleUserRoles: vi.fn().mockResolvedValue({}),

    getAgentsList: vi.fn().mockResolvedValue({ agents: [] }),
    getClaudeCodePluginsList: vi.fn().mockResolvedValue({
      plugins: [
        { name: "public-skill", enabled: true },
        { name: "private-skill", enabled: false },
      ],
      count: 2,
    }),
    getPassThroughEndpointsCall: vi.fn().mockResolvedValue({ endpoints: [] }),
    vectorStoreListCall: vi.fn().mockResolvedValue({ data: [] }),
    listMCPTools: vi.fn().mockResolvedValue(emptyMcpTools),
    getRouterSettingsCall: vi.fn().mockResolvedValue({ router_settings: {} }),
  };
});

const OPENAPI_SCHEMA = {
  components: {
    schemas: {
      GenerateKeyRequest: {
        properties: {
          key: { type: "string", title: "Key" },
          soft_budget: { type: "number", title: "Soft Budget" },
          blocked: { type: "boolean", title: "Blocked" },
          max_budget: { type: "number", title: "Max Budget" },
        },
      },
    },
  },
};

const SECTIONS = {
  mcp: /MCP Settings/i,
  agent: /Agent Settings/i,
  skill: /Skill Settings/i,
  logging: /Logging Settings/i,
  router: /Router Settings/i,
  aliases: /Model Aliases/i,
  lifecycle: /Key Lifecycle/i,
  advanced: /Advanced Settings/i,
} as const;

const ALL_CLOSED_PAYLOAD = {
  team_id: null,
  key_alias: "contract-key",
  models: [],
  key_type: "llm_api",
  user_id: "test-user-id",
  duration: null,
  metadata: "{}",
};

const OPTIONAL_OPEN_PAYLOAD = {
  ...ALL_CLOSED_PAYLOAD,
  max_budget: undefined,
  budget_duration: undefined,
  tpm_limit: undefined,
  tpm_limit_type: null,
  rpm_limit: undefined,
  rpm_limit_type: null,
  throttle_on_budget_exceeded: undefined,
  enable_prompt_caching: undefined,
  guardrails: undefined,
  access_group_ids: undefined,
  allowed_passthrough_routes: undefined,
  tags: undefined,
};

const ROUTER_SETTINGS_DEFAULT = {
  routing_strategy: null,
  allowed_fails: null,
  cooldown_time: null,
  num_retries: null,
  timeout: null,
  retry_after: null,
  fallbacks: null,
  context_window_fallbacks: null,
  retry_policy: null,
  model_group_alias: null,
  enable_tag_filtering: false,
  routing_strategy_args: null,
};

const SECTION_PAYLOAD_ADDITIONS: Record<keyof typeof SECTIONS, Record<string, unknown>> = {
  mcp: { allowed_mcp_servers_and_groups: { servers: [], accessGroups: [] } },
  agent: { allowed_agents_and_groups: undefined },
  skill: {},
  logging: {},
  router: { router_settings: ROUTER_SETTINGS_DEFAULT },
  aliases: {},
  lifecycle: {},
  advanced: { key: undefined, soft_budget: undefined, blocked: undefined },
};

const ALL_OPEN_PAYLOAD = {
  ...OPTIONAL_OPEN_PAYLOAD,
  ...SECTION_PAYLOAD_ADDITIONS.mcp,
  ...SECTION_PAYLOAD_ADDITIONS.agent,
  ...SECTION_PAYLOAD_ADDITIONS.router,
  ...SECTION_PAYLOAD_ADDITIONS.advanced,
};

const renderCreateKey = (props: Partial<React.ComponentProps<typeof CreateKey>> = {}) =>
  renderWithProviders(<CreateKey team={null} teams={[]} data={[]} addKey={vi.fn()} {...props} />);

const openModal = async (props: Partial<React.ComponentProps<typeof CreateKey>> = {}) => {
  const view = renderCreateKey(props);
  await userEvent.click(screen.getByTestId("create-key-button"));
  await screen.findByRole("button", { name: /^create key$/i });
  return view;
};

const openSection = async (name: RegExp) => {
  await userEvent.click(await screen.findByRole("button", { name }));
};

const nameTheKey = async (alias = "contract-key") => {
  await userEvent.type(await screen.findByLabelText(/Key Name/), alias);
};

const submit = async () => {
  await userEvent.click(screen.getByRole("button", { name: /^create key$/i }));
};

const createdPayload = async () => {
  await waitFor(() => {
    expect(vi.mocked(keyCreateCall)).toHaveBeenCalled();
  });
  return vi.mocked(keyCreateCall).mock.calls[0][2] as Record<string, unknown>;
};

describe("CreateKey", () => {
  beforeEach(() => {
    testQueryClient.clear();
    state.authorized = { accessToken: "test-token", userId: "test-user-id", userRole: "Admin", premiumUser: false };
    state.can = {};
    state.uiSettings = {};
    state.tags = {};
    state.teams = [];
    state.organizations = [];
    state.projects = [];
    state.accessGroups = [];
    vi.mocked(keyCreateCall).mockClear().mockResolvedValue({ key: "sk-created", soft_budget: null });
    vi.mocked(keyCreateServiceAccountCall)
      .mockClear()
      .mockResolvedValue({ key: "sk-service-account", soft_budget: null });
    vi.mocked(toast.fromError).mockClear();
    vi.mocked(modelAvailableCall)
      .mockClear()
      .mockResolvedValue({ data: [{ id: "gpt-4" }] });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("openapi.json")) {
          return { ok: true, status: 200, json: async () => OPENAPI_SCHEMA } as unknown as Response;
        }
        return { ok: true, status: 200, json: async () => ({}) } as unknown as Response;
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe("submit payload contract", () => {
    it("sends only the always-mounted fields when every collapsible section is closed", async () => {
      await openModal();
      await nameTheKey();
      await submit();

      expect(await createdPayload()).toStrictEqual(ALL_CLOSED_PAYLOAD);
    });

    it("registers the Optional Settings fields as undefined-valued keys once that section is open", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await submit();

      expect(await createdPayload()).toStrictEqual(OPTIONAL_OPEN_PAYLOAD);
    });

    it("sends the full mounted field set when every section is open", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      for (const trigger of Object.values(SECTIONS)) {
        await openSection(trigger);
      }
      await screen.findByLabelText("Soft Budget");
      await submit();

      expect(await createdPayload()).toStrictEqual(ALL_OPEN_PAYLOAD);
    });

    it.each(Object.keys(SECTIONS) as (keyof typeof SECTIONS)[])(
      "adds exactly the %s section's own keys when it is the only nested section open",
      async (section) => {
        await openModal();
        await nameTheKey();
        await openSection(/Optional Settings/i);
        await openSection(SECTIONS[section]);
        if (section === "advanced") {
          await screen.findByLabelText("Soft Budget");
        }
        await submit();

        expect(await createdPayload()).toStrictEqual({
          ...OPTIONAL_OPEN_PAYLOAD,
          ...SECTION_PAYLOAD_ADDITIONS[section],
        });
      },
    );

    it.each([
      [
        "every section closed",
        false,
        ["team_id", "key_alias", "models", "key_type", "user_id", "duration", "metadata"],
      ],
      [
        "Optional Settings open",
        true,
        [
          "team_id",
          "key_alias",
          "models",
          "key_type",
          "tpm_limit_type",
          "rpm_limit_type",
          "user_id",
          "duration",
          "metadata",
        ],
      ],
    ])("serialises to exactly the wire keys with %s", async (_label, openOptional, wireKeys) => {
      await openModal();
      await nameTheKey();
      if (openOptional) {
        await openSection(/Optional Settings/i);
      }
      await submit();

      const serialised = JSON.parse(JSON.stringify(await createdPayload())) as Record<string, unknown>;
      expect(Object.keys(serialised).sort()).toStrictEqual([...wireKeys].sort());
    });

    it("moves a picked private skill under object_permission.skills and off the top level", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await openSection(SECTIONS.skill);
      await userEvent.click(await screen.findByRole("combobox", { name: "Select skills (optional)" }));
      await userEvent.click(await screen.findByRole("option", { name: "private-skill (private)" }));
      await userEvent.keyboard("{Escape}");
      await submit();

      const payload = await createdPayload();
      expect(payload.object_permission).toStrictEqual({ skills: ["private-skill"] });
      expect(payload).not.toHaveProperty("allowed_skills");
    });

    it("omits a budget typed into a section the user closed again, rather than sending it as null", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.type(await screen.findByLabelText(/Max Budget \(USD\)/), "150.75");
      await openSection(/Optional Settings/i);
      await submit();

      const payload = await createdPayload();
      expect(payload).not.toHaveProperty("max_budget");
      expect(payload).toStrictEqual(ALL_CLOSED_PAYLOAD);
    });

    it("restores the typed budget when the closed section is expanded again before submit", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.type(await screen.findByLabelText(/Max Budget \(USD\)/), "150.75");
      await openSection(/Optional Settings/i);
      await openSection(/Optional Settings/i);

      expect(await screen.findByLabelText(/Max Budget \(USD\)/)).toHaveValue(150.75);

      await submit();

      expect(await createdPayload()).toStrictEqual({ ...OPTIONAL_OPEN_PAYLOAD, max_budget: "150.75" });
    });

    it("sends a typed max budget as a string, not a number", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.type(await screen.findByLabelText(/Max Budget \(USD\)/), "150.75");
      await submit();

      const payload = await createdPayload();
      expect(payload.max_budget).toBe("150.75");
    });

    it.each([
      ["Tokens per minute Limit (TPM)", "tpm_limit"],
      ["Requests per minute Limit (RPM)", "rpm_limit"],
    ])("routes a typed %s into the %s payload key", async (label, key) => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.type(await screen.findByLabelText(label), "42");
      await submit();

      expect((await createdPayload())[key]).toBe("42");
    });

    it("routes the shared rate-limit-type control into its own payload key", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.click(await screen.findByLabelText(/TPM Rate Limit Type/));
      await userEvent.click(await screen.findByRole("option", { name: /Guaranteed throughput/ }));
      await submit();

      const payload = await createdPayload();
      expect(payload.tpm_limit_type).toBe("guaranteed_throughput");
      expect(payload.rpm_limit_type).toBeNull();
    });

    it("routes a typed expiry into duration, which is otherwise coalesced to null", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await openSection(SECTIONS.lifecycle);
      await userEvent.type(await screen.findByLabelText("Expire Key"), "45d");
      await submit();

      expect((await createdPayload()).duration).toBe("45d");
    });

    it("drops a typed expiry back to null when its section is collapsed before submit", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await openSection(SECTIONS.lifecycle);
      await userEvent.type(await screen.findByLabelText("Expire Key"), "45d");
      await openSection(SECTIONS.lifecycle);
      await submit();

      expect((await createdPayload()).duration).toBeNull();
    });

    it("routes a selected budget reset window into budget_duration", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.click(await screen.findByLabelText("Reset Budget"));
      await userEvent.click(await screen.findByRole("option", { name: "daily" }));
      await submit();

      expect((await createdPayload()).budget_duration).toBe("24h");
    });

    it("sends an explicit null budget_duration when the never-resets window is chosen", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.click(await screen.findByLabelText("Reset Budget"));
      await userEvent.click(await screen.findByRole("option", { name: /never resets/i }));
      await submit();

      const payload = await createdPayload();
      expect("budget_duration" in payload).toBe(true);
      expect(payload.budget_duration).toBeNull();
    });

    it("routes typed tags into the tags key", async () => {
      state.tags = { production: { name: "production" } };
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.type(await screen.findByLabelText("Tags"), "production{Enter}");
      await submit();

      expect((await createdPayload()).tags).toStrictEqual(["production"]);
    });

    it("routes a chosen access group into access_group_ids", async () => {
      state.accessGroups = [{ access_group_id: "ag-1", access_group_name: "Group One" }];
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);

      await userEvent.click(await screen.findByLabelText("Select access groups (optional)"));
      await userEvent.click(await screen.findByRole("option", { name: /Group One/ }));
      await userEvent.keyboard("{Escape}");
      await submit();

      expect((await createdPayload()).access_group_ids).toStrictEqual(["ag-1"]);
    });

    it("moves the schema-driven Advanced Settings fields onto the payload under their own keys", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await openSection(SECTIONS.advanced);
      await userEvent.type(await screen.findByLabelText("Soft Budget"), "12");
      await submit();

      const payload = await createdPayload();
      expect(payload.soft_budget).toBe(12);
      expect(payload).toHaveProperty("key");
    });

    it("drops the schema-driven custom key field when the proxy disables custom API keys", async () => {
      state.uiSettings = { disable_custom_api_keys: true };
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await openSection(SECTIONS.advanced);
      await screen.findByLabelText("Soft Budget");
      await submit();

      const payload = await createdPayload();
      expect(payload).not.toHaveProperty("key");
      expect(payload).toHaveProperty("soft_budget");
    });

    it("drops the policy and prompt keys entirely for a role that cannot see those fields", async () => {
      state.can = { viewPolicies: false, viewPrompts: false };
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await submit();

      const payload = await createdPayload();
      expect(payload).not.toHaveProperty("policies");
      expect(payload).not.toHaveProperty("prompts");
      expect(payload).toHaveProperty("guardrails");
    });

    it("does not ask for a project when creating a key", async () => {
      state.uiSettings = { enable_projects_ui: true };
      await openModal();
      await nameTheKey();
      await submit();

      expect(await createdPayload()).not.toHaveProperty("project_id");
      expect(screen.queryByLabelText("Project")).not.toBeInTheDocument();
    });

    it("keeps disable_global_guardrails out of the payload while the switch is off", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await submit();

      expect(await createdPayload()).not.toHaveProperty("disable_global_guardrails");
    });

    it("sends disable_global_guardrails once the switch is on", async () => {
      state.authorized = { ...state.authorized, premiumUser: true };
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.click(await screen.findByRole("switch", { name: /Disable Global Guardrails/ }));
      await submit();

      expect((await createdPayload()).disable_global_guardrails).toBe(true);
    });

    it("folds a metadata JSON string back through JSON.stringify", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.type(await screen.findByLabelText("Metadata"), '{{"team":"research"}');
      await submit();

      expect((await createdPayload()).metadata).toBe('{"team":"research"}');
    });

    it("carries the typed key alias and the user's team onto the wire", async () => {
      state.teams = [{ team_id: "team-1", team_alias: "Team One", models: [] }];
      await openModal({ teams: state.teams as unknown as Team[] });
      await nameTheKey("wire-alias");
      await submit();

      expect(await createdPayload()).toMatchObject({ key_alias: "wire-alias", team_id: "team-1" });
    });

    it("sends team_id as an explicit null when no team is chosen", async () => {
      await openModal();
      await nameTheKey();
      await submit();

      expect(await createdPayload()).toHaveProperty("team_id", null);
    });

    it.fails(
      "adds no keys for an Optional Settings section the user opened but never filled (expected to fail until the forms revamp, tri-state PATCH tracker)",
      async () => {
        await openModal();
        await nameTheKey();
        await openSection(/Optional Settings/i);
        await submit();

        expect(await createdPayload()).toStrictEqual(ALL_CLOSED_PAYLOAD);
      },
    );
  });

  describe("key ownership", () => {
    it("stamps the signed-in user onto user_id and does not ask who owns the key", async () => {
      await openModal();
      await nameTheKey();

      expect(screen.queryByText("Key Ownership")).not.toBeInTheDocument();
      expect(screen.queryByRole("radio", { name: "You" })).not.toBeInTheDocument();
      expect(screen.queryByRole("radio", { name: "Service Account" })).not.toBeInTheDocument();
      expect(screen.queryByLabelText("Organization")).not.toBeInTheDocument();
      expect(screen.queryByPlaceholderText("Search or select a team")).not.toBeInTheDocument();
      await submit();

      expect((await createdPayload()).user_id).toBe("test-user-id");
    });
  });

  describe("required field validation", () => {
    it("blocks the submit and marks the alias invalid when it is blank", async () => {
      await openModal();
      await submit();

      await waitFor(() => {
        expect(screen.getByLabelText(/Key Name/)).toHaveAttribute("aria-invalid", "true");
      });
      expect(vi.mocked(keyCreateCall)).not.toHaveBeenCalled();
    });

    it("suppresses the required message behind the always-visible help text", async () => {
      await openModal();
      await submit();

      await waitFor(() => {
        expect(screen.getByLabelText(/Key Name/)).toHaveAttribute("aria-invalid", "true");
      });
      expect(screen.queryByText("Please input a key name")).not.toBeInTheDocument();
      expect(screen.getByText("required")).toBeInTheDocument();
    });

    it("blocks the submit when the budget exceeds the team ceiling", async () => {
      await openModal({ team: { team_id: "team-1", max_budget: 10 } as unknown as Team });
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.type(await screen.findByLabelText(/Max Budget \(USD\)/), "50");
      await submit();

      await waitFor(() => {
        expect(screen.getByLabelText(/Max Budget \(USD\)/)).toHaveAttribute("aria-invalid", "true");
      });
      expect(vi.mocked(keyCreateCall)).not.toHaveBeenCalled();
    });
  });

  describe("deep link prefill", () => {
    it("prefills the key alias", async () => {
      renderCreateKey({ autoOpenCreate: true, prefillData: { key_alias: "prefilled-key" } });

      expect(await screen.findByLabelText(/Key Name/)).toHaveValue("prefilled-key");
    });

    it("prefills models once the available model list arrives", async () => {
      renderCreateKey({ autoOpenCreate: true, prefillData: { models: ["gpt-4"] } });

      expect(await screen.findByLabelText("gpt-4", {}, { timeout: 5000 })).toBeInTheDocument();
    });

    it("ignores a team the user has no access to and keeps their own team", async () => {
      state.teams = [{ team_id: "team-1", team_alias: "Team One", models: [] }];
      renderCreateKey({
        teams: state.teams as unknown as Team[],
        autoOpenCreate: true,
        prefillData: { team_id: "team-404", key_alias: "example-key" },
      });

      await userEvent.type(await screen.findByLabelText(/Key Name/), "-suffix");
      await submit();

      const payload = await createdPayload();
      expect(payload.team_id).toBe("team-1");
      expect(payload.key_alias).toBe("example-key-suffix");
    });

    it("prefills the key type", async () => {
      renderCreateKey({ autoOpenCreate: true, prefillData: { key_type: "management" } });

      await screen.findByLabelText(/Key Name/);
      await userEvent.type(await screen.findByLabelText(/Key Name/), "prefilled-type");
      await submit();

      expect((await createdPayload()).key_type).toBe("management");
    });
  });

  describe("models dropdown team gating", () => {
    it("offers all-proxy-models but not all-team-models when no team is selected", async () => {
      await openModal();
      await userEvent.click(await screen.findByLabelText("Models"));

      expect(await screen.findByRole("option", { name: "All Proxy Models" })).toBeInTheDocument();
      expect(screen.queryByRole("option", { name: "All Team Models" })).not.toBeInTheDocument();
    });

    it("offers all-team-models but hides all-proxy-models for the user's team", async () => {
      state.teams = [{ team_id: "team-1", team_alias: "Team One", models: ["team-model-1"] }];
      await openModal({ teams: state.teams as unknown as Team[] });

      await userEvent.click(await screen.findByLabelText("Models"));

      expect(await screen.findByRole("option", { name: "All Team Models" })).toBeInTheDocument();
      expect(screen.queryByRole("option", { name: "All Proxy Models" })).not.toBeInTheDocument();
    });
  });

  describe("created key display", () => {
    it("surfaces the generated key after a successful create", async () => {
      await openModal();
      await nameTheKey();
      await submit();
      await createdPayload();

      expect(await screen.findByText("Save your Key")).toBeInTheDocument();
    });

    it("rejects a duplicate alias within the same team without calling the API", async () => {
      await openModal({ data: [{ team_id: null, key_alias: "contract-key" }] });
      await nameTheKey();
      await submit();

      await waitFor(() => {
        expect(screen.getByRole("button", { name: /^create key$/i })).toBeInTheDocument();
      });
      expect(vi.mocked(keyCreateCall)).not.toHaveBeenCalled();
    });
  });

  describe("dialog accessible names", () => {
    it("names the create form dialog", async () => {
      await openModal();

      expect(screen.getByRole("dialog", { name: "Create New Key" })).toBeInTheDocument();
    });

    it("names the created key dialog", async () => {
      await openModal();
      await nameTheKey();
      await submit();
      await createdPayload();

      expect(await screen.findByRole("dialog", { name: "Save your Key" })).toBeInTheDocument();
    });
  });

  describe("key type gating", () => {
    it("labels the llm_api option AI APIs", async () => {
      await openModal();
      await userEvent.click(await screen.findByLabelText("Key Type"));

      expect(
        await screen.findByText("Can call only AI API routes (chat/completions, embeddings, etc.)"),
      ).toBeInTheDocument();
      expect(screen.queryByText("LLM API")).not.toBeInTheDocument();
    });

    it("clears and disables models when a management key type is chosen", async () => {
      await openModal();
      await nameTheKey();

      await userEvent.click(await screen.findByLabelText("Key Type"));
      await userEvent.click(await screen.findByRole("option", { name: /^Management/ }));
      await submit();

      const payload = await createdPayload();
      expect(payload.key_type).toBe("management");
      expect(payload.models).toStrictEqual([]);
    });
  });

  describe("mount-gate liveness", () => {
    it("keeps the MCP tool permissions key out of the payload even with its section open", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await openSection(SECTIONS.mcp);
      await submit();

      const payload = await createdPayload();
      expect(payload).not.toHaveProperty("mcp_tool_permissions");
      expect(payload).toHaveProperty("allowed_mcp_servers_and_groups");
    });

    it("registers no Optional Settings field while a team choice is still required", async () => {
      vi.mocked(modelAvailableCall).mockResolvedValue({ data: [{ id: "no-default-models" }] });
      renderCreateKey();
      await userEvent.click(screen.getByTestId("create-key-button"));

      expect(await screen.findByText(/Please select a team to continue/)).toBeInTheDocument();
      expect(screen.queryByLabelText(/Key Name/)).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /Optional Settings/i })).not.toBeInTheDocument();
    });
  });


  describe("validation follows the mounted set", () => {
    it("submits an over-ceiling budget typed into a section the user closed again, omitting the key", async () => {
      await openModal({ team: { team_id: "team-1", max_budget: 10 } as unknown as Team });
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.type(await screen.findByLabelText(/Max Budget \(USD\)/), "50");
      await openSection(/Optional Settings/i);
      await submit();

      const payload = await createdPayload();
      expect(payload).not.toHaveProperty("max_budget");
      expect(payload.key_alias).toBe("contract-key");
    });
  });

  describe("submit gestures", () => {
    it("creates the key when Enter is pressed inside a text field", async () => {
      await openModal();
      await userEvent.type(await screen.findByLabelText(/Key Name/), "enter-key{Enter}");

      expect((await createdPayload()).key_alias).toBe("enter-key");
    });
  });

  describe("switch coercion", () => {
    it("sends enable_prompt_caching as a boolean once the switch is on", async () => {
      await openModal();
      await nameTheKey();
      await openSection(/Optional Settings/i);
      await userEvent.click(await screen.findByRole("switch", { name: /Enable Prompt Caching/ }));
      await submit();

      expect((await createdPayload()).enable_prompt_caching).toBe(true);
    });
  });
});
