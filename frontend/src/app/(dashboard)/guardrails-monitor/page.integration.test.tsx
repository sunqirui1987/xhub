import { screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import GuardrailsMonitor from "./page";
import { renderWithProviders, testQueryClient } from "../../../../tests/test-utils";

const { useAuthorizedMock } = vi.hoisted(() => ({ useAuthorizedMock: vi.fn() }));

/** 按服务器能力验证访问门禁；角色夹具只用于组织场景，能力不由页面自行推导。 */
vi.mock("@/app/(dashboard)/hooks/useCan", () => ({
  default: (capability: string) => capability === "viewGuardrailUsage" &&
    useAuthorizedMock().capabilities?.includes("platform_admin"),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: useAuthorizedMock,
}));

const fetchMock = vi.fn();

const requestUrl = (input: RequestInfo | URL) => (input instanceof Request ? input.url : String(input));

const requestedUrls = () => fetchMock.mock.calls.map(([input]) => requestUrl(input));

const emptyOverview = {
  rows: [],
  chart: [],
  totalRequests: 0,
  totalBlocked: 0,
  passRate: 100,
  totalUsageUnits: {},
  totalCost: null,
  totalUntrackedUsageUnits: {},
};

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });

const renderAs = (userRole: string) => {
  useAuthorizedMock.mockReturnValue({
    accessToken: "sk-test", userId: "u1", userRole,
    capabilities: userRole === "Admin" ? ["platform_admin"] : [],
  });
  return renderWithProviders(<GuardrailsMonitor />);
};

// `/guardrails/usage/*` aggregates across tenants and is listed in
// admin_viewer_routes, so it is proxy-admin-only. Nothing on this page works
// for a non-admin, hence the whole page is gated rather than a section of it.
describe("Guardrails Monitor page access by role", () => {
  beforeEach(() => {
    testQueryClient.clear();
    vi.clearAllMocks();
    fetchMock.mockImplementation(async (input: RequestInfo | URL) =>
      jsonResponse(requestUrl(input).includes("/guardrails/usage/overview") ? emptyOverview : []),
    );
    vi.stubGlobal("fetch", fetchMock);
  });

  it("fetches the guardrails usage overview for an admin", async () => {
    renderAs("Admin");

    await waitFor(() => expect(requestedUrls().some((url) => url.includes("/guardrails/usage/overview"))).toBe(true));
  });

  it.each(["Internal User", "Internal Viewer", "Org Admin", "Unknown Role"])(
    "renders the admin-only notice and fires no usage request for %s",
    async (userRole) => {
      renderAs(userRole);

      expect(await screen.findByText("Guardrails Monitor is only available to admin users.")).toBeInTheDocument();
      await waitFor(() => expect(requestedUrls().filter((url) => url.includes("/guardrails/usage"))).toEqual([]));
    },
  );
});

/** 固定导入期英文翻译，验证表单和接口契约；本组不测试语言切换，DOM 由框架自动清理。 */
vi.mock("@/i18n", async () => {
  const { translate } = await import("@/i18n/translate");
  return { getActiveLocale: () => "en", t: (key: string, params?: Record<string, string | number>) => translate("en", key, params) };
});
