"use client";
import { Input } from "@/components/ui/input";
import { policyError, strategyLabels, type AllocationPolicy } from "./allocationPolicy";
import type { SplitDeployment } from "./templateForm";
/** 编辑客户覆盖策略；参数为草稿、完整部署和回调，返回受控表单，不保存或调用上游。 */
export default function AllocationPolicyEditor({
  policy,
  deployments,
  onChange,
  disabled = false,
}: {
  policy: AllocationPolicy;
  deployments: SplitDeployment[];
  onChange: (policy: AllocationPolicy) => void;
  disabled?: boolean;
}) {
  const rows = deployments.filter((row) => row.deployment_id);
  const error = policyError(policy);
  return (
    <fieldset disabled={disabled} className="space-y-3">
      {
        <label className="flex items-center gap-3">
          分配方式{" "}
          <select
            aria-label="分配方式"
            className="rounded-md border bg-background p-2"
            value={policy.strategy}
            onChange={(event) => {
              const strategy = event.target.value;
              onChange(
                strategy === "traffic-split"
                  ? { strategy, allocations: rows.map((row) => ({ deployment_id: row.deployment_id!, weight: 1 })) }
                  : { strategy },
              );
            }}
          >
            {Object.entries(strategyLabels).map(([id, label]) => (
              <option key={id} value={id}>
                {label}
              </option>
            ))}
          </select>
        </label>
      }
      {policy.strategy === "random" && (
        <p className="text-sm text-muted-foreground">每次新分配从可用且兼容的部署中等概率随机选择。</p>
      )}
      {policy.strategy === "traffic-split" && (
        <>
          <p className="text-sm text-muted-foreground">
            按相对权重随机分配：3 和 7 约为 30% 和 70%。默认权重为 1，0 不参与分配。
          </p>
          {rows.map((row) => (
            <div
              key={row.deployment_id}
              className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-3"
            >
              <div>
                <p>
                  {row.supplier || row.provider || "独立连接"} · {row.model}
                </p>
                <p className="text-xs text-muted-foreground">
                  {row.transport} · {row.deployment_id}
                </p>
                {row.unavailable_reason && <p className="text-xs text-destructive">{row.unavailable_reason}</p>}
              </div>
              <label className="flex items-center gap-2">
                <Input
                  className="w-24"
                  type="number"
                  min={0}
                  step="any"
                  aria-label={"部署 " + row.deployment_id + " 权重"}
                  value={policy.allocations?.find((a) => a.deployment_id === row.deployment_id)?.weight ?? 1}
                  onChange={(event) => {
                    const allocations = (policy.allocations ?? []).filter((a) => a.deployment_id !== row.deployment_id);
                    onChange({
                      strategy: "traffic-split",
                      allocations: [
                        ...allocations,
                        { deployment_id: row.deployment_id!, weight: Number(event.target.value) },
                      ],
                    });
                  }}
                />
              </label>
            </div>
          ))}
        </>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </fieldset>
  );
}
