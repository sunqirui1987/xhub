"use client";

import React, { useEffect, useState } from "react";
import { apiClient } from "../networking";
import { Button } from "@/components/ui/button";

/** 预览安全投影只接收部署和选路事实，不接收密钥或上游地址。 */
export type RoutePreviewResult = {
  source: string;
  rule_source: string;
  routing_strategy: string;
  data: { deployment_id: string; connection: string; supplier: string; upstream_model: string; protocol: string; excluded_reason?: string; weight?: number; target_share?: number }[];
};

/**
 * RoutePreview 将当前草稿交给后端真实规则解析和候选筛选。
 * 参数为令牌、公开模型、草稿及模板身份；返回只读预览 UI。
 * 模板编辑页调用；不保存草稿、不调用供应商；草稿或入口变化后清除过期结果。
 */
export default function RoutePreview({ accessToken, models, body, templateId, organizationId, teamId }: {
  accessToken: string | null; models: string[]; body: Record<string, unknown> | null;
  templateId?: string; organizationId?: string; teamId?: string;
}) {
  const [entries, setEntries] = useState<{ id: string; label: string }[]>([]);
  const [model, setModel] = useState("");
  const [endpoint, setEndpoint] = useState("chat");
  const [result, setResult] = useState<RoutePreviewResult | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  const serialized = JSON.stringify(body);
  useEffect(() => {
    let canceled = false;
    apiClient.get<{ endpoint_types?: { id: string; label: string }[] }>("/public/endpoints")
      .then(data => { if (!canceled) setEntries(data.endpoint_types ?? []); })
      .catch(() => { if (!canceled) setError("无法加载用户入口目录"); });
    return () => { canceled = true; };
  }, []);
  useEffect(() => { setResult(null); setError(""); }, [serialized, model, endpoint, templateId]);
  useEffect(() => {
    if (!revision || !accessToken || !body || !model) return;
    let canceled = false;
    setBusy(true);
    setError("");
    apiClient.post<RoutePreviewResult>("/route_template/preview", { accessToken, body: {
      model_name: model, endpoint_id: endpoint, body, template_id: templateId,
      organization_id: organizationId, team_id: teamId,
    } }).then(data => { if (!canceled) setResult(data); })
      .catch(() => { if (!canceled) setError("预览失败，请检查模板权限和草稿配置"); })
      .finally(() => { if (!canceled) setBusy(false); });
    return () => { canceled = true; };
    // 预览由按钮触发；变化会取消旧请求，只有再次点击才发送新草稿。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [revision]);
  return <section aria-label="真实路由预览" className="space-y-3 rounded-md border p-3">
    <p className="text-sm font-medium">路由预览</p>
    <p className="text-xs text-muted-foreground">先筛选兼容部署，再应用模板策略。份额表示新会话目标比例；已有会话优先固定原部署。预览不会调用上游。</p>
    <div className="flex flex-wrap gap-2">
      <label>公开模型 <select aria-label="预览公开模型" value={model} onChange={e => setModel(e.target.value)}>
        <option value="">选择模型</option>{models.map(name => <option key={name} value={name}>{name}</option>)}
      </select></label>
      <label>用户入口 <select aria-label="预览用户入口" value={endpoint} onChange={e => setEndpoint(e.target.value)}>
        {entries.map(entry => <option key={entry.id} value={entry.id}>{entry.label}</option>)}
      </select></label>
      <Button type="button" variant="outline" disabled={busy || !body || !accessToken || !model} onClick={() => setRevision(value => value + 1)}>
        {busy ? "正在预览" : "预览草稿路由"}
      </Button>
    </div>
    {error && <p role="alert">{error}</p>}
    {result && <div>
      <p>模板来源：{result.source} · 规则：{result.rule_source} · 策略：{result.routing_strategy}</p>
      {result.data.length === 0 && <p>没有匹配的部署</p>}
      <ul>{result.data.map(row => <li key={row.deployment_id} className="border-t py-2 text-sm">
        <p>{row.connection || row.supplier} · {row.upstream_model} · {row.protocol} · {row.deployment_id}</p>
        {row.excluded_reason ? <p>排除：{row.excluded_reason}</p> : <p>兼容候选{row.weight !== undefined && ` · 权重 ${row.weight}`}{row.target_share !== undefined && ` · 目标份额 ${(row.target_share * 100).toFixed(1)}%`}</p>}
      </li>)}</ul>
    </div>}
  </section>;
}
