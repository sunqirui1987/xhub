import { useState } from "react";
import { Check, ChevronDown, ChevronRight, CircleAlert, Copy, Info } from "lucide-react";
import moment from "moment";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { UiLoadingSpinner } from "@/components/ui/ui-loading-spinner";
import { LogEntry } from "../columns";
import { fullErrorDetails, isRequestFailure } from "../logErrors";
import { formatNumberWithCommas } from "@/utils/dataUtils";
import { PROMPT_CACHE_CREATION_TOOLTIP, PROMPT_CACHE_READ_TOOLTIP } from "@/utils/promptCacheUsage";
import GuardrailViewer from "../GuardrailViewer/GuardrailViewer";
import EvalViewer from "../EvalViewer/EvalViewer";
import {
  getBatchIdFromRequestId,
  getBatchModels,
  getBatchRequestCounts,
  getReasoningTokens,
  isBatchCallType,
} from "../batchLogUtils";
import { CostBreakdownViewer } from "../CostBreakdownViewer";
import { ConfigInfoMessage } from "../ConfigInfoMessage";
import { VectorStoreViewer } from "../VectorStoreViewer";
import { TruncatedValue } from "./TruncatedValue";
import { TokenFlow } from "./TokenFlow";
import { JsonViewer } from "./JsonViewer";
import { loggedResponse, requestBody, requestHeaders } from "./prettyMessagesUtils";
import { RoutingDecisionCard, type RoutingDecision } from "./RoutingDecisionCard";
import {
  formatData,
  checkHasMessages,
  checkHasResponse,
  normalizeGuardrailEntries,
  calculateTotalMaskedEntities,
  getGuardrailLabel,
  checkHasVectorStoreData,
} from "./utils";
import {
  DRAWER_CONTENT_PADDING,
  API_BASE_MAX_WIDTH,
  METADATA_MAX_HEIGHT,
  FONT_SIZE_SMALL,
  FONT_FAMILY_MONO,
} from "./constants";
import { ToolsSection } from "../ToolsSection";
import { PrettyMessagesView } from "./PrettyMessagesView";
import { ClassifierAuditView } from "./ClassifierAuditView";
import { AUTOROUTER_CLASSIFIER_ORIGIN } from "./ClassifyTag";
import { t } from "@/i18n";
import { logPromptCacheTokens } from "./promptCacheMetrics";

export interface LogDetailContentProps {
  logEntry: LogEntry;
  /** When true, log details (messages/response) are still being lazy-loaded. */
  isLoadingDetails?: boolean;
  accessToken?: string | null;
}

/**
 * 在单请求或会话抽屉中展示日志详情；接收日志、加载状态和可选鉴权凭据，
 * 返回请求、响应、费用和护栏视图。失败请求始终展示完整诊断正文，
 * 真实响应优先于旧版错误摘要；加载详情时不显示缺失捕获提示。
 * 上游响应默认折叠，用户通过标题按钮展开；切换请求时重置折叠状态，复制始终保留完整诊断。
 */
