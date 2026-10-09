import { render, screen, fireEvent } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { LocalModelList } from "./LocalModelList";
import { priceCatalogRows } from "./priceCatalogRows";
/** 前置本地公开、私有及下架记录；验证完整列表、搜索、上下架回调和不可售禁用；DOM由测试框架清理。 */
it("本地列表保留记录并允许上下架", () => {
  const rows = priceCatalogRows({
    models: [
      { id: "a", market_catalog: true },
      { id: "b", market_catalog: true, delisted: true, private: true },
    ],
  });
  const listing = vi.fn(),
    detail = vi.fn();
  render(<LocalModelList rows={rows} onDetail={detail} onListing={listing} />);
  expect(screen.getByRole("status")).toHaveTextContent("本地共 2 个模型");
  expect(screen.getByRole("button", { name: "上架" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "下架" }));
  expect(listing).toHaveBeenCalledWith(rows[0], true);
  fireEvent.click(screen.getByRole("button", { name: "a" }));
  expect(detail).toHaveBeenCalledWith(rows[0]);
  fireEvent.change(screen.getByRole("textbox", { name: "搜索本地模型" }), { target: { value: "missing" } });
  expect(screen.getByText("没有匹配的本地模型。")).toBeVisible();
});
