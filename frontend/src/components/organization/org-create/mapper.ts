import { z } from "zod/v4";

import type { components } from "@/lib/http/schema";

import type { OrgSettingsFormValues } from "../org-settings/schema";

export type OrgCreateBody = components["schemas"]["NewOrganizationRequest"];

export const emptyOrgFormValues: OrgSettingsFormValues = {
  organization_alias: "",
  models: [],
  max_budget: "",
  budget_duration: "",
  tpm_limit: "",
  rpm_limit: "",
  metadata: "",
};

const metadataRecordSchema = z.record(z.string(), z.unknown());

export const buildOrgCreateBody = (values: OrgSettingsFormValues): OrgCreateBody => {
  return {
    organization_alias: values.organization_alias,
    models: values.models,
    ...(values.max_budget.trim() !== "" && { max_budget: Number(values.max_budget) }),
    ...(values.tpm_limit.trim() !== "" && { tpm_limit: Number(values.tpm_limit) }),
    ...(values.rpm_limit.trim() !== "" && { rpm_limit: Number(values.rpm_limit) }),
    ...(values.budget_duration !== "" && { budget_duration: values.budget_duration }),
    ...(values.metadata.trim() !== "" && { metadata: metadataRecordSchema.parse(JSON.parse(values.metadata)) }),
  };
};