export function LogDetailContent({ logEntry, isLoadingDetails = false, accessToken }: LogDetailContentProps) {
  const metadata = logEntry.metadata || {};
  const hasError = isRequestFailure(logEntry);
  const errorDetails = hasError ? fullErrorDetails(logEntry) : "";
  const errorInfo = hasError ? metadata.error_information : null;
  const isClassifier =
    metadata.internal_call_origin === AUTOROUTER_CLASSIFIER_ORIGIN &&
    ["completion", "acompletion", "responses", "aresponses"].includes(logEntry.call_type);
  const rawRequest = formatData(logEntry.proxy_server_request || logEntry.messages);
  const hasClassifierAudit =
    isClassifier && (rawRequest?.classifier_input != null || rawRequest?.originating_request_masked != null);

  const hasMessages = checkHasMessages(logEntry.messages);
  const hasResponse = checkHasResponse(logEntry.response);
  // Don't show "missing data" warning while details are still loading
  const missingData = !hasMessages && !hasResponse && !hasError && !isLoadingDetails;

  // Guardrail data
  const guardrailInfo = metadata?.guardrail_information;
  const guardrailEntries = normalizeGuardrailEntries(guardrailInfo);
  const hasGuardrailData = guardrailEntries.length > 0;
  const totalMaskedEntities = calculateTotalMaskedEntities(guardrailEntries);
  const primaryGuardrailLabel = getGuardrailLabel(guardrailEntries);

  // LLM Judge data
  const evalInfo = metadata?.eval_information;
  const hasEvalData = evalInfo != null;

  // Vector store data
  const hasVectorStoreData = checkHasVectorStoreData(metadata);

  const getFormattedResponse = () => {
    // 错误响应包含供应商扩展诊断字段，必须完整保留，不能被兼容摘要或流式成功解析覆盖。
    if (hasResponse) return hasError ? formatData(logEntry.response) : loggedResponse(formatData(logEntry.response));
    if (logEntry.error?.trim()) return formatData(logEntry.error);
    if (hasError && errorInfo) {
      return {
        error: {
          message: errorInfo.error_message || "An error occurred",
          type: errorInfo.error_class || "error",
          code: errorInfo.error_code || "unknown",
          param: null,
        },
      };
    }
    return loggedResponse(formatData(logEntry.response));
  };

  return (
    <div style={{ padding: `${DRAWER_CONTENT_PADDING} ${DRAWER_CONTENT_PADDING} 0` }}>
      {metadata.upstream_response && (
        <section aria-label={t("Upstream Response")} className="mb-6 rounded-lg border p-4">
          {/* 以请求 ID 隔离展开状态，避免查看下一条日志时继承上一条的展开状态。 */}
          <Collapsible key={logEntry.request_id} defaultOpen={false}>
            <div className="flex items-center justify-between gap-3">
              <CollapsibleTrigger className="group flex flex-1 items-center gap-2 rounded-sm text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                <ChevronRight aria-hidden="true" className="size-4 shrink-0 text-muted-foreground group-aria-expanded:hidden" />
                <ChevronDown aria-hidden="true" className="hidden size-4 shrink-0 text-muted-foreground group-aria-expanded:block" />
                <h3 className="font-semibold">{t("Upstream Response")}</h3>
              </CollapsibleTrigger>
              <CopyButton getText={() => JSON.stringify(metadata.upstream_response, null, 2)} label={t("Copy upstream response")} />
            </div>
            <CollapsibleContent className="pt-3">
              <p>{t("HTTP Status")}: {metadata.upstream_response.status_code}</p>
              {metadata.upstream_response.usage_parse_error && <p role="alert">{metadata.upstream_response.usage_parse_error}</p>}
              <h4>{t("Upstream Response Headers")}</h4>
              <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(metadata.upstream_response.headers, null, 2)}</pre>
              {metadata.upstream_response.trailers && <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(metadata.upstream_response.trailers, null, 2)}</pre>}
              <h4>{t("Upstream Usage")}</h4>
              {metadata.upstream_response.usage_reported ? <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(metadata.upstream_response.usage, null, 2)}</pre> : <p>{t("Not reported")}</p>}
              <h4>{t("Billing Usage")}</h4>
              <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(metadata.upstream_response.billing_usage ?? {}, null, 2)}</pre>
            </CollapsibleContent>
          </Collapsible>
        </section>
      )}
      {/* Error Alert */}
      {hasError && (
        <div
          role="alert"
          className="mb-6 flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm"
        >
          <CircleAlert className="size-4 shrink-0 text-destructive" />
          <div>
            <div className="font-medium text-destructive">{t("Request Failed")}</div>
            {metadata.http_status != null && <div>{t("HTTP Status")}: {metadata.http_status}</div>}
            {errorInfo && <ErrorDescription errorInfo={errorInfo} />}
          </div>
        </div>
      )}
      {hasError && errorDetails && !isLoadingDetails && (
        <section aria-label={t("Full Error Details")} className="mb-6 rounded-lg border border-destructive/30">
          <div className="flex items-center justify-between border-b px-3 py-2">
            <h3 className="text-sm font-semibold">{t("Full Error Details")}</h3>
            <CopyButton getText={() => errorDetails} label={t("Copy error details")} />
          </div>
          <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-all p-3 text-xs">{errorDetails}</pre>
        </section>
      )}

      {/* Tags */}
      {logEntry.request_tags && Object.keys(logEntry.request_tags).length > 0 && (
        <TagsSection tags={logEntry.request_tags} />
      )}

      {/* Request Details */}
      <div className="bg-card rounded-lg shadow-sm w-full max-w-full overflow-hidden mb-6">
        <Card size="sm" style={{ marginBottom: 0 }}>
          <CardHeader>
            <CardTitle>{t("Request Details")}</CardTitle>
          </CardHeader>
          <CardContent>
            <DescriptionList>
              <DescriptionItem label={t("Model")}>{logEntry.model}</DescriptionItem>
              <DescriptionItem label={t("Provider")}>{logEntry.custom_llm_provider || "-"}</DescriptionItem>
              <DescriptionItem label={t("Call Type")}>{logEntry.call_type}</DescriptionItem>
              <DescriptionItem label={t("Model ID")}>
                <TruncatedValue value={logEntry.model_id} />
              </DescriptionItem>
              <DescriptionItem label={t("API Base")}>
                <TruncatedValue value={logEntry.api_base} maxWidth={API_BASE_MAX_WIDTH} />
              </DescriptionItem>
              {logEntry.requester_ip_address && (
                <DescriptionItem label={t("IP Address")}>{logEntry.requester_ip_address}</DescriptionItem>
              )}
              {hasGuardrailData && (
                <DescriptionItem label={t("Guardrail")}>
                  <GuardrailLabel label={primaryGuardrailLabel} maskedCount={totalMaskedEntities} />
                </DescriptionItem>
              )}
            </DescriptionList>
          </CardContent>
        </Card>
      </div>

      {/* Batch Results */}
      {isBatchCallType(logEntry.call_type) && <BatchResultsSection logEntry={logEntry} metadata={metadata} />}

      {/* Routing */}
      <RoutingDecisionCard decision={metadata?.routing_decision as RoutingDecision | undefined} />

      {/* Metrics */}
      <MetricsSection logEntry={logEntry} metadata={metadata} />

      {/* Cost Breakdown */}
      <CostBreakdownViewer
        costBreakdown={metadata?.cost_breakdown}
        totalSpend={logEntry.spend ?? 0}
        promptTokens={logEntry.prompt_tokens}
        completionTokens={logEntry.completion_tokens}
        cacheHit={logEntry.cache_hit}
        rawInputTokens={metadata?.additional_usage_values?.prompt_tokens_details?.text_tokens}
        cacheReadTokens={metadata?.additional_usage_values?.cache_read_input_tokens}
        cacheCreationTokens={metadata?.additional_usage_values?.cache_creation_input_tokens}
      />

      {/* Tools */}
      <ToolsSection log={logEntry} />

      {/* Configuration Info Message */}
      {missingData && (
        <div className="mb-6">
          <ConfigInfoMessage show={missingData} />
        </div>
      )}

      {/* Request/Response JSON */}
      {isLoadingDetails ? (
        <div className="bg-card rounded-lg shadow-sm w-full max-w-full overflow-hidden mb-6 p-8 text-center">
          <UiLoadingSpinner className="inline-block size-5" />
          <div style={{ marginTop: 8, color: "var(--color-muted-foreground)" }}>
            {t("Loading request & response data...")}
          </div>
        </div>
      ) : null}
      {!isLoadingDetails && hasClassifierAudit && (
        <ClassifierAuditView request={rawRequest} response={getFormattedResponse()} />
      )}
      {!isLoadingDetails && !hasClassifierAudit && (
        <RequestResponseSection
          hasResponse={hasResponse}
          hasError={hasError}
          getRawRequest={() => rawRequest}
          getFormattedResponse={getFormattedResponse}
          logEntry={logEntry}
        />
      )}

      {/* Guardrail Data */}
      {hasGuardrailData && (
        <div id="guardrail-section">
          <GuardrailViewer
            data={guardrailInfo}
            accessToken={accessToken ?? null}
            logEntry={{
              request_id: logEntry.request_id,
              user: logEntry.user,
              model: logEntry.model,
              startTime: logEntry.startTime,
              metadata: logEntry.metadata,
            }}
          />
        </div>
      )}

      {/* LLM Judge Results */}
      {hasEvalData && <EvalViewer data={evalInfo} />}

      {/* Vector Store Data */}
      {hasVectorStoreData && <VectorStoreViewer data={metadata.vector_store_request_metadata} />}

      {/* Metadata */}
      {logEntry.metadata && Object.keys(logEntry.metadata).length > 0 && (
        <MetadataSection metadata={logEntry.metadata} />
      )}

      {/* Bottom spacing */}
      <div style={{ height: DRAWER_CONTENT_PADDING }} />
    </div>
  );
}

