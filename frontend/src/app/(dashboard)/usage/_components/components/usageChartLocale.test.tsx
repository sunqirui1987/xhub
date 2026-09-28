import { useCustomers } from "@/app/(dashboard)/hooks/customers/useCustomers";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import useIsOrgAdmin from "@/app/(dashboard)/hooks/useIsOrgAdmin";
import { useCurrentUser } from "@/app/(dashboard)/hooks/users/useCurrentUser";
import { screen, waitFor } from "@testing-library/react";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "@/../tests/test-utils";
import * as networking from "@/components/networking";
import { setActiveLocale } from "@/i18n/runtime";
import UsagePage from "./UsagePageView";

beforeAll(() => {
  if (typeof window !== "undefined" && !window.ResizeObserver) {
    window.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as any;
  }
});

vi.mock("@/components/networking", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/networking")>();
  return {
    ...actual,
    userDailyActivityCall: vi.fn(),
    userDailyActivityAggregatedCall: vi.fn(),
    gatewayDailyActivityCall: vi.fn(),
    tagListCall: vi.fn(),
    modelAvailableCall: vi.fn(),
    userListCall: vi.fn(),
    keyInfoV1Call: vi.fn(),
  };
});

vi.mock("@/app/(dashboard)/hooks/customers/useCustomers", () => ({
  useCustomers: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  __esModule: true,
  default: vi.fn(),
}));

vi.mock("@/app/(dashboard)/hooks/useIsOrgAdmin", () => ({
  __esModule: true,
  default: vi.fn(() => false),
}));

vi.mock("@/app/(dashboard)/hooks/users/useCurrentUser", () => ({
  useCurrentUser: vi.fn(),
}));

const spend = {
  spend: 1.25,
  prompt_tokens: 100,
  completion_tokens: 40,
  total_tokens: 140,
  api_requests: 3,
  successful_requests: 2,
  failed_requests: 1,
  cache_read_input_tokens: 10,
  cache_creation_input_tokens: 4,
};

const modelRow = {
  metrics: spend,
  metadata: {},
  api_key_breakdown: {
    "": {
      metrics: spend,
      metadata: { key_alias: null, team_id: null },
    },
  },
};

const day = (date: string) => ({
  date,
  metrics: spend,
  breakdown: {
    models: { "gpt-6-astra": modelRow },
    model_groups: { "gpt-6-astra": modelRow },
    mcp_servers: {},
    providers: {},
    api_keys: {
      "": {
        metrics: spend,
        metadata: { key_alias: null, team_id: null },
      },
    },
    entities: {},
    endpoints: {
      "/chat/completions": {
        metrics: spend,
        metadata: {},
        api_key_breakdown: {},
      },
    },
  },
});

const activity = {
  results: [day("2026-09-28"), day("2026-09-25")],
  metadata: {
    total_spend: 2.5,
    total_api_requests: 6,
    total_successful_requests: 4,
    total_failed_requests: 2,
    total_tokens: 280,
    total_prompt_tokens: 200,
    total_completion_tokens: 80,
    total_cache_read_input_tokens: 20,
    total_cache_creation_input_tokens: 8,
  },
};

describe("usage charts in zh-CN", () => {
  beforeEach(() => {
    vi.mocked(useAuthorized).mockReturnValue({
      isLoading: false,
      isAuthorized: true,
      token: "mock-token",
      accessToken: "test-token",
      userId: "admin",
      userEmail: "admin",
      userRole: "Admin",
      premiumUser: true,
      disabledPersonalKeyCreation: false,
      showSSOBanner: false,
    });
    vi.mocked(useCurrentUser).mockReturnValue({
      data: { user_id: "admin", max_budget: null },
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useCustomers).mockReturnValue({ data: [], isLoading: false, error: null } as any);
    vi.mocked(useIsOrgAdmin).mockReturnValue(false);
    vi.mocked(networking.userDailyActivityAggregatedCall).mockResolvedValue(activity);
    vi.mocked(networking.gatewayDailyActivityCall).mockResolvedValue({
      total_successful_requests: 4,
      total_failed_requests: 2,
      by_date: [{ date: "2026-09-25", successful_requests: 4, failed_requests: 2 }],
      by_route: [{ category: "llm", route: "/chat/completions", successful_requests: 4, failed_requests: 2 }],
    });
    vi.mocked(networking.tagListCall).mockResolvedValue({});
    vi.mocked(networking.modelAvailableCall).mockResolvedValue({ data: [] });
    vi.mocked(networking.userListCall).mockResolvedValue({
      users: [],
      page: 1,
      total_pages: 1,
      total_count: 0,
    });
  });

  it("translates cost, model, key, and endpoint chart copy", async () => {
    setActiveLocale("zh-CN");
    renderWithProviders(<UsagePage teams={[]} organizations={[]} />);

    await waitFor(() => {
      expect(screen.getByTestId("gateway-requests-by-endpoint")).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(document.body.textContent).toContain("总体用量");
    });

    const text = document.body.textContent ?? "";
    for (const broken of [
      "metrics.spend",
      "failed_requests",
      "successful_requests",
      "Overall 用量",
      "总计 Successful 请求",
      "每 successful 请求",
      "请求每天",
      "提示词 Caching",
      "缓存 Creation",
      "缓存 Read",
      "型号用途",
      "超过时间",
      "Sep ",
      "否数据",
      "No provider usage data",
      "平均费用每请求",
      "成功费率",
    ]) {
      expect(text, broken).not.toContain(broken);
    }

    for (const phrase of [
      "总体用量",
      "成功请求总数",
      "每日请求",
      "提示词缓存指标",
      "缓存读取：",
      "缓存创建：",
      "请求随时间变化",
      "令牌随时间变化",
      "模型用量",
      "消费",
      "成功请求",
      "失败请求",
      "接口请求",
      "提示词令牌",
      "补全令牌",
      "缓存读取输入令牌",
      "缓存创建输入令牌",
      "key-hash-",
      "/chat/completions",
      "9月25日",
      "9月28日",
      "gpt-6-astra",
      "每请求平均费用",
      "每次成功请求平均",
      "成功率",
    ]) {
      expect(text, phrase).toContain(phrase);
    }
  });
});
