import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import { MarketPriceCatalog } from "./MarketPriceCatalog";
import { priceCatalogRows } from "./priceCatalogRows";

beforeEach(() => setActiveLocale("zh-CN"));
/** 前置缓存字段排在输入输出之前的真实价格结构；验证卡片优先展示主要费用，完整缓存价仍由详情提供；DOM自动清理。 */
it("卡片优先显示输入输出价格", () => {
  const rows = priceCatalogRows({
    models: {
      a: {
        market_catalog: true,
        pricing_rules_v2: [
          {
            details_v2: {
              cache: { name: "缓存", unit_name: "token", unit_size: 1000000, unit_price_usd: 1 },
              input: { name: "输入费用", unit_name: "token", unit_size: 1000000, unit_price_usd: 2 },
              output: { name: "输出费用", unit_name: "token", unit_size: 1000000, unit_price_usd: 6 },
            },
          },
        ],
      },
    },
  });
  render(<MarketPriceCatalog rows={rows} />);
  expect(screen.getByRole("article")).toHaveTextContent("输入费用");
  expect(screen.getByRole("article")).toHaveTextContent("输出费用");
  expect(screen.queryByText("缓存", { exact: true })).not.toBeInTheDocument();
});
/** 前置两家公开供应商和隐藏记录；验证组合筛选、供应商计数、清空与空结果；DOM自动清理。 */
it("按市场声明组合筛选并显示真实供应商计数", async () => {
  const user = userEvent.setup();
  const rows = priceCatalogRows({
    models: {
      a: {
        market_catalog: true,
        issuer: "Alpha",
        input_modalities: ["text", "image"],
        output_modalities: ["video"],
        max_input_tokens: 256000,
        architecture: { reasoning: { supported: true } },
        hot_tags: ["热门"],
        features: ["视频生成"],
      },
      b: { market_catalog: true, issuer: "Beta", input_modalities: ["text"], output_modalities: ["text"] },
      private: { market_catalog: true, private: true, issuer: "Hidden" },
      down: { market_catalog: true, delisted: true },
    },
  });
  render(<MarketPriceCatalog rows={rows} />);
  expect(screen.getAllByRole("article")).toHaveLength(2);
  expect(screen.getByText(/共 2 个供应商，当前匹配 2 个/)).toBeInTheDocument();
  await user.type(screen.getByRole("textbox", { name: "搜索供应商" }), "alpha");
  expect(screen.getByText(/共 2 个供应商，当前匹配 1 个/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: /Alpha\s*1/ }));
  await user.click(within(screen.getByRole("group", { name: "输入模态" })).getByRole("button", { name: "图片 1" }));
  await user.click(within(screen.getByRole("group", { name: "输出模态" })).getByRole("button", { name: "视频 1" }));
  await user.click(screen.getByRole("button", { name: /推理能力\s*1/ }));
  await user.click(screen.getByRole("button", { name: "热门 1" }));
  await user.click(screen.getByRole("button", { name: "视频生成 1" }));
  expect(screen.getAllByRole("article")).toHaveLength(1);
  await user.type(screen.getByRole("textbox", { name: "搜索" }), "missing");
  expect(screen.getByText("没有符合条件的模型")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "清除筛选" }));
  expect(screen.getAllByRole("article")).toHaveLength(2);
});
/** 前置61个公开模型；验证加载更多、详情和下架回调以及加载失败重试；DOM自动清理。 */
it("完整模型可分页查看并提供详情下架及失败重试", async () => {
  const user = userEvent.setup();
  const rows = priceCatalogRows({
    models: Object.fromEntries(
      Array.from({ length: 61 }, (_, i) => ["model-" + String(i).padStart(2, "0"), { market_catalog: true }]),
    ),
  });
  const detail = vi.fn(),
    delist = vi.fn(),
    retry = vi.fn();
  const view = render(<MarketPriceCatalog rows={rows} onDetail={detail} onDelete={delist} />);
  expect(screen.getAllByRole("article")).toHaveLength(60);
  await user.click(screen.getByRole("button", { name: "加载更多模型（剩余 1 个）" }));
  expect(screen.getAllByRole("article")).toHaveLength(61);
  const card = screen.getByTestId("price-row-model-00");
  await user.click(within(card).getByRole("button", { name: "查看 model-00 详情" }));
  expect(detail).toHaveBeenCalledWith(rows[0]);
  await user.click(within(card).getByRole("button", { name: "下架" }));
  expect(delist).toHaveBeenCalledWith(rows[0]);
  view.rerender(<MarketPriceCatalog rows={[]} isError onRetry={retry} />);
  expect(screen.getByRole("alert")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "重试" }));
  expect(retry).toHaveBeenCalledOnce();
});
