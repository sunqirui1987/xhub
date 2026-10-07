import React, { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { measureLabel, rateLabel, rateText } from "@/lib/rateDisplay";
import { t } from "@/i18n";

/** 这次调用实际用到的一条费率，单位自带，不能假设都是 token。 */
export interface AppliedRate {
  measure?: string;
  side?: string;
  variant?: string;
  unit_size?: number;
  usd?: number;
  quantity?: number;
  cost?: number;
}

export interface CostBreakdown {
  input_cost?: number;
  cache_read_cost?: number;
  cache_creation_cost?: number;
  output_cost?: number;
  total_cost?: number;
  tool_usage_cost?: number;
  additional_costs?: Record<string, number>;
  original_cost?: number;
  input_cost_per_token?: number;
  output_cost_per_token?: number;
  discount_percent?: number;
  discount_amount?: number;
  margin_percent?: number;
  margin_fixed_amount?: number;
  margin_total_amount?: number;
  /**
   * 这次调用落在哪个计费时段。分时定价的模型（忙时价是闲时价的两倍）
   * 靠它解释账单为什么是这个数。
   */
  window?: string;
  /** 这次实际用到的费率，带数量和单位。日志详情直接读它，不重算。 */
  applied?: AppliedRate[];
  /**
   * 这组数字的来源。
   *
   *   - snapshot：调用当时存下来的费率，是原始记录。
   *   - recomputed：那一行没有快照（早于这个字段上线，或当时没定上价），
   *     数字是按**那次调用的时刻**重算的。单价表一改，这个数就会变，
   *     所以必须标出来，不能假装是原始记录。
   */
  source?: "snapshot" | "recomputed";
}

interface CostBreakdownViewerProps {
  costBreakdown: CostBreakdown | null | undefined;
  totalSpend: number;
  promptTokens?: number;
  completionTokens?: number;
  cacheHit?: string | boolean | null;
  rawInputTokens?: number;
  cacheReadTokens?: number;
  cacheCreationTokens?: number;
}

const formatCost = (cost: number | undefined): string => {
  if (cost === undefined || cost === null) return "-";
  return `$${cost.toFixed(2)}`;
};

const formatTokens = (tokens: number): string => {
  if (Math.abs(tokens) >= 1000) {
    const wan = tokens / 10000;
    const digits = Math.abs(wan) >= 100 ? 0 : Math.abs(wan) >= 10 ? 1 : 2;
    return `${wan.toFixed(digits)}万`;
  }
  return tokens.toLocaleString("en-US");
};

const formatMillionRate = (rate: number): string => `$${(rate * 1_000_000).toFixed(2)}/1M`;

const rateLine = (tokens: number | undefined, unit: string, rate: number | undefined): string | null => {
  if (tokens === undefined || rate === undefined || rate === null) return null;
  return `${formatTokens(tokens)} ${unit} × ${formatMillionRate(rate)}`;
};

const formatPercent = (percent: number | undefined): string => {
  if (percent === undefined || percent === null) return "-";
  return `${(percent * 100).toFixed(2)}%`;
};

export const CostBreakdownViewer: React.FC<CostBreakdownViewerProps> = ({
  costBreakdown,
  totalSpend,
  promptTokens,
  completionTokens,
  cacheHit,
  rawInputTokens,
  cacheReadTokens,
  cacheCreationTokens,
}) => {
  const [open, setOpen] = useState(false);
  const isCached = String(cacheHit ?? "").toLowerCase() === "true";
  const hasTokenCounts = promptTokens !== undefined || completionTokens !== undefined;

  const hasCostBreakdown = costBreakdown?.input_cost !== undefined || costBreakdown?.output_cost !== undefined;
  const hasAdditionalCosts =
    costBreakdown?.additional_costs &&
    Object.entries(costBreakdown.additional_costs).some(([, value]) => value != null && value !== 0);
  // 缓存那一侧单独出现时也算有明细。一条只收缓存费的调用以前会让整块消失，
  // 而它明明是扣了钱的。
  const hasCacheCosts =
    (costBreakdown?.cache_read_cost ?? 0) !== 0 || (costBreakdown?.cache_creation_cost ?? 0) !== 0;
  // 只有真被计费过才有费率可列。命中缓存的那一次是零费用，硬列出来只会让人以为收过钱。
  const appliedRates = isCached ? [] : (costBreakdown?.applied ?? []);

  const hasMeaningfulData =
    hasCostBreakdown ||
    hasCacheCosts ||
    hasTokenCounts ||
    hasAdditionalCosts ||
    (costBreakdown &&
      ((costBreakdown.discount_percent !== undefined && costBreakdown.discount_percent !== 0) ||
        (costBreakdown.discount_amount !== undefined && costBreakdown.discount_amount !== 0) ||
        (costBreakdown.margin_percent !== undefined && costBreakdown.margin_percent !== 0) ||
        (costBreakdown.margin_fixed_amount !== undefined && costBreakdown.margin_fixed_amount !== 0) ||
        (costBreakdown.margin_total_amount !== undefined && costBreakdown.margin_total_amount !== 0)));

  if (!hasMeaningfulData) {
    return null;
  }

  const hasDiscount =
    costBreakdown &&
    ((costBreakdown.discount_percent !== undefined && costBreakdown.discount_percent !== 0) ||
      (costBreakdown.discount_amount !== undefined && costBreakdown.discount_amount !== 0));

  const hasMargin =
    costBreakdown &&
    ((costBreakdown.margin_percent !== undefined && costBreakdown.margin_percent !== 0) ||
      (costBreakdown.margin_fixed_amount !== undefined && costBreakdown.margin_fixed_amount !== 0) ||
      (costBreakdown.margin_total_amount !== undefined && costBreakdown.margin_total_amount !== 0));

  // When cached, show $0 (authoritative total) instead of pre-cache costs from cost_breakdown
  const inputCost = isCached ? 0 : costBreakdown?.input_cost;
  const outputCost = isCached ? 0 : costBreakdown?.output_cost;
  const pricedOriginal =
    inputCost !== undefined || outputCost !== undefined ? (inputCost ?? 0) + (outputCost ?? 0) : undefined;
  const originalCost = isCached ? 0 : costBreakdown?.original_cost ?? pricedOriginal;
  const totalCost = isCached ? 0 : costBreakdown?.total_cost ?? totalSpend;
  const inputRateLine = isCached
    ? null
    : rateLine(promptTokens, "prompt tokens", costBreakdown?.input_cost_per_token);
  const outputRateLine = isCached
    ? null
    : rateLine(completionTokens, "completion tokens", costBreakdown?.output_cost_per_token);

  return (
    <div className="bg-card rounded-lg shadow-sm w-full max-w-full overflow-hidden mb-6">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger className="flex w-full items-center gap-3 px-4 py-3 text-left">
          {open ? (
            <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
          ) : (
            <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
          )}
          <div className="flex items-center justify-between w-full">
            <h3 className="text-lg font-medium text-foreground">Cost Breakdown</h3>
            <div className="flex items-center space-x-2 mr-4">
              {/* 分时定价的模型要标出这次落在哪一档：忙时价是闲时价的两倍，
                  不说的话运维会以为价目表记错了。 */}
              {costBreakdown?.window === "peak" && (
                <span className="rounded bg-amber-100 px-1.5 py-0.5 text-xs font-medium text-amber-900">
                  {t("Peak hours")}
                </span>
              )}
              {costBreakdown?.window === "offpeak" && (
                <span className="rounded bg-muted px-1.5 py-0.5 text-xs font-medium text-muted-foreground">
                  {t("Off-peak")}
                </span>
              )}
              <span className="text-sm text-muted-foreground">Total:</span>
              <span className="text-sm font-semibold text-foreground">
                {formatCost(totalSpend)}
                {isCached && t("(Cached)")}
              </span>
            </div>
          </div>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="p-6 space-y-4">
            {/* Step 1: Base Token Costs */}
            <div className="space-y-2 max-w-2xl">
              {(() => {
                const hasCacheBreakdown =
                  costBreakdown?.cache_read_cost !== undefined || costBreakdown?.cache_creation_cost !== undefined;
                if (hasCacheBreakdown) {
                  // Separate line items: Input / Cache Read / Cache Write
                  //
                  // input_cost 是提示侧的**全部**，包含缓存的那部分；缓存那两行
                  // 是从它里面拆出来单独显示的。所以要减掉才能得到未命中那部分的价。
                  //
                  // 减完可能正好是零（整段提示都命中缓存），这时显示 $0.00 是对的；
                  // 但只要它变成负数就说明两边的口径不一致，宁可夹到零也不把一个
                  // 负的输入成本摆给运维看。
                  const rawCost = isCached
                    ? 0
                    : Math.max(
                        0,
                        (inputCost ?? 0) -
                          (costBreakdown?.cache_read_cost ?? 0) -
                          (costBreakdown?.cache_creation_cost ?? 0),
                      );
                  return (
                    <>
                      <div className="flex text-sm">
                        <span className="text-muted-foreground font-medium w-1/3">Input Cost:</span>
                        <span className="text-foreground">
                          {formatCost(rawCost)}
                          {rawInputTokens !== undefined && rawInputTokens !== null && (
                            <span className="text-muted-foreground font-normal ml-1">
                              ({rawInputTokens.toLocaleString()} tokens)
                            </span>
                          )}
                        </span>
                      </div>
                      {(costBreakdown?.cache_read_cost ?? 0) > 0 && (
                        <div className="flex text-sm">
                          <span className="text-muted-foreground font-medium w-1/3">Prompt Cache Read Cost:</span>
                          <span className="text-foreground">
                            {formatCost(isCached ? 0 : costBreakdown?.cache_read_cost)}
                            {(cacheReadTokens ?? 0) > 0 && (
                              <span className="text-muted-foreground font-normal ml-1">
                                ({(cacheReadTokens ?? 0).toLocaleString()} tokens)
                              </span>
                            )}
                          </span>
                        </div>
                      )}
                      {(costBreakdown?.cache_creation_cost ?? 0) > 0 && (
                        <div className="flex text-sm">
                          <span className="text-muted-foreground font-medium w-1/3">Prompt Cache Write Cost:</span>
                          <span className="text-foreground">
                            {formatCost(isCached ? 0 : costBreakdown?.cache_creation_cost)}
                            {(cacheCreationTokens ?? 0) > 0 && (
                              <span className="text-muted-foreground font-normal ml-1">
                                ({(cacheCreationTokens ?? 0).toLocaleString()} tokens)
                              </span>
                            )}
                          </span>
                        </div>
                      )}
                    </>
                  );
                }
                return (
                  <div className="flex text-sm">
                    <span className="text-muted-foreground font-medium w-1/3">Input Cost:</span>
                    <span className="text-foreground">
                      {formatCost(inputCost)}
                      {(inputRateLine || promptTokens !== undefined) && (
                        <span className="text-muted-foreground font-normal ml-1">
                          ({inputRateLine ?? `${promptTokens!.toLocaleString("en-US")} prompt tokens`})
                        </span>
                      )}
                    </span>
                  </div>
                );
              })()}
              <div className="flex text-sm">
                <span className="text-muted-foreground font-medium w-1/3">Output Cost:</span>
                <span className="text-foreground">
                  {formatCost(outputCost)}
                  {(outputRateLine || completionTokens !== undefined) && (
                    <span className="text-muted-foreground font-normal ml-1">
                      ({outputRateLine ?? `${completionTokens!.toLocaleString("en-US")} completion tokens`})
                    </span>
                  )}
                </span>
              </div>
              {costBreakdown?.tool_usage_cost !== undefined && costBreakdown.tool_usage_cost > 0 && (
                <div className="flex text-sm">
                  <span className="text-muted-foreground font-medium w-1/3">Tool Usage Cost:</span>
                  <span className="text-foreground">{formatCost(costBreakdown.tool_usage_cost)}</span>
                </div>
              )}
              {costBreakdown?.additional_costs &&
                Object.entries(costBreakdown.additional_costs)
                  .filter(([, value]) => value != null && value !== 0)
                  .map(([key, value]) => (
                    <div key={key} className="flex text-sm">
                      <span className="text-muted-foreground font-medium w-1/3">{key}:</span>
                      <span className="text-foreground">{formatCost(value)}</span>
                    </div>
                  ))}
            </div>

            {/* 这次实际用到的费率。它是账单的依据，也是"当时按什么价扣的"的答案：
                这一列来自调用当时存下的快照，不会因为后来改价而变。 */}
            {appliedRates.length > 0 && (
              <div className="pt-2 border-t border-border max-w-2xl">
                <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  {t("Rates applied")}
                </div>
                <table className="w-full text-xs">
                  <thead>
                    <tr className="text-muted-foreground">
                      <th className="py-1 text-left font-medium">{t("Rate")}</th>
                      <th className="py-1 text-right font-medium">{t("Quantity")}</th>
                      <th className="py-1 text-right font-medium">{t("Unit price")}</th>
                      <th className="py-1 text-right font-medium">{t("Cost")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {appliedRates.map((rate, index) => (
                      <tr key={`${rate.side}-${rate.variant}-${index}`} className="text-foreground">
                        <td className="py-1">{rateLabel(rate.side, rate.variant)}</td>
                        <td className="py-1 text-right tabular-nums">
                          {(rate.quantity ?? 0).toLocaleString("en-US")} {measureLabel(rate.measure)}
                        </td>
                        <td className="py-1 text-right tabular-nums">{rateText(rate.usd, rate.measure)}</td>
                        <td className="py-1 text-right tabular-nums">{formatCost(rate.cost)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {/* 没有快照的历史行：数字是按那次调用的时刻重算的。标出来，
                不把它当成原始记录——单价表一改这个数就变了。 */}
            {costBreakdown?.source === "recomputed" && (
              <p className="max-w-2xl text-xs text-muted-foreground">
                {t(
                  "This call predates stored rates. The amounts are recalculated from the price catalog as of the call time and change when prices change.",
                )}
              </p>
            )}

            {/* Subtotal / Original Cost - hide when cached since it would be $0 */}
            {!isCached && (
              <div className="pt-2 border-t border-border max-w-2xl">
                <div className="flex text-sm font-semibold">
                  <span className="text-foreground w-1/3">Original LLM Cost:</span>
                  <span className="text-foreground">{formatCost(originalCost)}</span>
                </div>
              </div>
            )}

            {/* Step 2: Adjustments (Discount & Margin) */}
            {(hasDiscount || hasMargin) && (
              <div className="pt-2 space-y-2 max-w-2xl">
                {/* Discounts */}
                {hasDiscount && (
                  <div className="space-y-2">
                    {costBreakdown.discount_percent !== undefined && costBreakdown.discount_percent !== 0 && (
                      <div className="flex text-sm text-muted-foreground">
                        <span className="font-medium w-1/3">
                          Discount ({formatPercent(costBreakdown.discount_percent)}):</span>
                        <span className="text-foreground">-{formatCost(costBreakdown.discount_amount)}</span>
                      </div>
                    )}
                    {costBreakdown.discount_amount !== undefined && costBreakdown.discount_percent === undefined && (
                      <div className="flex text-sm text-muted-foreground">
                        <span className="font-medium w-1/3">Discount Amount:</span>
                        <span className="text-foreground">-{formatCost(costBreakdown.discount_amount)}</span>
                      </div>
                    )}
                  </div>
                )}

                {/* Margins */}
                {hasMargin && (
                  <div className="space-y-2">
                    {costBreakdown.margin_percent !== undefined && costBreakdown.margin_percent !== 0 && (
                      <div className="flex text-sm text-muted-foreground">
                        <span className="font-medium w-1/3">
                          Margin ({formatPercent(costBreakdown.margin_percent)}):</span>
                        <span className="text-foreground">
                          +
                          {formatCost(
                            (costBreakdown.margin_total_amount || 0) - (costBreakdown.margin_fixed_amount || 0),
                          )}
                        </span>
                      </div>
                    )}
                    {costBreakdown.margin_fixed_amount !== undefined && costBreakdown.margin_fixed_amount !== 0 && (
                      <div className="flex text-sm text-muted-foreground">
                        <span className="font-medium w-1/3">Margin:</span>
                        <span className="text-foreground">+{formatCost(costBreakdown.margin_fixed_amount)}</span>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )}

            {/* Final Summary */}
            <div className="mt-4 pt-4 border-t border-border max-w-2xl">
              <div className="flex items-center">
                <span className="font-bold text-sm text-foreground w-1/3">Final Calculated Cost:</span>
                <span className="text-sm font-bold text-foreground">
                  {formatCost(totalCost)}
                  {isCached && t("(Cached)")}
                </span>
              </div>
            </div>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
};

export default CostBreakdownViewer;
