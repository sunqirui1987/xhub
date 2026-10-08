"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Copy, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "@/lib/toast";
import { t } from "@/i18n";
import { formatStrategyLabel } from "@/components/routing_groups/strategy";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
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
  formatUpdatedAt,
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
};
type RenameTarget = { id: string; name: string; body: Record<string, unknown> };
type UsageView = { name: string; rows: RouteTemplateUsage[]; refused: boolean };
type JsonView = { name: string; body: Record<string, unknown> };

const draftFrom = (id: string, name: string, body: Record<string, unknown>, lockedName: boolean): Draft => {
  const form = formFromBody(body);
  return { id, name, form, lockedName, routingGroupsAtOpen: lockedName ? form.routing_groups : "" };
};

const sheetTitle = (draft: Draft | null) => {
  if (draft?.lockedName) return t("pages.routeTemplates.platformDefault");
  if (draft?.id) return t("pages.routeTemplates.edit");
  return t("pages.routeTemplates.create");
};

const usedByLabel = (count: number) =>
  count > 0 ? t("pages.routeTemplates.usedByCount", { count }) : t("pages.routeTemplates.unused");

const libraryIsEmpty = (count: number, loading: boolean) => count === 0 && !loading;

const documentText = (row: JsonView | null) => (row ? prettyDocument(row.body) : "");

const scopeLabel = (scope: RouteTemplateUsage["scope_type"]) => {
  if (scope === "organization") return t("pages.routeTemplates.scopeOrganization");
  if (scope === "team") return t("pages.routeTemplates.scopeTeam");
  return t("pages.routeTemplates.scopeKey");
};

/**
 * The route template library.
 *
 * A template is one whole router settings document. The first row is the platform
 * default: the document every scope uses when it has not selected a template.
 * Named rows are what an organization, a team or a key can select.
 */
