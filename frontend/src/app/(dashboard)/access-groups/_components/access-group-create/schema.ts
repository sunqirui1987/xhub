import { z } from "zod/v4";
import { t } from "@/i18n";

export const accessGroupCreateSchema = z.object({
  name: z.string().refine((value) => value.trim() !== "", t("Please enter the access group name")),
  description: z.string(),
  modelIds: z.array(z.string()),
  mcpServerIds: z.array(z.string()),
  agentIds: z.array(z.string()),
});

export type AccessGroupCreateFormValues = z.output<typeof accessGroupCreateSchema>;
