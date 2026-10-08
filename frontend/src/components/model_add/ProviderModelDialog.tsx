"use client";

import { useEffect, useState } from "react";
import { CredentialItem, apiClient, upsertPriceModel } from "@/components/networking";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { toast } from "@/lib/toast";
import { providerCatalogEntry, type ListedModel } from "./providerCatalogImport";

export function ProviderModelDialog({
  provider,
  initialCredentialName,
  credentials,
  accessToken,
  onClose,
  onAdded,
}: {
  provider: string;
  initialCredentialName?: string;
  credentials: CredentialItem[];
  accessToken: string;
  onClose: () => void;
  onAdded: () => void;
}) {
  const [models, setModels] = useState<ListedModel[]>([]);
  const [picked, setPicked] = useState<string[]>([]);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const supplier = credentials.find((item) => item.credential_name === initialCredentialName);
  useEffect(() => {
    const controller = new AbortController();
    apiClient
      .post<{ models?: ListedModel[]; error?: string }>("/model/builtin/models", {
        accessToken,
        signal: controller.signal,
        body: { provider, credential_name: initialCredentialName },
      })
      .then((body) => {
        if (body.error) throw new Error(String(body.error));
        setModels(body.models ?? []);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(cause.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [provider, initialCredentialName, accessToken]);
  const importSelected = async () => {
    if (!supplier) {
      setError("未找到提供商，请重新选择。");
      return;
    }
    setSaving(true);
    setError("");
    let imported = 0;
    try {
      // Save sequentially; only remove successful rows from the retry selection.
      for (const model of models.filter((item) => picked.includes(item.id))) {
        await upsertPriceModel(accessToken, providerCatalogEntry(model, supplier));
        imported++;
        setPicked((current) => current.filter((id) => id !== model.id));
      }
      toast.success("已导入 " + imported + " 个模型，请继续选择模型和价格。");
      onAdded();
    } catch (cause) {
      setError("已导入 " + imported + " 个模型。" + (cause instanceof Error ? cause.message : "导入失败，请重试。"));
    } finally {
      setSaving(false);
    }
  };
  const visible = models.filter((item) => item.id.toLowerCase().includes(search.toLowerCase()));
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !saving) onClose();
      }}
    >
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>获取模型列表并导入 AI 大模型广场</DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          {initialCredentialName} · 导入后，在统一表单中确认模型和价格，再添加部署。未提供明确 Token
          单位的价格保持未设置。
        </p>
        <Input
          aria-label="搜索上游模型"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="搜索上游模型 ID…"
        />
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {loading ? (
          <p role="status" className="py-8 text-center text-sm text-muted-foreground">
            正在获取模型列表…
          </p>
        ) : (
          <ul className="max-h-96 divide-y overflow-auto rounded-lg border">
            {visible.map((item) => (
              <li key={item.id} className="p-3">
                <label className="flex items-start gap-3 text-sm">
                  <input
                    type="checkbox"
                    className="mt-1"
                    aria-label={item.id}
                    checked={picked.includes(item.id)}
                    disabled={saving}
                    onChange={() =>
                      setPicked((current) =>
                        current.includes(item.id) ? current.filter((id) => id !== item.id) : [...current, item.id],
                      )
                    }
                  />
                  <span className="min-w-0">
                    <span className="break-all font-medium">{item.id}</span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {item.category ?? "其他"} ·{" "}
                      {item.input_price == null && item.output_price == null
                        ? "未提供价格，导入后选择或补充定价"
                        : "单价导入后可在广场确认"}
                    </span>
                  </span>
                </label>
              </li>
            ))}
            {!visible.length && <li className="p-8 text-center text-sm text-muted-foreground">没有找到模型</li>}
          </ul>
        )}
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs text-muted-foreground">已选择 {picked.length} 个模型</span>
          <Button disabled={saving || loading || !picked.length} onClick={importSelected}>
            {saving ? "正在导入…" : "导入所选模型及价格"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
