import { screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SpendLogsTable from "./index";
import { renderWithProviders, testQueryClient } from "../../../tests/test-utils";

const { useAuthorizedMock, useOrganizationsMock } = vi.hoisted(() => ({
  useAuthorizedMock: vi.fn(),
  useOrganizationsMock: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: useAuthorizedMock,
}));

vi.mock("@/app/(dashboard)/hooks/useCan", () => ({
  default: function useCanMock(capability: string) {
    const { userRole } = useAuthorizedMock();
    return capability === "viewAuditLogs" && userRole === "Admin";
  },
}));

vi.mock("@/app/(dashboard)/hooks/organizations/useOrganizations", () => ({
  useOrganizations: useOrganizationsMock,
}));

vi.mock("./RequestLogsPanel", () => ({
  default: function RequestLogsPanelMock() {
    return <div data-testid="request-logs-panel" />;
  },
}));

const fetchMock = vi.fn();

const jsonResponse = (body: unknown) => ({
  ok: true,
  status: 200,
  statusText: "OK",
  json: async () => body,
});

const requestedUrls = () => fetchMock.mock.calls.map(([url]) => String(url));

const emptyAuditLogs = { audit_logs: [], total: 0, page: 1, page_size: 50, total_pages: 0 };

const defaultProps = {
  accessToken: "sk-test",
  token: "jwt-test",
  userRole: "Admin",
  userID: "user-1",
  premiumUser: true,
};

const ORG_ADMIN_MEMBERSHIPS = [{ organization_id: "org-1", members: [{ user_id: "user-1", user_role: "org_admin" }] }];

/** 构造指定角色及组织成员的日志页；返回测试渲染句柄，不访问真实服务，由测试框架卸载并清理缓存。 */
const renderAs = (sessionRole: string, organizations: unknown[] = []) => {
  useAuthorizedMock.mockReturnValue({
    accessToken: "sk-test",
    userId: "user-1",
    userRole: sessionRole,
    premiumUser: true,
  });
  useOrganizationsMock.mockReturnValue({ data: organizations });
  return renderWithProviders(<SpendLogsTable {...defaultProps} userRole={sessionRole} />);
};

describe("SpendLogsTable network access by role", () => {
  beforeEach(() => {
    testQueryClient.clear();
    vi.clearAllMocks();
    useOrganizationsMock.mockReturnValue({ data: [] });
    fetchMock.mockImplementation(async (url: string) => {
      if (String(url).includes("/audit")) {
        return jsonResponse(emptyAuditLogs);
      }
      if (String(url).includes("/v2/team/list")) {
        return jsonResponse({ teams: [] });
      }
      return jsonResponse({ keys: [], total_count: 0 });
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  /** 前置普通用户或组织管理员会话；验证两种日志入口可见且不请求审计资源，测试缓存由 beforeEach 清理。 */
  it.each([
    ["Internal User", []],
    ["Internal Viewer", []],
    ["Internal User", ORG_ADMIN_MEMBERSHIPS],
  ])("does not request audit or deleted resources for %s", async (role, organizations) => {
    renderAs(role as string, organizations as unknown[]);
    expect(screen.getByTestId("request-logs-panel")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Error Logs" })).toBeInTheDocument();
    expect(requestedUrls()).toEqual([]);
  });

  /** 前置平台管理员会话；验证错误日志入口不引入审计接口请求，模拟网络及组件由测试框架清理。 */
  it("does not request audit logs from the request logs page even for an admin", () => {
    renderAs("Admin");
    expect(screen.getByRole("tab", { name: "Error Logs" })).toBeInTheDocument();
    expect(requestedUrls()).toEqual([]);
  });
});
