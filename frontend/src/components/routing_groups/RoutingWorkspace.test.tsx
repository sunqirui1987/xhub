import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import RoutingWorkspace from "./RoutingWorkspace";
vi.mock("@/app/(dashboard)/route-templates/_components/RouteTemplatesPanel", () => ({
  /** 模拟模板库边界，接收凭据并输出可观察内容；仅内存，无外部清理。 */
  default: ({accessToken}: {accessToken: string | null}) => <div>模板库 {accessToken}</div>,
}));
describe("统一路由工作区", () => {
  /** 验证路由页直接委托整份模板库并传递身份；不产生独立全局组管理，卸载清理。 */
  it("delegates the complete template workspace", () => {
    render(<RoutingWorkspace accessToken="admin" />);
    expect(screen.getByText("模板库 admin")).toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
  });
});
