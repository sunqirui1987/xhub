import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ModelEditor from "./ModelEditor";
import type { EditorModel, PriceRate } from "./modelEditorPricing";

const mocks = vi.hoisted(() => ({
  catalog: undefined as undefined | { models: Record<string, unknown>[] },
  create: vi.fn(),
  patch: vi.fn(),
  apiPost: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: () => ({ accessToken: "token" }) }));
vi.mock("@/app/(dashboard)/hooks/credentials/useCredentials", () => ({
  useCredentials: () => ({
    data: {
      credentials: [
        {
          credential_name: "relay-a",
          credential_values: {},
          credential_info: { custom_llm_provider: "openai", api_base: "https://relay-a.example" },
        },
        {
          credential_name: "relay-b",
          credential_values: {},
          credential_info: { custom_llm_provider: "openai", api_base: "https://relay-b.example" },
        },
      ],
    },
    isLoading: false,
  }),
}));
vi.mock("@/app/(dashboard)/hooks/models/usePriceCatalog", () => ({
  priceCatalogKeys: { all: ["priceCatalog"] },
  usePriceCatalog: () => ({ data: mocks.catalog, isLoading: false, error: null }),
}));
vi.mock("@/app/(dashboard)/hooks/teams/useTeams", () => ({ useTeams: () => ({ data: [] }) }));
vi.mock("@/components/networking", () => ({
  apiClient: { post: (...args: unknown[]) => mocks.apiPost(...args) },
  modelCreateCall: (...args: unknown[]) => mocks.create(...args),
  modelPatchUpdateCall: (...args: unknown[]) => mocks.patch(...args),
}));
vi.mock("./EndpointTypeField", () => ({ default: () => null }));
vi.mock("../model_add/ProviderModelDialog", () => ({ ProviderModelDialog: () => null }));
vi.mock("@/lib/toast", () => ({ toast: { success: vi.fn() } }));

const mixedRates: PriceRate[] = [
  { measure: "token", side: "input", window: "offpeak", usd: 0.000002 },
  { measure: "token", side: "input", window: "peak", usd: 0.000004 },
  { measure: "token", side: "cache_read", window: "peak", variant: "long-context", usd: 0 },
  { measure: "token", side: "cache_write", window: "all", usd: 0.000005 },
  { measure: "picture", side: "output", window: "all", variant: "1024x1024", usd: 0.08 },
  { measure: "second", side: "output", window: "all", variant: "720p", usd: 0.12 },
];

const catalog = {
  models: [
    {
      id: "anthropic/claude-sonnet",
      display_name: "Claude Sonnet",
      litellm_provider: "anthropic",
      upstream_model: "claude-sonnet",
      endpoint_type: "chat",
      rates: mixedRates,
    },
    {
      id: "relay-a/private",
      display_name: "Relay A Private",
      litellm_provider: "openai",
      supplier_name: "relay-a",
      upstream_model: "private-a",
      endpoint_type: "chat",
      input_cost_per_token: 0.000001,
    },
    {
      id: "relay-b/private",
      display_name: "Relay B Private",
      litellm_provider: "openai",
      supplier_name: "relay-b",
      upstream_model: "private-b",
      input_cost_per_token: 0.000001,
    },
  ],
};

const relayA = {
  credential_name: "relay-a",
  credential_values: {},
  credential_info: { custom_llm_provider: "openai", api_base: "https://relay-a.example" },
};

function renderEditor(props: React.ComponentProps<typeof ModelEditor> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ModelEditor {...props} />
    </QueryClientProvider>,
  );
}

async function chooseUpstream(user: ReturnType<typeof userEvent.setup>, value: string) {
  const input = screen.getByRole("combobox", { name: "上游模型 *" });
  await user.clear(input);
  await user.click(input);
  await user.click(await screen.findByRole("option", { name: new RegExp(`^${value}`) }));
}

