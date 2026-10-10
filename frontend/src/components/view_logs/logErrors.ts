import type { LogEntry } from "./columns";

/** 判断日志是否失败：兼容后台 error 和旧版 failure 状态、HTTP 错误码及独立错误正文；供表格和详情共享，不改变输入。 */
export function isRequestFailure(log: Pick<LogEntry, "status" | "metadata" | "error">): boolean {
  return [log.status, log.metadata?.status].some((status) =>
    typeof status === "string" && ["error", "failure", "failed"].includes(status.toLowerCase()),
  ) || Number(log.metadata?.http_status) >= 400 || Boolean(log.error?.trim());
}

/** 返回完整诊断正文：优先独立 error，其次完整响应，最后旧版错误元数据；JSON 可读格式化，纯文本原样保留，空捕获返回空字符串。 */
export function fullErrorDetails(log: Pick<LogEntry, "error" | "response" | "metadata">): string {
  if (log.error?.trim()) return log.error;
  const response = log.response;
  if (typeof response === "string" && response.trim()) return response;
  if (response && typeof response === "object" && Object.keys(response).length > 0) {
    return JSON.stringify(response, null, 2);
  }
  return log.metadata?.error_information ? JSON.stringify(log.metadata.error_information, null, 2) : "";
}
