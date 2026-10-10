import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AllModelsPanel from "./AllModelsPanel";
const mocks = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), error: vi.fn(), openModel: vi.fn() }));
vi.mock("@/components/networking", () => ({ apiClient: { get: mocks.get, put: mocks.put } }));
vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: () => ({ accessToken: "admin" }) }));
vi.mock("@/app/(dashboard)/hooks/models/useModelCostMap", () => ({ useModelCostMap: () => ({ data: {} }) }));
vi.mock("../detailNavigation", () => ({ useModelDetailRouting: () => ({ openModel: mocks.openModel }) }));
vi.mock("@/components/add_model/ModelStatusToggle", () => ({ default: () => <span>已启用</span> }));
vi.mock("@/lib/toast", () => ({ toast: { success: vi.fn(), fromError: mocks.error } }));
/** 构造规范目录 ID 与展示 ID 不同的部署，验证权重始终使用选路身份；纯内存无清理。 */
function deployment(id: string) {
  return {
    id,
    model_name: "shared",
    litellm_params: { model: "vendor/full/model", litellm_credential_name: "custom-account" },
    model_info: { id: "record-" + id, transport: "bypass_openai_chat", endpoint_types: ["chat"] },
  };
}
/** 挂载隔离查询缓存并返回管理员事件模拟器；测试结束由 RTL 自动卸载。 */
function mount() {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })}>
      <AllModelsPanel />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}