// ============================================================================
// Helper Components
// ============================================================================

function DescriptionList({ children }: { children: React.ReactNode }) {
  return <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">{children}</div>;
}

function DescriptionItem({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-wrap items-start gap-x-2 gap-y-0.5">
      <span className="shrink-0 text-muted-foreground after:content-[':']">{label}</span>
      <span className="min-w-0 break-words">{children}</span>
    </div>
  );
}

function CopyButton({
  getText,
  label,
  disabled = false,
}: {
  getText: () => string;
  label: string;
  disabled?: boolean;
}) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(getText());
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    } catch {
      /* clipboard unavailable in non-secure contexts */
    }
  };

  return (
    <Button
      variant="ghost"
      size="icon-sm"
      onClick={handleCopy}
      disabled={disabled}
      aria-label={copied ? t("Copied!") : label}
    >
      {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
    </Button>
  );
}

function ErrorDescription({ errorInfo }: { errorInfo: any }) {
  return (
    <div>
      {errorInfo.error_code && (
        <div>
          <span className="font-semibold">{t("Error Code:")}</span> {errorInfo.error_code}
        </div>
      )}
      {errorInfo.error_message && (
        <div>
          <span className="font-semibold">{t("Message:")}</span> {errorInfo.error_message}
        </div>
      )}
    </div>
  );
}

