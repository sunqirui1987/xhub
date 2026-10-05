import { iamRoles } from "@/utils/iamRoles";

/**
 * The values the create-account form collects.
 *
 * The account, its budget and its team membership travel in one request.
 * Organization administration is not collected here.
 */
export interface CreateUserFormValues {
  user_email: string;
  user_alias: string;
  password: string;
  user_role: string;
  max_budget: string;
  unlimited_budget: boolean;
  team_id: string | null;
  team_role: string;
}

export const EMPTY_CREATE_USER_FORM: CreateUserFormValues = {
  user_email: "",
  user_alias: "",
  password: "",
  user_role: iamRoles.RoleUser,
  max_budget: "",
  unlimited_budget: true,
  team_id: null,
  team_role: "user",
};

/** The payload POST /user/new accepts, built from the form values. */
export interface CreateUserPayload {
  user_email: string;
  user_alias: string;
  password: string;
  user_role: string;
  auto_create_key: false;
  max_budget?: number;
  team_id?: string;
  team_role?: string;
}

/** The field that stopped the form, or null when every value is usable. */
export type CreateUserProblem = "email" | "password" | "budget" | null;

/**
 * validateCreateUser reports the first field the server would refuse.
 *
 * The password length is the server's own rule (iam.hashPassword refuses
 * anything under eight characters), repeated here so the answer arrives without
 * a round trip. It is not the source of truth: a client that skipped this would
 * still get a 400.
 */
export function validateCreateUser(values: CreateUserFormValues): CreateUserProblem {
  if (!values.user_email.trim()) return "email";
  if (values.password.length < 8) return "password";
  if (!values.unlimited_budget) {
    const budget = Number(values.max_budget);
    if (!Number.isFinite(budget) || budget < 0) return "budget";
  }
  return null;
}

/**
 * buildCreateUserPayload turns the form into the request body.
 *
 * A field is omitted rather than sent empty when the form left it blank. That
 * distinction matters at the server: a team role with no team is a rejected
 * request, and an empty budget string parsed as a number would be zero, which
 * is a spending ceiling rather than no ceiling.
 */
export function buildCreateUserPayload(values: CreateUserFormValues): CreateUserPayload {
  const payload: CreateUserPayload = {
    user_email: values.user_email.trim(),
    user_alias: values.user_alias.trim(),
    password: values.password,
    user_role: values.user_role || iamRoles.RoleUser,
    auto_create_key: false,
  };
  if (!values.unlimited_budget) {
    payload.max_budget = Number(values.max_budget);
  }
  if (values.team_id) {
    payload.team_id = values.team_id;
    payload.team_role = values.team_role === "admin" ? "admin" : "user";
  }
  return payload;
}
