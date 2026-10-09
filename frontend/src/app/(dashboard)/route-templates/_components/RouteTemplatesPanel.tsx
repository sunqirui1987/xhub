"use client";

import React, { useEffect, useMemo, useState } from "react";
import { ArrowLeft, Copy, FileJson, Loader2, Pencil, Plus, Shield } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "@/lib/toast";
import { t } from "@/i18n";
import { formatTemplateStrategyLabel } from "@/components/route_templates/strategyLabel";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import RouteTemplateGuide from "@/components/route_templates/RouteTemplateGuide";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import RouteTemplateLibrary from "./RouteTemplateLibrary";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import {
  getRouterSettingsCall,
  modelInfoCall,
  setCallbacksCall,
  type RouteTemplateUsage,
} from "@/components/networking";
import {
  deleteRefusalText,
  loadRouteTemplates,
  loadTemplateUsage,
  removeRouteTemplate,
  saveRouteTemplate,
  summarizeTemplate,
  type RouteTemplateRow,
} from "@/components/route_templates/routeTemplatePayload";
import TemplateEditor from "@/components/route_templates/TemplateEditor";
import {
  bodyFromForm,
  deploymentsFromInfo,
  documentFromSettingsResponse,
  formFromBody,
  nextCopyName,
  omitUntouchedRoutingGroups,
  prettyDocument,
  type SplitDeployment,
  type TemplateFormState,
} from "@/components/route_templates/templateForm";

const PLATFORM_ID = "platform";

type Draft = {
  id: string;
  name: string;
  form: TemplateFormState;
  lockedName: boolean;
  routingGroupsAtOpen: string;
  hadModelRouting: boolean;
};
type UsageView = { name: string; rows: RouteTemplateUsage[]; refused: boolean };
type JsonView = { name: string; body: Record<string, unknown> };

const draftFrom = (id: string, name: string, body: Record<string, unknown>, lockedName: boolean): Draft => {
  const form = formFromBody(body);
  return {
    id,
    name,
    form,
    lockedName,
    routingGroupsAtOpen: lockedName ? form.routing_groups : "",
    hadModelRouting: "model_routing" in body,
  };
};

const editorTitle = (draft: Draft | null) => {
  if (draft?.lockedName) return t("pages.routeTemplates.platformDefault");
  if (draft?.id) return t("pages.routeTemplates.edit");
  return t("pages.routeTemplates.create");
};

const editorDescription = (draft: Draft | null) => {
  if (draft?.lockedName) return t("pages.routeTemplates.platformEditHint");
  if (draft?.id) return t("pages.routeTemplates.editHint");
  return t("pages.routeTemplates.createHint");
};

const documentText = (row: JsonView | null) => (row ? prettyDocument(row.body) : "");

const scopeLabel = (scope: RouteTemplateUsage["scope_type"]) => {
  if (scope === "organization") return t("pages.routeTemplates.scopeOrganization");
  if (scope === "team") return t("pages.routeTemplates.scopeTeam");
  return t("pages.routeTemplates.scopeKey");
};

/**
 * The route template library.
 *
 * A template is one whole router settings document. The platform default card
 * holds the document every scope uses when it has not selected a template.
 * Named rows are what an organization, a team or a key can select.
 */
