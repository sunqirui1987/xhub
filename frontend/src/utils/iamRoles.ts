/**
 * The account roles, spelled the way the gateway stores them.
 *
 * There are exactly two, and the server's check constraint accepts nothing
 * else: `admin` runs the deployment, `user` reaches what their teams are
 * granted. Team administration is not here because it is per-team, which is a
 * different question from what an account may do globally.
 *
 * The console used to build its role menu from a server-supplied list that no
 * longer exists, so it fell back to a vocabulary from an older product and
 * offered roles the server would refuse. These constants are the whole list.
 */
export const iamRoles = {
  RoleAdmin: "admin",
  RoleUser: "user",
} as const;

export type IamRole = (typeof iamRoles)[keyof typeof iamRoles];

/** isIamRole reports whether a string is one of the two account roles. */
export const isIamRole = (value: unknown): value is IamRole =>
  value === iamRoles.RoleAdmin || value === iamRoles.RoleUser;

/** accountRoleKind recognizes the spellings the console and the gateway have used. Anything else is not an account role. */
export const accountRoleKind = (role: string | null | undefined): IamRole | null => {
  const normalized = (role ?? "").toLowerCase().replace(/\s+/g, "_");
  if (normalized === "admin" || normalized === "proxy_admin") return iamRoles.RoleAdmin;
  if (normalized === "user" || normalized === "internal_user") return iamRoles.RoleUser;
  return null;
};

/** normalizeAccountRole maps a role onto the two stored values. An empty or unknown role becomes a regular user. */
export const normalizeAccountRole = (role: string | null | undefined): IamRole =>
  accountRoleKind(role) ?? iamRoles.RoleUser;

/** isTeamAdminMembership reports a team role, which is not an account role. */
export const isTeamAdminMembership = (role: string | null | undefined): boolean => {
  const normalized = (role ?? "").toLowerCase();
  return normalized === "team_admin" || normalized === "admin";
};
