"use client";

import { CircleHelp } from "lucide-react";
import { t } from "@/i18n";

const points = [
  "pages.projects.guideWhat",
  "pages.projects.guideWho",
  "pages.projects.guideModels",
  "pages.projects.guideBudget",
  "pages.projects.guideBlock",
  "pages.projects.guideDelete",
] as const;

export function ProjectGuide() {
  return (
    <section className="mt-6 rounded-lg border bg-card px-4 py-4" aria-labelledby="project-guide-title">
      <div className="flex items-center gap-2">
        <CircleHelp className="size-4 text-muted-foreground" aria-hidden="true" />
        <h2 id="project-guide-title" className="text-sm font-semibold text-foreground">
          {t("pages.projects.guideTitle")}
        </h2>
      </div>
      <ul className="mt-3 space-y-2 text-sm text-muted-foreground">
        {points.map((key) => (
          <li key={key}>{t(key)}</li>
        ))}
      </ul>
    </section>
  );
}
