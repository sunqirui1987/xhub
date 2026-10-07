import { renderHook, screen, waitFor, renderWithProviders } from "../../../tests/test-utils";
import { prepareModelAddRequest } from "./handle_add_model_submit";
import userEvent, { PointerEventsCheckLevel } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Team } from "../key_team_helpers/key_list";
import type { CredentialItem } from "../networking";
import { Providers } from "../provider_info_helpers";
import { projectMountedValues, useMountRegistry, type MountedFormValues } from "../common_components/MountedFormField";
import { useForm } from "react-hook-form";
import AddModelForm from "./AddModelForm";

vi.mock("../molecules/models/ProviderLogo", () => ({
  ProviderLogo: ({ provider, className }: { provider: string; className?: string }) => (
    <div className={className} data-testid={`provider-logo-${provider}`}>
      {provider}
    </div>
  ),
}));

vi.mock("../networking", async () => {
  const actual = await vi.importActual("../networking");
  return {
    ...actual,
    getGuardrailsList: vi.fn().mockResolvedValue({
      guardrails: [{ guardrail_name: "test-guardrail-1" }, { guardrail_name: "test-guardrail-2" }],
    }),
    tagListCall: vi.fn().mockResolvedValue({}),
    modelAvailableCall: vi.fn().mockResolvedValue({
      data: [{ id: "model-group-1" }, { id: "model-group-2" }],
    }),
    modelHubCall: vi.fn().mockResolvedValue({
      data: [
        { model_group: "gpt-4", mode: "chat" },
        { model_group: "gpt-3.5-turbo", mode: "chat" },
      ],
    }),
    testConnectionRequest: vi.fn().mockResolvedValue({ status: "success" }),
    getProviderCreateMetadata: vi.fn().mockResolvedValue([
      {
        provider: "OpenAI",
        provider_display_name: "OpenAI",
        litellm_provider: "openai",
        default_model_placeholder: "gpt-3.5-turbo",
        credential_fields: [],
      },
    ]),
  };
});

vi.mock("@/app/(dashboard)/hooks/providers/useProviderFields", () => ({
  useProviderFields: vi.fn().mockReturnValue({
    data: [
      {
        provider: "OpenAI",
        provider_display_name: "OpenAI",
        litellm_provider: "openai",
        default_model_placeholder: "gpt-3.5-turbo",
        credential_fields: [],
      },
    ],
    isLoading: false,
    error: null,
  }),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/teams/useTeams", () => ({
  useInfiniteTeams: () => ({
    data: {
      pages: [
        {
          teams: [{ team_id: "team-1", team_alias: "Test Team", organization_id: "org-1" }],
          total: 1,
          page: 1,
          page_size: 20,
          total_pages: 1,
        },
      ],
    },
    fetchNextPage: vi.fn(),
    hasNextPage: false,
    isFetchingNextPage: false,
    isLoading: false,
  }),
}));

vi.mock("@/app/(dashboard)/hooks/guardrails/useGuardrails", () => ({
  useGuardrails: vi.fn().mockReturnValue({
    data: {
      guardrails: [{ guardrail_name: "test-guardrail" }],
      globalGuardrailNames: new Set<string>(),
      optionalGuardrailNames: new Set<string>(["test-guardrail"]),
    },
    isLoading: false,
    error: null,
  }),
}));

vi.mock("@/app/(dashboard)/hooks/tags/useTags", () => ({
  useTags: vi.fn().mockReturnValue({
    data: { tag1: ["model1", "model2"] },
    isLoading: false,
    error: null,
  }),
}));

const mockAuthorizedUser = (userRole: string, userId: string, premiumUser: boolean) => ({
  token: "test-token",
  accessToken: "test-access-token",
  userId,
  userEmail: "test@example.com",
  userRole,
  premiumUser,
  disabledPersonalKeyCreation: false,
  showSSOBanner: false,
});

