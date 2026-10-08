"use client";

import { useState } from "react";
import { ArrowRight, BookOpen } from "lucide-react";
import { t } from "@/i18n";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import RouteTemplateJsonGuide from "./RouteTemplateJsonGuide";

export default function RouteTemplateGuide({ compact = false }: { compact?: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <section
        aria-label={t("pages.routeTemplates.jsonGuide.tab")}
        className="overflow-hidden rounded-xl border border-border bg-muted/20"
      >
        <div className="space-y-4 p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h3 className="flex items-center gap-2 text-sm font-semibold">
              <BookOpen className="size-4 text-primary" />
              {t("pages.routeTemplates.jsonGuide.tab")}
            </h3>
            <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
              {t("pages.routeTemplates.openJsonGuide")}
            </Button>
          </div>
          {!compact && (
            <>
              <div className="grid gap-4 md:grid-cols-3">
                {[
                  {
                    title: t("pages.routeTemplates.guide.createTitle"),
                    body: t("pages.routeTemplates.guide.createBody"),
                  },
                  {
                    title: t("pages.routeTemplates.guide.scopeTitle"),
                    body: t("pages.routeTemplates.guide.scopeBody"),
                  },
                  {
                    title: t("pages.routeTemplates.guide.precedenceTitle"),
                    body: t("pages.routeTemplates.guide.precedenceBody"),
                  },
                ].map((item, index) => (
                  <div key={item.title} className="flex items-start gap-3">
                    <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-medium text-primary">
                      {index + 1}
                    </span>
                    <div className="space-y-1">
                      <p className="text-sm font-medium">{item.title}</p>
                      <p className="text-xs leading-5 text-muted-foreground">{item.body}</p>
                    </div>
                  </div>
                ))}
              </div>
              <div className="flex flex-wrap items-center gap-2 border-t border-border pt-3 text-xs text-muted-foreground">
                {["platformDefault", "scopeOrganization", "scopeTeam", "scopeKey"].map((scope, index) => (
                  <span key={scope} className="inline-flex items-center gap-2">
                    {index > 0 && <ArrowRight className="size-3" />}
                    {t(`pages.routeTemplates.${scope}`)}
                  </span>
                ))}
                <span className="md:ml-auto">{t("pages.routeTemplates.guide.supportHint")}</span>
              </div>
            </>
          )}
        </div>
      </section>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="flex max-h-[90vh] flex-col sm:max-w-5xl">
          <DialogHeader>
            <DialogTitle>{t("pages.routeTemplates.jsonGuide.tab")}</DialogTitle>
            <DialogDescription>{t("pages.routeTemplates.guide.supportHint")}</DialogDescription>
          </DialogHeader>
          <div className="min-h-0 overflow-y-auto pr-1">
            <RouteTemplateJsonGuide />
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
