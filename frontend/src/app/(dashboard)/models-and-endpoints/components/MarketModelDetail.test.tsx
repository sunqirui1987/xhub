import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { apiClient, modelInfoCall } from "@/components/networking";
import { MarketModelDetail, loadMarketAccess } from "./MarketModelDetail";
import { priceCatalogRows } from "./priceCatalogRows";

vi.mock("@/components/networking", () => ({
  apiClient: { get: vi.fn() },
  modelInfoCall: vi.fn(),
  getProxyBaseUrl: () => "https://internal.test",
}));
beforeEach(() => vi.clearAllMocks());
/** 构造独立详情视图；参数为是否登录，返回渲染结果；不重试网络，DOM和查询缓存由测试隔离。 */
function showDetail(loggedIn = false) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const row = priceCatalogRows({
    models: {
      "test/model": {
        market_catalog: true,
        input_modalities: ["text"],
        model_doc_url: "javascript:alert(1)",
        integration_doc_url: "https://docs.test",
        pricing_rules_v2: [
          {
            input_range: [0, 100],
            details_v2: {
              input: { name: "输入", unit_name: "token", unit_size: 1000000, unit_price_usd: 0, unit_price: 1 },
            },
          },
          { input_range: [101, 200], details_v2: { output: { name: "输出", unit_name: "second", unit_price: 2 } } },
        ],
      },
    },
  })[0];
  return render(
    <QueryClientProvider client={client}>
      <MarketModelDetail
        row={row}
        accessToken={loggedIn ? "test-token" : null}
        userId="test-user"
        userRole="Admin"
        onClose={vi.fn()}
      />
    </QueryClientProvider>,
  );
}
/** 前置完整双档位且未登录；验证全部价格、缺失价格、内部登录边界及链接协议过滤；DOM自动清理。 */
it("详情保留全部价格并隔离内部接入信息", async () => {
  const user = userEvent.setup();
  showDetail();
  const table = screen.getByRole("table", { name: "完整市场价格" });
  expect(within(table).getAllByRole("row")).toHaveLength(3);
  expect(within(table).getByText("$0")).toBeInTheDocument();
  expect(within(table).getByText("未提供")).toBeInTheDocument();
  await user.click(screen.getByRole("tab", { name: "API 接入" }));
  expect(screen.getByText("登录后查看内部接入信息。")).toBeInTheDocument();
  expect(modelInfoCall).not.toHaveBeenCalled();
  await user.click(screen.getByRole("tab", { name: "调用文档" }));
  expect(screen.getByRole("link", { name: "原厂接入文档" })).toHaveAttribute("href", "https://docs.test/");
  expect(screen.queryByRole("link", { name: "模型文档" })).not.toBeInTheDocument();
});
/** 前置登录后部署查询失败；验证错误可观察而非误报未部署；DOM及查询缓存隔离。 */
it("查询失败时显示内部接入错误", async () => {
  vi.mocked(apiClient.get).mockRejectedValue(new Error("offline"));
  const user = userEvent.setup();
  showDetail(true);
  await user.click(screen.getByRole("tab", { name: "接入信息" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("接入信息读取失败");
  expect(screen.queryByText(/尚未部署此模型/)).not.toBeInTheDocument();
});
/** 前置两页部署及合法端点；验证完整分页读取和坏响应拒绝；无持久数据，无需清理。 */
it("读取完整部署分页并拒绝非法响应", async () => {
  vi.mocked(apiClient.get).mockResolvedValue({ endpoint_types: [] });
  vi.mocked(modelInfoCall)
    .mockResolvedValueOnce({ data: [{ model_name: "first" }], total_pages: 2 } as never)
    .mockResolvedValueOnce({ data: [{ model_name: "second" }], total_pages: 2 } as never);
  expect((await loadMarketAccess("token", "user", "Admin")).deployments).toHaveLength(2);
  expect(modelInfoCall).toHaveBeenNthCalledWith(2, "token", "user", "Admin", 2, 1000);
  vi.mocked(modelInfoCall).mockResolvedValueOnce({ data: null } as never);
  await expect(loadMarketAccess("token", "user", "Admin")).rejects.toThrow("部署列表读取失败");
});