const testTeam: Team = {
  team_id: "team-1",
  team_alias: "Test Team",
  models: ["gpt-4"],
  max_budget: 100,
  budget_duration: "monthly",
  tpm_limit: null,
  rpm_limit: null,
  organization_id: "org-1",
  created_at: "2024-01-01T00:00:00Z",
  keys: [],
  members_with_roles: [],
};

const createTestProps = (userRole = "proxy_admin", userId = "user-1", isTeamAdmin = false) => {
  const { result } = renderHook(() => {
    const form = useForm<MountedFormValues>({ mode: "onChange" });
    const registry = useMountRegistry();
    return { form, registry };
  });
  const { form, registry } = result.current;

  const teams = [
    {
      ...testTeam,
      members_with_roles: isTeamAdmin ? [{ user_id: userId, role: "admin" }] : [],
    },
  ];

  const credentials: CredentialItem[] = [
    {
      credential_name: "test-credential",
      credential_values: {},
      credential_info: {
        custom_llm_provider: "openai",
        description: "Test credential",
      },
    },
  ];

  return {
    form,
    registry,
    mountedValues: () => projectMountedValues(registry, form.getValues),
    handleOk: vi.fn().mockResolvedValue(true),
    setSelectedProvider: vi.fn(),
    setProviderModelsFn: vi.fn(),
    getPlaceholder: vi.fn((provider: string) => `Enter ${provider} model name`),
    setShowAdvancedSettings: vi.fn(),
    selectedProvider: Providers.OpenAI,
    providerModels: ["gpt-4", "gpt-3.5-turbo"],
    showAdvancedSettings: false,
    teams,
    credentials,
    userRole,
    userId,
  };
};