function TagsSection({ tags }: { tags: Record<string, any> }) {
  return (
    <div className="bg-card rounded-lg shadow-sm w-full max-w-full overflow-hidden p-4 mb-6">
      <span className="font-semibold" style={{ display: "block", marginBottom: 8, fontSize: 16 }}>
        {t("Tags")}
      </span>
      <div className="flex flex-wrap items-center gap-2">
        {Object.entries(tags).map(([key, value]) => (
          <Badge key={key} variant="outline">
            {key}: {String(value)}
          </Badge>
        ))}
      </div>
    </div>
  );
}

function GuardrailLabel({ label, maskedCount }: { label: string; maskedCount: number }) {
  const handleClick = () => {
    const el = document.getElementById("guardrail-section");
    if (el) el.scrollIntoView({ behavior: "smooth" });
  };

  return (
    <span className="inline-flex items-center gap-2">
      <a onClick={handleClick} style={{ cursor: "pointer" }}>
        {label}
      </a>
      {maskedCount > 0 && <Badge variant="secondary">{maskedCount} masked</Badge>}
    </span>
  );
}

/**
 * Uncached input token count (billable non-cache prompt text), aligned with Cost Breakdown "Input".
 * Same sources as CostBreakdownViewer rawInputTokens.
 */
function getUncachedInputTextTokens(metadata: Record<string, any>): number | undefined {
  const raw =
    metadata?.additional_usage_values?.prompt_tokens_details?.text_tokens ??
    metadata?.usage_object?.prompt_tokens_details?.text_tokens;
  if (raw === undefined || raw === null) return undefined;
  const n = Number(raw);
  return Number.isFinite(n) ? n : undefined;
}

const RESPONSE_CACHE_TOOLTIP =
  "Whether this request was served from XHub's response cache (e.g. Redis / in-memory), skipping the LLM provider call entirely. This is separate from provider prompt caching; a Miss here does not mean prompt caching failed.";
