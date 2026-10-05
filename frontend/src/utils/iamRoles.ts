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
