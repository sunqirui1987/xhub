import { CircleCheck, ChevronDown, TriangleAlert, X } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import moment from "moment";
import React, { useState } from "react";
import { Button } from "@/components/ui/button";
import { UiLoadingSpinner } from "@/components/ui/ui-loading-spinner";
import { uiSpendLogsCall } from "@/components/networking";
import { LogDetailsDrawer } from "@/components/view_logs/LogDetailsDrawer";
import type { LogEntry as ViewLogsLogEntry } from "@/components/view_logs/columns";
import type { LogEntry } from "./mockData";
import { t } from "@/i18n";

const actionConfig: Record<
  "blocked" | "passed" | "flagged",
  { icon: React.ElementType; color: string; bg: string; border: string; label: string }
> = {
  blocked: {
    icon: X,
    color: "text-destructive",
    bg: "bg-destructive/10",
    border: "border-destructive/20",
    label: t("Blocked"),
  },
  passed: {
    icon: CircleCheck,
    color: "text-success",
    bg: "bg-success/10",
    border: "border-success/20",
    label: t("Passed"),
  },
  flagged: {
    icon: TriangleAlert,
    color: "text-warning",
    bg: "bg-warning/10",
    border: "border-warning/20",
    label: t("Flagged"),
  },
};

interface LogViewerProps {
  guardrailName?: string;
  filterAction?: "all" | "blocked" | "passed" | "flagged";
  logs?: LogEntry[];
  logsLoading?: boolean;
  totalLogs?: number;
  accessToken?: string | null;
  startDate?: string;
  endDate?: string;
}

/**
 * 用途：展示护栏执行日志的动作、原因和关键运行信息，并允许打开对应请求详情。
 * 参数：LogViewerProps 提供已筛选日志、时间范围、访问令牌及加载状态。
 * 返回：带筛选控件的日志列表；日志不含原因时展示明确占位说明。
 * 调用：护栏监控详情页及测试；点击日志后按请求 ID 查询并打开请求抽屉。
 * 异常与边界：无日志、无令牌、可选元数据缺失时仍可显示动作和说明，不展示空内容破折号。
 */