const CACHE_KEY_TOOLTIP =
  "The key XHub computed for this request in the response cache. Requests with the same cache key share a cached response; a different key means the request content did not match any cached entry.";

function MetricLabel({ label, tooltip }: { label: string; tooltip: string }) {
  return (
    <span className="inline-flex items-center gap-1">
      {label}
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger
            render={<span role="img" aria-label={t("{label} info", { label })} className="inline-flex text-muted-foreground" />}
          >
            <Info className="size-3.5" />
          </TooltipTrigger>
          <TooltipContent>{tooltip}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </span>
  );
}

/**
 * Aggregate per-request outcomes for a batch cost row: batch id, success/failure counts
 * from the parsed output and error files, and the models the batch actually ran on.
 */
function BatchResultsSection({ logEntry, metadata }: { logEntry: LogEntry; metadata: Record<string, unknown> }) {
  const counts = getBatchRequestCounts(metadata);
  const batchId = getBatchIdFromRequestId(logEntry.request_id);
  const batchModels = getBatchModels(metadata);
  if (!counts && !batchId && !batchModels) return null;

  return (
    <div className="bg-card rounded-lg shadow-sm w-full max-w-full overflow-hidden mb-6">
      <Card size="sm" style={{ marginBottom: 0 }}>
        <CardHeader>
          <CardTitle>{t("Batch Results")}</CardTitle>
        </CardHeader>
        <CardContent>
          <DescriptionList>
            {batchId && (
              <DescriptionItem label={t("Batch ID")}>
                <TruncatedValue value={batchId} />
              </DescriptionItem>
            )}
            {counts && (
              <>
                <DescriptionItem label={t("Successful Requests")}>
                  {formatNumberWithCommas(counts.successful)}
                </DescriptionItem>
                <DescriptionItem label={t("Failed Requests")}>
                  {counts.failed > 0 ? (
                    <Badge variant="secondary" className="bg-destructive/15 text-destructive">
                      {formatNumberWithCommas(counts.failed)}
                    </Badge>
                  ) : (
                    formatNumberWithCommas(counts.failed)
                  )}
                </DescriptionItem>
              </>
            )}
            {batchModels && <DescriptionItem label={t("Models")}>{batchModels.join(", ")}</DescriptionItem>}
          </DescriptionList>
        </CardContent>
      </Card>
    </div>
  );
}

