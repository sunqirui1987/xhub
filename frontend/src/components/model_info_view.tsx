"use client";

import { useState } from "react";
import { ArrowLeft, Copy, Pencil, RefreshCw, Trash2, Check, Cable, Code2 } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useModelsInfo } from "@/app/(dashboard)/hooks/models/useModels";
import { useModelCostMap } from "@/app/(dashboard)/hooks/models/useModelCostMap";
import { useCredentials } from "@/app/(dashboard)/hooks/credentials/useCredentials";
import { isProxyAdminRole } from "@/utils/roles";
import ModelStatusToggle from "./add_model/ModelStatusToggle";
import { copyToClipboard } from "@/utils/dataUtils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { toast } from "@/lib/toast";
import { modelDeleteCall, testConnectionRequest } from "./networking";
import DeleteResourceModal from "./common_components/DeleteResourceModal";
import ModelEditor from "./add_model/ModelEditor";
import PricingTable from "./add_model/PricingTable";
import { effectivePricing, type EditorModel, type CatalogRow } from "./add_model/modelEditorPricing";

function callType(info: Record<string, unknown>): string {
  if (typeof info.transport === "string") return info.transport;
  return Array.isArray(info.endpoint_types) ? String(info.endpoint_types[0] ?? "") : "";
}

type Props = {
  modelId: string;
  onClose: () => void;
  accessToken: string | null;
  userID: string | null;
  userRole: string | null;
  isViewOnly: boolean;
  onModelUpdate?: (model: unknown) => void;
  modelAccessGroups?: string[] | null;
};

function Detail({ label, value }: { label: string; value: unknown }) {
  return (
    <div className="space-y-1.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="break-all text-sm font-medium">{String(value || "—")}</dd>
    </div>
  );
}

