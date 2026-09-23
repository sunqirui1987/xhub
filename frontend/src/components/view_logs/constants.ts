import { t } from "@/i18n";
export const ERROR_CODE_OPTIONS: { label: string; value: string }[] = [
  { label: t("400 - Bad Request"), value: "400" },
  { label: t("401 - Invalid Authentication"), value: "401" },
  { label: t("403 - Permission Denied"), value: "403" },
  { label: t("404 - Not Found"), value: "404" },
  { label: t("408 - Request Timeout"), value: "408" },
  { label: t("422 - Unprocessable Entity"), value: "422" },
  { label: t("429 - Rate Limited"), value: "429" },
  { label: t("500 - Internal Server Error"), value: "500" },
  { label: t("502 - Bad Gateway"), value: "502" },
  { label: t("503 - Service Unavailable"), value: "503" },
  { label: t("529 - Overloaded"), value: "529" },
];

/** Call types that represent MCP tool invocations (shared across columns, index, drawer). */
export const MCP_CALL_TYPES = ["call_mcp_tool", "list_mcp_tools"];

/** Call types that represent agent/A2A requests (e.g. asend_message). */
export const AGENT_CALL_TYPES = ["asend_message"];

/** Call types that represent Batch API operations (creation and retrieval, sync and async). */
export const BATCH_CALL_TYPES = ["acreate_batch", "create_batch", "aretrieve_batch", "retrieve_batch"];

export const QUICK_SELECT_OPTIONS: { label: string; value: number; unit: string }[] = [
  { label: t("Last Minute"), value: 1, unit: "minutes" },
  { label: t("Last 15 Minutes"), value: 15, unit: "minutes" },
  { label: t("Last Hour"), value: 1, unit: "hours" },
  { label: t("Last 4 Hours"), value: 4, unit: "hours" },
  { label: t("Last 24 Hours"), value: 24, unit: "hours" },
  { label: t("Last 7 Days"), value: 7, unit: "days" },
];