describe("AddModelForm", () => {
  // 这一条钉住这次拆分：转发方式和能力是两个独立的控件，而不是一个下拉。
  //
  // 旧的写法是一个 "Endpoint type" 下拉，值写进 model_info.mode，把「这条模型
  // 应答什么」和「网关怎么把请求送到上游」压成了一格。压在一起的结果是一条只答
  // 对话的模型和一个原样转发的端点无法同时表达。
  it("writes capabilities and the transport as two separate fields", async () => {
    const auth = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
    auth.default.mockReturnValue(mockAuthorizedUser("proxy_admin", "user-1", true));
    const props = createTestProps();
    const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never });
    renderWithProviders(<AddModelForm {...props} />);

    // 转发方式默认是协议适配。
    const transport = await screen.findByRole("combobox", { name: /Transport|转发方式/ });
    await user.click(transport);
    await user.click(await screen.findByRole("option", { name: /Protocol adaptation|协议适配/ }));
    expect(props.form.getValues("transport")).toBe("adapted");
    // 协议适配不写 endpoint 对象：上游地址由 (op, 供应商) 决定，这里没有要存的东西。
    expect(props.form.getValues("endpoint")).toBeUndefined();

    // 能力是多选。默认预选 chat，取消之后再把 chat 和 embedding 都选上。
    const chat = await screen.findByRole("checkbox", { name: /Chat/ });
    expect(chat).toBeChecked();
    expect(props.form.getValues("endpoint_types")).toEqual(["chat"]);
    const embedding = await screen.findByRole("checkbox", { name: /Embedding/ });
    await user.click(embedding);
    expect(props.form.getValues("endpoint_types")).toEqual(["chat", "embedding"]);
    // 第一条能力也写进 mode，给还在读这个字符串的旧界面用。
    expect(props.form.getValues("mode")).toBe("chat");

    // 提交体放在 model_info 上，不是 litellm_params。
    const deployments = await prepareModelAddRequest(
      {
        endpoint_types: props.form.getValues("endpoint_types"),
        transport: props.form.getValues("transport"),
        mode: props.form.getValues("mode"),
        model_mappings: [{ public_name: "test", litellm_model: "openai/test" }],
      },
      "token",
      null,
    );
    expect(deployments?.[0].modelInfoObj).toMatchObject({
      endpoint_types: ["chat", "embedding"],
      transport: "adapted",
      mode: "chat",
    });
    expect(deployments?.[0].litellmParamsObj).not.toHaveProperty("endpoint_types");
    expect(deployments?.[0].litellmParamsObj).not.toHaveProperty("transport");
  });

  it("should render", async () => {
    const mockUseAuthorized = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
    mockUseAuthorized.default.mockReturnValue(mockAuthorizedUser("proxy_admin", "user-1", true));

    const props = createTestProps();

    renderWithProviders(<AddModelForm {...props} />);

    expect(await screen.findByRole("heading", { name: "Add Model" })).toBeInTheDocument();
  });

  it("should show proxy admin only (not team admin) - should not see Select Team dropdown unless switch is toggled", async () => {
    const mockUseAuthorized = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
    mockUseAuthorized.default.mockReturnValue(mockAuthorizedUser("proxy_admin", "user-1", true));

    const props = createTestProps("proxy_admin", "user-1", false);

    renderWithProviders(<AddModelForm {...props} />);

    await screen.findByText("Provider");

    expect(screen.queryByText("Team Selection Required")).not.toBeInTheDocument();
    expect(screen.queryByText("Select Team")).not.toBeInTheDocument();

    const teamSwitch = screen.getByRole("switch");
    expect(teamSwitch).toBeInTheDocument();

    expect(screen.queryByText("Select Team")).not.toBeInTheDocument();

    await userEvent.click(teamSwitch);

    expect(await screen.findByText("Select Team")).toBeInTheDocument();
  });

  it("should show proxy admin who is also team admin - should not see Select Team dropdown unless switch is toggled", async () => {
    const mockUseAuthorized = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
    mockUseAuthorized.default.mockReturnValue(mockAuthorizedUser("proxy_admin", "user-1", true));

    const props = createTestProps("proxy_admin", "user-1", true);

    renderWithProviders(<AddModelForm {...props} />);

    await screen.findByText("Provider");

    expect(screen.queryByText("Team Selection Required")).not.toBeInTheDocument();
    expect(screen.queryByText("Select Team")).not.toBeInTheDocument();

    const teamSwitch = screen.getByRole("switch");
    expect(teamSwitch).toBeInTheDocument();

    expect(screen.queryByText("Select Team")).not.toBeInTheDocument();

    await userEvent.click(teamSwitch);

    expect(await screen.findByText("Select Team")).toBeInTheDocument();
  });

  it("should show team admin (not proxy admin) - should see alert and team select, must select team before seeing remaining fields", async () => {
    const mockUseAuthorized = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
    mockUseAuthorized.default.mockReturnValue(mockAuthorizedUser("team_member", "user-1", true));

    const props = createTestProps("team_member", "user-1", true);

    renderWithProviders(<AddModelForm {...props} />);

    await screen.findByRole("heading", { name: "Add Model" });

    expect(screen.getByText("Team Selection Required")).toBeInTheDocument();

    expect(screen.getByText("Select Team")).toBeInTheDocument();

    expect(screen.queryByText("Provider")).not.toBeInTheDocument();

    const teamSelect = screen.getByRole("combobox");
    await userEvent.click(teamSelect);
    await userEvent.click(screen.getByText("Test Team"));

    await waitFor(() => {
      expect(screen.getByText("Provider")).toBeInTheDocument();
    });
  });

  it("should show team admin (not proxy admin) - should not see team-BYOK switch", async () => {
    const mockUseAuthorized = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
    mockUseAuthorized.default.mockReturnValue(mockAuthorizedUser("team_member", "user-1", true));

    const props = createTestProps("team_member", "user-1", true);

    renderWithProviders(<AddModelForm {...props} />);

    await screen.findByText("Select Team");

    const teamSelect = screen.getByRole("combobox");
    await userEvent.click(teamSelect);
    await userEvent.click(screen.getByText("Test Team"));

    await waitFor(() => {
      expect(screen.getByText("Provider")).toBeInTheDocument();
    });

    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });

  it("should handle non-admin, non-team-admin users - should not see team selection or switch", async () => {
    const mockUseAuthorized = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
    mockUseAuthorized.default.mockReturnValue(mockAuthorizedUser("user", "user-1", false));

    const props = createTestProps("user", "user-1", false);

    renderWithProviders(<AddModelForm {...props} />);

    await screen.findByRole("heading", { name: "Add Model" });

    expect(screen.queryByText("Team Selection Required")).not.toBeInTheDocument();

    expect(screen.queryByText("Select Team")).not.toBeInTheDocument();

    expect(screen.queryByText("Provider")).not.toBeInTheDocument();

    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });

  it("should display the provider field and the Test Connect / Add Model buttons", async () => {
    const mockUseAuthorized = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
    mockUseAuthorized.default.mockReturnValue(mockAuthorizedUser("proxy_admin", "user-1", true));

    const props = createTestProps();

    renderWithProviders(<AddModelForm {...props} />);

    expect(await screen.findByText("Provider")).toBeInTheDocument();
    expect((await screen.findAllByRole("button", { name: "Test Connect" })).length).toBeGreaterThan(0);
    expect(await screen.findByRole("button", { name: "Add Model" })).toBeInTheDocument();
  });

  it("shows only the Close button in the connection test dialog footer", async () => {
    const mockUseAuthorized = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
    mockUseAuthorized.default.mockReturnValue(mockAuthorizedUser("proxy_admin", "user-1", true));

    renderWithProviders(<AddModelForm {...createTestProps()} />);

    await userEvent.click(await screen.findByTestId("test-connect-btn"));

    const dialog = await screen.findByRole("dialog");
    const footer = dialog.querySelector('[data-slot="dialog-footer"]');
    expect(footer).not.toBeNull();
    expect(footer!.textContent?.trim()).toBe("Close");
  });

  describe("the enterprise gate on the Team-BYOK switch", () => {
    const renderForm = async (premiumUser: boolean) => {
      const mockUseAuthorized = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
      mockUseAuthorized.default.mockReturnValue(mockAuthorizedUser("proxy_admin", "user-1", premiumUser));
      renderWithProviders(<AddModelForm {...createTestProps()} />);
      return screen.findByRole("switch", { name: "Team-BYOK Model" });
    };

    it("explains the gate on hover even though the switch it sits on is disabled", async () => {
      const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never });
      const teamOnlySwitch = await renderForm(false);
      expect(teamOnlySwitch).toHaveAttribute("aria-disabled", "true");

      await user.hover(teamOnlySwitch);

      expect(await screen.findByText(/enterprise-only feature/)).toBeInTheDocument();
    });

    it("says nothing on hover once the user is premium", async () => {
      const user = userEvent.setup();
      const teamOnlySwitch = await renderForm(true);
      expect(teamOnlySwitch).not.toHaveAttribute("aria-disabled", "true");

      await user.hover(teamOnlySwitch);

      expect(screen.queryByText(/enterprise-only feature/)).not.toBeInTheDocument();
    });
  });

  describe("cache control bindings reach the parent form store", () => {
    const renderWithForm = async () => {
      const mockUseAuthorized = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
      mockUseAuthorized.default.mockReturnValue(mockAuthorizedUser("proxy_admin", "user-1", true));
      const props = createTestProps();
      const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never });
      renderWithProviders(<AddModelForm {...props} />);
      await screen.findByText("Provider");

      return {
        user,
        openCacheControl: async () => {
          await user.click(await screen.findByText("Advanced Settings"));
          await user.click(screen.getByRole("switch", { name: "Cache Control Injection Points" }));
          await screen.findByText("Add Injection Point");
        },
        closeCacheControl: async () => {
          await user.click(screen.getByRole("switch", { name: "Cache Control Injection Points" }));
          await waitFor(() => expect(screen.queryByText("Add Injection Point")).not.toBeInTheDocument());
        },
        mountedValues: async (): Promise<Record<string, unknown>> => props.mountedValues(),
      };
    };

    it("omits both cache control keys while the section is untouched", async () => {
      const { mountedValues } = await renderWithForm();
      const values = await mountedValues();
      expect(values).not.toHaveProperty("cache_control_injection_points");
      expect(values.cache_control).toBeUndefined();
    });

    it("sends the seeded injection point once the toggle is on", async () => {
      const { openCacheControl, mountedValues } = await renderWithForm();
      await openCacheControl();
      const values = await mountedValues();
      expect(values.cache_control).toBe(true);
      expect(values.cache_control_injection_points).toEqual([{ location: "message" }]);
    });

    it("carries an edited role and keeps the index a string, as the antd control did", async () => {
      const { user, openCacheControl, mountedValues } = await renderWithForm();
      await openCacheControl();

      await user.click(screen.getByText("Select a role"));
      await user.click(await screen.findByText("System"));
      await user.type(screen.getByPlaceholderText("Optional"), "3");

      const values = await mountedValues();
      expect(values.cache_control_injection_points).toEqual([{ location: "message", role: "system", index: "3" }]);
    });

    it("adds a second injection point row", async () => {
      const { user, openCacheControl, mountedValues } = await renderWithForm();
      await openCacheControl();

      await user.click(screen.getByText("Add Injection Point"));
      await waitFor(() => expect(screen.getAllByPlaceholderText("Optional")).toHaveLength(2));
      await user.type(screen.getAllByPlaceholderText("Optional")[1], "7");

      const values = await mountedValues();
      expect(values.cache_control_injection_points).toEqual([
        { location: "message" },
        { location: "message", index: "7" },
      ]);
    });

    it("drops the injection points again when the toggle goes back off", async () => {
      const { openCacheControl, closeCacheControl, mountedValues } = await renderWithForm();
      await openCacheControl();
      await closeCacheControl();

      const values = await mountedValues();
      expect(values.cache_control).toBe(false);
      expect(values).not.toHaveProperty("cache_control_injection_points");
    });
  });

  it("lists saved providers and submits the chosen credential name", async () => {
    const mockUseAuthorized = vi.mocked(await import("@/app/(dashboard)/hooks/useAuthorized"));
    mockUseAuthorized.default.mockReturnValue(mockAuthorizedUser("proxy_admin", "user-1", true));
    const user = userEvent.setup();
    const props = createTestProps();
    props.credentials = [
      {
        credential_name: "openai-prod",
        credential_values: {},
        credential_info: { custom_llm_provider: "openai", description: "OpenAI" },
      },
      {
        credential_name: "azure-prod",
        credential_values: {},
        credential_info: { custom_llm_provider: "azure", description: "Azure" },
      },
    ];
    const sent: string[] = [];
    props.handleOk = vi.fn(async () => {
      const deployments = await prepareModelAddRequest(
        {
          ...props.mountedValues(),
          model_mappings: [{ public_name: "my-model", litellm_model: "openai/gpt-4" }],
          model: ["gpt-4"],
          custom_llm_provider: "OpenAI",
        },
        "token",
        {},
      );
      sent.push(...(deployments ?? []).map((deployment) => String(deployment.litellmParamsObj.litellm_credential_name)));
      return true;
    });

    renderWithProviders(<AddModelForm {...props} />);
    const picker = await screen.findByRole("combobox", { name: "Saved provider" });
    await user.click(picker);
    expect(await screen.findByRole("option", { name: "openai-prod" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "azure-prod" })).toBeInTheDocument();
    await user.click(screen.getByRole("option", { name: "azure-prod" }));
    await user.click(screen.getByTestId("add-model-btn"));

    await waitFor(() => expect(sent).toEqual(["azure-prod"]));
  });
});