export default function ModelInfoView({
  modelId,
  onClose,
  accessToken,
  userID,
  userRole,
  isViewOnly,
  onModelUpdate,
}: Props) {
  const queryClient = useQueryClient();
  const models = useModelsInfo(1, 1, undefined, modelId);
  const { data: costMap } = useModelCostMap();
  const { data: credentials } = useCredentials();
  const [editing, setEditing] = useState(false);
  const [showJSON, setShowJSON] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [busy, setBusy] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; text: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const model = models.data?.data?.find((item) => item.model_info?.id === modelId) as EditorModel | undefined;
  const updated = async (result: unknown) => {
    setEditing(false);
    setTestResult(null);
    await queryClient.invalidateQueries({ queryKey: ["models"] });
    onModelUpdate?.(result);
  };
  const remove = async () => {
    if (!accessToken) return;
    setBusy(true);
    try {
      await modelDeleteCall(accessToken, modelId);
      await updated({ deleted: true, model_info: { id: modelId } });
      toast.success("模型已删除");
      onClose();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除失败");
    } finally {
      setBusy(false);
      setDeleting(false);
    }
  };
  const test = async () => {
    if (!accessToken || !model) return;
    setTesting(true);
    setTestResult(null);
    try {
      const response = await testConnectionRequest(
        accessToken,
        {
          model: model.litellm_params.model,
          custom_llm_provider: model.litellm_params.custom_llm_provider,
          litellm_credential_name: model.litellm_params.litellm_credential_name,
        },
        { id: modelId, endpoint_types: model.model_info.endpoint_types, transport: model.model_info.transport },
        callType(model.model_info),
      );
      if (response.status !== "success") throw new Error(response.result?.error || response.message || "连接测试失败");
      setTestResult({ ok: true, text: "连接正常，模型已响应。" });
    } catch (error) {
      setTestResult({ ok: false, text: error instanceof Error ? error.message : "连接测试失败" });
    } finally {
      setTesting(false);
    }
  };
  const copy = async () => {
    if (await copyToClipboard(modelId)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };
  if (models.isLoading)
    return (
      <div className="rounded-xl border bg-card p-8 text-sm text-muted-foreground" role="status">
        正在加载模型详情…
      </div>
    );
  if (models.error || !model)
    return (
      <div className="space-y-4 rounded-xl border bg-card p-8">
        <Button variant="ghost" onClick={onClose}>
          <ArrowLeft />
          返回模型列表
        </Button>
        <p role="alert">{models.error ? "模型加载失败，请重试。" : "未找到该模型。"}</p>
        <Button variant="outline" onClick={() => models.refetch()}>
          重新加载
        </Button>
      </div>
    );
  const params = model.litellm_params;
  const info = model.model_info;
  const supplier = credentials?.credentials.find((item) => item.credential_name === params.litellm_credential_name);
  const price = effectivePricing(model, (costMap ?? {}) as Record<string, CatalogRow>);
  const canEdit = !isViewOnly && isProxyAdminRole(userRole ?? "") && info.db_model === true;
  return (
    <div className="space-y-6">
      <Button variant="ghost" size="sm" onClick={onClose}>
        <ArrowLeft className="size-4" />
        返回模型列表
      </Button>
      <header className="flex flex-wrap items-start justify-between gap-4 rounded-xl border bg-card p-6">
        <div className="min-w-0 space-y-3">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="break-all text-2xl font-semibold tracking-tight">{model.model_name}</h1>
            <ModelStatusToggle model={model} />
            <Badge variant="outline">{callType(info)}</Badge>
          </div>
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <span className="break-all font-mono">{modelId}</span>
            <button type="button" aria-label="复制模型 ID" onClick={copy} className="rounded p-1 hover:bg-muted">
              {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
            </button>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {!isViewOnly && (
            <Button variant="outline" onClick={test} disabled={testing || editing || info.disabled === true}>
              <RefreshCw className={testing ? "size-4 animate-spin" : "size-4"} />
              {testing ? "测试中…" : "测试连接"}
            </Button>
          )}
          {canEdit && !editing && (
            <Button onClick={() => setEditing(true)}>
              <Pencil className="size-4" />
              编辑模型
            </Button>
          )}
          {canEdit && !editing && (
            <Button variant="ghost" className="text-destructive" onClick={() => setDeleting(true)}>
              <Trash2 className="size-4" />
              删除
            </Button>
          )}
        </div>
      </header>
      {testResult && (
        <div
          role="status"
          className={
            testResult.ok
              ? "rounded-lg border border-green-500/30 bg-green-500/5 p-4 text-sm"
              : "rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm"
          }
        >
          <p className="break-words">{testResult.text}</p>
        </div>
      )}
      {editing ? (
        <ModelEditor key={modelId} model={model} onCancel={() => setEditing(false)} onSaved={updated} />
      ) : (
        <>
          <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)]">
            <section className="space-y-6 rounded-xl border bg-card p-6">
              <h2 className="flex items-center gap-2 font-semibold">
                <Cable className="size-4 text-muted-foreground" />
                提供商与调用配置
              </h2>
              <dl className="grid gap-x-6 gap-y-6 sm:grid-cols-2">
                <Detail label="模型提供商" value={params.litellm_credential_name || "独立连接（历史配置）"} />
                <Detail
                  label="接口协议"
                  value={params.custom_llm_provider || supplier?.credential_info?.custom_llm_provider}
                />
                <Detail label="上游模型 ID" value={params.model} />
                <Detail label="对外模型名称" value={model.model_name} />
                <div className="sm:col-span-2">
                  <Detail
                    label="连接地址"
                    value={
                      params.api_base ||
                      supplier?.credential_info?.api_base ||
                      supplier?.credential_values?.api_base ||
                      "提供商默认地址"
                    }
                  />
                </div>
                <Detail label="调用方式" value={callType(info)} />
                <Detail label="传输方式" value={info.transport || "未配置上游执行配置"} />
                <Detail label="配置来源" value={info.db_model ? "控制台" : "配置文件"} />
                <Detail
                  label="创建时间"
                  value={info.created_at ? new Date(String(info.created_at)).toLocaleString() : undefined}
                />
              </dl>
              {!info.db_model && (
                <p className="border-t pt-4 text-sm text-muted-foreground">
                  此模型由配置文件管理，请在配置文件中修改。
                </p>
              )}
            </section>
            <section className="space-y-4 rounded-xl border bg-card p-6">
              <div>
                <h2 className="font-semibold">计价规则</h2>
                <p className="mt-1 break-all text-xs text-muted-foreground">{price.source} · USD</p>
              </div>
              <PricingTable row={price.row} />
              <p className="text-xs leading-5 text-muted-foreground">
                高峰：北京时间工作日 09:00–12:00、14:00–18:00，法定节假日除外。未设置高峰价时，基础价适用于全天。
              </p>
            </section>
          </div>
          <section className="overflow-hidden rounded-xl border bg-card">
            <button
              type="button"
              aria-expanded={showJSON}
              onClick={() => setShowJSON(!showJSON)}
              className="flex w-full items-center gap-2 p-5 text-left text-sm font-medium"
            >
              <Code2 className="size-4" />
              {showJSON ? "收起" : "查看"}模型配置 JSON
              <span className="ml-auto text-xs font-normal text-muted-foreground">敏感字段已隐藏</span>
            </button>
            {showJSON && (
              <pre className="max-h-96 overflow-auto border-t bg-muted/30 p-5 text-xs leading-6">
                {JSON.stringify(model, null, 2)}
              </pre>
            )}
          </section>
        </>
      )}
      <DeleteResourceModal
        isOpen={deleting}
        title="删除模型"
        message="删除后，该部署将不再接受新的模型请求。"
        resourceInformation={[
          { label: "模型", value: model.model_name },
          { label: "ID", value: modelId },
        ]}
        onCancel={() => setDeleting(false)}
        onOk={remove}
        confirmLoading={busy}
        requiredConfirmation={model.model_name}
      />
    </div>
  );
}
