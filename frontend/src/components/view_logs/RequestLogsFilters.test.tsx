import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { chooseSelectOption, renderWithProviders, testQueryClient } from "../../../tests/test-utils";
import { ERROR_CODE_OPTIONS } from "./constants";
import { LOG_FILTER_IDS } from "./log_filter_logic";
import { RequestLogsFilters } from "./RequestLogsFilters";

const { authorizedMock } = vi.hoisted(() => ({ authorizedMock: vi.fn() }));

vi.mock("@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity")>();
  return { ...actual, useIsPlatformAdmin: () => true, useIsTeamAdminForAnyTeam: () => false };
});
vi.mock("@/app/(dashboard)/hooks/useIsOrgAdmin", () => ({ default: () => false }));

vi.mock("@/app/(dashboard)/hooks/keys/useKeyAliases", () => ({
  useInfiniteKeyAliases: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: authorizedMock,
}));

vi.mock("@/components/networking", () => ({
  userAvailableModelsCall: vi.fn().mockResolvedValue({ data: [] }),
}));

vi.mock("@/app/(dashboard)/hooks/spendLogs/useSpendLogUsers", () => ({
  useInfiniteSpendLogUsers: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/spendLogs/useSpendLogEndUsers", () => ({
  useInfiniteSpendLogEndUsers: vi.fn(),
}));

import { useInfiniteSpendLogEndUsers } from "@/app/(dashboard)/hooks/spendLogs/useSpendLogEndUsers";
import { useInfiniteSpendLogUsers } from "@/app/(dashboard)/hooks/spendLogs/useSpendLogUsers";
import { useInfiniteKeyAliases } from "@/app/(dashboard)/hooks/keys/useKeyAliases";
import { userAvailableModelsCall } from "@/components/networking";

const emptyInfiniteQuery = {
  data: { pages: [], pageParams: [] },
  fetchNextPage: vi.fn(),
  hasNextPage: false,
  isFetchingNextPage: false,
  isLoading: false,
};

const LOGS_WINDOW = { start_date: "2026-07-23 00:00:00", end_date: "2026-07-24 00:00:00" };


/** 渲染日志筛选器并返回查询写入回调；参数是初始筛选值，不写后台，组件由测试框架卸载。 */
function renderFilters(filters: Record<string, string> = {}) {
  const set = vi.fn();
  renderWithProviders(
    <RequestLogsFilters get={(id: string) => filters[id]} set={set} teams={[]} logsWindow={LOGS_WINDOW} />,
  );
  return { set };
}

/** 在测试中保存筛选状态；无参数，返回可交互筛选器，状态仅存在于测试组件且随卸载清理。 */
function StatefulFilters() {
  const [filters, setFilters] = useState<Record<string, string | undefined>>({});
  return (
    <RequestLogsFilters
      get={(id: string) => filters[id]}
      set={(id: string, value: unknown) =>
        setFilters((previous) => ({ ...previous, [id]: typeof value === "string" ? value : undefined }))
      }
      teams={[]}
      logsWindow={LOGS_WINDOW}
    />
  );
}

