import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "@/components/networking";
import { t } from "@/i18n";
import { PriceModelDialog } from "./PriceModelDialog";
import { priceCatalogRows, type PriceCatalogRow, type PriceProviderRow } from "./priceCatalogRows";

vi.mock("@/components/networking", () => ({ apiClient: { get: vi.fn() } }));

const providers: PriceProviderRow[] = [
  { slug: "openai", name: "OpenAI", apiBase: null, modelCount: 0, baseline: true, overridden: false },
  { slug: "custom", name: "Custom", apiBase: null, modelCount: 0, baseline: true, overridden: false },
];

/** 渲染价格弹窗；参数为可选编辑行和供应商目录，返回用户驱动器及保存桩，供组件单测调用。
 * 目录请求由用例配置；全局 afterEach 卸载组件，beforeEach 重置请求桩，无数据库副作用。 */
function setup(editing: PriceCatalogRow | null = null, choices = providers) {
  const onSave = vi.fn();
  const user = userEvent.setup();
  render(<PriceModelDialog open onOpenChange={vi.fn()} editing={editing} providers={choices} onSave={onSave} />);
  return { user, onSave };
}

describe("PriceModelDialog classification selects", () => {
  beforeEach(() => {
    vi.mocked(apiClient.get).mockReset();
    vi.mocked(apiClient.get).mockResolvedValue({
      transports: [
        { id: "bypass_openai_chat", label: "OpenAI Chat" },
        { id: "ark_contents_generation", label: "Ark Video" },
      ],
    });
  });

  /** 前置正常目录与新建表单；验证三个下拉、目录值及保存正文；全局卸载清理，不访问真实后台。 */
  it("selects catalog provider, mode and execution endpoint when saving", async () => {
    const { user, onSave } = setup();
    const provider = screen.getByRole("combobox", { name: t("Provider"), exact: true });
    const mode = screen.getByRole("combobox", { name: t("priceData.endpointMode"), exact: true });
    const endpoint = screen.getByRole("combobox", { name: t("priceData.endpointType"), exact: true });
    await within(endpoint).findByRole("option", { name: /Ark Video/ });
    expect(apiClient.get).toHaveBeenCalledWith("/public/endpoints");
    expect(within(provider).getByRole("option", { name: "OpenAI (openai)" })).toBeInTheDocument();
    await user.type(screen.getByLabelText(t("priceData.modelId")), "select-test");
    await user.selectOptions(provider, "custom");
    await user.selectOptions(mode, "video_generation");
    await user.selectOptions(endpoint, "ark_contents_generation");
    await user.click(screen.getByRole("button", { name: t("priceData.addModel"), exact: true }));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "select-test",
        litellm_provider: "custom",
        mode: "video_generation",
        endpoint_id: "ark_contents_generation",
      }),
    );
  });

  /** 前置三个字段均含目录外旧值；验证请求完成仍回显且保存保留；全局卸载与重置 mock 清理。 */
  it("retains legacy values outside all three catalogs", async () => {
    const row = priceCatalogRows({
      models: [{ id: "legacy", litellm_provider: "old-provider", mode: "old-mode", endpoint_id: "old-endpoint" }],
    })[0];
    const { user, onSave } = setup(row);
    await screen.findByRole("option", { name: /Ark Video/ });
    expect(screen.getByLabelText(t("Provider"))).toHaveValue("old-provider");
    expect(screen.getByLabelText(t("priceData.endpointMode"))).toHaveValue("old-mode");
    expect(screen.getByLabelText(t("priceData.endpointType"))).toHaveValue("old-endpoint");
    await user.click(screen.getByRole("button", { name: t("Save"), exact: true }));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ litellm_provider: "old-provider", mode: "old-mode", endpoint_id: "old-endpoint" }),
    );
  });

  /** 前置空目录；验证可选分类为空、缺少供应商时禁止保存；全局卸载清理，无后台写入。 */
  it("keeps optional values unset and blocks saving without a provider", async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ transports: [] });
    const { user, onSave } = setup(null, []);
    await user.type(screen.getByLabelText(t("priceData.modelId")), "empty");
    expect(screen.getByLabelText(t("priceData.endpointMode"))).toHaveValue("");
    expect(screen.getByLabelText(t("priceData.endpointType"))).toHaveValue("");
    expect(screen.getByRole("button", { name: t("priceData.addModel"), exact: true })).toBeDisabled();
    expect(onSave).not.toHaveBeenCalled();
  });

  /** 前置目录请求失败与历史 endpoint_type；验证错误可见、旧值仍可保存；全局卸载清理。 */
  it("preserves the legacy endpoint alias on a catalog failure", async () => {
    vi.mocked(apiClient.get).mockRejectedValue(new Error("offline"));
    const row = priceCatalogRows({
      models: [{ id: "legacy", litellm_provider: "openai", endpoint_type: "ark_contents_generation" }],
    })[0];
    const { user, onSave } = setup(row);
    expect(await screen.findByRole("alert")).toHaveTextContent("已有值已保留");
    expect(screen.getByLabelText(t("priceData.endpointType"))).toHaveValue("ark_contents_generation");
    await user.click(screen.getByRole("button", { name: t("Save"), exact: true }));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ endpoint_id: "ark_contents_generation", endpoint_type: null }),
    );
  });

  /** 前置新建表单收到不合法端点响应；验证失败提示及端点禁用，其余价格可保存；全局卸载清理。 */
  it("shows degraded behavior when the endpoint catalog is malformed", async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ transports: null });
    const { user, onSave } = setup();
    await screen.findByRole("alert");
    expect(screen.getByLabelText(t("priceData.endpointType"))).toBeDisabled();
    await user.type(screen.getByLabelText(t("priceData.modelId")), "offline");
    await user.click(screen.getByRole("button", { name: t("priceData.addModel"), exact: true }));
    expect(onSave.mock.calls[0][0]).not.toHaveProperty("endpoint_id");
  });

  /** 前置已有分类；验证未设置可清除模型类型和执行类型；全局卸载清理，不影响原始数据。 */
  it("clears both optional selections while editing", async () => {
    const row = priceCatalogRows({
      models: [{ id: "clear", litellm_provider: "openai", mode: "chat", endpoint_id: "bypass_openai_chat" }],
    })[0];
    const { user, onSave } = setup(row);
    await screen.findByRole("option", { name: /Ark Video/ });
    await user.selectOptions(screen.getByLabelText(t("priceData.endpointMode")), "");
    await user.selectOptions(screen.getByLabelText(t("priceData.endpointType")), "");
    await user.click(screen.getByRole("button", { name: t("Save"), exact: true }));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ mode: null, endpoint_id: null }));
  });
});
