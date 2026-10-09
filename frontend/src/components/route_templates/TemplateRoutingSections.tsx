"use client";
import { useMemo, useState } from "react";
import DeploymentIdentity from "./DeploymentIdentity";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  groupFormError,
  groupHints,
  groupStrategies,
  reconcileGroup,
  type GroupConfig,
  type CatalogModel,
} from "@/components/routing_groups/groupForm";
import { fallbackFields, type FallbackField, validateSections } from "./templateSections";
import type { TemplateFormState, SplitDeployment } from "./templateForm";
const categories: Record<FallbackField, string> = {
  fallbacks: "通用错误",
  context_window_fallbacks: "上下文超限",
  content_policy_fallbacks: "内容策略错误",
};
type Props = { form: TemplateFormState; deployments: SplitDeployment[]; onChange: (form: TemplateFormState) => void };
/** TemplateGroups 编辑模板内组数组；参数为草稿、目录和更新回调，返回列表与完整组弹窗。
 * 弹窗确认仅更新本地草稿；成员占用范围限于当前模板，权重引用真实部署，无独立接口写入。 */
export function TemplateGroups({ form, deployments, onChange }: Props) {
  const groups = form.routing_groups ?? [];
  const [draft, setDraft] = useState<GroupConfig | null>(null),
    [editing, setEditing] = useState<number | null>(null);
  const [search, setSearch] = useState(""),
    [memberSearch, setMemberSearch] = useState(""),
    [expanded, setExpanded] = useState("");
  const catalog = useMemo<CatalogModel[]>(
    () =>
      [...new Set(deployments.map((d) => d.model_name))].map((name) => ({
        model_name: name,
        deployments: deployments
          .filter((d) => d.model_name === name)
          .map((d) => ({ id: d.deployment_id!, litellm_params: { model: d.model } })),
      })),
    [deployments],
  );
  const error = draft
    ? groupFormError(
        draft,
        groups.filter((_, i) => i !== editing),
        catalog,
      )
    : "";
  /** open 创建或复制组弹窗草稿；参数为索引，返回空，仅设置本地状态。 */
  const open = (index: number | null) => {
    setEditing(index);
    setMemberSearch("");
    setDraft(
      index === null
        ? { group_name: "", models: [], routing_strategy: "simple-shuffle" }
        : structuredClone(groups[index]),
    );
  };
  /** confirm 将合法弹窗值替换到模板数组；无参数和返回，不调用后台，保存模板时统一提交。 */
  const confirm = () => {
    if (!draft || error) return;
    onChange({
      ...form,
      routing_groups: editing === null ? [...groups, draft] : groups.map((g, i) => (i === editing ? draft : g)),
    });
    setDraft(null);
  };
  return (
    <section className="space-y-5" aria-label="模板路由组">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h3 className="text-lg font-semibold">路由组</h3>
          <p className="mt-1 text-sm text-muted-foreground">为一组模型设置独立策略，也可以直接使用组名调用。</p>
        </div>
        <Button onClick={() => open(null)}>创建路由组</Button>
      </div>
      <Input
        aria-label="搜索路由组"
        placeholder="搜索组名或成员模型"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-md"
      />
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-left text-sm">
          <thead className="bg-muted/30 text-muted-foreground">
            <tr>
              <th className="p-4">组名称</th>
              <th className="p-4">成员模型</th>
              <th className="p-4">路由策略</th>
              <th className="p-4 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {groups
              .map((g, index) => ({ g, index }))
              .filter(({ g }) => (g.group_name + g.models.join(" ")).toLowerCase().includes(search.toLowerCase()))
              .map(({ g, index }) => (
                <GroupRow
                  key={g.group_name}
                  group={g}
                  expanded={expanded === g.group_name}
                  onExpand={() => setExpanded(expanded === g.group_name ? "" : g.group_name)}
                  onEdit={() => open(index)}
                  deleteBlocked={fallbackFields.some((field) =>
                    (form[field] ?? []).some((row) =>
                      Object.entries(row).some(
                        ([source, targets]) => source === g.group_name || targets.includes(g.group_name),
                      ),
                    ),
                  )}
                  onDelete={() => onChange({ ...form, routing_groups: groups.filter((_, i) => i !== index) })}
                />
              ))}
            {!groups.length && (
              <tr>
                <td colSpan={4} className="p-12 text-center text-muted-foreground">
                  暂无路由组。未加入组的模型使用负载均衡规则或模型管理的端点权重。
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <Dialog open={!!draft} onOpenChange={(value) => !value && setDraft(null)}>
        <DialogContent className="flex max-h-[90dvh] flex-col overflow-hidden sm:max-w-2xl">
          <DialogHeader className="shrink-0">
            <DialogTitle>{editing === null ? "创建路由组" : "编辑路由组"}</DialogTitle>
            <DialogDescription>保存到当前模板草稿，提交模板后生效。</DialogDescription>
          </DialogHeader>
          {draft && (
            <div className="min-h-0 space-y-5 overflow-y-auto pr-1">
              <label className="block space-y-2">
                <span className="font-medium">组名称 *</span>
                <Input
                  aria-label="路由组名称"
                  disabled={editing !== null}
                  value={draft.group_name}
                  onChange={(e) => setDraft({ ...draft, group_name: e.target.value })}
                />
                <p className="text-xs text-muted-foreground">API model 可填写组名。1–64 字符，不能与公开模型重名。</p>
              </label>
              <section className="space-y-2">
                <h4 className="font-medium">
                  成员模型 * <span className="text-xs text-muted-foreground">已选 {draft.models.length} 个</span>
                </h4>
                <Input
                  aria-label="搜索成员模型"
                  placeholder="搜索公开模型"
                  value={memberSearch}
                  onChange={(e) => setMemberSearch(e.target.value)}
                />
                <div className="max-h-48 overflow-y-auto rounded-md border p-2">
                  {catalog
                    .filter(
                      (m) =>
                        !m.model_name.includes("*") && m.model_name.toLowerCase().includes(memberSearch.toLowerCase()),
                    )
                    .map((m) => {
                      const owner = groups.find((g, i) => i !== editing && g.models.includes(m.model_name));
                      return (
                        <label
                          key={m.model_name}
                          className="flex items-center justify-between gap-3 rounded p-2 hover:bg-muted/40"
                        >
                          <span className="flex items-center gap-2">
                            <input
                              type="checkbox"
                              aria-label={"成员模型 " + m.model_name}
                              checked={draft.models.includes(m.model_name)}
                              disabled={!!owner}
                              onChange={(e) =>
                                setDraft(
                                  reconcileGroup(
                                    {
                                      ...draft,
                                      models: e.target.checked
                                        ? [...draft.models, m.model_name]
                                        : draft.models.filter((n) => n !== m.model_name),
                                    },
                                    catalog,
                                  ),
                                )
                              }
                            />
                            <span className="break-all">{m.model_name}</span>
                          </span>
                          <span className="text-xs text-muted-foreground">
                            {owner ? "已加入 " + owner.group_name : m.deployments.length + " 个部署"}
                          </span>
                        </label>
                      );
                    })}
                </div>
                <p className="text-xs text-muted-foreground">同一模板内，每个公开模型最多属于一个路由组。</p>
              </section>
              <label className="block space-y-2">
                <span className="font-medium">路由策略 *</span>
                <select
                  aria-label="路由组策略"
                  className="w-full rounded-md border bg-background p-3"
                  value={draft.routing_strategy}
                  onChange={(e) => setDraft(reconcileGroup({ ...draft, routing_strategy: e.target.value }, catalog))}
                >
                  {Object.entries({ ...groupStrategies, random: "简单随机（兼容值）" }).map(([key, label]) => (
                    <option key={key} value={key}>
                      {label} · {key}
                    </option>
                  ))}
                </select>
                <p className="text-xs text-muted-foreground">{groupHints[draft.routing_strategy]}</p>
              </label>
              {draft.routing_strategy === "traffic-split" && (
                <section className="space-y-3 rounded-lg border p-4">
                  <h4 className="font-medium">部署相对权重</h4>
                  <p className="text-xs text-muted-foreground">
                    3 / 7 表示约 30% / 70%；0 排除部署。未填写的部署权重为 1。
                  </p>
                  {draft.routing_strategy_args?.allocations.map((row, i) => (
                    <label key={row.deployment_id} className="flex items-center justify-between gap-3">
                      <DeploymentIdentity
                        id={row.deployment_id}
                        deployment={deployments.find((d) => d.deployment_id === row.deployment_id)}
                      />
                      <Input
                        className="w-28 shrink-0"
                        type="number"
                        min="0"
                        step="any"
                        aria-label={"路由组部署 " + row.deployment_id + " 权重"}
                        value={Number.isNaN(row.weight) ? "" : row.weight}
                        onChange={(e) =>
                          setDraft({
                            ...draft,
                            routing_strategy_args: {
                              allocations: draft.routing_strategy_args!.allocations.map((r, j) =>
                                j === i ? { ...r, weight: e.target.value === "" ? NaN : Number(e.target.value) } : r,
                              ),
                            },
                          })
                        }
                      />
                    </label>
                  ))}
                </section>
              )}
              {error && draft.group_name && (
                <p role="alert" className="text-sm text-destructive">
                  {error}
                </p>
              )}
            </div>
          )}
          <DialogFooter className="shrink-0 border-t pt-4">
            <Button variant="outline" onClick={() => setDraft(null)}>
              取消
            </Button>
            <Button disabled={!!error} onClick={confirm}>
              保存到模板
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
/** GroupRow 呈现模板组及三种可复制调用示例；参数为配置和草稿回调，无网络请求。 */
function GroupRow({
  group: g,
  expanded,
  onExpand,
  onEdit,
  onDelete,
  deleteBlocked,
}: {
  group: GroupConfig;
  expanded: boolean;
  onExpand: () => void;
  onEdit: () => void;
  onDelete: () => void;
  deleteBlocked: boolean;
}) {
  const [language, setLanguage] = useState("cURL");
  const request = { model: g.group_name, messages: [{ role: "user", content: "你好" }] };
  const endpoint = "$XHUB_BASE_URL";
  const code =
    language === "cURL"
      ? [
          "curl -X POST '" + endpoint + "/v1/chat/completions' \\",
          "  -H 'Authorization: Bearer $XHUB_API_KEY' \\",
          "  -H 'Content-Type: application/json' \\",
          "  -d '" + JSON.stringify(request, null, 2) + "'",
        ].join("\n")
      : language === "Python"
        ? 'import os\nfrom openai import OpenAI\n\nclient = OpenAI(base_url=os.environ["XHUB_BASE_URL"] + "/v1", api_key=os.environ["XHUB_API_KEY"])\nresponse = client.chat.completions.create(\n    model="' +
          g.group_name +
          '",\n    messages=[{"role": "user", "content": "你好"}]\n)'
        : 'import OpenAI from "openai";\n\nconst client = new OpenAI({ baseURL: process.env.XHUB_BASE_URL + "/v1", apiKey: process.env.XHUB_API_KEY });\nconst response = await client.chat.completions.create(' +
          JSON.stringify(request, null, 2) +
          ");";
  return (
    <>
      <tr className="border-t">
        <td className="p-4">
          <Button
            variant="ghost"
            className="whitespace-normal text-left text-primary"
            aria-label={"展开路由组 " + g.group_name}
            aria-expanded={expanded}
            onClick={onExpand}
          >
            {expanded ? "−" : "+"} {g.group_name}
          </Button>
        </td>
        <td className="p-4">
          <div className="flex flex-wrap gap-2">
            {g.models.map((n) => (
              <span key={n} className="rounded border bg-muted/20 px-2 py-1 text-xs">
                {n}
              </span>
            ))}
          </div>
        </td>
        <td className="p-4">
          <p>{groupStrategies[g.routing_strategy] ?? g.routing_strategy}</p>
          <p className="mt-1 font-mono text-xs text-muted-foreground">{g.routing_strategy}</p>
        </td>
        <td className="p-4">
          <div className="flex justify-end gap-2">
            <Button variant="outline" aria-label={"编辑路由组 " + g.group_name} onClick={onEdit}>
              编辑
            </Button>
            <Button
              variant="ghost"
              aria-label={"删除路由组 " + g.group_name}
              onClick={onDelete}
              disabled={deleteBlocked}
              title={deleteBlocked ? "请先移除故障转移中对该组的引用" : undefined}
            >
              删除
            </Button>
          </div>
        </td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={4} className="border-t bg-muted/20 p-5">
            <p className="font-medium">调用这个路由组</p>
            <p className="mt-2 text-sm text-muted-foreground">
              使用绑定当前模板的密钥，填写组名作为 model。{groupHints[g.routing_strategy]}
            </p>
            <div className="my-3 flex flex-wrap gap-2">
              {["cURL", "Python", "JavaScript"].map((n) => (
                <Button
                  key={n}
                  size="sm"
                  variant={language === n ? "secondary" : "ghost"}
                  onClick={() => setLanguage(n)}
                >
                  {n}
                </Button>
              ))}
            </div>
            <pre className="overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-slate-100">{code}</pre>
          </td>
        </tr>
      )}
    </>
  );
}
/** TemplateFallbacks 用表单维护模板三类有序回退映射；确认写本地草稿，清空或删除随模板整体保存。
 * 参数为草稿和实时目录；允许模型或本模板组名，禁止重复、自引用及跨类型循环，无独立API写入。 */
export function TemplateFallbacks({ form, deployments, onChange }: Props) {
  const [draft, setDraft] = useState<{ source: string; field: FallbackField; targets: string[] } | null>(null);
  const [editing, setEditing] = useState<{ source: string; field: FallbackField } | null>(null);
  const names = [
    ...new Set([...deployments.map((d) => d.model_name), ...(form.routing_groups ?? []).map((g) => g.group_name)]),
  ].filter((n) => !n.includes("*"));
  /** candidate 生成单条变更后的完整草稿，参数为空，返回副本，保留其他类型和主模型配置。 */
  const candidate = () => {
    if (!draft) return form;
    let next = { ...form };
    if (editing)
      next = {
        ...next,
        [editing.field]: (next[editing.field] ?? []).filter((row) => !Object.hasOwn(row, editing.source)),
      };
    return { ...next, [draft.field]: [...(next[draft.field] ?? []), { [draft.source]: draft.targets }] };
  };
  const invalid = !draft
    ? ""
    : !names.includes(draft.source)
      ? "请选择主模型。"
      : draft.targets.some((t) => !names.includes(t))
        ? "请选择存在的回退目标。"
        : (form[draft.field] ?? []).some(
              (row) =>
                Object.hasOwn(row, draft.source) &&
                !(editing?.field === draft.field && editing.source === draft.source),
            )
          ? "此主模型和错误类型已有回退链。"
          : validateSections(candidate())
            ? "回退目标不能重复、自引用或形成循环。"
            : "";
  /** move 按索引交换相邻目标；只更新弹窗草稿，边界按钮被禁用，无返回值。 */
  const move = (i: number, offset: number) => {
    if (!draft) return;
    const targets = [...draft.targets];
    [targets[i], targets[i + offset]] = [targets[i + offset], targets[i]];
    setDraft({ ...draft, targets });
  };
  return (
    <section className="space-y-5" aria-label="模板故障转移">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h3 className="text-lg font-semibold">故障转移</h3>
          <p className="mt-1 text-sm text-muted-foreground">当前模型或组尝试耗尽后，按顺序调用回退目标。</p>
        </div>
        <Button
          onClick={() => {
            setEditing(null);
            setDraft({ source: "", field: "fallbacks", targets: [] });
          }}
        >
          添加故障转移
        </Button>
      </div>
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-left text-sm">
          <thead className="bg-muted/30 text-muted-foreground">
            <tr>
              <th className="p-4">主模型 / 路由组</th>
              <th className="p-4">触发条件</th>
              <th className="p-4">有序回退链</th>
              <th className="p-4 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {fallbackFields.flatMap((field) =>
              (form[field] ?? []).flatMap((row) =>
                Object.entries(row).map(([source, targets]) => (
                  <tr key={field + source} className="border-t">
                    <td className="p-4 font-medium">{source}</td>
                    <td className="p-4">{categories[field]}</td>
                    <td className="p-4">
                      <div className="flex flex-wrap items-center gap-2">
                        {targets.length ? (
                          targets.map((name, i) => (
                            <span key={name} className="rounded border bg-muted/20 px-2 py-1 text-xs">
                              {i + 1}. {name}
                            </span>
                          ))
                        ) : (
                          <span className="text-muted-foreground">不跨模型回退</span>
                        )}
                      </div>
                    </td>
                    <td className="p-4">
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="outline"
                          aria-label={"编辑故障转移 " + source + " " + categories[field]}
                          onClick={() => {
                            setEditing({ source, field });
                            setDraft({ source, field, targets: [...targets] });
                          }}
                        >
                          编辑
                        </Button>
                        <Button
                          variant="ghost"
                          aria-label={"删除故障转移 " + source + " " + categories[field]}
                          onClick={() =>
                            onChange({ ...form, [field]: (form[field] ?? []).filter((r) => !Object.hasOwn(r, source)) })
                          }
                        >
                          删除
                        </Button>
                      </div>
                    </td>
                  </tr>
                )),
              ),
            )}
            {!fallbackFields.some((field) => form[field]?.length) && (
              <tr>
                <td colSpan={4} className="p-12 text-center text-muted-foreground">
                  暂无故障转移。添加主模型及回退目标，设置尝试顺序。
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <div className="rounded-lg bg-muted/30 p-4 text-sm text-muted-foreground">
        未配置的主模型与错误类型继承模型管理的默认回退；显式清空目标可禁用对应链。通用错误：限流、超时和服务异常。上下文超限与内容策略错误使用各自的回退链。流式内容发出后停止重放。
      </div>
      <Dialog open={!!draft} onOpenChange={(value) => !value && setDraft(null)}>
        <DialogContent className="flex max-h-[90dvh] flex-col overflow-hidden sm:max-w-2xl">
          <DialogHeader className="shrink-0">
            <DialogTitle>配置模板故障转移</DialogTitle>
            <DialogDescription>目标按从上到下的顺序尝试；最多 32 个目标。</DialogDescription>
          </DialogHeader>
          {draft && (
            <div className="min-h-0 space-y-5 overflow-y-auto pr-1">
              <label className="block space-y-2">
                <span className="font-medium">主模型 / 路由组 *</span>
                <select
                  aria-label="故障转移主模型"
                  className="w-full rounded-md border bg-background p-3"
                  value={draft.source}
                  onChange={(e) => setDraft({ ...draft, source: e.target.value })}
                >
                  <option value="">选择主模型或路由组</option>
                  {names.map((n) => (
                    <option key={n}>{n}</option>
                  ))}
                </select>
              </label>
              <label className="block space-y-2">
                <span className="font-medium">触发条件</span>
                <select
                  aria-label="故障转移触发条件"
                  className="w-full rounded-md border bg-background p-3"
                  value={draft.field}
                  onChange={(e) => setDraft({ ...draft, field: e.target.value as FallbackField })}
                >
                  {fallbackFields.map((field) => (
                    <option key={field} value={field}>
                      {categories[field]}
                    </option>
                  ))}
                </select>
              </label>
              <div className="rounded-lg border p-4">
                <p className="mb-3 text-center text-xs font-medium text-primary">↓ 失败后，依次尝试</p>
                <div className="space-y-3">
                  {draft.targets.map((target, i) => (
                    <div key={i} className="flex flex-wrap items-center gap-2">
                      <span className="text-sm text-muted-foreground">{i + 1}</span>
                      <select
                        aria-label={"回退目标 " + (i + 1)}
                        className="min-w-0 flex-1 rounded-md border bg-background p-2"
                        value={target}
                        onChange={(e) =>
                          setDraft({ ...draft, targets: draft.targets.map((t, j) => (i === j ? e.target.value : t)) })
                        }
                      >
                        <option value="">选择回退目标</option>
                        {names
                          .filter((n) => n !== draft.source)
                          .map((n) => (
                            <option key={n} disabled={draft.targets.includes(n) && n !== target}>
                              {n}
                            </option>
                          ))}
                      </select>
                      <Button
                        variant="ghost"
                        size="sm"
                        aria-label={"回退上移 " + (i + 1)}
                        disabled={i === 0}
                        onClick={() => move(i, -1)}
                      >
                        ↑
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        aria-label={"回退下移 " + (i + 1)}
                        disabled={i === draft.targets.length - 1}
                        onClick={() => move(i, 1)}
                      >
                        ↓
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        aria-label={"移除回退 " + (i + 1)}
                        onClick={() => setDraft({ ...draft, targets: draft.targets.filter((_, j) => i !== j) })}
                      >
                        移除
                      </Button>
                    </div>
                  ))}
                </div>
                <Button
                  className="mt-4"
                  variant="outline"
                  disabled={!draft.source || draft.targets.length >= 32}
                  onClick={() => setDraft({ ...draft, targets: [...draft.targets, ""] })}
                >
                  添加回退目标
                </Button>
              </div>
              {invalid && draft.source && (
                <p role="alert" className="text-sm text-destructive">
                  {invalid}
                </p>
              )}
            </div>
          )}
          <DialogFooter className="shrink-0 border-t pt-4">
            <Button
              variant="ghost"
              disabled={!draft?.targets.length}
              onClick={() => draft && setDraft({ ...draft, targets: [] })}
            >
              清空回退
            </Button>
            <Button variant="outline" onClick={() => setDraft(null)}>
              取消
            </Button>
            <Button
              disabled={!!invalid || !draft}
              onClick={() => {
                onChange(candidate());
                setDraft(null);
              }}
            >
              保存到模板
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
