import type { StatusTone } from "@/components/shared/table_cells";
import type { LogEntry } from "./columns";
import { isRequestFailure } from "./logErrors";

/** 用途：表格和详情共享生命周期展示。参数 log：状态及错误；返回标签与颜色。普通请求沿用成功失败，未知态兼容旧日志，无副作用。 */
export function requestLogStatus(log: Pick<LogEntry, "status" | "metadata" | "error">): { label: string; tone: StatusTone } {
  if (isRequestFailure(log)) return { label: "Failure", tone: "error" };
  switch (log.status) {
    case "executing": return { label: "Executing", tone: "info" };
    case "polling": return { label: "Polling", tone: "warning" };
    case "completed": return { label: "Completed", tone: "success" };
    default: return { label: "Success", tone: "success" };
  }
}
