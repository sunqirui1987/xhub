import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AuditLogsPage from "./page";
import { uiAuditLogsCall } from "@/components/networking";

const mocks = vi.hoisted(() => ({ authorized: vi.fn(), identity: vi.fn() }));
vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({ default: mocks.authorized }));
vi.mock("@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity")>()),
  useSessionIdentity: mocks.identity,
}));
vi.mock("@/components/networking", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/components/networking")>()),
  uiAuditLogsCall: vi.fn(),
}));

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <AuditLogsPage />
    </QueryClientProvider>,
  );

describe("Audit logs page permissions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.authorized.mockReturnValue({
      accessToken: "admin-session",
      token: "jwt",
      userId: "user-1",
      userRole: "Admin",
      premiumUser: false,
    });
    mocks.identity.mockReturnValue({ data: { capabilities: ["platform.admin", "audit.read"] }, isLoading: false });
    vi.mocked(uiAuditLogsCall).mockResolvedValue({ audit_logs: [], total: 0, page: 1, page_size: 50, total_pages: 0 });
  });

  it("loads audit data for a platform administrator without an enterprise subscription", async () => {
    renderPage();
    await waitFor(() =>
      expect(uiAuditLogsCall).toHaveBeenCalledWith(expect.objectContaining({ accessToken: "admin-session" })),
    );
    expect(screen.getByRole("heading", { name: "Audit Logs" })).toBeInTheDocument();
  });

  it.each([
    ["member", ["logs.self"]],
    ["team administrator", ["teams.manage", "members.manage"]],
    ["organization administrator", ["orgs.manage", "teams.manage"]],
    ["audit capability without platform administration", ["audit.read"]],
    ["platform administration without audit permission", ["platform.admin"]],
  ])("denies direct access to a %s without requesting audit data", (_, capabilities) => {
    mocks.identity.mockReturnValue({ data: { capabilities }, isLoading: false });
    renderPage();
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(uiAuditLogsCall).not.toHaveBeenCalled();
  });

  it("waits for identity verification before requesting audit data", () => {
    mocks.identity.mockReturnValue({ data: undefined, isLoading: true });
    renderPage();
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(uiAuditLogsCall).not.toHaveBeenCalled();
  });

  it("denies access when identity verification fails", () => {
    mocks.identity.mockReturnValue({ data: undefined, isLoading: false, error: new Error("Unavailable") });
    renderPage();
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(uiAuditLogsCall).not.toHaveBeenCalled();
  });
});
