"use client";

import { useCapabilities, CAPABILITIES, type Capability as ServerCapability } from "@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity";

/**
 * Console capability names, mapped onto the capabilities the gateway reports.
 *
 * The console used to decide this from a role string in the session token. That
 * could not express team administration, which is per-team, and it kept names
 * from a vocabulary the server no longer has. Each entry here names the server
 * capability that actually answers it, so the UI and the server read the same
 * list.
 *
 * A page whose feature was removed maps to a capability that is never granted
 * to anyone, and so hides itself. That is deliberate: an entry pointing at a
 * capability nobody can hold is a page nobody should see.
 */
const CAPABILITY_MAP = {
  viewToolPolicies: CAPABILITIES.platformAdmin,
  viewAuditLogs: CAPABILITIES.audit,
  viewDeletedTeams: CAPABILITIES.teamsPlatform,
  viewPolicies: CAPABILITIES.platformAdmin,
  viewPrompts: CAPABILITIES.platformAdmin,
  viewOrganizationUsage: CAPABILITIES.globalUsage,
  viewAgentUsage: CAPABILITIES.platformAdmin,
  viewGlobalSpend: CAPABILITIES.globalUsage,
  viewWorkflowRuns: CAPABILITIES.platformAdmin,
  viewMemory: CAPABILITIES.platformAdmin,
  viewGuardrailUsage: CAPABILITIES.platformAdmin,
  viewProxyWideCostData: CAPABILITIES.globalUsage,
  viewAccessGroups: CAPABILITIES.platformAdmin,
  manageProjects: CAPABILITIES.projects,
  manageMembers: CAPABILITIES.members,
  manageTeam: CAPABILITIES.teamManage,
} as const satisfies Record<string, ServerCapability>;

export type Capability = keyof typeof CAPABILITY_MAP;

/**
 * hasCapability reports whether a server capability set answers a console
 * capability. It takes the set rather than a role, so a caller cannot decide by
 * role: the role is not what the server checks.
 */
export const hasCapability = (
  capabilities: ReadonlySet<ServerCapability>,
  capability: Capability,
): boolean => capabilities.has(CAPABILITY_MAP[capability]);

/**
 * useCan reads one capability from the session identity the gateway reports.
 *
 * It denies while the answer is still loading. A control that appears and then
 * disappears is worse than one that appears a moment late, and offering an
 * action the server would refuse is worse than both.
 */
const useCan = (capability: Capability): boolean =>
  hasCapability(useCapabilities(), capability);

export default useCan;