describe("RequestLogsFilters", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    authorizedMock.mockReturnValue({ accessToken: "sk-test", userId: "user-1", userRole: "Internal User" });
    testQueryClient.clear();
    vi.mocked(useInfiniteKeyAliases).mockReturnValue(
      emptyInfiniteQuery as unknown as ReturnType<typeof useInfiniteKeyAliases>,
    );
    vi.mocked(userAvailableModelsCall).mockResolvedValue({ data: [] });
    vi.mocked(useInfiniteSpendLogUsers).mockReturnValue(
      emptyInfiniteQuery as unknown as ReturnType<typeof useInfiniteSpendLogUsers>,
    );
    vi.mocked(useInfiniteSpendLogEndUsers).mockReturnValue(
      emptyInfiniteQuery as unknown as ReturnType<typeof useInfiniteSpendLogEndUsers>,
    );
  });

  /** 前置错误日志视图；验证状态选择隐藏且缓存筛选仍可使用，自动卸载和查询缓存清理。 */
  it("hides conflicting status choices in error logs", () => {
    renderWithProviders(<RequestLogsFilters get={() => undefined} set={vi.fn()} teams={[]} logsWindow={LOGS_WINDOW} errorsOnly />);
    expect(screen.queryByText("Status", { exact: true })).not.toBeInTheDocument();
    expect(screen.getByText("Cache", { exact: true })).toBeInTheDocument();
  });

  /** 前置普通日志筛选表单；展开状态选项后应只有成功和全部非失败状态，卸载与查询缓存由测试框架清理。 */
  it("does not offer failure status in ordinary logs", async () => {
    const user = userEvent.setup();
    renderFilters();
    await user.click(screen.getByText("All Statuses"));
    expect(screen.queryByRole("option", { name: "Failure" })).not.toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Success" })).toBeInTheDocument();
  });

  it("renders every backend-supported filter field", async () => {
    renderFilters();

    for (const label of [
      "Team ID",
      "Status",
      "Cache",
      "Key Alias",
      "User ID",
      "End User",
      "Error Code",
      "Error Message",
      "Key Hash",
      "Session ID",
      "Model",
      "Public model / search tool",
    ]) {
      expect(await screen.findByText(label)).toBeInTheDocument();
    }
  });

  it("places User ID between Key Alias and End User", async () => {
    renderFilters();

    const labels = ["Key Alias", "User ID", "End User"].map((label) => screen.getByText(label));
    expect(labels[0].compareDocumentPosition(labels[1]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(labels[1].compareDocumentPosition(labels[2]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("selects an internal user value from the caller's visible spend logs", async () => {
    vi.mocked(useInfiniteSpendLogUsers).mockReturnValue({
      ...emptyInfiniteQuery,
      data: {
        pages: [
          {
            data: ["alice@example.com"],
            meta: { page: 1, page_size: 50, has_more: false },
            links: { self: "", next: null },
          },
        ],
        pageParams: [1],
      },
    } as unknown as ReturnType<typeof useInfiniteSpendLogUsers>);
    const user = userEvent.setup();
    const { set } = renderFilters();

    await chooseSelectOption(user, await screen.findByPlaceholderText("Search an internal user"), "alice@example.com");

    expect(set).toHaveBeenCalledWith(LOG_FILTER_IDS.USER_ID, "alice@example.com");
  });

  it("pushes the User ID picker query to the paginated user lookup", async () => {
    const user = userEvent.setup();
    renderFilters();

    const input = await screen.findByPlaceholderText("Search an internal user");
    await user.click(input);
    fireEvent.change(input, { target: { value: "alice@example.com" } });

    await waitFor(() => expect(useInfiniteSpendLogUsers).toHaveBeenCalledWith(LOGS_WINDOW, 50, "alice@example.com"));
  });

  it("loads the next page when the User ID list is scrolled near the end", async () => {
    const fetchNextPage = vi.fn();
    vi.mocked(useInfiniteSpendLogUsers).mockReturnValue({
      ...emptyInfiniteQuery,
      fetchNextPage,
      hasNextPage: true,
      data: {
        pages: [
          {
            data: ["alice@example.com"],
            meta: { page: 1, page_size: 50, has_more: true },
            links: { self: "", next: "?page=2" },
          },
        ],
        pageParams: [1],
      },
    } as unknown as ReturnType<typeof useInfiniteSpendLogUsers>);
    const user = userEvent.setup();
    renderFilters();

    await user.click(await screen.findByPlaceholderText("Search an internal user"));
    const list = await screen.findByTestId("paginated-search-select-list");
    Object.defineProperty(list, "scrollTop", { value: 90, configurable: true });
    Object.defineProperty(list, "clientHeight", { value: 10, configurable: true });
    Object.defineProperty(list, "scrollHeight", { value: 100, configurable: true });
    list.dispatchEvent(new Event("scroll", { bubbles: true }));

    await waitFor(() => expect(fetchNextPage).toHaveBeenCalled());
  });

  it("scopes the Key Alias lookup to the selected team", async () => {
    renderFilters({ [LOG_FILTER_IDS.TEAM_ID]: "team-42" });

    await waitFor(() => expect(useInfiniteKeyAliases).toHaveBeenCalled());
    expect(useInfiniteKeyAliases).toHaveBeenCalledWith(50, undefined, "team-42");
  });

  it("leaves the Key Alias lookup unscoped when no team is selected", async () => {
    renderFilters();

    await waitFor(() => expect(useInfiniteKeyAliases).toHaveBeenCalled());
    expect(useInfiniteKeyAliases).toHaveBeenCalledWith(50, undefined, undefined);
  });

  it("loads the model list from the caller's own models, not the deployment catalog", async () => {
    renderFilters({ [LOG_FILTER_IDS.TEAM_ID]: "team-42" });

    await waitFor(() => expect(userAvailableModelsCall).toHaveBeenCalledWith("sk-test"));
    expect(userAvailableModelsCall).not.toHaveBeenCalledWith("sk-test", expect.anything());
  });

  /** 前置正常模型列表含重复与空 ID；验证只显示有效唯一项，模拟接口和组件由每次测试清理。 */
  it("deduplicates available models and ignores empty ids", async () => {
    vi.mocked(userAvailableModelsCall).mockResolvedValue({ data: [{ id: "visible-model" }, { id: "visible-model" }, { id: "" }, {}] });
    renderFilters();
    await waitFor(() => expect(userAvailableModelsCall).toHaveBeenCalled());
    await userEvent.setup().click(screen.getByPlaceholderText("Search a model"));
    expect(await screen.findAllByRole("option", { name: "visible-model" })).toHaveLength(1);
  });

  /** 前置没有登录凭据；验证不调用模型接口且显示空选项，测试不生成持久化数据。 */
  it("does not load models without an access token", async () => {
    authorizedMock.mockReturnValue({ accessToken: null, userId: "user-1", userRole: "Internal User" });
    renderFilters();
    await userEvent.setup().click(screen.getByPlaceholderText("Search a model"));
    expect(await screen.findByText("No models found")).toBeInTheDocument();
    expect(userAvailableModelsCall).not.toHaveBeenCalled();
  });

  /** 前置模型接口拒绝请求；验证页面降级为空列表且无未处理异常，模拟接口下一例重置。 */
  it("shows empty model options when the lookup fails", async () => {
    vi.mocked(userAvailableModelsCall).mockRejectedValue(new Error("model lookup unavailable"));
    renderFilters();
    await waitFor(() => expect(userAvailableModelsCall).toHaveBeenCalled());
    await userEvent.setup().click(screen.getByPlaceholderText("Search a model"));
    expect(await screen.findByText("No models found")).toBeInTheDocument();
  });

  it("asks the server for a bounded page of end users scoped to the visible time window", async () => {
    renderFilters();

    await waitFor(() => expect(useInfiniteSpendLogEndUsers).toHaveBeenCalled());
    expect(useInfiniteSpendLogEndUsers).toHaveBeenCalledWith(LOGS_WINDOW, 50, undefined);
  });

  it("pushes the End User query to the server rather than filtering a preloaded list", async () => {
    const user = userEvent.setup();
    renderFilters();

    const input = await screen.findByPlaceholderText("Search an end user");
    await user.click(input);
    fireEvent.change(input, { target: { value: "acme" } });

    await waitFor(() => expect(useInfiniteSpendLogEndUsers).toHaveBeenCalledWith(LOGS_WINDOW, 50, "acme"));
  });

  it("renders only the end users the current page returned", async () => {
    vi.mocked(useInfiniteSpendLogEndUsers).mockReturnValue({
      ...emptyInfiniteQuery,
      data: {
        pages: [
          {
            data: ["cust-a", "cust-b"],
            meta: { page: 1, page_size: 50, has_more: true },
            links: { self: "", next: "?page=2" },
          },
        ],
        pageParams: [1],
      },
    } as unknown as ReturnType<typeof useInfiniteSpendLogEndUsers>);
    const user = userEvent.setup();
    renderFilters();

    await user.click(await screen.findByPlaceholderText("Search an end user"));

    expect(await screen.findByText("cust-a")).toBeInTheDocument();
    expect(screen.getByText("cust-b")).toBeInTheDocument();
  });

  it("loads the next page when the End User list is scrolled near the end", async () => {
    const fetchNextPage = vi.fn();
    vi.mocked(useInfiniteSpendLogEndUsers).mockReturnValue({
      ...emptyInfiniteQuery,
      fetchNextPage,
      hasNextPage: true,
      data: {
        pages: [
          { data: ["cust-a"], meta: { page: 1, page_size: 50, has_more: true }, links: { self: "", next: "?page=2" } },
        ],
        pageParams: [1],
      },
    } as unknown as ReturnType<typeof useInfiniteSpendLogEndUsers>);
    const user = userEvent.setup();
    renderFilters();

    await user.click(await screen.findByPlaceholderText("Search an end user"));
    const list = await screen.findByTestId("paginated-search-select-list");
    Object.defineProperty(list, "scrollTop", { value: 90, configurable: true });
    Object.defineProperty(list, "clientHeight", { value: 10, configurable: true });
    Object.defineProperty(list, "scrollHeight", { value: 100, configurable: true });
    list.dispatchEvent(new Event("scroll", { bubbles: true }));

    await waitFor(() => expect(fetchNextPage).toHaveBeenCalled());
  });

  it("scopes the End User lookup to the window the logs table is showing", async () => {
    const otherWindow = { start_date: "2026-01-01 00:00:00", end_date: "2026-01-02 00:00:00" };
    renderWithProviders(<RequestLogsFilters get={() => undefined} set={vi.fn()} teams={[]} logsWindow={otherWindow} />);

    await waitFor(() => expect(useInfiniteSpendLogEndUsers).toHaveBeenCalledWith(otherWindow, 50, undefined));
  });

  it.each([
    ["", "All Statuses"],
    ["success", "Success"],
    ["failure", "All Statuses"],
  ])("shows the human label on the Status trigger for %s", async (status, label) => {
    renderFilters(status === "" ? {} : { [LOG_FILTER_IDS.STATUS]: status });

    expect(await screen.findByText(label)).toBeInTheDocument();
  });

  it.each([
    ["", "All Requests"],
    ["hit", "Cache Hit"],
    ["miss", "Cache Miss"],
  ])("shows the human label on the Cache trigger for %s", async (cacheState, label) => {
    renderFilters(cacheState === "" ? {} : { [LOG_FILTER_IDS.CACHE_STATUS]: cacheState });

    expect(await screen.findByText(label)).toBeInTheDocument();
  });

  it.each([
    ["Cache Hit", "hit"],
    ["Cache Miss", "miss"],
  ])("selecting %s sets the cache filter to %s", async (label, expected) => {
    const user = userEvent.setup();
    const { set } = renderFilters();

    await user.click(await screen.findByText("All Requests"));
    await user.click(await screen.findByRole("option", { name: label }));

    expect(set).toHaveBeenCalledWith(LOG_FILTER_IDS.CACHE_STATUS, expected);
  });

  it("stores the raw status code when a labeled error code is picked", async () => {
    const user = userEvent.setup();
    const { set } = renderFilters();

    await user.click(await screen.findByPlaceholderText("Select or type an error code"));
    await user.click(await screen.findByRole("option", { name: "429 - Rate Limited" }));

    expect(set).toHaveBeenCalledWith(LOG_FILTER_IDS.ERROR_CODE, "429");
  });

  it("offers every error code again after one was picked", async () => {
    const user = userEvent.setup();
    renderWithProviders(<StatefulFilters />);

    const input = await screen.findByPlaceholderText("Select or type an error code");
    await user.click(input);
    await user.click(await screen.findByRole("option", { name: "429 - Rate Limited" }));
    await user.click(input);

    const list = await screen.findByTestId("error-code-filter-list");
    expect(within(list).getAllByRole("option")).toHaveLength(ERROR_CODE_OPTIONS.length);
    expect(within(list).queryByText(/^Use custom code:/)).not.toBeInTheDocument();
  });

  it("filters by an error code the list does not offer", async () => {
    const user = userEvent.setup();
    const { set } = renderFilters();

    const input = await screen.findByPlaceholderText("Select or type an error code");
    await user.click(input);
    await user.type(input, "418");
    await user.click(await screen.findByRole("option", { name: "Use custom code: 418" }));

    expect(set).toHaveBeenCalledWith(LOG_FILTER_IDS.ERROR_CODE, "418");
  });

  it("selecting All Requests clears the cache filter", async () => {
    const user = userEvent.setup();
    const { set } = renderFilters({ [LOG_FILTER_IDS.CACHE_STATUS]: "hit" });

    await user.click(await screen.findByText("Cache Hit"));
    await user.click(await screen.findByRole("option", { name: "All Requests" }));

    expect(set).toHaveBeenCalledWith(LOG_FILTER_IDS.CACHE_STATUS, undefined);
  });
});
