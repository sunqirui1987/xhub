import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { teamSpendByUserCall } from "@/components/networking";
import TeamUserSpendCard from "./TeamUserSpendCard";
import { t, setActiveLocale } from "@/i18n";
vi.mock("@/components/networking", () => ({ teamSpendByUserCall: vi.fn() }));
const call = vi.mocked(teamSpendByUserCall);
const props = { accessToken: "account-a", startTime: new Date(2025, 2, 1), endTime: new Date(2025, 2, 1), teamIds: ["team-a"] };
const response = { start_date: "2025-03-01", end_date: "2025-03-01", results: [{ team_id: "team-a", team_alias: "Team A", user_id: "user-a", user_alias: "Account A", user_email: null, spend: 0.125, prompt_tokens: 8, completion_tokens: 2, total_tokens: 10, api_requests: 2, successful_requests: 1, failed_requests: 1 }] };
/** 创建禁止重试的独立查询缓存；无参数，返回测试客户端；各例组件卸载并回收缓存，无真实数据写入。 */
const client = () => new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
describe("team user usage", () => {
  beforeEach(() => { call.mockReset(); setActiveLocale("en"); });
  /** 前置有费用与失败的用户账单；验证身份与请求表格可见、允许导出；参数为测试闭包，无返回值，自动清理渲染。 */
  it("displays settled usage and permits export", async () => {
    call.mockResolvedValue(response);
    render(<QueryClientProvider client={client()}><TeamUserSpendCard {...props} /></QueryClientProvider>);
    expect(await screen.findByText("Account A")).toBeVisible();
    expect(screen.getByRole("row", { name: /Team A.*Account A/ })).toHaveTextContent(/\$0\.125.*2.*1.*1.*10/);
    expect(screen.getByRole("button", { name: "Download CSV" })).toBeEnabled();
  });
  /** 前置空账单或无团队；验证空态禁止导出，无团队不调用接口；参数为是否选择团队，无返回值，组件自动清理。 */
  it.each([true, false])("empty scope disables export, hasTeams=%s", async (hasTeams) => {
    call.mockResolvedValue({ ...response, results: [] });
    render(<QueryClientProvider client={client()}><TeamUserSpendCard {...props} teamIds={hasTeams ? props.teamIds : []} /></QueryClientProvider>);
    expect(await screen.findByText(hasTeams ? "No user spend in this range" : "Select a team to see spend per user")).toBeVisible();
    expect(screen.getByRole("button", { name: "Download CSV" })).toBeDisabled();
    if (!hasTeams) expect(call).not.toHaveBeenCalled();
  });
  /** 前置接口故障；验证明确错误且不能导出伪造的零账单，无返回值，缓存及组件自动回收。 */
  it("reports failed reads", async () => {
    call.mockRejectedValue(new Error("unavailable"));
    render(<QueryClientProvider client={client()}><TeamUserSpendCard {...props} /></QueryClientProvider>);
    expect(await screen.findByRole("alert")).toHaveTextContent(t("Failed to load team user usage"));
    expect(screen.getByRole("button", { name: "Download CSV" })).toBeDisabled();
  });
  /** 前置共享组件缓存中存在上一身份账单；更换令牌后必须重新读取且不展示旧身份，返回无，自动卸载及回收缓存。 */
  it("isolates cached data when identity changes", async () => {
    call.mockResolvedValueOnce(response).mockRejectedValueOnce(new Error("denied"));
    const queryClient = client();
    const { rerender } = render(<QueryClientProvider client={queryClient}><TeamUserSpendCard {...props} /></QueryClientProvider>);
    expect(await screen.findByText("Account A")).toBeVisible();
    rerender(<QueryClientProvider client={queryClient}><TeamUserSpendCard {...props} accessToken="account-b" /></QueryClientProvider>);
    expect(screen.queryByText("Account A")).toBeNull();
    await waitFor(() => expect(call).toHaveBeenCalledWith("account-b", props.startTime, props.endTime, props.teamIds));
    expect(await screen.findByRole("alert")).toBeVisible();
  });
});
