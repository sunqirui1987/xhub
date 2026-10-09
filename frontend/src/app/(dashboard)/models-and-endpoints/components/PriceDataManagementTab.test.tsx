/* @vitest-environment jsdom */
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import PriceDataManagementTab from "./PriceDataManagementTab";

vi.mock("@/components/price_data_reload", () => ({ default: () => <div>reload</div> }));
vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: () => ({ accessToken: "sk-test" }) }));
vi.mock("@/components/networking", () => ({
  getPriceCatalog: vi.fn().mockResolvedValue({ models: {}, providers: [] }),
  upsertPriceModel: vi.fn(),
  deletePriceModel: vi.fn(),
  resetPriceModel: vi.fn(),
  upsertPriceProvider: vi.fn(),
  deletePriceProvider: vi.fn(),
  apiClient: { get: vi.fn(), post: vi.fn() },
  getProxyBaseUrl: vi.fn(() => ""),
  modelInfoCall: vi.fn(),
}));

describe("PriceDataManagementTab", () => {
  /** 前置空目录，验证独立页面容器与管理入口可渲染；DOM自动清理。 */
  it("renders its content standalone, without a tab-panel ancestor", () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <PriceDataManagementTab />
      </QueryClientProvider>,
    );
    expect(screen.getByText("Price Data Management")).toBeInTheDocument();
  });
});
