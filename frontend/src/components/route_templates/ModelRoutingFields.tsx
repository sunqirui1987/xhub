"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import DeploymentIdentity from "./DeploymentIdentity";
import { Input } from "@/components/ui/input";
import { strategyLabels } from "./allocationPolicy";
import type { ModelRoutingRule, SplitDeployment } from "./templateForm";

/** ModelRoutingFields 编辑模板的按模型负载均衡规则；参数为规则、部署目录和更新回调，
 * 返回可访问的受控表单；添加时使用 simple-shuffle，去重并允许移除，不写后台。 */
export default function ModelRoutingFields({
  rules,
  deployments,
  onChange,
}: {
  rules: ModelRoutingRule[];
  deployments: SplitDeployment[];
  onChange: (rules: ModelRoutingRule[]) => void;
}) {
  const [model, setModel] = useState("");
  const configured = new Set(rules.map((rule) => rule.model));
  const names = [...new Set(deployments.map((row) => row.model_name))].filter((name) => !configured.has(name)).sort();

  return (
    <section className="space-y-4" aria-label="模型规则">
      <div>
        <h3 className="font-semibold">模型负载均衡规则</h3>
        <p className="text-sm text-muted-foreground">
          只添加需要不同策略的公开模型；其余模型继承模型管理中的端点权重。每个模型在自己的兼容部署之间分配请求。
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        <select
          aria-label="公开模型"
          className="rounded border bg-background p-2"
          value={model}
          onChange={(event) => setModel(event.target.value)}
        >
          <option value="">选择公开模型</option>
          {names.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
        <Button
          type="button"
          disabled={!model}
          onClick={() => {
            onChange([...rules, { model, strategy: "simple-shuffle" }]);
            setModel("");
          }}
        >
          添加模型规则
        </Button>
      </div>
      {rules.map((rule, index) => (
        <section
          key={rule.model}
          className="flex flex-wrap items-center justify-between gap-4 rounded-lg border p-4"
          aria-label={"模型规则 " + rule.model}
        >
          <div>
            <h4 className="font-semibold">{rule.model}</h4>
            {rule.strategy === "traffic-split" && (
              <p className="text-sm text-muted-foreground">可继承模型默认权重，也可在当前模板中单独配置。</p>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <label className="flex items-center gap-2">
              路由逻辑
              <select
                aria-label={rule.model + " 路由逻辑"}
                className="rounded border bg-background p-2"
                value={rule.strategy}
                onChange={(event) =>
                  onChange(
                    rules.map((row, i) =>
                      i === index
                        ? {
                            model: row.model,
                            strategy: event.target.value,
                            ...(event.target.value === "traffic-split"
                              ? {
                                  allocations: deployments
                                    .filter((d) => d.model_name === row.model && d.deployment_id)
                                    .map((d) => ({ deployment_id: d.deployment_id!, weight: 1 })),
                                }
                              : {}),
                          }
                        : row,
                    ),
                  )
                }
              >
                {Object.entries(strategyLabels).map(([id, label]) => (
                  <option key={id} value={id}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
            <Button type="button" variant="ghost" onClick={() => onChange(rules.filter((_, i) => i !== index))}>
              删除规则
            </Button>
          </div>
          {rule.strategy === "traffic-split" && (
            <div className="w-full space-y-3 border-t pt-4">
              <label className="flex flex-wrap items-center gap-3">
                权重来源
                <select
                  aria-label={rule.model + " 权重来源"}
                  className="rounded border bg-background p-2"
                  value={rule.allocations === undefined ? "inherit" : "custom"}
                  onChange={(e) =>
                    onChange(
                      rules.map((r, i) =>
                        i !== index
                          ? r
                          : e.target.value === "inherit"
                            ? { model: r.model, strategy: r.strategy }
                            : {
                                ...r,
                                allocations: deployments
                                  .filter((d) => d.model_name === r.model && d.deployment_id)
                                  .map((d) => ({ deployment_id: d.deployment_id!, weight: 1 })),
                              },
                      ),
                    )
                  }
                >
                  <option value="custom">自定义权重</option>
                  <option value="inherit">继承默认权重</option>
                </select>
              </label>
              {rule.allocations !== undefined ? (
                <>
                  <p className="text-sm text-muted-foreground">
                    仅当前模板生效。3 / 7 表示约 30% / 70%；0 排除部署；未列出的部署权重为 1。
                  </p>
                  {[
                    ...new Set([
                      ...deployments
                        .filter((d) => d.model_name === rule.model && d.deployment_id)
                        .map((d) => d.deployment_id!),
                      ...rule.allocations.map((a) => a.deployment_id),
                    ]),
                  ].map((id) => {
                    const weight = rule.allocations!.find((a) => a.deployment_id === id)?.weight ?? 1;
                    return (
                      <label key={id} className="flex items-center justify-between gap-3 rounded border p-3">
                        <DeploymentIdentity id={id} deployment={deployments.find((d) => d.deployment_id === id)} />
                        <Input
                          type="number"
                          min="0"
                          step="any"
                          className="w-28 shrink-0"
                          aria-label={"模板部署 " + id + " 权重"}
                          value={Number.isNaN(weight) ? "" : weight}
                          onChange={(e) =>
                            onChange(
                              rules.map((r, i) =>
                                i !== index
                                  ? r
                                  : {
                                      ...r,
                                      allocations: [
                                        ...r.allocations!.filter((a) => a.deployment_id !== id),
                                        {
                                          deployment_id: id,
                                          weight: e.target.value === "" ? NaN : Number(e.target.value),
                                        },
                                      ],
                                    },
                              ),
                            )
                          }
                        />
                      </label>
                    );
                  })}
                </>
              ) : (
                <p className="text-sm text-muted-foreground">
                  实时继承模型管理的默认权重；切换自定义后由模板独立控制。
                </p>
              )}
            </div>
          )}
        </section>
      ))}
      {rules.length === 0 && (
        <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          尚未配置模型负载均衡规则，所有模型读取模型管理中的端点权重。
        </p>
      )}
    </section>
  );
}