describe("ModelEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.catalog = catalog;
    mocks.apiPost.mockResolvedValue({ models: [{ id: "discovered-only" }, { id: "claude-sonnet" }] });
    mocks.create.mockImplementation(async (_token, payload) => payload);
    mocks.patch.mockImplementation(async (_token, payload) => payload);
  });

  it("auto-fetches upstream suggestions only from the saved provider and refreshes on request", async () => {
    const user = userEvent.setup();
    mocks.apiPost.mockImplementation(async (_path, options: { body: { credential_name: string } }) => ({
      models:
        options.body.credential_name === "relay-a"
          ? [{ id: "relay-a-discovered" }, { id: "claude-sonnet" }]
          : [{ id: "relay-b-discovered" }],
    }));
    renderEditor();
    const upstream = screen.getByRole("combobox", { name: "上游模型 *" });
    expect(upstream).toBeDisabled();
    await user.selectOptions(screen.getByLabelText("模型提供商 *"), "relay-a");

    await waitFor(() => expect(mocks.apiPost).toHaveBeenCalledOnce());
    expect(mocks.apiPost).toHaveBeenCalledWith(
      "/model/builtin/models",
      expect.objectContaining({
        accessToken: "token",
        body: { provider: "openai", credential_name: "relay-a" },
      }),
    );
    expect(upstream).toBeEnabled();
    await user.click(upstream);
    expect(await screen.findByRole("option", { name: /^relay-a-discovered/ })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "claude-sonnet" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /private-a|Relay A Private/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /private-b/ })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "获取模型列表" }));
    await waitFor(() => expect(mocks.apiPost).toHaveBeenCalledTimes(2));

    await user.selectOptions(screen.getByLabelText("模型提供商 *"), "relay-b");
    await waitFor(() => expect(mocks.apiPost).toHaveBeenCalledTimes(3));
    await user.click(upstream);
    expect(await screen.findByRole("option", { name: /^relay-b-discovered/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /^relay-a-discovered/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "claude-sonnet" })).not.toBeInTheDocument();
  });

  it("keeps catalog pricing independent when selecting or typing an upstream model", async () => {
    const user = userEvent.setup();
    renderEditor();
    await user.selectOptions(screen.getByLabelText("模型提供商 *"), "relay-a");
    await chooseUpstream(user, "claude-sonnet");
    const pricing = screen.getByLabelText("目录定价模型 *");
    await user.selectOptions(pricing, "anthropic/claude-sonnet");
    await chooseUpstream(user, "discovered-only");
    expect(pricing).toHaveValue("anthropic/claude-sonnet");

    const upstream = screen.getByRole("combobox", { name: "上游模型 *" });
    await user.clear(upstream);
    await user.type(upstream, "typed-custom-model");
    expect(upstream).toHaveValue("typed-custom-model");
    expect(pricing).toHaveValue("anthropic/claude-sonnet");
  });

  it("retains selected pricing and advanced manual values across upstream and supplier changes", async () => {
    const user = userEvent.setup();
    renderEditor();
    await user.selectOptions(screen.getByLabelText("模型提供商 *"), "relay-a");
    await user.type(screen.getByRole("combobox", { name: "上游模型 *" }), "first-upstream");
    await user.selectOptions(screen.getByLabelText("目录定价模型 *"), "anthropic/claude-sonnet");
    await user.selectOptions(screen.getByLabelText("价格来源"), "manual");

    expect(screen.getByLabelText("费率设置方式")).toHaveValue("rates");
    expect(screen.getByLabelText("费率 3 项目")).toHaveValue("cache_read");
    expect(screen.getByLabelText("费率 3 时段")).toHaveValue("peak");
    expect(screen.getByLabelText("费率 3 规格")).toHaveValue("long-context");

    await user.clear(screen.getByRole("combobox", { name: "上游模型 *" }));
    await user.type(screen.getByRole("combobox", { name: "上游模型 *" }), "changed-upstream");
    await user.selectOptions(screen.getByLabelText("模型提供商 *"), "relay-b");

    expect(screen.getByLabelText("目录定价模型（参考）")).toHaveValue("anthropic/claude-sonnet");
    expect(screen.getByLabelText("价格来源")).toHaveValue("manual");
    expect(screen.getByLabelText("费率 3 规格")).toHaveValue("long-context");
    expect(screen.getAllByLabelText(/费率 \d+ 单价/)).toHaveLength(mixedRates.length);
  });

  it("uses initialCatalogId only for pricing and keeps the discovered upstream independent", async () => {
    const user = userEvent.setup();
    renderEditor({ initialSupplier: relayA, initialCatalogId: "anthropic/claude-sonnet" });
    await waitFor(() => expect(mocks.apiPost).toHaveBeenCalledOnce());
    expect(screen.getByRole("combobox", { name: "上游模型 *" })).toHaveValue("");
    expect(screen.getByLabelText("目录定价模型 *")).toHaveValue("anthropic/claude-sonnet");
    await chooseUpstream(user, "discovered-only");
    expect(screen.getByLabelText("目录定价模型 *")).toHaveValue("anthropic/claude-sonnet");
    expect(screen.getByRole("combobox", { name: "上游模型 *" })).toHaveValue("discovered-only");
    expect(screen.getByLabelText("目录定价模型 *")).toHaveValue("anthropic/claude-sonnet");
  });

  it("loads edited pricing defaults directly without asynchronous reinitialization", () => {
    const model: EditorModel = {
      model_name: "existing-name",
      litellm_params: { model: "custom-upstream", custom_llm_provider: "openai", litellm_credential_name: "relay-a" },
      model_info: { endpoint_types: ["chat"], transport: "adapted", pricing_source: "catalog", base_model: "anthropic/claude-sonnet" },
    };
    renderEditor({ model });
    expect(screen.getByRole("combobox", { name: "上游模型 *" })).toHaveValue("custom-upstream");
    expect(screen.getByLabelText("目录定价模型 *")).toHaveValue("anthropic/claude-sonnet");
    expect(screen.getByLabelText("对外模型名称 *")).toHaveValue("existing-name");
  });

  it("persists disabled state without exposing or submitting team ownership", async () => {
    const user = userEvent.setup();
    const model: EditorModel = {
      model_name: "existing-name",
      litellm_params: { model: "custom-upstream", custom_llm_provider: "openai", litellm_credential_name: "relay-a" },
      model_info: {
        id: "model-123",
        db_model: true,
        disabled: true,
        endpoint_types: ["chat"],
        transport: "adapted",
        pricing_source: "catalog",
        base_model: "anthropic/claude-sonnet",
      },
    };
    renderEditor({ model });

    expect(screen.queryByLabelText("所属团队")).not.toBeInTheDocument();
    expect(screen.queryByText("所属团队")).not.toBeInTheDocument();
    expect(screen.getByRole("switch", { name: "启用模型" })).not.toBeChecked();

    await user.click(screen.getByRole("button", { name: "保存修改" }));

    await waitFor(() => expect(mocks.patch).toHaveBeenCalledOnce());
    const payload = mocks.patch.mock.calls[0][1] as EditorModel;
    expect(mocks.patch).toHaveBeenCalledWith("token", expect.anything(), "model-123");
    expect(payload.model_info).toMatchObject({
      disabled: true,
      pricing_source: "catalog",
      base_model: "anthropic/claude-sonnet",
    });
    expect(payload.model_info).not.toHaveProperty("team_id");
  });
});
