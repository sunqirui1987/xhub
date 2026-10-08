"use client";

import { Copy, FileJson, MoreHorizontal, Plus, Search, Trash2 } from "lucide-react";
import { t } from "@/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { summarizeTemplate, type RouteTemplateRow } from "@/components/route_templates/routeTemplatePayload";
import { formatUpdatedAt } from "@/components/route_templates/templateForm";

const usedByLabel = (count: number) =>
  count > 0 ? t("pages.routeTemplates.usedByCount", { count }) : t("pages.routeTemplates.unused");

type Props = {
  rows: RouteTemplateRow[];
  search: string;
  onSearch: (value: string) => void;
  summaryLabels: Parameters<typeof summarizeTemplate>[1];
  onCreate: () => void;
  onViewJson: (row: RouteTemplateRow) => void;
  onCopy: (row: RouteTemplateRow) => void;
  onDelete: (row: RouteTemplateRow) => void;
  onEdit: (row: RouteTemplateRow) => void;
  onUsage: (row: RouteTemplateRow) => void;
};

export default function RouteTemplateLibrary({
  rows,
  search,
  onSearch,
  summaryLabels,
  onCreate,
  onViewJson,
  onCopy,
  onDelete,
  onEdit,
  onUsage,
}: Props) {
  const filteredRows = rows.filter((row) => row.name.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()));
  return (
    <Card>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <h3 className="font-semibold">{t("pages.routeTemplates.templates")}</h3>
            <Badge variant="secondary">{rows.length}</Badge>
          </div>
          <div className="relative w-full sm:w-64">
            <Search className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground" />
            <Input
              className="pl-9"
              value={search}
              onChange={(event) => onSearch(event.target.value)}
              aria-label={t("pages.routeTemplates.search")}
              placeholder={t("pages.routeTemplates.search")}
            />
          </div>
        </div>
        {rows.length === 0 ? (
          <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed px-4 py-10 text-center">
            <Copy className="size-7 text-muted-foreground" />
            <p className="font-medium">{t("pages.routeTemplates.emptyTitle")}</p>
            <p className="max-w-md text-sm text-muted-foreground">{t("pages.routeTemplates.empty")}</p>
            <Button size="sm" variant="outline" onClick={onCreate}>
              <Plus />
              {t("pages.routeTemplates.create")}
            </Button>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <Table className="min-w-[640px]">
              <TableHeader>
                <TableRow>
                  <TableHead>{t("pages.routeTemplates.name")}</TableHead>
                  <TableHead>{t("pages.routeTemplates.summary")}</TableHead>
                  <TableHead>{t("pages.routeTemplates.usedBy")}</TableHead>
                  <TableHead>{t("pages.routeTemplates.updatedAt")}</TableHead>
                  <TableHead className="text-right">
                    <span className="sr-only">{t("pages.routeTemplates.actions")}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredRows.map((row) => (
                  <TableRow key={row.id}>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{row.name}</span>
                        {!row.writable && <Badge variant="outline">{t("pages.routeTemplates.readOnly")}</Badge>}
                      </div>
                    </TableCell>
                    <TableCell className="max-w-sm whitespace-normal text-xs leading-5 text-muted-foreground">
                      {summarizeTemplate(row.body, summaryLabels)}
                    </TableCell>
                    <TableCell>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-auto px-0 py-1 text-xs text-muted-foreground"
                        onClick={() => onUsage(row)}
                      >
                        {usedByLabel(row.usedBy)}
                      </Button>
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                      {formatUpdatedAt(row.updated_at) || "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button size="sm" variant="ghost" disabled={!row.writable} onClick={() => onEdit(row)}>
                          {t("pages.routeTemplates.edit")}
                        </Button>
                        <DropdownMenu>
                          <DropdownMenuTrigger
                            render={
                              <Button
                                variant="ghost"
                                size="icon-sm"
                                aria-label={t("pages.routeTemplates.templateActions", { name: row.name })}
                              />
                            }
                          >
                            <MoreHorizontal />
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-40">
                            <DropdownMenuItem onClick={() => onViewJson(row)}>
                              <FileJson />
                              {t("pages.routeTemplates.viewJson")}
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => onCopy(row)}>
                              <Copy />
                              {t("pages.routeTemplates.copy")}
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              variant="destructive"
                              disabled={!row.writable}
                              onClick={() => onDelete(row)}
                            >
                              <Trash2 />
                              {t("pages.routeTemplates.delete")}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
                {filteredRows.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                      {t("pages.routeTemplates.noResults")}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
