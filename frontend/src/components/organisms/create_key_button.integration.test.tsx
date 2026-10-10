import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders, screen, testQueryClient, waitFor } from "../../../tests/test-utils";
import type { Team } from "../key_team_helpers/key_list";
import { getRouteTemplatesCall, keyCreateCall, modelAvailableCall } from "../networking";
import { toast } from "@/lib/toast";
import CreateKey from "./create_key_button";

const state = vi.hoisted(() => ({
  authorized: { accessToken: "test-token", userId: "test-user-id", userRole: "Admin", premiumUser: false },
  teams: [] as Team[],
}));

vi.mock("@/lib/toast", () => ({
  toast: {
    success: vi.fn(), info: vi.fn(), warning: vi.fn(), error: vi.fn(), fromError: vi.fn(), dismiss: vi.fn(),
  },
}));
vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: () => state.authorized }));
vi.mock("@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity", () => ({
  useSessionIdentity: () => ({
    data: { teams: state.teams.map((team) => ({ team_id: team.team_id, role: "member" as const })) },
    isFetched: true,
  }),
}));
vi.mock("../networking", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../networking")>();
  return {
    ...actual,
    keyCreateCall: vi.fn().mockResolvedValue({ key: "sk-created", soft_budget: null }),
    keyCreateServiceAccountCall: vi.fn().mockResolvedValue({ key: "sk-service-account", soft_budget: null }),
    modelAvailableCall: vi.fn().mockResolvedValue({ data: [{ id: "gpt-4" }] }),
    getRouteTemplatesCall: vi.fn().mockResolvedValue([
      { id: "tpl-low-cost", name: "Low-cost route", body: {} },
    ]),
  };
});

const DEFAULT_PAYLOAD = {
  team_id: null,
  key_alias: "contract-key",
  models: [],
  key_type: "llm_api",
  max_budget: undefined,
  budget_duration: undefined,
  user_id: "test-user-id",
  duration: null,
  metadata: "{}",
};

const renderCreateKey = (props: Partial<React.ComponentProps<typeof CreateKey>> = {}) =>
  renderWithProviders(<CreateKey team={null} teams={[]} data={[]} addKey={vi.fn()} {...props} />);

const openModal = async (props: Partial<React.ComponentProps<typeof CreateKey>> = {}) => {
  const view = renderCreateKey(props);
  await userEvent.click(screen.getByTestId("create-key-button"));
  await screen.findByRole("button", { name: /^create key$/i });
  return view;
};

const nameTheKey = async (alias = "contract-key") => {
  await userEvent.type(await screen.findByLabelText(/Key Name/), alias);
};
const submit = async () => {
  await userEvent.click(screen.getByRole("button", { name: /^create key$/i }));
};
const createdPayload = async () => {
  await waitFor(() => expect(vi.mocked(keyCreateCall)).toHaveBeenCalled());
  return vi.mocked(keyCreateCall).mock.calls[0][2] as Record<string, unknown>;
};

