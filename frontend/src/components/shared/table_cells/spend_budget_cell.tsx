"use client";

import { InheritedBudgetHint, type InheritedBudgetGate } from "@/components/shared/InheritedBudgetHint";
import { Meter, MeterIndicator, MeterTrack } from "@/components/shared/Meter";
import { formatNumberWithCommas, getSpendString } from "@/utils/dataUtils";
import { t } from "@/i18n";

interface SpendBudgetCellProps {
  spend: number | null | undefined;
  root?: boolean;
  maxBudget: number | null | undefined;
  inheritedGates?: readonly InheritedBudgetGate[];
  spendDecimals?: number;
  budgetDecimals?: number;
}

/** 根据消费比例选择颜色；参数 pct 为百分比，返回状态，供额度单元格使用，无副作用。 */
const meterTone = (pct: number): "default" | "warning" | "over" => {
  if (pct > 100) return "over";
  if (pct >= 80) return "warning";
  return "default";
};

/** 显示消费和额度；参数为金额、继承链与根节点标记，返回可访问的仪表。空子额度共享父级，根组织为空表示不限额，无写入。 */
export function SpendBudgetCell({
  spend,
  maxBudget,
  root = false,
  inheritedGates = [],
  spendDecimals = 4,
  budgetDecimals = 0,
}: SpendBudgetCellProps) {
  const spendValue = typeof spend === "number" && !Number.isNaN(spend) ? spend : 0;
  const budget = maxBudget ?? null;
  const hasBudget = typeof budget === "number" && budget > 0;
  const pct = hasBudget ? (spendValue / budget) * 100 : 0;

  const spendText = spendValue > 0 ? getSpendString(spendValue, spendDecimals) : "$0.00";
  const budgetLabel =
    budget === null
      ? t(root ? "Unlimited" : "quotaGuide.blank")
      : t("quotaGuide.amount", { amount: formatNumberWithCommas(budget, budgetDecimals) });

  return (
    <div className="flex min-w-[130px] flex-col gap-1">
      <div className="whitespace-nowrap text-xs">
        <span className="font-medium tabular-nums text-foreground">{spendText}</span>{" "}
        <span className="text-muted-foreground">{budgetLabel}</span>
        {budget === null && <InheritedBudgetHint gates={inheritedGates} />}
      </div>
      {hasBudget && (
        <Meter
          value={spendValue}
          max={budget}
          aria-valuetext={t("{spendText} of ${value1}", {
            spendText,
            value1: formatNumberWithCommas(budget, budgetDecimals),
          })}
        >
          <MeterTrack>
            <MeterIndicator tone={meterTone(pct)} />
          </MeterTrack>
        </Meter>
      )}
    </div>
  );
}
