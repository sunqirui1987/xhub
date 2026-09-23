import { t } from "@/i18n";
export const ACTION_ITEMS = [
  { value: "BLOCK", label: t("Block") },
  { value: "MASK", label: t("Mask") },
] as const;

export const SEVERITY_ITEMS = [
  { value: "high", label: t("High") },
  { value: "medium", label: t("Medium") },
  { value: "low", label: t("Low") },
] as const;
