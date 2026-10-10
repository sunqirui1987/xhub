import { iamRoles } from "@/utils/iamRoles";

/** 新建个人表单收集账户、唯一团队及金额/RPM/TPM，留空速率共享上级剩余容量。 */
export interface CreateUserFormValues {
  user_email: string;
  user_alias: string;
  password: string;
  user_role: string;
  max_budget: string;
  rpm_limit: string;
  tpm_limit: string;
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
  rpm_limit: "",
  tpm_limit: "",
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
  rpm_limit?: number;
  tpm_limit?: number;
  team_id?: string;
  team_role?: string;
}

/** The field that stopped the form, or null when every value is usable. */
export type CreateUserProblem = "email" | "password" | "budget" | "rate" | null;

/** 校验个人创建参数，返回首个错误字段或 null；供提交前使用，拒绝空固定预算和非法速率，无写入副作用。 */
export function validateCreateUser(values: CreateUserFormValues): CreateUserProblem {
  if (!values.user_email.trim()) return "email";
  if (values.password.length < 8) return "password";
  if (!values.unlimited_budget) {
    const budget = Number(values.max_budget);
    if (!values.max_budget.trim() || !Number.isFinite(budget) || budget < 0) return "budget";
  }
  if (!isValidRateAllocation(values.rpm_limit) || !isValidRateAllocation(values.tpm_limit)) return "rate";
  return null;
}

/** 将已校验表单转换为创建接口参数；留空额度省略、0 保留，团队角色与唯一团队一起提交，返回请求体。 */
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
  for (const field of ["rpm_limit", "tpm_limit"] as const) {
    if (values[field].trim()) payload[field] = Number(values[field]);
  }
  return payload;
}

/** 校验共享或固定速率；参数为表单字符串、数字或 undefined，返回是否合法；供个人和团队创建校验，整数范围与后台一致。 */
export function isValidRateAllocation(value: string | number | undefined): boolean {
  if (value === undefined || String(value).trim() === "") return true;
  const raw = String(value).trim();
  return /^\d+$/.test(raw) && Number(raw) <= 2147483647;
}
