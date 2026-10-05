import { useQuery, UseQueryResult } from "@tanstack/react-query";
import { getProxyBaseUrl, getGlobalLitellmHeaderName, deriveErrorMessage } from "@/components/networking";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { createQueryKeys } from "@/app/(dashboard)/hooks/common/queryKeysFactory";

/**
 * The capabilities the gateway says this session holds.
 *
 * They are a fixed vocabulary, declared once in the Go authorization core and
 * emitted by /auth/me from the same roles the decision matrix reads. The console
 * names them here rather than deriving them from a role string, so a page cannot
 * offer an action the server would refuse.
 */
export const CAPABILITIES = {
  profile: "profile.self",
  personalKeys: "keys.personal",
  selfUsage: "usage.self",
  selfLogs: "logs.self",
  infer: "infer",
  teamRead: "teams.read",
  teamManage: "teams.manage",
  members: "members.manage",
  projects: "projects.manage",
  serviceKeys: "keys.service",
  teamUsage: "usage.team",
  teamUsageDetail: "usage.team.detail",
  platformAdmin: "platform.admin",
  users: "users.manage",
  orgs: "orgs.manage",
  teamsPlatform: "teams.platform",
  accessGroups: "access_groups.manage",
  globalUsage: "usage.global",
  audit: "audit.read",
} as const;

export type Capability = (typeof CAPABILITIES)[keyof typeof CAPABILITIES];

/** One team the session belongs to, and the role it holds there. */
export interface SessionTeam {
  team_id: string;
  role: "team_admin" | "member";
}

export interface SessionIdentity {
  user_id: string;
  /** The account role: `admin` or `user`, not the old console label. */
  user_role: string;
  /** `session` for a console login, `key` or `master` otherwise. */
  kind: string;
  capabilities: Capability[];
  teams: SessionTeam[];
}

const identityKeys = createQueryKeys("sessionIdentity");

const fetchIdentity = async (accessToken: string): Promise<SessionIdentity> => {
  const baseUrl = getProxyBaseUrl();
  const response = await fetch(`${baseUrl}/auth/me`, {
    method: "GET",
    headers: {
      [getGlobalLitellmHeaderName()]: `Bearer ${accessToken}`,
      "Content-Type": "application/json",
    },
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(deriveErrorMessage(errorData) || `Identity request failed (${response.status})`);
  }

  const data = (await response.json()) as Partial<SessionIdentity>;
  // A response that omits the lists must not read as "no capabilities": that
  // would hide every page rather than fail loudly, and the two are different
  // answers. An absent list is an empty one only when it is explicitly absent
  // from a successful answer, which is what the gateway always sends.
  return {
    user_id: data.user_id ?? "",
    user_role: data.user_role ?? "",
    kind: data.kind ?? "",
    capabilities: Array.isArray(data.capabilities) ? data.capabilities : [],
    teams: Array.isArray(data.teams) ? data.teams : [],
  };
};

/**
 * useSessionIdentity reads what the session may do, from the server.
 *
 * The console used to infer this from a JWT claim naming a role, which is why it
 * could not tell a team administrator from a plain member: the claim carries the
 * account role, and team administration is per-team. The server answers that
 * question directly, so the page reads the answer instead of guessing.
 */
export const useSessionIdentity = (): UseQueryResult<SessionIdentity> => {
  const { accessToken } = useAuthorized();
  return useQuery<SessionIdentity>({
    queryKey: identityKeys.list({}),
    queryFn: () => fetchIdentity(accessToken!),
    enabled: Boolean(accessToken),
    staleTime: 30000,
  });
};

/**
 * useCan reports whether the session holds a capability.
 *
 * It is deliberately a plain boolean with no role fallback. A page that cannot
 * read its identity yet sees "not yet", not "yes because the role looked right":
 * a false negative hides a control until the answer arrives, while a false
 * positive offers an action that fails at the server.
 */
export const useCan = (capability: Capability): boolean => {
  const { data } = useSessionIdentity();
  return Boolean(data?.capabilities?.includes(capability));
};

/** useCapabilities returns the whole set, for a page that gates several things. */
export const useCapabilities = (): ReadonlySet<Capability> => {
  const { data } = useSessionIdentity();
  return new Set(data?.capabilities ?? []);
};

/** useIsPlatformAdmin reports the account-level administrator role. */
export const useIsPlatformAdmin = (): boolean => useCan(CAPABILITIES.platformAdmin);

/** useSessionTeamRole reports the role this session holds in one team, if any. */
export const useSessionTeamRole = (teamId: string | null | undefined): SessionTeam["role"] | null => {
  const { data } = useSessionIdentity();
  if (!data || !teamId) return null;
  return data.teams.find((team) => team.team_id === teamId)?.role ?? null;
};

/**
 * useIsTeamAdminForAnyTeam reports whether the session administers a team.
 *
 * It reads the team list rather than the teamManage capability. A platform
 * administrator holds teamManage without being a team's administrator, and the
 * question here is about team membership: a page that asks it wants to know
 * whether to offer team-level management, not whether the caller is powerful.
 */
export const useIsTeamAdminForAnyTeam = (): boolean => {
  const { data } = useSessionIdentity();
  return Boolean(data?.teams?.some((team) => team.role === "team_admin"));
};
