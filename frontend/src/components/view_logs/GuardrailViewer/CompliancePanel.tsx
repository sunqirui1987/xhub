import React, { useState, useEffect } from "react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import {
  checkEuAiActCompliance,
  checkGdprCompliance,
  ComplianceResponse,
  ComplianceCheckRequest,
} from "@/components/networking";
import { t } from "@/i18n";

interface CompliancePanelProps {
  accessToken: string | null;
  logEntry: {
    request_id: string;
    user?: string;
    model?: string;
    startTime?: string;
    metadata?: Record<string, any>;
  };
}

// -- Icons --

const CheckIcon = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <circle cx="8" cy="8" r="7" stroke="#16A34A" strokeWidth="1.5" fill="#F0FDF4" />
    <path d="M5 8l2 2 4-4" stroke="#16A34A" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

const CrossIcon = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <circle cx="8" cy="8" r="7" stroke="#DC2626" strokeWidth="1.5" fill="#FEF2F2" />
    <path d="M6 6l4 4M10 6l-4 4" stroke="#DC2626" strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

const SpinnerIcon = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" className="animate-spin">
    <circle cx="8" cy="8" r="6" stroke="#D1D5DB" strokeWidth="2" />
    <path d="M8 2a6 6 0 0 1 6 6" stroke="#6366F1" strokeWidth="2" strokeLinecap="round" />
  </svg>
);

// -- Sub-components --

/**
 * 校验合规接口的运行时响应。
 * 参数 value 是 EU AI Act 或 GDPR 接口返回值；返回可供卡片展示的 ComplianceResponse，供加载流程调用。
 * 缺少 checks 或 compliant 的目录桩、空值和其他协议形态均按当前语言抛错，避免日志页映射异常。
 */
const asComplianceResponse = (value: unknown): ComplianceResponse => {
  if (!value || typeof value !== "object") {
    throw new Error(t("Compliance check returned an unexpected response"));
  }
  const data = value as Partial<ComplianceResponse>;
  if (!Array.isArray(data.checks) || typeof data.compliant !== "boolean") {
    throw new Error(t("Compliance check returned an unexpected response"));
  }
  return data as ComplianceResponse;
};

/**
 * 展示一项法规的合规加载、失败或检查结果。
 * 参数 title 为法规名，data 为已校验响应，loading/error 表示请求状态；返回可展开的合规卡片。
 * CompliancePanel 分别为 EU AI Act 与 GDPR 调用；失败时只展示错误提示，不读取 data，合规状态按当前语言读取。
 */
const ComplianceCard = ({
  title,
  data,
  loading,
  error,
}: {
  title: string;
  data: ComplianceResponse | null;
  loading: boolean;
  error: string | null;
}) => {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="border border-border rounded-lg bg-card">
      <div
        className="flex items-center justify-between px-4 py-3 cursor-pointer hover:bg-accent transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex items-center gap-2">
          {loading ? (
            <SpinnerIcon />
          ) : error ? (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger render={<span className="text-muted-foreground text-sm" />}>--</TooltipTrigger>
                <TooltipContent>{error}</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          ) : data?.compliant ? (
            <CheckIcon />
          ) : (
            <CrossIcon />
          )}
          <span className="font-medium text-sm text-foreground">{title}</span>
        </div>
        <div className="flex items-center gap-2">
          {!loading && !error && data && (
            <span
              className={`px-2 py-0.5 rounded text-[11px] font-semibold uppercase ${
                data.compliant
                  ? "bg-success/15 text-success border border-success/20"
                  : "bg-destructive/15 text-destructive border border-destructive/20"
              }`}
            >
              {data.compliant ? t("COMPLIANT") : t("NON-COMPLIANT")}
            </span>
          )}
          {error && (
            <span className="px-2 py-0.5 rounded-sm text-[11px] font-medium bg-muted text-muted-foreground border border-border">
              UNAVAILABLE
            </span>
          )}
          <svg
            width="20"
            height="20"
            viewBox="0 0 20 20"
            fill="none"
            className={`transition-transform ${expanded ? "rotate-180" : ""}`}
          >
            <path d="M6 8l4 4 4-4" stroke="#6B7280" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </div>
      </div>

      {expanded && (
        <div className="border-t border-border px-4 py-3">
          {loading && <p className="text-sm text-muted-foreground">{t("Checking compliance...")}</p>}
          {error && <p className="text-sm text-destructive">{error}</p>}
          {data && (
            <div className="space-y-2">
              {(data.checks ?? []).map((check, idx) => (
                <div key={idx} className="flex items-start gap-2">
                  <div className="shrink-0 mt-0.5">{check.passed ? <CheckIcon /> : <CrossIcon />}</div>
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-foreground">{check.check_name}</span>
                      <span className="text-[10px] font-mono text-muted-foreground">{check.article}</span>
                    </div>
                    <p className="text-xs text-muted-foreground mt-0.5">{check.detail}</p>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

// -- Main Component --

const CompliancePanel: React.FC<CompliancePanelProps> = ({ accessToken, logEntry }) => {
  const [euAiActData, setEuAiActData] = useState<ComplianceResponse | null>(null);
  const [gdprData, setGdprData] = useState<ComplianceResponse | null>(null);
  const [euAiActLoading, setEuAiActLoading] = useState(false);
  const [gdprLoading, setGdprLoading] = useState(false);
  const [euAiActError, setEuAiActError] = useState<string | null>(null);
  const [gdprError, setGdprError] = useState<string | null>(null);

  useEffect(() => {
    if (!accessToken || !logEntry.request_id) return;

    const payload: ComplianceCheckRequest = {
      request_id: logEntry.request_id,
      user_id: logEntry.user,
      model: logEntry.model,
      timestamp: logEntry.startTime,
      guardrail_information: logEntry.metadata?.guardrail_information,
    };

    setEuAiActLoading(true);
    setEuAiActError(null);
    checkEuAiActCompliance(accessToken, payload)
      .then((res) => setEuAiActData(asComplianceResponse(res)))
      .catch((err) => setEuAiActError(err.message || "Failed to check EU AI Act compliance"))
      .finally(() => setEuAiActLoading(false));

    setGdprLoading(true);
    setGdprError(null);
    checkGdprCompliance(accessToken, payload)
      .then((res) => setGdprData(asComplianceResponse(res)))
      .catch((err) => setGdprError(err.message || "Failed to check GDPR compliance"))
      .finally(() => setGdprLoading(false));
  }, [accessToken, logEntry]);

  return (
    <div>
      <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-4">
        {t("Regulatory Compliance")}
      </h4>
      <div className="space-y-3">
        <ComplianceCard title={t("EU AI Act")} data={euAiActData} loading={euAiActLoading} error={euAiActError} />
        <ComplianceCard title="GDPR" data={gdprData} loading={gdprLoading} error={gdprError} />
      </div>
    </div>
  );
};

export default CompliancePanel;
