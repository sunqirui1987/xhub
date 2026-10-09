"use client";

import React, { useEffect, useState } from "react";
import { t } from "@/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import { TemplateGroups, TemplateFallbacks } from "./TemplateRoutingSections";
import ModelRoutingFields from "./ModelRoutingFields";
import {
  bodyFromForm,
  formFromBody,
  parseDocument,
  prettyDocument,
  type SplitDeployment,
  type TemplateFormState,
} from "./templateForm";

/** NumberField 渲染可靠性数值输入；参数为名称、说明、草稿值和回调，返回标签关联控件。
 * 编辑器调用；只更新父级草稿，非法数值由文档校验阻止保存，无后台副作用。 */
const NumberField: React.FC<{
  label: string;
  hint: string;
  value: string;
  onChange: (value: string) => void;
}> = ({ label, hint, value, onChange }) => (
  <label className="space-y-1">
    <span className="text-xs font-medium uppercase tracking-wide text-foreground">{label}</span>
    <p className="text-xs text-muted-foreground">{hint}</p>
    <Input
      type="number"
      inputMode="decimal"
      aria-label={label}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className="font-mono"
    />
  </label>
);

/** TemplateEditor 编辑模板内负载均衡、路由组、故障转移与JSON，供模板库创建和编辑调用。
 * 参数包含名称、草稿、部署目录及更新回调；返回三个业务表单与双向同步JSON。
 * 父级拥有草稿；所有字段通过表单编辑，导入文件仍严格校验，组与回退仅随完整模板提交。 */