/** 展示实测指标；参数为日志和元数据，返回指标视图。由详情组件调用，缓存统计缺失保留未知状态。 */
function MetricsSection({ logEntry, metadata }: { logEntry: LogEntry; metadata: Record<string, any> }) {
  const completionStartTime = logEntry.completionStartTime;
  const ttftMs =
    completionStartTime && completionStartTime !== logEntry.endTime
      ? new Date(completionStartTime).getTime() - new Date(logEntry.startTime).getTime()
      : null;

  const responseCacheValue = String(logEntry.cache_hit ?? "").toLowerCase();
  const responseCacheKey = logEntry.cache_key && logEntry.cache_key !== "Cache OFF" ? logEntry.cache_key : undefined;
  const isResponseCacheHit = responseCacheValue === "true";
  const showResponseCache = isResponseCacheHit || responseCacheValue === "false" || responseCacheKey != null;
  const { read: promptCacheReadTokens, creation: promptCacheCreationTokens } = logPromptCacheTokens(logEntry, metadata);

  const uncachedInputTokens = getUncachedInputTextTokens(metadata);
  const showAnthropicMessagesInputOutput =
    logEntry.call_type === "anthropic_messages" && uncachedInputTokens !== undefined;
  const reasoningTokens = getReasoningTokens(metadata);

  return (
    <div className="bg-card rounded-lg shadow-sm w-full max-w-full overflow-hidden mb-6">
      <Card size="sm" style={{ marginBottom: 0 }}>
        <CardHeader>
          <CardTitle>{t("Metrics")}</CardTitle>
        </CardHeader>
        <CardContent>
          <DescriptionList>
            {showAnthropicMessagesInputOutput ? (
              <>
                <DescriptionItem label={t("Input Tokens")}>{formatNumberWithCommas(uncachedInputTokens)}</DescriptionItem>
                <DescriptionItem label={t("Output Tokens")}>
                  {formatNumberWithCommas(logEntry.completion_tokens)}
                </DescriptionItem>
              </>
            ) : (
              <DescriptionItem label={t("Tokens")}>
                <TokenFlow
                  prompt={logEntry.prompt_tokens}
                  completion={logEntry.completion_tokens}
                  total={logEntry.total_tokens}
                />
              </DescriptionItem>
            )}
            {reasoningTokens !== undefined && reasoningTokens > 0 && (
              <DescriptionItem label={t("Reasoning Tokens")}>{formatNumberWithCommas(reasoningTokens)}</DescriptionItem>
            )}
            <DescriptionItem label={t("Cost")}>${formatNumberWithCommas(logEntry.spend || 0, 8)}</DescriptionItem>
            <DescriptionItem label={t("Duration")}>
              {logEntry.request_duration_ms != null ? (logEntry.request_duration_ms / 1000).toFixed(3) : "-"} s
            </DescriptionItem>
            {ttftMs != null && ttftMs > 0 && (
              <DescriptionItem label={t("Time to First Token")}>{(ttftMs / 1000).toFixed(3)} s</DescriptionItem>
            )}

            {showResponseCache && (
              <DescriptionItem
                label={
                  <MetricLabel
                    label={t("Response Cache")}
                    tooltip={RESPONSE_CACHE_TOOLTIP}
                  />
                }
              >
                <Badge variant="secondary" className={isResponseCacheHit ? "bg-success/15 text-success" : undefined}>
                  {isResponseCacheHit ? t("Hit") : t("Miss")}
                </Badge>
              </DescriptionItem>
            )}
            {responseCacheKey && (
              <DescriptionItem
                label={<MetricLabel label={t("Cache Key")} tooltip={CACHE_KEY_TOOLTIP} />}
              >
                <TruncatedValue value={responseCacheKey} />
              </DescriptionItem>
            )}
            {(promptCacheReadTokens != null || !isResponseCacheHit) && (
              <DescriptionItem
                label={
                  <MetricLabel
                    label={t("Prompt Cache Read Tokens")}
                    tooltip={PROMPT_CACHE_READ_TOOLTIP}
                  />
                }
              >
                {promptCacheReadTokens != null ? formatNumberWithCommas(promptCacheReadTokens) : t("Not reported")}
              </DescriptionItem>
            )}
            {promptCacheCreationTokens != null && promptCacheCreationTokens > 0 && (
              <DescriptionItem
                label={
                  <MetricLabel
                    label={t("Prompt Cache Creation Tokens")}
                    tooltip={PROMPT_CACHE_CREATION_TOOLTIP}
                  />
                }
              >
                {formatNumberWithCommas(promptCacheCreationTokens)}
              </DescriptionItem>
            )}

            {metadata?.litellm_overhead_time_ms !== undefined && metadata.litellm_overhead_time_ms !== null && (
              <DescriptionItem label={t("LiteLLM Overhead")}>
                {metadata.litellm_overhead_time_ms.toFixed(2)} ms
              </DescriptionItem>
            )}

            <DescriptionItem label={t("Retries")}>
              {metadata?.attempted_retries != null && metadata.attempted_retries > 0 && (
                <>
                  {metadata.attempted_retries}
                  {metadata.max_retries !== undefined && metadata.max_retries !== null
                    ? ` / ${metadata.max_retries}`
                    : ""}
                </>
              )}
              {metadata?.attempted_retries != null && metadata.attempted_retries <= 0 && (
                <Badge variant="secondary" className="bg-success/15 text-success">
                  {t("None")}
                </Badge>
              )}
              {metadata?.attempted_retries == null && "-"}
            </DescriptionItem>

            <DescriptionItem label={t("Start Time")}>
              {moment(logEntry.startTime).format("YYYY-MM-DDTHH:mm:ss.SSS[Z]")}
            </DescriptionItem>
            <DescriptionItem label={t("End Time")}>
              {moment(logEntry.endTime).format("YYYY-MM-DDTHH:mm:ss.SSS[Z]")}
            </DescriptionItem>
          </DescriptionList>
        </CardContent>
      </Card>
    </div>
  );
}