const RouteTemplatesPanel: React.FC<{ accessToken: string | null }> = ({ accessToken }) => {
  const { userId, userRole } = useAuthorized();
  const [rows, setRows] = useState<RouteTemplateRow[]>([]);
  const [platform, setPlatform] = useState<Record<string, unknown>>({});
  const [deployments, setDeployments] = useState<SplitDeployment[]>([]);
  const [jsonValid, setJsonValid] = useState(true);
  const [jsonView, setJsonView] = useState<JsonView | null>(null);
  const [loading, setLoading] = useState(true);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saving, setSaving] = useState(false);
  const [rename, setRename] = useState<RenameTarget | null>(null);
  const [usage, setUsage] = useState<UsageView | null>(null);
  const [pendingDelete, setPendingDelete] = useState<RouteTemplateRow | null>(null);

  const refresh = useCallback(async () => {
    if (!accessToken) return;
    setLoading(true);
    try {
      const [next, settings] = await Promise.all([
        loadRouteTemplates(accessToken),
        getRouterSettingsCall(accessToken).catch(() => null),
      ]);
      setRows(next);
      setPlatform(documentFromSettingsResponse(settings));
    } catch {
      toast.fromError(t("Failed to load the templates"));
    } finally {
      setLoading(false);
    }
  }, [accessToken]);

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

  useEffect(() => {
    if (!accessToken) return;
    let cancelled = false;
    void Promise.all([loadRouteTemplates(accessToken), getRouterSettingsCall(accessToken).catch(() => null)])
      .then(([next, settings]) => {
        if (cancelled) return;
        setRows(next);
        setPlatform(documentFromSettingsResponse(settings));
        setLoading(false);
      })
      .catch(() => {
        if (cancelled) return;
        setLoading(false);
        toast.fromError(t("Failed to load the templates"));
      });
    return () => {
      cancelled = true;
    };
  }, [accessToken]);

  const summaryLabels = useMemo(
    () => ({
      strategy: (value: string) => t(formatStrategyLabel(value)),
      retries: (value: number) => t("{count} retries", { count: value }),
      timeout: (seconds: number) => t("pages.routeTemplates.timeoutSummary", { seconds }),
      fallbacks: (value: number) => t("{count} fallbacks", { count: value }),
      none: t("no fallbacks"),
    }),
    [],
  );

  const startCreate = () => {
    setJsonValid(true);
    setDraft(draftFrom("", "", platform, false));
  };

  const save = async () => {
    if (!accessToken || !draft || !jsonValid) return;
    const written = bodyFromForm(draft.form);
    if (!written.ok) {
      toast.fromError(t("pages.routeTemplates.invalidNumber"));
      return;
    }
    setSaving(true);
    try {
      if (draft.lockedName) {
        const patch = omitUntouchedRoutingGroups(written.body, draft.form.routing_groups, draft.routingGroupsAtOpen);
        await setCallbacksCall(accessToken, { router_settings: patch });
        toast.success(t("pages.routeTemplates.platformSaved"));
      } else {
        await saveRouteTemplate(accessToken, { id: draft.id || undefined, name: draft.name, body: written.body });
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

  const copyDocument = async (name: string, body: Record<string, unknown>) => {
    if (!accessToken) return;
    const nextName = nextCopyName(
      name,
      rows.map((item) => item.name),
      (source) => t("pages.routeTemplates.copyOf", { name: source }),
    );
    try {
      await saveRouteTemplate(accessToken, { name: nextName, body });
      toast.success(t("pages.routeTemplates.copied"));
      await refresh();
    } catch {
      toast.fromError(t("Failed to save the template"));
    }
  };

  const commitRename = async () => {
    if (!accessToken || !rename) return;
    try {
      await saveRouteTemplate(accessToken, { id: rename.id, name: rename.name, body: rename.body });
      toast.success(t("pages.routeTemplates.renamed"));
      setRename(null);
      await refresh();
    } catch {
      toast.fromError(t("Failed to save the template"));
    }
  };

  const showUsage = async (row: RouteTemplateRow, refused: boolean) => {
    if (!accessToken) return;
    const listed = await loadTemplateUsage(accessToken, row.id).catch(() => [] as RouteTemplateUsage[]);
    setUsage({ name: row.name, rows: listed, refused });
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
      <div className="max-w-3xl">
        <h2 className="text-2xl font-semibold text-foreground">{t("pages.routeTemplates.title")}</h2>
        <p className="mt-1 text-sm text-muted-foreground">{t("pages.routeTemplates.subtitle")}</p>
        <p className="mt-2 text-sm text-muted-foreground">{t("pages.routeTemplates.platformDefaultHint")}</p>
      </div>

      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-medium">{t("pages.routeTemplates.templates")}</h3>
            <Button size="sm" onClick={startCreate}>
              <Plus className="mr-1 size-4" />
              {t("pages.routeTemplates.create")}
            </Button>
          </div>

          <div className="overflow-x-auto">
            <Table className="min-w-[760px]">
              <TableHeader>
                <TableRow>
                  <TableHead>{t("pages.routeTemplates.name")}</TableHead>
                  <TableHead>{t("pages.routeTemplates.summary")}</TableHead>
                  <TableHead>{t("pages.routeTemplates.usedBy")}</TableHead>
                  <TableHead>{t("pages.routeTemplates.updatedAt")}</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow>
                  <TableCell className="font-medium">{t("pages.routeTemplates.platformDefault")}</TableCell>
                  <TableCell className="max-w-md text-muted-foreground">
                    <p>{summarizeTemplate(platform, summaryLabels)}</p>
                    <pre className="mt-1 max-h-24 overflow-auto rounded bg-muted p-2 font-mono text-[11px] text-foreground">
                      {prettyDocument(platform)}
                    </pre>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="mt-1 h-auto px-2 py-1"
                      onClick={() => setJsonView({ name: t("pages.routeTemplates.platformDefault"), body: platform })}
                    >
                      {t("pages.routeTemplates.viewJson")}
                    </Button>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{t("pages.routeTemplates.platformUsedBy")}</TableCell>
                  <TableCell className="text-muted-foreground">—</TableCell>
                  <TableCell className="text-right whitespace-nowrap">
                    <Button size="sm" variant="ghost" onClick={openPlatform}>
                      <Pencil className="mr-1 size-3.5" />
                      {t("pages.routeTemplates.edit")}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => void copyDocument(t("pages.routeTemplates.platformDefault"), platform)}
                    >
                      <Copy className="mr-1 size-3.5" />
                      {t("pages.routeTemplates.copy")}
                    </Button>
                  </TableCell>
                </TableRow>
                {rows.map((row) => (
                  <TableRow key={row.id}>
                    <TableCell className="font-medium">{row.name}</TableCell>
                    <TableCell className="max-w-md text-muted-foreground">
                      <p>{summarizeTemplate(row.body, summaryLabels)}</p>
                      <pre className="mt-1 max-h-24 overflow-auto rounded bg-muted p-2 font-mono text-[11px] text-foreground">
                        {prettyDocument(row.body)}
                      </pre>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="mt-1 h-auto px-2 py-1"
                        onClick={() => setJsonView(row)}
                      >
                        {t("pages.routeTemplates.viewJson")}
                      </Button>
                    </TableCell>
                    <TableCell>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-auto px-2 py-1 text-muted-foreground"
                        onClick={() => void showUsage(row, false)}
                      >
                        {usedByLabel(row.usedBy)}
                      </Button>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{formatUpdatedAt(row.updated_at) || "—"}</TableCell>
                    <TableCell className="text-right whitespace-nowrap">
                      <Button size="sm" variant="ghost" disabled={!row.writable} onClick={() => openNamed(row)}>
                        <Pencil className="mr-1 size-3.5" />
                        {t("pages.routeTemplates.edit")}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={!row.writable}
                        onClick={() => setRename({ id: row.id, name: row.name, body: row.body })}
                      >
                        {t("pages.routeTemplates.rename")}
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => void copyDocument(row.name, row.body)}>
                        <Copy className="mr-1 size-3.5" />
                        {t("pages.routeTemplates.copy")}
                      </Button>
                      <Button size="sm" variant="ghost" disabled={!row.writable} onClick={() => setPendingDelete(row)}>
                        <Trash2 className="size-4" />
                        <span className="sr-only">{t("pages.routeTemplates.delete")}</span>
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
                {libraryIsEmpty(rows.length, loading) && (
                  <TableRow>
                    <TableCell colSpan={5} className="text-muted-foreground">
                      {t("pages.routeTemplates.empty")}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>

      <Sheet open={draft !== null} onOpenChange={(open) => !open && setDraft(null)}>
        <SheetContent side="right" className="w-full data-[side=right]:sm:max-w-3xl">
          <SheetHeader>
            <SheetTitle>{sheetTitle(draft)}</SheetTitle>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
            {draft && (
              <TemplateEditor
                name={draft.name}
                nameLocked={draft.lockedName}
                form={draft.form}
                deployments={deployments}
                accessToken={accessToken}
                onName={(name) => setDraft({ ...draft, name })}
                onChange={(form) => setDraft({ ...draft, form })}
                onJsonValid={setJsonValid}
              />
            )}
          </div>
          <SheetFooter className="flex-row justify-end border-t border-border">
            <Button variant="ghost" onClick={() => setDraft(null)}>
              {t("Cancel")}
            </Button>
            <Button onClick={() => void save()} disabled={saving || !jsonValid}>
              {t("Save")}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

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

      <Dialog open={rename !== null} onOpenChange={(open) => !open && setRename(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("pages.routeTemplates.rename")}</DialogTitle>
          </DialogHeader>
          <Input
            aria-label={t("pages.routeTemplates.name")}
            value={rename?.name ?? ""}
            onChange={(event) => rename && setRename({ ...rename, name: event.target.value })}
          />
          <DialogFooter>
            <Button variant="ghost" onClick={() => setRename(null)}>
              {t("Cancel")}
            </Button>
            <Button onClick={() => void commitRename()}>{t("Save")}</Button>
          </DialogFooter>
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
