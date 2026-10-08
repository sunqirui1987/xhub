import { z } from "zod/v4";

import type { Organization } from "@/components/networking";
import type { components } from "@/lib/http/schema";

import type { OrgSettingsFormValues } from "./schema";

export type OrgPatchBody = components["schemas"]["OrganizationUpdateRequestV2"] & {
  route_template_id?: string;
};

const budgetSliceSchema = z.object({
  max_budget: z.number().nullish(),
  budget_duration: z.string().nullish(),
  tpm_limit: z.number().nullish(),
  rpm_limit: z.number().nullish(),
});

const metadataRecordSchema = z.record(z.string(), z.unknown());

export const orgToForm = (org: Organization): OrgSettingsFormValues => {
  const budget = budgetSliceSchema.parse(org.litellm_budget_table ?? {});
  const maxBudget = typeof org.max_budget === "number" ? org.max_budget : budget.max_budget;
  return {
    organization_alias: org.organization_alias ?? "",
    models: org.models ?? [],
    max_budget: maxBudget === undefined || maxBudget === null ? "" : String(maxBudget),
    budget_duration: budget.budget_duration ?? "",
    tpm_limit: budget.tpm_limit?.toString() ?? "",
    rpm_limit: budget.rpm_limit?.toString() ?? "",
    metadata: org.metadata && Object.keys(org.metadata).length > 0 ? JSON.stringify(org.metadata, null, 2) : "",
    route_template_id: org.route_template_id ?? "",
  };
};

const numberOrNull = (raw: string): number | null => (raw.trim() === "" ? null : Number(raw));

const metadataOrNull = (raw: string): OrgPatchBody["metadata"] =>
  raw.trim() === "" ? null : metadataRecordSchema.parse(JSON.parse(raw));

export const buildOrgPatch = (dirty: Partial<OrgSettingsFormValues>): OrgPatchBody => {
  return {
    ...(dirty.organization_alias !== undefined && { organization_alias: dirty.organization_alias }),
    ...(dirty.models !== undefined && { models: dirty.models }),
    ...(dirty.max_budget !== undefined && { max_budget: numberOrNull(dirty.max_budget) }),
    ...(dirty.tpm_limit !== undefined && { tpm_limit: numberOrNull(dirty.tpm_limit) }),
    ...(dirty.rpm_limit !== undefined && { rpm_limit: numberOrNull(dirty.rpm_limit) }),
    ...(dirty.budget_duration !== undefined && {
      budget_duration: dirty.budget_duration === "" ? null : dirty.budget_duration,
    }),
    ...(dirty.metadata !== undefined && { metadata: metadataOrNull(dirty.metadata) }),
    ...(dirty.route_template_id !== undefined && { route_template_id: dirty.route_template_id }),
  };
};