const RouteTemplatesPanel: React.FC<{ accessToken: string | null }> = ({ accessToken }) => {
  const { userId, userRole } = useAuthorized();
  const [deployments, setDeployments] = useState<SplitDeployment[]>([]);
  const [jsonValid, setJsonValid] = useState(true);
  const [jsonView, setJsonView] = useState<JsonView | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saving, setSaving] = useState(false);
  const [search, setSearch] = useState("");
  const [usage, setUsage] = useState<UsageView | null>(null);
  const [pendingDelete, setPendingDelete] = useState<RouteTemplateRow | null>(null);

  const library = useQuery({
    queryKey: ["route-template-library", accessToken],
    enabled: !!accessToken,
    queryFn: async () => {
      const [rows, settings] = await Promise.all([
        loadRouteTemplates(accessToken!),
        getRouterSettingsCall(accessToken!),
      ]);
      return { rows, platform: documentFromSettingsResponse(settings) };
    },
  });
  const rows = library.data?.rows ?? [];
  const platform = library.data?.platform ?? {};
  const loading = library.isPending;
  const refresh = () => library.refetch();
  const showLibrary = !draft && !library.isError && !loading;

  useEffect(() => {
    if (!accessToken || !userId || !userRole) return;
    let cancelled = false;
    void modelInfoCall(accessToken, userId, userRole, 1, 200)
      .then((data) => {
        if (!cancelled) setDeployments(deploymentsFromInfo(data?.data ?? []));
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [accessToken, userId, userRole]);

  const summaryLabels = useMemo(
    () => ({
      strategy: (value: string) => t(formatTemplateStrategyLabel(value)),
      retries: (value: number) => t("pages.routeTemplates.attemptsSummary", { count: value }),
      timeout: (seconds: number) => t("pages.routeTemplates.timeoutSummary", { seconds }),
      fallbacks: (value: number) => t("pages.routeTemplates.fallbacksSummary", { count: value }),
      models: (value: number) => t("pages.routeTemplates.modelsSummary", { count: value }),
      none: t("no fallbacks"),
    }),
    [],
  );

  const startCreate = () => {
    setJsonValid(true);
    setDraft(draftFrom("", "", platform, false));
  };

  const save = async () => {
    if (!accessToken || !draft) return;
    if (!jsonValid || saving || !draft.name.trim()) return;
    const written = bodyFromForm(draft.form);
    if (!written.ok) {
      toast.fromError(t("pages.routeTemplates.invalidField", { field: written.field }));
      return;
    }
    setSaving(true);
    try {
      if (draft.lockedName) {
        const patch = omitUntouchedRoutingGroups(written.body, draft.form.routing_groups, draft.routingGroupsAtOpen);
        // The platform endpoint merges a patch: omission would keep deleted rules.
        if (draft.hadModelRouting && !("model_routing" in patch)) patch.model_routing = [];
        await setCallbacksCall(accessToken, { router_settings: patch });
        toast.success(t("pages.routeTemplates.platformSaved"));
      } else {
        await saveRouteTemplate(accessToken, {
          id: draft.id || undefined,
          name: draft.name.trim(),
          body: written.body,
        });
        toast.success(t("pages.routeTemplates.saved"));
      }
      setDraft(null);
      await refresh();
    } catch {
      toast.fromError(t("Failed to save the template"));
    } finally {
      setSaving(false);
    }
  };

  const copyDocument = (name: string, body: Record<string, unknown>) => {
    const nextName = nextCopyName(
      name,
      rows.map((item) => item.name),
      (source) => t("pages.routeTemplates.copyOf", { name: source }),
    );
    setJsonValid(true);
    setDraft(draftFrom("", nextName, body, false));
  };

  const showUsage = async (row: RouteTemplateRow, refused: boolean) => {
    if (!accessToken) return;
    try {
      const listed = await loadTemplateUsage(accessToken, row.id);
      setUsage({ name: row.name, rows: listed, refused });
      setPendingDelete(null);
    } catch (error) {
      toast.fromError(error);
    }
  };

  const remove = async (row: RouteTemplateRow) => {
    if (!accessToken) return;
    if (row.usedBy > 0) {
      await showUsage(row, true);
      return;
    }
    try {
      await removeRouteTemplate(accessToken, row.id);
      toast.success(t("pages.routeTemplates.deleted"));
      setPendingDelete(null);
      setUsage(null);
      await refresh();
    } catch (error) {
      const refusal = deleteRefusalText(error, {
        inUse: (count) => t("pages.routeTemplates.inUseBy", { count }),
        failed: t("pages.routeTemplates.deleteFailed"),
      });
      toast.fromError(refusal.message);
      setPendingDelete(null);
      if (refusal.usage.length > 0) {
        setUsage({ name: row.name, rows: refusal.usage, refused: true });
      }
    }
  };

  const openNamed = (row: RouteTemplateRow) => {
    setJsonValid(true);
    setDraft(draftFrom(row.id, row.name, row.body, false));
  };

  const openPlatform = () => {
    setJsonValid(true);
    setDraft(draftFrom(PLATFORM_ID, t("pages.routeTemplates.platformDefault"), platform, true));
  };

  if (!accessToken) return null;

  return (
    <div className="w-full space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="space-y-2">
          {draft && (
            <Button variant="ghost" size="sm" className="-ml-3" disabled={saving} onClick={() => setDraft(null)}>
              <ArrowLeft />
              {t("pages.routeTemplates.backToLibrary")}
            </Button>
          )}
          <h2 className="text-2xl font-semibold tracking-tight text-foreground">
            {draft ? editorTitle(draft) : t("pages.routeTemplates.title")}
          </h2>
          <p className="text-sm text-muted-foreground">
            {draft ? editorDescription(draft) : t("pages.routeTemplates.pageIntro")}
          </p>
        </div>
        {!draft && (
          <Button onClick={startCreate} disabled={loading || library.isError}>
            <Plus />
            {t("pages.routeTemplates.create")}
          </Button>
        )}
      </div>

      <RouteTemplateGuide compact={!!draft} />

      {library.isError && (
        <Card>
          <CardContent className="flex flex-wrap items-center justify-between gap-3 text-sm">
            <p className="text-destructive">{t("Failed to load the templates")}</p>
            <Button variant="outline" size="sm" onClick={() => void refresh()}>
              {t("pages.routeTemplates.reload")}
            </Button>
          </CardContent>
        </Card>
      )}
      {!library.isError && loading && (
        <div
          className="flex items-center justify-center gap-2 rounded-xl border border-dashed py-12 text-sm text-muted-foreground"
          role="status"
        >
          <Loader2 className="size-4 animate-spin" />
          {t("pages.routeTemplates.loadingTemplates")}
        </div>
      )}
      {showLibrary && (
        <>
          <Card>
            <CardContent className="flex flex-wrap items-center justify-between gap-4">
              <div className="flex items-start gap-3">
                <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <Shield className="size-5" />
                </div>
                <div className="space-y-1.5">
                  <div className="flex items-center gap-2">
                    <h3 className="font-semibold">{t("pages.routeTemplates.platformDefault")}</h3>
                    <Badge variant="secondary">{t("pages.routeTemplates.defaultBadge")}</Badge>
                  </div>
                  <p className="text-sm text-muted-foreground">{summarizeTemplate(platform, summaryLabels)}</p>
                  <p className="text-xs text-muted-foreground">{t("pages.routeTemplates.platformDefaultHint")}</p>
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setJsonView({ name: t("pages.routeTemplates.platformDefault"), body: platform })}
                >
                  <FileJson />
                  {t("pages.routeTemplates.jsonTab")}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => copyDocument(t("pages.routeTemplates.platformDefault"), platform)}
                >
                  <Copy />
                  {t("pages.routeTemplates.copy")}
                </Button>
                <Button size="sm" variant="outline" onClick={openPlatform}>
                  <Pencil />
                  {t("pages.routeTemplates.edit")}
                </Button>
              </div>
            </CardContent>
          </Card>

          <RouteTemplateLibrary
            rows={rows}
            search={search}
            onSearch={setSearch}
            summaryLabels={summaryLabels}
            onCreate={startCreate}
            onViewJson={setJsonView}
            onCopy={(row) => copyDocument(row.name, row.body)}
            onDelete={setPendingDelete}
            onEdit={openNamed}
            onUsage={(row) => void showUsage(row, false)}
          />
        </>
      )}

      {draft && (
        <Card role="region" aria-label={editorTitle(draft)} className="gap-0 py-0">
          <div className="sticky top-0 z-sticky flex flex-wrap items-center justify-between gap-3 rounded-t-xl border-b bg-card px-5 py-3">
            <p className="text-sm text-muted-foreground">{t("pages.routeTemplates.draftHint")}</p>
            <div className="flex items-center gap-2">
              <Button variant="ghost" disabled={saving} onClick={() => setDraft(null)}>
                {t("Cancel")}
              </Button>
              <Button onClick={() => void save()} disabled={saving || !jsonValid || !draft.name.trim()}>
                {saving && <Loader2 className="animate-spin" />}
                {t(draft.id ? "Save" : "pages.routeTemplates.create")}
              </Button>
            </div>
          </div>
          <CardContent className="py-6">
            <fieldset disabled={saving} className="min-w-0">
              <TemplateEditor
                key={draft.id || "new"}
                name={draft.name}
                nameLocked={draft.lockedName}
                form={draft.form}
                deployments={deployments}
                accessToken={accessToken}
                onName={(name) => setDraft({ ...draft, name })}
                onChange={(form) => setDraft({ ...draft, form })}
                onJsonValid={setJsonValid}
              />
            </fieldset>
          </CardContent>
        </Card>
      )}

      <Dialog open={jsonView !== null} onOpenChange={(open) => !open && setJsonView(null)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{jsonView?.name}</DialogTitle>
            <DialogDescription>{t("pages.routeTemplates.jsonHint")}</DialogDescription>
          </DialogHeader>
          <pre className="max-h-[60vh] overflow-auto rounded bg-muted p-3 font-mono text-xs">
            {documentText(jsonView)}
          </pre>
        </DialogContent>
      </Dialog>

      <Dialog open={usage !== null} onOpenChange={(open) => !open && setUsage(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("pages.routeTemplates.usageTitle", { name: usage?.name ?? "" })}</DialogTitle>
            {usage?.refused && (
              <DialogDescription>{t("pages.routeTemplates.inUseBy", { count: usage.rows.length })}</DialogDescription>
            )}
          </DialogHeader>
          {usage && usage.rows.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("pages.routeTemplates.unused")}</p>
          ) : (
            <ul className="space-y-1 text-sm">
              {usage?.rows.map((entry) => (
                <li key={`${entry.scope_type}-${entry.scope_id}`}>
                  {scopeLabel(entry.scope_type)}
                  {" · "}
                  {entry.name}
                </li>
              ))}
            </ul>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={pendingDelete !== null} onOpenChange={(open) => !open && setPendingDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("pages.routeTemplates.confirmDelete", { name: pendingDelete?.name ?? "" })}</DialogTitle>
            <DialogDescription>{t("pages.routeTemplates.confirmDeleteBody")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setPendingDelete(null)}>
              {t("Cancel")}
            </Button>
            <Button variant="destructive" onClick={() => pendingDelete && void remove(pendingDelete)}>
              {t("pages.routeTemplates.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default RouteTemplatesPanel;