const TemplateEditor: React.FC<{
  name: string;
  form: TemplateFormState;
  deployments: SplitDeployment[];
  accessToken: string | null;
  templateId?: string;
  organizationId?: string;
  teamId?: string;
  onName: (name: string) => void;
  onChange: (form: TemplateFormState) => void;
  onJsonValid: (valid: boolean) => void;
}> = ({ name, form, deployments, onName, onChange, onJsonValid }) => {
  const [jsonOverride, setJsonOverride] = useState<string | null>(null);
  const [jsonError, setJsonError] = useState("");
  const [activeTab, setActiveTab] = useState("basic");
  /** patch 合并表单局部修改；参数为变更字段，无返回值，清除 JSON 覆盖并通知父级更新草稿。 */
  const patch = (next: Partial<TemplateFormState>) => {
    setJsonOverride(null);
    setJsonError("");
    onChange({ ...form, ...next });
  };
  const written = bodyFromForm(form);
  useEffect(() => {
    onJsonValid(written.ok && !jsonError);
  }, [written.ok, jsonError, onJsonValid]);
  const generatedJson = written.ok ? prettyDocument(written.body) : "";
  const jsonText = jsonOverride ?? generatedJson;

  /** editJson 校验输入正文；参数为 JSON 文本，无返回值，合法时同步表单，失败时保留文本并阻止保存。 */
  const editJson = (text: string) => {
    setJsonOverride(text);
    const parsed = parseDocument(text);
    if (!parsed.ok) {
      setJsonError(t("pages.routeTemplates.jsonInvalid"));
      onJsonValid(false);
      return;
    }
    setJsonError("");
    onChange(formFromBody(parsed.body));
  };

  /** uploadJson 读取用户选择的文件；参数允许未选择，无返回值，读取失败显示错误且不覆盖当前草稿。 */
  const uploadJson = (file: File | undefined) => {
    if (!file) return;
    void file
      .text()
      .then((text) => editJson(text))
      .catch(() => {
        setJsonError("无法读取 JSON 文件，请重新选择。");
        onJsonValid(false);
      });
  };

  /** downloadJson 导出当前文本；无参数和返回值，使用模板名命名文件并释放临时对象 URL。 */
  const downloadJson = () => {
    const blob = new Blob([jsonText], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${name.trim() || "router-settings"}.json`;
    link.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="space-y-4">
      <label className="block max-w-xl space-y-1">
        <span className="text-xs font-medium uppercase tracking-wide text-foreground">
          {t("pages.routeTemplates.name")}
        </span>
        <Input
          aria-label={t("pages.routeTemplates.name")}
          value={name}
          placeholder={t("pages.routeTemplates.namePlaceholder")}
          required
          autoFocus
          onChange={(event) => onName(event.target.value)}
        />
      </label>

      <div className="min-w-0">
        <Tabs orientation="horizontal" value={activeTab} onValueChange={setActiveTab} className="min-w-0">
          <TabsList variant="line" aria-label="路由配置分区" className="h-auto w-full flex-wrap justify-start border-b">
            <TabsTrigger value="basic" disabled={!!jsonError} className="!h-auto !flex-none px-4 py-2">
              负载均衡
            </TabsTrigger>
            <TabsTrigger value="groups" disabled={!!jsonError}>
              路由组
            </TabsTrigger>
            <TabsTrigger value="fallbacks" disabled={!!jsonError}>
              故障转移
            </TabsTrigger>
            <TabsTrigger value="json" className="!h-auto !flex-none px-4 py-2">
              {t("pages.routeTemplates.jsonTab")}
            </TabsTrigger>
          </TabsList>

          <div className="min-w-0">
            <TabsContent value="basic" className="pt-6 space-y-6">
              <div>
                <h3 className="text-lg font-semibold">负载均衡</h3>
                <p className="mt-2 text-sm text-muted-foreground">
                  选择需要调整策略的模型；其余模型读取模型管理中的端点权重。
                </p>
              </div>
              <ModelRoutingFields
                rules={form.model_routes}
                deployments={deployments}
                onChange={(model_routes) => patch({ model_routes })}
              />
              <p className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
                部署地址、凭据、能力和权重在“模型与端点”中维护；模板负责选择策略。
              </p>

              <div>
                <h3 className="font-semibold">重试、超时与被动冷却</h3>
                <p className="mt-2 text-sm text-muted-foreground">
                  先在当前公开模型的部署池中尝试；429、5xx
                  和传输失败可以重试或切换部署。流式内容已经发出后不会重新开始响应。
                </p>
              </div>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <NumberField
                  label={t("pages.routeTemplates.retries")}
                  hint={t("pages.routeTemplates.retriesHint")}
                  value={form.max_attempts}
                  onChange={(max_attempts) => patch({ max_attempts })}
                />
                <NumberField
                  label={t("pages.routeTemplates.timeout")}
                  hint={t("pages.routeTemplates.timeoutHint")}
                  value={form.timeout_seconds}
                  onChange={(timeout_seconds) => patch({ timeout_seconds })}
                />
                <NumberField
                  label={t("pages.routeTemplates.allowedFails")}
                  hint={t("pages.routeTemplates.allowedFailsHint")}
                  value={form.failure_threshold}
                  onChange={(failure_threshold) => patch({ failure_threshold })}
                />
                <NumberField
                  label={t("pages.routeTemplates.cooldown")}
                  hint={t("pages.routeTemplates.cooldownHint")}
                  value={form.cooldown_seconds}
                  onChange={(cooldown_seconds) => patch({ cooldown_seconds })}
                />
              </div>
            </TabsContent>
            <TabsContent value="groups" className="pt-6">
              <TemplateGroups form={form} deployments={deployments} onChange={patch} />
            </TabsContent>
            <TabsContent value="fallbacks" className="pt-6">
              <TemplateFallbacks form={form} deployments={deployments} onChange={patch} />
            </TabsContent>
            <TabsContent value="json" className="space-y-3 pt-4">
              <p className="text-xs text-muted-foreground">
                JSON 与三个表单编辑同一份模板。导入、下载或修改 JSON 后，保存模板统一生效。
              </p>
              <textarea
                aria-label={t("pages.routeTemplates.jsonTab")}
                className="h-96 w-full rounded-md border border-border bg-transparent p-3 font-mono text-xs"
                value={jsonText}
                spellCheck={false}
                onChange={(event) => editJson(event.target.value)}
              />
              {jsonError && (
                <p role="alert" className="text-xs text-destructive">
                  {jsonError} · {t("pages.routeTemplates.jsonFixHint")}
                </p>
              )}
              <div className="flex flex-wrap gap-2">
                <Button type="button" size="sm" variant="outline" onClick={downloadJson}>
                  {t("pages.routeTemplates.downloadJson")}
                </Button>
                <label className="inline-flex cursor-pointer items-center">
                  <input
                    type="file"
                    accept="application/json,.json"
                    className="sr-only"
                    onChange={(event) => {
                      uploadJson(event.target.files?.[0]);
                      event.target.value = "";
                    }}
                  />
                  <span className="inline-flex h-8 items-center rounded-md border border-border px-3 text-sm">
                    {t("pages.routeTemplates.importJson")}
                  </span>
                </label>
              </div>
            </TabsContent>
          </div>
        </Tabs>
      </div>
      {!written.ok && !jsonError && (
        <p role="alert" className="text-xs text-destructive">
          {t("pages.routeTemplates.invalidField", { field: written.field })}
        </p>
      )}
    </div>
  );
};

export default TemplateEditor;
