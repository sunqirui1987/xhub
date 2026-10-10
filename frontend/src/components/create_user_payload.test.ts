import { describe, expect, it } from "vitest";
import {
  isValidRateAllocation,
  buildCreateUserPayload,
  EMPTY_CREATE_USER_FORM,
  validateCreateUser,
  type CreateUserFormValues,
} from "./create_user_payload";

const form = (overrides: Partial<CreateUserFormValues> = {}): CreateUserFormValues => ({
  ...EMPTY_CREATE_USER_FORM,
  user_email: "ada@example.com",
  password: "correct-horse",
  ...overrides,
});

describe("validateCreateUser", () => {
  it("accepts a form with an email, a long enough password and no budget", () => {
    expect(validateCreateUser(form())).toBeNull();
  });

  it("refuses an account with no email", () => {
    expect(validateCreateUser(form({ user_email: "   " }))).toBe("email");
  });

  it("refuses a password shorter than the eight the server hashes", () => {
    expect(validateCreateUser(form({ password: "short" }))).toBe("password");
    expect(validateCreateUser(form({ password: "eightsym" }))).toBeNull();
  });

  it("refuses a budget that is not a number, but only when one was asked for", () => {
    expect(validateCreateUser(form({ unlimited_budget: false, max_budget: "abc" }))).toBe("budget");
    expect(validateCreateUser(form({ unlimited_budget: false, max_budget: "-5" }))).toBe("budget");
    expect(validateCreateUser(form({ unlimited_budget: false, max_budget: "25.5" }))).toBeNull();
  });

  it("does not read the budget field while the account is unlimited", () => {
    expect(validateCreateUser(form({ unlimited_budget: true, max_budget: "not-a-number" }))).toBeNull();
  });
});

describe("buildCreateUserPayload", () => {
  it("omits the budget rather than sending zero when the account is unlimited", () => {
    const payload = buildCreateUserPayload(form({ unlimited_budget: true, max_budget: "10" }));
    expect(payload).not.toHaveProperty("max_budget");
  });

  it("sends the typed budget as a number when a ceiling was asked for", () => {
    const payload = buildCreateUserPayload(form({ unlimited_budget: false, max_budget: "25.5" }));
    expect(payload.max_budget).toBe(25.5);
  });

  it("omits the team fields when no team was chosen", () => {
    const payload = buildCreateUserPayload(form());
    expect(payload).not.toHaveProperty("team_id");
    expect(payload).not.toHaveProperty("team_role");
  });

  it("sends the team and its role together, and never a platform role as a team role", () => {
    const payload = buildCreateUserPayload(form({ team_id: "team-1", team_role: "admin" }));
    expect(payload.team_id).toBe("team-1");
    expect(payload.team_role).toBe("admin");

    const escalated = buildCreateUserPayload(form({ team_id: "team-1", team_role: "proxy_admin" }));
    expect(escalated.team_role).toBe("user");
  });

  it("never sends an organization on account creation", () => {
    expect(buildCreateUserPayload(form())).not.toHaveProperty("organization_id");
  });

  it("trims the address and the display name", () => {
    const payload = buildCreateUserPayload(form({ user_email: " ada@example.com ", user_alias: " Ada " }));
    expect(payload.user_email).toBe("ada@example.com");
    expect(payload.user_alias).toBe("Ada");
  });

  it("defaults an unset role to a regular user and never asks for a virtual key", () => {
    const payload = buildCreateUserPayload(form({ user_role: "" }));
    expect(payload.user_role).toBe("user");
    expect(payload.auto_create_key).toBe(false);
  });
});

/** 新建配额纯函数测试：正常、边界和失败输入，验证共享与 0 区分；不写入数据，无需清理。 */
describe("personal creation rate allocations", () => {
  it.each(["", "  ", "0", "12", "2147483647", 0, undefined])("accepts shared or integer rate %s", value => {
    expect(isValidRateAllocation(value)).toBe(true);
  });
  it.each(["-1", "1.5", "NaN", "2147483648", "1e2", "abc", Infinity])("rejects invalid rate %s", value => {
    expect(isValidRateAllocation(value)).toBe(false);
    expect(validateCreateUser(form({ rpm_limit: String(value) }))).toBe("rate");
    expect(validateCreateUser(form({ tpm_limit: String(value) }))).toBe("rate");
  });
  it("preserves zero and fixed rates while omitting shared fields", () => {
    expect(buildCreateUserPayload(form({ rpm_limit: "0", tpm_limit: "128" }))).toMatchObject({ rpm_limit: 0, tpm_limit: 128 });
    expect(buildCreateUserPayload(form())).not.toHaveProperty("rpm_limit");
    expect(buildCreateUserPayload(form())).not.toHaveProperty("tpm_limit");
    expect(validateCreateUser(form({ unlimited_budget: false, max_budget: "" }))).toBe("budget");
  });
});