function JsonPanel({
  title,
  data,
  copyLabel,
  empty,
}: {
  title: string;
  data: unknown;
  copyLabel: string;
  empty?: string;
}) {
  return (
    <section className="rounded-md border">
      <div className="flex items-center justify-between border-b px-3 py-2">
        <h4 className="text-sm font-semibold text-foreground">{title}</h4>
        <CopyButton getText={() => JSON.stringify(data ?? {}, null, 2)} label={copyLabel} disabled={data == null} />
      </div>
      <div className="p-3">
        {data == null ? (
          <p className="text-sm italic text-muted-foreground">{empty}</p>
        ) : (
          <JsonViewer data={data} mode="formatted" />
        )}
      </div>
    </section>
  );
}

interface RequestResponseSectionProps {
  hasResponse: boolean;
  hasError: boolean;
  getRawRequest: () => any;
  getFormattedResponse: () => any;
  logEntry: LogEntry;
}

function RequestResponseSection({
  hasResponse,
  hasError,
  getRawRequest,
  getFormattedResponse,
  logEntry,
}: RequestResponseSectionProps) {
  const [open, setOpen] = useState(true);
  const [viewMode, setViewMode] = useState<"pretty" | "json">("pretty");

  const totalSpend = logEntry.spend ?? 0;
  const promptTokens = logEntry.prompt_tokens || 0;
  const completionTokens = logEntry.completion_tokens || 0;
  const totalTokens = promptTokens + completionTokens;
  const costBreakdown = logEntry.metadata?.cost_breakdown;
  // 有明细就用明细，只在**真的没有**的时候才按 token 比例估。
  //
  // 用 && 要求两侧都在是错的：按秒、按张的模型只有输出侧，缓存全命中的调用
  // 只有缓存侧，这些都会被判成"没有明细"，然后显示一个按 token 摊出来的数字——
  // 编出来的数，但看起来和真的一模一样。
  const hasStoredBreakdown =
    costBreakdown?.input_cost !== undefined ||
    costBreakdown?.output_cost !== undefined ||
    costBreakdown?.cache_read_cost !== undefined ||
    costBreakdown?.cache_creation_cost !== undefined;
  const estimatedInputCost = totalTokens > 0 ? (totalSpend * promptTokens) / totalTokens : 0;
  const estimatedOutputCost = totalTokens > 0 ? (totalSpend * completionTokens) / totalTokens : 0;
  const inputCost = hasStoredBreakdown ? costBreakdown!.input_cost ?? 0 : estimatedInputCost;
  const outputCost = hasStoredBreakdown ? costBreakdown!.output_cost ?? 0 : estimatedOutputCost;

  return (
    <div className="bg-card rounded-lg shadow-sm w-full max-w-full overflow-hidden mb-6">
      <Collapsible open={open} onOpenChange={setOpen}>
        <Tabs value={viewMode} onValueChange={(value) => setViewMode(value as "pretty" | "json")}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", width: "100%" }}>
            <CollapsibleTrigger className="flex flex-1 items-center gap-3 px-4 py-3 text-left">
              {open ? (
                <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
              ) : (
                <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
              )}
              <h3 className="text-lg font-medium text-foreground" style={{ margin: 0 }}>
                {t("Request & Response")}
              </h3>
            </CollapsibleTrigger>
            <TabsList className="mr-4">
              <TabsTrigger value="pretty">{t("Pretty")}</TabsTrigger>
              <TabsTrigger value="json">JSON</TabsTrigger>
            </TabsList>
          </div>
          <CollapsibleContent>
            <div>
              <TabsContent value="pretty">
                <PrettyMessagesView
                  request={getRawRequest()}
                  response={getFormattedResponse()}
                  model={logEntry.model}
                  metrics={{
                    prompt_tokens: promptTokens,
                    completion_tokens: completionTokens,
                    input_cost: inputCost,
                    output_cost: outputCost,
                  }}
                />
              </TabsContent>
              <TabsContent value="json">
                <div className="space-y-4 p-4">
                  <JsonPanel title={t("Headers")} data={requestHeaders(getRawRequest()) ?? {}} copyLabel={t("Copy headers")} />
                  <JsonPanel title={t("Request")} data={requestBody(getRawRequest())} copyLabel={t("Copy request")} />
                  <JsonPanel
                    title={t("Response")}
                    data={hasResponse || hasError ? getFormattedResponse() : null}
                    copyLabel={t("Copy response")}
                    empty={t("Response data not available")}
                  />
                </div>
              </TabsContent>
            </div>
          </CollapsibleContent>
        </Tabs>
      </Collapsible>
    </div>
  );
}

