/* @vitest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import PriceDataPage from "./page";

/** 在 mock 提升前创建身份和导航夹具，验证管理员、拒绝和只读边界；每次用例清理调用记录。 */
const { mockUseAuthorized, mockReplace } = vi.hoisted(() => ({ mockUseAuthorized: vi.fn(), mockReplace: vi.fn() }));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: mockUseAuthorized }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace: mockReplace }) }));
vi.mock("@/components/common_components/LoadingScreen", () => ({ default: () => <div>loading</div> }));
vi.mock("@/app/(dashboard)/models-and-endpoints/components/PriceDataManagementTab", () => ({
  default: () => <div>price-data-content</div>,
}));

describe("PriceDataPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuthorized.mockReturnValue({ userRole: "Admin", isViewOnly: false, isLoading: false });
  });

  it("renders the price data page for a writable administrator", () => {
    render(<PriceDataPage />);
    expect(screen.getByText("price-data-content")).toBeInTheDocument();
  });

  it("redirects non-administrators away without rendering price data", async () => {
    mockUseAuthorized.mockReturnValue({ userRole: "Internal User", isViewOnly: false, isLoading: false });

    render(<PriceDataPage />);

    expect(screen.queryByText("price-data-content")).not.toBeInTheDocument();
    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/ui/api-keys"));
  });

  it("does not expose management to a view-only administrator", async () => {
    mockUseAuthorized.mockReturnValue({ userRole: "Admin", isViewOnly: true, isLoading: false });

    render(<PriceDataPage />);

    expect(screen.queryByText("price-data-content")).not.toBeInTheDocument();
    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/ui/api-keys"));
  });
});
