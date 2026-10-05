import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React, { ReactNode } from "react";
import {
  CAPABILITIES,
  useSessionIdentity,
  useCan,
  useIsPlatformAdmin,
  useIsTeamAdminForAnyTeam,
  useSessionTeamRole,
} from "./useSessionIdentity";

const mockUseAuthorized = vi.fn();
vi.mock("@/app/(dashboard)/hooks/useAuthorized", () => ({
  default: () => mockUseAuthorized(),
}));

vi.mock("@/components/networking", () => ({
  getProxyBaseUrl: vi.fn(() => ""),
  getGlobalLitellmHeaderName: vi.fn(() => "Authorization"),
  deriveErrorMessage: vi.fn((data: { error?: string }) => data?.error || "Error"),
}));

const wrapper = ({ children }: { children: ReactNode }) =>
  React.createElement(
    QueryClientProvider,
    { client: new QueryClient({ defaultOptions: { queries: { retry: false } } }) },
    children,
  );

/** Stubs /auth/me with one answer and records the request it received. */
const stubIdentity = (body: unknown, ok = true) => {
  const fetchMock = vi.fn(() =>
    Promise.resolve({
      ok,
      status: ok ? 200 : 403,
      json: () => Promise.resolve(body),
    } as Response),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
};

const platformAdmin = {
  user_id: "u-1",
  user_role: "admin",
  kind: "session",
  capabilities: [
    CAPABILITIES.profile,
    CAPABILITIES.platformAdmin,
    CAPABILITIES.users,
    CAPABILITIES.teamManage,
  ],
  teams: [{ team_id: "team-1", role: "team_admin" }],
};

describe("useSessionIdentity", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuthorized.mockReturnValue({ accessToken: "sess-abc" });
  });

  it("reads the session's capabilities from the gateway, not from a role claim", async () => {
    const fetchMock = stubIdentity(platformAdmin);
    const { result } = renderHook(() => useSessionIdentity(), { wrapper });

    await waitFor(() => expect(result.current.data).toBeDefined());

    expect(result.current.data?.capabilities).toContain(CAPABILITIES.platformAdmin);
    expect(result.current.data?.user_role).toBe("admin");
    expect(fetchMock).toHaveBeenCalledWith(
      "/auth/me",
      expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer sess-abc" }) }),
    );
  });

  it("carries the per-team role, which the account role cannot express", async () => {
    stubIdentity(platformAdmin);
    const { result } = renderHook(() => useSessionIdentity(), { wrapper });
    await waitFor(() => expect(result.current.data).toBeDefined());

    // The account role is "admin", and the team role is a separate answer. A page
    // that needs to know whether this session administers team-1 must read this,
    // not the account role: team administration is per-team.
    expect(result.current.data?.teams).toEqual([{ team_id: "team-1", role: "team_admin" }]);
  });

  it("does not request anything without a token", () => {
    const fetchMock = stubIdentity(platformAdmin);
    mockUseAuthorized.mockReturnValue({ accessToken: null });
    renderHook(() => useSessionIdentity(), { wrapper });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("surfaces a refusal instead of reporting an empty set", async () => {
    stubIdentity({ error: "forbidden" }, false);
    const { result } = renderHook(() => useSessionIdentity(), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
    // An error is not the same answer as "no capabilities": one means the console
    // cannot tell, the other means the session genuinely holds none. Collapsing
    // them would hide pages for an operator whose request merely failed.
    expect(result.current.data).toBeUndefined();
  });

  it("treats a missing capability list as empty rather than undefined", async () => {
    // A successful answer always carries the lists; this guards the shape rather
    // than a real server response, so a caller can always iterate.
    stubIdentity({ user_id: "u-1", user_role: "user", kind: "session" });
    const { result } = renderHook(() => useSessionIdentity(), { wrapper });
    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.data?.capabilities).toEqual([]);
    expect(result.current.data?.teams).toEqual([]);
  });
});

describe("capability helpers", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuthorized.mockReturnValue({ accessToken: "sess-abc" });
  });

  it("useCan reports a held capability and denies one that is absent", async () => {
    stubIdentity(platformAdmin);
    const { result } = renderHook(
      () => ({ admin: useCan(CAPABILITIES.platformAdmin), audit: useCan(CAPABILITIES.audit) }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.admin).toBe(true));
    expect(result.current.audit).toBe(false);
  });

  it("denies while the answer is still loading", () => {
    stubIdentity(platformAdmin);
    const { result } = renderHook(() => useCan(CAPABILITIES.platformAdmin), { wrapper });
    // Deny, do not allow: a control that appears before the answer and then
    // disappears is worse than one that appears a moment late.
    expect(result.current).toBe(false);
  });

  it("useIsPlatformAdmin and useIsTeamAdminForAnyTeam read different capabilities", async () => {
    // A team administrator is an ordinary account: the account role must not be
    // what the team-administration control reads.
    stubIdentity({
      user_id: "u-2",
      user_role: "user",
      kind: "session",
      capabilities: [CAPABILITIES.profile, CAPABILITIES.teamRead, CAPABILITIES.teamManage],
      teams: [{ team_id: "team-9", role: "team_admin" }],
    });
    const { result } = renderHook(
      () => ({ isAdmin: useIsPlatformAdmin(), isTeamAdmin: useIsTeamAdminForAnyTeam() }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isTeamAdmin).toBe(true));
    expect(result.current.isAdmin).toBe(false);
  });

  it("report a plain member as neither", async () => {
    stubIdentity({
      user_id: "u-3",
      user_role: "user",
      kind: "session",
      capabilities: [CAPABILITIES.profile, CAPABILITIES.teamRead],
      teams: [{ team_id: "team-9", role: "member" }],
    });
    const { result } = renderHook(
      () => ({ isAdmin: useIsPlatformAdmin(), isTeamAdmin: useIsTeamAdminForAnyTeam() }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isAdmin).toBe(false));
    expect(result.current.isTeamAdmin).toBe(false);
  });
});

describe("useSessionTeamRole", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuthorized.mockReturnValue({ accessToken: "sess-abc" });
  });

  it("answers for the named team only", async () => {
    stubIdentity(platformAdmin);
    const { result } = renderHook(
      () => ({
        inTeam: useSessionTeamRole("team-1"),
        otherTeam: useSessionTeamRole("team-2"),
        noTeam: useSessionTeamRole(null),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.inTeam).toBe("team_admin"));
    expect(result.current.otherTeam).toBeNull();
    expect(result.current.noTeam).toBeNull();
  });
});
