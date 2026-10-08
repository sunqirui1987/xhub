"use client";

import React, { useMemo, useState } from "react";
import { GitBranch, Pencil, Plus, Trash2 } from "lucide-react";
import { t } from "@/i18n";
import { formatStrategyLabel } from "@/components/routing_groups/strategy";
import RoutingGroupModal from "@/components/routing_groups/RoutingGroupModal";
import type { RoutingGroup } from "@/components/routing_groups/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { parseRoutingGroups, prettyRoutingGroups } from "./templateForm";

const strategyDescriptions = (strategies: string[]): Record<string, string> =>
  Object.fromEntries(strategies.map((strategy) => [strategy, `pages.routeTemplates.strategyDescriptions.${strategy}`]));

const GroupsBody: React.FC<{
  valid: boolean;
  groups: RoutingGroup[];
  onCreate: () => void;
  onEdit: (group: RoutingGroup) => void;
  onDelete: (group: RoutingGroup) => void;
}> = ({ valid, groups, onCreate, onEdit, onDelete }) => {
  if (!valid) {
    return (
      <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-4">
        <p className="text-sm font-medium text-destructive">{t("pages.routeTemplates.routingGroupsInvalid")}</p>
        <p className="mt-1 text-xs text-muted-foreground">{t("pages.routeTemplates.routingGroupsInvalidHint")}</p>
      </div>
    );
  }
  if (groups.length === 0) {
    return (
      <button
        type="button"
        className="flex w-full flex-col items-center rounded-lg border border-dashed border-border px-6 py-10 text-center hover:bg-muted/40"
        onClick={onCreate}
      >
        <span className="flex size-10 items-center justify-center rounded-full bg-muted">
          <GitBranch className="size-5 text-muted-foreground" />
        </span>
        <span className="mt-3 text-sm font-medium text-foreground">{t("No routing groups yet")}</span>
        <span className="mt-1 text-xs text-muted-foreground">{t("pages.routeTemplates.routingGroupsEmptyHint")}</span>
      </button>
    );
  }
  return (
    <div className="divide-y rounded-lg border border-border">
      {groups.map((group) => (
        <div key={group.group_name} className="flex items-center gap-4 px-4 py-3">
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium text-foreground">{group.group_name}</span>
              <Badge variant="secondary">{t(formatStrategyLabel(group.routing_strategy))}</Badge>
            </div>
            <div className="mt-2 flex flex-wrap gap-1.5">
              {group.models.map((model) => (
                <Badge key={model} variant="outline" className="font-normal">
                  {model}
                </Badge>
              ))}
            </div>
          </div>
          <div className="flex shrink-0 items-center">
            <Button type="button" size="icon-sm" variant="ghost" onClick={() => onEdit(group)}>
              <Pencil className="size-4" />
              <span className="sr-only">{t("Edit")}</span>
            </Button>
            <Button type="button" size="icon-sm" variant="ghost" onClick={() => onDelete(group)}>
              <Trash2 className="size-4 text-destructive" />
              <span className="sr-only">{t("Delete")}</span>
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
};

/** Visual editor for the routing groups stored inside one route template. */
const TemplateRoutingGroups: React.FC<{
  value: string;
  modelOptions: string[];
  availableStrategies: string[];
  onChange: (value: string) => void;
}> = ({ value, modelOptions, availableStrategies, onChange }) => {
  const parsed = useMemo(() => parseRoutingGroups(value), [value]);
  const groups = parsed.ok ? parsed.groups : [];
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<RoutingGroup | null>(null);
  const [deleting, setDeleting] = useState<RoutingGroup | null>(null);

  const openCreate = () => {
    setEditing(null);
    setModalOpen(true);
  };

  const openEdit = (group: RoutingGroup) => {
    setEditing(group);
    setModalOpen(true);
  };

  const submit = (group: RoutingGroup) => {
    const next = editing
      ? groups.map((item) => (item.group_name === editing.group_name ? group : item))
      : [...groups, group];
    onChange(prettyRoutingGroups(next));
    setModalOpen(false);
    setEditing(null);
  };

  const remove = () => {
    if (!deleting) return;
    onChange(prettyRoutingGroups(groups.filter((group) => group.group_name !== deleting.group_name)));
    setDeleting(null);
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="max-w-xl">
          <h3 className="text-sm font-medium text-foreground">{t("pages.routeTemplates.routingGroups")}</h3>
          <p className="mt-1 text-xs text-muted-foreground">{t("pages.routeTemplates.routingGroupsHint")}</p>
        </div>
        <Button type="button" size="sm" onClick={openCreate} disabled={!parsed.ok}>
          <Plus className="size-4" />
          {t("Create Group")}
        </Button>
      </div>

      <GroupsBody
        valid={parsed.ok}
        groups={groups}
        onCreate={openCreate}
        onEdit={openEdit}
        onDelete={setDeleting}
      />

      <RoutingGroupModal
        open={modalOpen}
        mode={editing ? "edit" : "create"}
        initialValue={editing}
        availableStrategies={availableStrategies}
        strategyDescriptions={strategyDescriptions(availableStrategies)}
        modelOptions={modelOptions}
        existingGroupNames={groups.map((group) => group.group_name)}
        onClose={() => setModalOpen(false)}
        onSubmit={submit}
      />

      <Dialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("Delete routing group?")}</DialogTitle>
            <DialogDescription>{t("pages.routeTemplates.deleteRoutingGroupHint")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleting(null)}>
              {t("Cancel")}
            </Button>
            <Button type="button" variant="destructive" onClick={remove}>
              {t("Delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default TemplateRoutingGroups;