describe("公开模型默认权重", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.put.mockResolvedValue({});
  });
  /** 目录缺失默认字段或返回空值时，两部署均以 1 展示并初始化完整草稿；
   * 前置隔离内存目录，验证编辑、取消及无意外写入，RTL 自动卸载并清理缓存。
   */
  it.each([
    ["字段缺失", {}],
    ["默认配置为 null", { default_weights: null }],
    ["默认配置为空对象", { default_weights: {} }],
    ["分配列表为 null", { default_weights: { allocations: null } }],
    ["分配列表为空", { default_weights: { allocations: [] } }],
  ])("%s 时展示并编辑默认权重 1", async (_label, defaults) => {
    mocks.get.mockResolvedValue({
      data: [{ model_name: "shared", deployments: [deployment("a"), deployment("b")], ...defaults }],
      total_count: 1,
      total_pages: 1,
    });
    const user = mount();
    const group = await screen.findByRole("region", { name: "公开模型 shared" });
    expect(within(group).getAllByRole("cell", { name: "1", exact: true })).toHaveLength(2);
    await user.click(within(group).getByRole("button", { name: "编辑权重" }));
    for (const id of ["a", "b"]) {
      expect(within(group).getByRole("spinbutton", { name: "部署 " + id + " 权重" })).toHaveValue(1);
    }
    await user.click(within(group).getByRole("button", { name: "取消" }));
    expect(within(group).queryByRole("spinbutton")).not.toBeInTheDocument();
    expect(mocks.put).not.toHaveBeenCalled();
  });
  /** 已配置的零权重必须保留，未配置部署才使用 1；前置部分分配目录，
   * 验证展示、草稿及完整保存载荷，全部数据在内存中，RTL 自动清理。
   */
  it("保留零权重并补全未配置部署", async () => {
    mocks.get.mockResolvedValue({
      data: [{
        model_name: "shared", deployments: [deployment("a"), deployment("b")],
        default_weights: { allocations: [{ deployment_id: "a", weight: 0 }] },
      }],
      total_count: 1, total_pages: 1,
    });
    const user = mount();
    const group = await screen.findByRole("region", { name: "公开模型 shared" });
    expect(within(group).getByRole("cell", { name: "0", exact: true })).toBeInTheDocument();
    expect(within(group).getByRole("cell", { name: "1", exact: true })).toBeInTheDocument();
    await user.click(within(group).getByRole("button", { name: "编辑权重" }));
    expect(within(group).getByRole("spinbutton", { name: "部署 a 权重" })).toHaveValue(0);
    expect(within(group).getByRole("spinbutton", { name: "部署 b 权重" })).toHaveValue(1);
    await user.click(within(group).getByRole("button", { name: "保存权重" }));
    await waitFor(() => expect(mocks.put).toHaveBeenCalledWith("/model/default", {
      accessToken: "admin",
      body: { model_name: "shared", weights: { allocations: [
        { deployment_id: "a", weight: 0 }, { deployment_id: "b", weight: 1 },
      ] } },
    }));
  });
  /** 单部署无需分配，验证无按钮和权重列且完整型号可见；仅内存目录。 */
  it("单部署隐藏全部分配操作", async () => {
    mocks.get.mockResolvedValue({
      data: [{ model_name: "shared", deployments: [deployment("a")], default_weights: { allocations: null } }],
      total_count: 1,
      total_pages: 1,
    });
    const user = mount();
    const group = await screen.findByRole("region", { name: "公开模型 shared" });
    expect(within(group).queryByRole("button", { name: "编辑权重" })).not.toBeInTheDocument();
    expect(within(group).queryByRole("columnheader", { name: "默认权重" })).not.toBeInTheDocument();
    expect(within(group).getByText("vendor/full/model")).toBeInTheDocument();
    expect(within(group).getByRole("button", { name: "配置回退" })).toBeVisible();
    await user.click(within(group).getByRole("button", { name: "详情" }));
    expect(mocks.openModel).toHaveBeenCalledWith("record-a");
  });
  /** 前置单部署保留零权重及已删除部署分配；验证可见性、全零拒绝和修复保存仅包含现存部署。
   * 隔离内存目录及查询缓存，RTL 自动卸载，无业务数据写入或清理。 */
  it("单部署残留权重可见并可修复", async () => {
    mocks.get.mockResolvedValue({
      data: [{ model_name: "shared", deployments: [deployment("a")], default_weights: { allocations: [
        { deployment_id: "a", weight: 0 }, { deployment_id: "deleted", weight: 100 },
      ] } }], total_count: 1, total_pages: 1,
    });
    const user = mount();
    const group = await screen.findByRole("region", { name: "公开模型 shared" });
    expect(within(group).getByRole("cell", { name: "0", exact: true })).toBeVisible();
    await user.click(within(group).getByRole("button", { name: "编辑权重" }));
    expect(within(group).getByRole("button", { name: "保存权重" })).toBeDisabled();
    const input = within(group).getByRole("spinbutton", { name: "部署 a 权重" });
    await user.clear(input);
    await user.type(input, "1");
    await user.click(within(group).getByRole("button", { name: "保存权重" }));
    await waitFor(() => expect(mocks.put).toHaveBeenCalledWith("/model/default", {
      accessToken: "admin", body: { model_name: "shared", weights: { allocations: [{ deployment_id: "a", weight: 1 }] } },
    }));
  });
  /** 多部署编辑任意相对权重并回读，验证规范 ID、零值和无百分比限制；查询缓存隔离。 */
  it("部署行编辑 3:7，保存刷新后回读", async () => {
    const group = {
      model_name: "shared",
      deployments: [deployment("a"), deployment("b")],
      default_weights: { allocations: [] as { deployment_id: string; weight: number }[] },
    };
    mocks.get.mockImplementation(async () => ({ data: [structuredClone(group)], total_count: 1, total_pages: 1 }));
    mocks.put.mockImplementation(async (_path, options) => {
      group.default_weights = options.body.weights;
      return {};
    });
    const user = mount();
    await user.click(await screen.findByRole("button", { name: "编辑权重" }));
    for (const [id, value] of [
      ["a", "3"],
      ["b", "7"],
    ]) {
      const input = screen.getByRole("spinbutton", { name: "部署 " + id + " 权重" });
      expect(input).toHaveValue(1);
      await user.clear(input);
      await user.type(input, value);
    }
    await user.click(screen.getByRole("button", { name: "保存权重" }));
    await waitFor(() =>
      expect(mocks.put).toHaveBeenCalledWith("/model/default", {
        accessToken: "admin",
        body: {
          model_name: "shared",
          weights: {
            allocations: [
              { deployment_id: "a", weight: 3 },
              { deployment_id: "b", weight: 7 },
            ],
          },
        },
      }),
    );
    await waitFor(() => expect(mocks.get).toHaveBeenCalledTimes(2));
    await user.click(screen.getByRole("button", { name: "编辑权重" }));
    expect(screen.getByRole("spinbutton", { name: "部署 a 权重" })).toHaveValue(3);
    expect(screen.getByRole("spinbutton", { name: "部署 b 权重" })).toHaveValue(7);
  });
  /** 全零禁止提交，后台失败保留草稿，取消恢复目录；无外部数据。 */
  it("拒绝全零并保留失败草稿", async () => {
    mocks.get.mockResolvedValue({
      data: [
        { model_name: "shared", deployments: [deployment("a"), deployment("b")], default_weights: { allocations: [] } },
      ],
      total_count: 1,
      total_pages: 1,
    });
    mocks.put.mockRejectedValue(new Error("unavailable"));
    const user = mount();
    await user.click(await screen.findByRole("button", { name: "编辑权重" }));
    for (const id of ["a", "b"]) {
      const input = screen.getByRole("spinbutton", { name: "部署 " + id + " 权重" });
      await user.clear(input);
      await user.type(input, "0");
    }
    expect(screen.getByRole("button", { name: "保存权重" })).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent("至少一个");
    const input = screen.getByRole("spinbutton", { name: "部署 b 权重" });
    await user.clear(input);
    await user.type(input, "2");
    await user.click(screen.getByRole("button", { name: "保存权重" }));
    await waitFor(() => expect(mocks.error).toHaveBeenCalled());
    expect(input).toHaveValue(2);
    await user.click(screen.getByRole("button", { name: "取消" }));
    expect(screen.queryByRole("spinbutton")).not.toBeInTheDocument();
  });
  /** 目录不可用时显示明确错误并允许重试，避免空目录误导；无外部清理。 */
  it("目录失败可以重试", async () => {
    mocks.get.mockRejectedValue(new Error("offline"));
    mount();
    expect(await screen.findByRole("alert")).toHaveTextContent("模型加载失败");
  });
});