export function LogViewer({
  guardrailName,
  filterAction = "all",
  logs = [],
  logsLoading = false,
  totalLogs,
  accessToken = null,
  startDate = "",
  endDate = "",
}: LogViewerProps) {
  const [sampleSize, setSampleSize] = useState(10);
  const [activeFilter, setActiveFilter] = useState<string>(filterAction);
  const [selectedRequestId, setSelectedRequestId] = useState<string | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);

  const filteredLogs = logs.filter((log) => activeFilter === "all" || log.action === activeFilter);
  const displayLogs = filteredLogs.slice(0, sampleSize);
  const total = totalLogs ?? logs.length;
  const sampleSizes = [10, 50, 100];
  const filters: Array<"all" | "blocked" | "flagged" | "passed"> = ["all", "blocked", "flagged", "passed"];

  const startTime = startDate
    ? moment(startDate).utc().format("YYYY-MM-DD HH:mm:ss")
    : moment().subtract(24, "hours").utc().format("YYYY-MM-DD HH:mm:ss");
  const endTime = endDate
    ? moment(endDate).utc().endOf("day").format("YYYY-MM-DD HH:mm:ss")
    : moment().utc().format("YYYY-MM-DD HH:mm:ss");

  const { data: fullLogResponse } = useQuery({
    queryKey: ["spend-log-by-request", selectedRequestId, startTime, endTime],
    queryFn: async () => {
      if (!accessToken || !selectedRequestId) return null;
      const res = await uiSpendLogsCall({
        accessToken,
        start_date: startTime,
        end_date: endTime,
        page: 1,
        page_size: 10,
        params: { request_id: selectedRequestId },
      });
      return res as { data: ViewLogsLogEntry[]; total: number };
    },
    enabled: Boolean(accessToken && selectedRequestId && drawerOpen),
  });

  const selectedLog: ViewLogsLogEntry | null =
    fullLogResponse?.data?.find((log) => log.request_id === selectedRequestId) ?? fullLogResponse?.data?.[0] ?? null;

  const handleLogClick = (log: LogEntry) => {
    setSelectedRequestId(log.id);
    setDrawerOpen(true);
  };

  const handleCloseDrawer = () => {
    setDrawerOpen(false);
    setSelectedRequestId(null);
  };

  return (
    <div className="bg-card border border-border rounded-lg">
      <div className="p-4 border-b border-border">
        <div className="flex items-center justify-between flex-wrap gap-3">
          <div>
            <h3 className="text-base font-semibold text-foreground">
              {guardrailName ? t("Logs — {guardrailName}", { guardrailName }) : t("Request Logs")}
            </h3>
            <p className="text-xs text-muted-foreground mt-0.5">
              {logsLoading
                ? t("Loading…")
                : logs.length > 0
                  ? t("Showing {value0} of {total} entries", { value0: (displayLogs.length), total })
                  : t("No logs for this period. Select a guardrail and date range.")}
            </p>
          </div>
          {logs.length > 0 && (
            <div className="flex items-center gap-4">
              <div className="flex items-center gap-1">
                {filters.map((f) => (
                  <Button
                    key={f}
                    variant={activeFilter === f ? "default" : "outline"}
                    size="sm"
                    onClick={() => setActiveFilter(f)}
                  >
                    {f.charAt(0).toUpperCase() + f.slice(1)}
                  </Button>
                ))}
              </div>
              <div className="h-4 w-px bg-border" />
              <div className="flex items-center gap-1">
                <span className="text-xs text-muted-foreground mr-1">{t("Sample:")}</span>
                {sampleSizes.map((size) => (
                  <Button
                    key={size}
                    variant={sampleSize === size ? "default" : "outline"}
                    size="sm"
                    onClick={() => setSampleSize(size)}
                  >
                    {size}
                  </Button>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      {logsLoading && (
        <div className="flex items-center justify-center py-12">
          <UiLoadingSpinner className="size-5" />
        </div>
      )}
      {!logsLoading && displayLogs.length === 0 && (
        <div className="py-12 text-center text-sm text-muted-foreground">
          {t("No logs to display. Adjust filters or date range.")}
        </div>
      )}
      {!logsLoading && displayLogs.length > 0 && (
        <div className="divide-y divide-border">
          {displayLogs.map((log) => {
            const config = actionConfig[log.action];
            const ActionIcon = config.icon;
            return (
              <button
                key={log.id}
                type="button"
                onClick={() => handleLogClick(log)}
                className="w-full text-left px-4 py-3 hover:bg-accent transition-colors flex items-start gap-3"
              >
                <ActionIcon className={`w-4 h-4 mt-0.5 shrink-0 ${config.color}`} />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-1 flex-wrap">
                    <span
                      className={`inline-flex items-center px-2 py-0.5 text-xs font-medium rounded-sm border ${config.bg} ${config.color} ${config.border}`}
                    >
                      {config.label}
                    </span>
                    <span className="text-xs text-muted-foreground">{log.timestamp}</span>
                    <span className="text-xs text-muted-foreground">·</span>
                    {log.model && (
                      <span className="min-w-0 text-xs break-words text-muted-foreground">{log.model}</span>
                    )}
                  </div>
                  <p className="text-sm text-foreground">
                    {log.reason || t("No reason was recorded for this evaluation.")}
                  </p>
                  <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
                    {log.guardrail_provider && <span>{t("Provider: {value0}", { value0: log.guardrail_provider })}</span>}
                    {log.guardrail_mode && <span>{t("Stage: {value0}", { value0: log.guardrail_mode.replace(/_/g, "-") })}</span>}
                    {log.latency_ms != null && <span>{t("Latency: {value0}ms", { value0: Math.round(log.latency_ms) })}</span>}
                    {log.guardrail_name && <span>{log.guardrail_name}</span>}
                  </div>
                </div>
                <ChevronDown className="w-4 h-4 text-muted-foreground shrink-0 mt-1" />
              </button>
            );
          })}
        </div>
      )}

      <LogDetailsDrawer
        open={drawerOpen}
        onClose={handleCloseDrawer}
        logEntry={selectedLog}
        accessToken={accessToken}
        allLogs={selectedLog ? [selectedLog] : []}
        startTime={startTime}
      />
    </div>
  );
}
