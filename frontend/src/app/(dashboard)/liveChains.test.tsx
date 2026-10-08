import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { NuqsTestingAdapter } from "nuqs/adapters/testing";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/components/networking", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/networking")>();
  return {
    ...actual,
    getCallbacksCall: async () => ({ router_settings: {} }),
  };
});

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => "/api-keys",
  useSearchParams: () => new URLSearchParams(),
  notFound: () => {
    throw new Error("not-found");
  },
}));

import LoginPage from "@/app/login/page";
import ApiKeysPage from "./api-keys/page";
import PlaygroundPage from "./playground/page";
import ModelsPage from "./models-and-endpoints/page";
import GuardrailsPage from "./guardrails/page";
import UsagePage from "./usage/page";
import LogsPage from "./logs/page";
import GuardrailsMonitorPage from "./guardrails-monitor/page";
import TeamsPage from "./teams/page";
import ProjectsPage from "./projects/page";
import UsersPage from "./users/page";
import OrganizationsPage from "./organizations/page";
import RouteTemplatesPage from "./route-templates/page";
import AdminPanelPage from "./admin-panel/page";
import { dashboardAppPath } from "@/middleware";

const livePages = [
  ["login", "/login", LoginPage],
  ["api-keys", "/api-keys", ApiKeysPage],
  ["playground", "/playground", PlaygroundPage],
  ["models-and-endpoints", "/models-and-endpoints", ModelsPage],
  ["guardrails", "/guardrails", GuardrailsPage],
  ["usage", "/usage", UsagePage],
  ["logs", "/logs", LogsPage],
  ["guardrails-monitor", "/guardrails-monitor", GuardrailsMonitorPage],
  ["teams", "/teams", TeamsPage],
  ["projects", "/projects", ProjectsPage],
  ["users", "/users", UsersPage],
  ["organizations", "/organizations", OrganizationsPage],
  ["route-templates", "/route-templates", RouteTemplatesPage],
  ["admin-panel", "/admin-panel", AdminPanelPage],
] as const;

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      ok: true,
      status: 200,
      headers: { get: () => "application/json" },
      json: async () => ({
        data: [],
        keys: [],
        teams: [],
        users: [],
        models: [],
        organizations: [],
        guardrails: [],
        values: {},
        current_values: {},
        router_settings: {},
        field_schema: {},
        fields: [],
        object: "list",
      }),
      text: async () => "{}",
    })),
  );
});

function mount(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <NuqsTestingAdapter>
      <QueryClientProvider client={client}>{node}</QueryClientProvider>
    </NuqsTestingAdapter>,
  );
}

describe("live dashboard chains", () => {
  it.each(livePages)("%s renders its page and stays a dashboard route", (name, path, Page) => {
    expect(dashboardAppPath(path), name).toBe(path);
    const view = mount(<Page />);
    expect(view.container.textContent?.trim().length).toBeGreaterThan(0);
  });
});