const GUARDRAIL_JUMP_LINK_STYLE = {
  passed: { className: "border border-success/20 bg-success/10 text-success", glyph: "\u2713" },
  flagged: { className: "border border-warning/20 bg-warning/10 text-warning", glyph: "\u26A0" },
  failed: { className: "border border-destructive/20 bg-destructive/10 text-destructive", glyph: "\u2717" },
} as const;

const isPassedStatus = (status: unknown) => status === "pass" || status === "passed" || status === "success";
const isFlaggedStatus = (status: unknown) => status === "flagged" || status === "guardrail_flagged";

const guardrailJumpLinkOutcome = (statuses: unknown[]): keyof typeof GUARDRAIL_JUMP_LINK_STYLE => {
  if (statuses.every(isPassedStatus)) return "passed";
  if (statuses.every((s) => isPassedStatus(s) || isFlaggedStatus(s))) return "flagged";
  return "failed";
};

export function GuardrailJumpLink({ guardrailEntries }: { guardrailEntries: any[] }) {
  const outcome = guardrailJumpLinkOutcome(guardrailEntries.map((e) => e?.guardrail_status || e?.status));
  const { className, glyph } = GUARDRAIL_JUMP_LINK_STYLE[outcome];

  const handleClick = () => {
    const el = document.getElementById("guardrail-section");
    if (el) el.scrollIntoView({ behavior: "smooth" });
  };

  return (
    <div style={{ textAlign: "left", marginBottom: 12 }}>
      <div
        onClick={handleClick}
        className={className}
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 6,
          padding: "4px 12px",
          borderRadius: 16,
          cursor: "pointer",
          fontSize: 13,
          fontWeight: 500,
        }}
      >
        {glyph} {guardrailEntries.length} {t("guardrail")}
        {guardrailEntries.length !== 1 ? "s" : ""} {t("evaluated")}
        <span style={{ fontSize: 11, opacity: 0.7 }}>{"\u2193"}</span>
      </div>
    </div>
  );
}

function MetadataSection({ metadata }: { metadata: Record<string, any> }) {
  const [open, setOpen] = useState(true);

  return (
    <div className="bg-card rounded-lg shadow-sm w-full max-w-full overflow-hidden mb-6">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger className="flex w-full items-center gap-3 px-4 py-3 text-left">
          {open ? (
            <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
          ) : (
            <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
          )}
          <h3 className="text-lg font-medium text-foreground">{t("Metadata")}</h3>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div>
            <div style={{ display: "flex", justifyContent: "flex-end", marginBottom: 8 }}>
              <CopyButton getText={() => JSON.stringify(metadata, null, 2)} label={t("Copy Metadata")} />
            </div>
            <pre
              style={{
                maxHeight: METADATA_MAX_HEIGHT,
                overflowY: "auto",
                fontSize: FONT_SIZE_SMALL,
                fontFamily: FONT_FAMILY_MONO,
                whiteSpace: "pre-wrap",
                wordBreak: "break-all",
                margin: 0,
              }}
            >
              {JSON.stringify(metadata, null, 2)}
            </pre>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}