describe("CreateKey", () => {
  beforeEach(() => {
    testQueryClient.clear();
    state.authorized = { accessToken: "test-token", userId: "test-user-id", userRole: "Admin", premiumUser: false };
    state.teams = [];
    vi.mocked(keyCreateCall).mockClear().mockResolvedValue({ key: "sk-created", soft_budget: null });
    vi.mocked(modelAvailableCall).mockClear().mockResolvedValue({ data: [{ id: "gpt-4" }] });
    vi.mocked(getRouteTemplatesCall).mockClear().mockResolvedValue([
      { id: "tpl-low-cost", name: "Low-cost route", body: {} },
    ]);
    vi.mocked(toast.fromError).mockClear();
  });

  describe("simplified controls and payload", () => {
    it("shows budget, budget reset, route template, and key lifecycle", async () => {
      await openModal();

      expect(screen.getByLabelText(/Max Budget \(USD\)/)).toBeInTheDocument();
      expect(screen.getByLabelText("Reset Budget")).toBeInTheDocument();
      expect(screen.getByLabelText("Route Templates")).toBeInTheDocument();
      expect(screen.getByLabelText("Expire Key")).toBeInTheDocument();
      expect(screen.getByText("Enable Auto-Rotation")).toBeInTheDocument();
    });

    it("removes the old advanced controls", async () => {
      await openModal();

      expect(screen.queryByText("Optional Settings")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("Tokens per minute Limit (TPM)")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("Requests per minute Limit (RPM)")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("Metadata")).not.toBeInTheDocument();
      expect(screen.queryByText("Guardrails")).not.toBeInTheDocument();
      expect(screen.queryByText("MCP Settings")).not.toBeInTheDocument();
      expect(screen.queryByText("Advanced Settings")).not.toBeInTheDocument();
    });

    it("submits the default simplified field set", async () => {
      await openModal();
      await nameTheKey();
      await submit();

      expect(await createdPayload()).toStrictEqual(DEFAULT_PAYLOAD);
    });

    it("submits the key budget", async () => {
      await openModal();
      await nameTheKey();
      await userEvent.type(await screen.findByLabelText(/Max Budget \(USD\)/), "150.75");
      await submit();

      expect((await createdPayload()).max_budget).toBe("150.75");
    });

    it("submits a daily budget reset", async () => {
      await openModal();
      await nameTheKey();
      await userEvent.click(await screen.findByLabelText("Reset Budget"));
      await userEvent.click(await screen.findByRole("option", { name: "daily" }));
      await submit();

      expect((await createdPayload()).budget_duration).toBe("24h");
    });

    it("submits null when the budget never resets", async () => {
      await openModal();
      await nameTheKey();
      await userEvent.click(await screen.findByLabelText("Reset Budget"));
      await userEvent.click(await screen.findByRole("option", { name: /never resets/i }));
      await submit();

      expect(await createdPayload()).toHaveProperty("budget_duration", null);
    });

    it("submits the selected route template", async () => {
      await openModal();
      await nameTheKey();
      await waitFor(() => expect(getRouteTemplatesCall).toHaveBeenCalledWith("test-token"));
      await userEvent.click(screen.getByLabelText("Route Templates"));
      await userEvent.click(await screen.findByRole("option", { name: "Low-cost route" }));
      await submit();

      expect((await createdPayload()).route_template_id).toBe("tpl-low-cost");
    });

    it("submits a key expiry", async () => {
      await openModal();
      await nameTheKey();
      await userEvent.type(screen.getByLabelText("Expire Key"), "45d");
      await submit();

      expect((await createdPayload()).duration).toBe("45d");
    });

    it("submits auto-rotation with its default interval", async () => {
      await openModal();
      await nameTheKey();
      await userEvent.click(screen.getByRole("switch"));
      await submit();

      expect(await createdPayload()).toMatchObject({ auto_rotate: true, rotation_interval: "30d" });
    });

    it("does not ask for a project", async () => {
      await openModal();
      await nameTheKey();
      await submit();

      expect(await createdPayload()).not.toHaveProperty("project_id");
      expect(screen.queryByLabelText("Project")).not.toBeInTheDocument();
    });

    /** 验证单团队和多团队用户均无需选择团队；前置真实表单及成员列表，提交只绑定用户；自动卸载清理。 */
    it.each([1, 2])("creates a personal key without selecting from %i member teams", async (count) => {
      state.teams = Array.from({ length: count }, (_, i) => ({ team_id: `team-${i}`, team_alias: `Team ${i}`, models: [] } as Team));
      await openModal({ teams: state.teams });
      expect(screen.queryByLabelText("Team")).not.toBeInTheDocument();
      await nameTheKey("wire-alias");
      await submit();
      expect(await createdPayload()).toMatchObject({ key_alias: "wire-alias", user_id: "test-user-id", team_id: null });
    });

    /** 验证旧预填链接不会绑定团队；前置自动打开个人表单和 team_id，提交为 null；自动卸载清理。 */
    it("ignores team prefill in the personal key form", async () => {
      state.teams = [{ team_id: "team-1", models: [] } as Team];
      renderCreateKey({ teams: state.teams, autoOpenCreate: true, prefillData: { team_id: "team-1", key_alias: "contract-key" } });
      await screen.findByRole("button", { name: /^create key$/i });
      await submit();
      expect((await createdPayload()).team_id).toBeNull();
    });
  });

  describe("validation", () => {
    it("requires a key name", async () => {
      await openModal();
      await submit();

      await waitFor(() => expect(screen.getByLabelText(/Key Name/)).toHaveAttribute("aria-invalid", "true"));
      expect(keyCreateCall).not.toHaveBeenCalled();
    });

    it("blocks a budget above the selected team's ceiling", async () => {
      const selectedTeam = { team_id: "team-1", max_budget: 10, models: [] } as Team;
      state.teams = [selectedTeam];
      await openModal({ team: selectedTeam, teams: state.teams });
      await nameTheKey();
      await userEvent.type(screen.getByLabelText(/Max Budget \(USD\)/), "50");
      await submit();

      await waitFor(() =>
        expect(screen.getByLabelText(/Max Budget \(USD\)/)).toHaveAttribute("aria-invalid", "true"),
      );
      expect(keyCreateCall).not.toHaveBeenCalled();
    });
  });

  describe("basic key fields", () => {
    it("stamps the signed-in user and hides ownership controls", async () => {
      await openModal();
      await nameTheKey();

      expect(screen.queryByText("Key Ownership")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("Organization")).not.toBeInTheDocument();
      await submit();

      expect((await createdPayload()).user_id).toBe("test-user-id");
    });

    it("prefills the key alias", async () => {
      renderCreateKey({ autoOpenCreate: true, prefillData: { key_alias: "prefilled-key" } });

      expect(await screen.findByLabelText(/Key Name/)).toHaveValue("prefilled-key");
    });

    it("prefills available models", async () => {
      renderCreateKey({ autoOpenCreate: true, prefillData: { models: ["gpt-4"] } });

      expect(await screen.findByLabelText("gpt-4", {}, { timeout: 5000 })).toBeInTheDocument();
    });

    it("offers proxy models when no team is selected", async () => {
      await openModal();
      await userEvent.click(await screen.findByLabelText("Models"));

      expect(await screen.findByRole("option", { name: "All Proxy Models" })).toBeInTheDocument();
      expect(screen.queryByRole("option", { name: "All Team Models" })).not.toBeInTheDocument();
    });

    /** 验证显式团队上下文继承团队模型；前置 team 参数，模型全选仅为团队范围；自动卸载清理。 */
    it("offers team models for an explicit team context", async () => {
      state.teams = [{ team_id: "team-1", team_alias: "Team One", models: ["team-model-1"] } as Team];
      await openModal({ team: state.teams[0], teams: state.teams });
      await userEvent.click(await screen.findByLabelText("Models"));

      expect(await screen.findByRole("option", { name: "All Team Models" })).toBeInTheDocument();
      expect(screen.queryByRole("option", { name: "All Proxy Models" })).not.toBeInTheDocument();
    });

    it("creates on Enter from the key name field", async () => {
      await openModal();
      await userEvent.type(await screen.findByLabelText(/Key Name/), "enter-key{Enter}");

      expect((await createdPayload()).key_alias).toBe("enter-key");
    });

    it("shows the generated key after creation", async () => {
      await openModal();
      await nameTheKey();
      await submit();
      await createdPayload();

      expect(await screen.findByText("Save your Key")).toBeInTheDocument();
    });

    it("rejects a duplicate alias in the same team", async () => {
      await openModal({ data: [{ team_id: null, key_alias: "contract-key" }] });
      await nameTheKey();
      await submit();

      await waitFor(() => expect(toast.fromError).toHaveBeenCalled());
      expect(keyCreateCall).not.toHaveBeenCalled();
    });
  });

  /** 验证缺省模型提示不阻止个人密钥创建；前置旧接口提示标识，表单可用且不展示伪模型；自动卸载清理。 */
  it("allows personal key creation when the user has no default models", async () => {
    vi.mocked(modelAvailableCall).mockResolvedValue({ data: [{ id: "no-default-models" }] });
    renderCreateKey();
    await userEvent.click(screen.getByTestId("create-key-button"));

    await nameTheKey();
    expect(screen.queryByText(/Please select a team to continue/)).not.toBeInTheDocument();
    await userEvent.click(await screen.findByLabelText("Models"));
    expect(await screen.findByRole("option", { name: "All Proxy Models" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "no-default-models" })).not.toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await submit();
    expect((await createdPayload()).team_id).toBeNull();
  });
});
