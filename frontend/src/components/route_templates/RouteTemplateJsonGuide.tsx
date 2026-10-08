"use client";

import React, { useState } from "react";
import { Check, Copy, Info, TriangleAlert } from "lucide-react";
import { t } from "@/i18n";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

export const ROUTE_TEMPLATE_EXAMPLE = {
  routing_strategy: "weighted-split",
  routing_strategy_args: {
    weights: [
      { model_name: "gpt-main", api_base: "https://api-a.example.com", model: "gpt-4o", weight: 7 },
      { model_name: "gpt-main", api_base: "https://api-b.example.com", model: "gpt-4o", weight: 3 },
    ],
  },
  routing_groups: [
    {
      group_name: "fast-chat",
      models: ["gpt-main", "gpt-4o-mini"],
      routing_strategy: "least-busy",
      routing_strategy_args: {},
    },
  ],
  num_retries: 2,
  timeout: 60,
  stream_timeout: 120,
  allowed_fails: 3,
  cooldown_time: 60,
  retry_after: 1,
  max_fallbacks: 2,
  fallback_causes: ["no_response", "status", "ambiguous"],
  fallbacks: [{ "gpt-main": ["claude-main", "gemini-main"] }],
  context_window_fallbacks: [{ "gpt-main": ["gpt-long-context"] }],
  content_policy_fallbacks: [{ "gpt-main": ["claude-main"] }],
  default_fallbacks: ["gpt-4o-mini"],
  retry_policy: {
    RateLimitErrorRetries: 2,
    TimeoutErrorRetries: 1,
    InternalServerErrorRetries: 1,
  },
  model_group_retry_policy: {
    "gpt-main": { RateLimitErrorRetries: 3 },
  },
  model_group_alias: { cheap: "gpt-4o-mini" },
  enable_pre_call_checks: true,
  enable_tag_filtering: false,
};

const EXAMPLE_TEXT = JSON.stringify(ROUTE_TEMPLATE_EXAMPLE, null, 2);
const normalFallbackExample = JSON.stringify(
  { fallbacks: [{ "gpt-main": ["claude-main", "gemini-main"] }], max_fallbacks: 2 },
  null,
  2,
);
const contextFallbackExample = JSON.stringify(
  {
    enable_pre_call_checks: true,
    context_window_fallbacks: [{ "gpt-main": ["gpt-long-context"] }],
  },
  null,
  2,
);
const safeFallbackExample = JSON.stringify({ fallback_causes: ["no_response", "status"] }, null, 2);

type FieldRow = {
  name: string;
  type: string;
  defaultValue: string;
  description: string;
  example: string;
  support: "active" | "stored";
};

const field = (row: FieldRow): FieldRow => row;

const fieldGroups = (): Array<{ title: string; description: string; rows: FieldRow[] }> => [
  {
    title: t("pages.routeTemplates.jsonGuide.balanceFields"),
    description: t("pages.routeTemplates.jsonGuide.balanceFieldsHint"),
    rows: [
      field({
        name: "routing_strategy",
        type: "string",
        defaultValue: "simple-shuffle",
        description: t("pages.routeTemplates.jsonGuide.fieldRoutingStrategy"),
        example: '"least-busy"',
        support: "active",
      }),
      field({
        name: "routing_strategy_args",
        type: "object",
        defaultValue: "{}",
        description: t("pages.routeTemplates.jsonGuide.fieldRoutingStrategyArgs"),
        example: '{"weights":[…]}',
        support: "active",
      }),
      field({
        name: "routing_groups",
        type: "array",
        defaultValue: "[]",
        description: t("pages.routeTemplates.jsonGuide.fieldRoutingGroups"),
        example: '[{"group_name":"fast",…}]',
        support: "stored",
      }),
      field({
        name: "enable_tag_filtering",
        type: "boolean",
        defaultValue: "false",
        description: t("pages.routeTemplates.jsonGuide.fieldTagFiltering"),
        example: "true",
        support: "stored",
      }),
    ],
  },
  {
    title: t("pages.routeTemplates.jsonGuide.retryFields"),
    description: t("pages.routeTemplates.jsonGuide.retryFieldsHint"),
    rows: [
      field({
        name: "num_retries",
        type: "number",
        defaultValue: t("pages.routeTemplates.jsonGuide.platformDefault"),
        description: t("pages.routeTemplates.jsonGuide.fieldNumRetries"),
        example: "2",
        support: "active",
      }),
      field({
        name: "retry_policy",
        type: "object",
        defaultValue: "{}",
        description: t("pages.routeTemplates.jsonGuide.fieldRetryPolicy"),
        example: '{"RateLimitErrorRetries":2}',
        support: "stored",
      }),
      field({
        name: "model_group_retry_policy",
        type: "object",
        defaultValue: "{}",
        description: t("pages.routeTemplates.jsonGuide.fieldGroupRetryPolicy"),
        example: '{"gpt-main":{"TimeoutErrorRetries":1}}',
        support: "stored",
      }),
      field({
        name: "retry_after",
        type: "number",
        defaultValue: "0",
        description: t("pages.routeTemplates.jsonGuide.fieldRetryAfter"),
        example: "1",
        support: "stored",
      }),
      field({
        name: "timeout",
        type: "number",
        defaultValue: t("pages.routeTemplates.jsonGuide.platformDefault"),
        description: t("pages.routeTemplates.jsonGuide.fieldTimeout"),
        example: "60",
        support: "active",
      }),
      field({
        name: "stream_timeout",
        type: "number | null",
        defaultValue: "null",
        description: t("pages.routeTemplates.jsonGuide.fieldStreamTimeout"),
        example: "120",
        support: "stored",
      }),
    ],
  },
  {
    title: t("pages.routeTemplates.jsonGuide.fallbackFields"),
    description: t("pages.routeTemplates.jsonGuide.fallbackFieldsHint"),
    rows: [
      ...[
        ["fallbacks", "array", "[]", "fieldFallbacks", '[{"gpt-main":["claude-main"]}]'],
        ["context_window_fallbacks", "array", "[]", "fieldContextFallbacks", '[{"gpt-main":["gpt-long"]}]'],
        ["content_policy_fallbacks", "array", "[]", "fieldContentFallbacks", '[{"gpt-main":["claude-main"]}]'],
        ["default_fallbacks", "string[]", "[]", "fieldDefaultFallbacks", '["gpt-4o-mini"]'],
        [
          "fallback_causes",
          "string[]",
          '["no_response","status","ambiguous"]',
          "fieldFallbackCauses",
          '["no_response","status"]',
        ],
        ["max_fallbacks", "number", "5", "fieldMaxFallbacks", "2"],
        ["enable_pre_call_checks", "boolean", "false", "fieldPreCallChecks", "true"],
      ].map(([name, type, defaultValue, descriptionKey, example]) =>
        field({
          name,
          type,
          defaultValue,
          description: t(`pages.routeTemplates.jsonGuide.${descriptionKey}`),
          example,
          support: "stored",
        }),
      ),
    ],
  },
  {
    title: t("pages.routeTemplates.jsonGuide.healthFields"),
    description: t("pages.routeTemplates.jsonGuide.healthFieldsHint"),
    rows: [
      field({
        name: "allowed_fails",
        type: "number",
        defaultValue: "3",
        description: t("pages.routeTemplates.jsonGuide.fieldAllowedFails"),
        example: "3",
        support: "active",
      }),
      field({
        name: "cooldown_time",
        type: "number",
        defaultValue: "0 (= 60s)",
        description: t("pages.routeTemplates.jsonGuide.fieldCooldown"),
        example: "60",
        support: "active",
      }),
      field({
        name: "model_group_alias",
        type: "object",
        defaultValue: "{}",
        description: t("pages.routeTemplates.jsonGuide.fieldAlias"),
        example: '{"cheap":"gpt-4o-mini"}',
        support: "stored",
      }),
    ],
  },
];

const CodeBlock: React.FC<{ children: string }> = ({ children }) => (
  <pre className="overflow-x-auto rounded-lg border border-border bg-muted/50 p-3 font-mono text-xs leading-5 text-foreground">
    {children}
  </pre>
);

const RouteTemplateJsonGuide: React.FC = () => {
  const [copied, setCopied] = useState(false);

  const copyExample = async () => {
    try {
      await navigator.clipboard.writeText(EXAMPLE_TEXT);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopied(false);
    }
  };

  return (
    <div className="space-y-6 pb-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold text-foreground">{t("pages.routeTemplates.jsonGuide.title")}</h3>
        <p className="text-sm leading-6 text-muted-foreground">{t("pages.routeTemplates.jsonGuide.intro")}</p>
      </div>

      <Alert>
        <Info />
        <AlertTitle>{t("pages.routeTemplates.jsonGuide.ruleTitle")}</AlertTitle>
        <AlertDescription>{t("pages.routeTemplates.jsonGuide.ruleBody")}</AlertDescription>
      </Alert>

      <Alert className="border-amber-300 bg-amber-50 text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-100">
        <TriangleAlert />
        <AlertTitle>{t("pages.routeTemplates.jsonGuide.currentStatusTitle")}</AlertTitle>
        <AlertDescription className="text-current/80">
          {t("pages.routeTemplates.jsonGuide.currentStatusBody")}
        </AlertDescription>
      </Alert>

      <section className="space-y-3">
        <div>
          <h4 className="font-medium text-foreground">{t("pages.routeTemplates.jsonGuide.flowTitle")}</h4>
          <p className="mt-1 text-xs text-muted-foreground">{t("pages.routeTemplates.jsonGuide.flowHint")}</p>
        </div>
        <ol className="grid gap-3 md:grid-cols-2">
          {[1, 2, 3, 4].map((step) => (
            <li key={step} className="rounded-lg border border-border bg-card p-4">
              <div className="mb-2 flex size-6 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
                {step}
              </div>
              <p className="font-medium text-foreground">{t("pages.routeTemplates.jsonGuide.flow" + step + "Title")}</p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t("pages.routeTemplates.jsonGuide.flow" + step + "Body")}
              </p>
            </li>
          ))}
        </ol>
      </section>

      <section className="space-y-3">
        <h4 className="font-medium text-foreground">{t("pages.routeTemplates.jsonGuide.fallbackRecipes")}</h4>
        <div className="grid gap-3">
          <Card size="sm">
            <CardHeader>
              <CardTitle>{t("pages.routeTemplates.jsonGuide.normalFallbackTitle")}</CardTitle>
              <CardDescription>{t("pages.routeTemplates.jsonGuide.normalFallbackBody")}</CardDescription>
            </CardHeader>
            <CardContent>
              <CodeBlock>{normalFallbackExample}</CodeBlock>
            </CardContent>
          </Card>
          <Card size="sm">
            <CardHeader>
              <CardTitle>{t("pages.routeTemplates.jsonGuide.contextFallbackTitle")}</CardTitle>
              <CardDescription>{t("pages.routeTemplates.jsonGuide.contextFallbackBody")}</CardDescription>
            </CardHeader>
            <CardContent>
              <CodeBlock>{contextFallbackExample}</CodeBlock>
            </CardContent>
          </Card>
          <Card size="sm">
            <CardHeader>
              <CardTitle>{t("pages.routeTemplates.jsonGuide.safeFallbackTitle")}</CardTitle>
              <CardDescription>{t("pages.routeTemplates.jsonGuide.safeFallbackBody")}</CardDescription>
            </CardHeader>
            <CardContent>
              <CodeBlock>{safeFallbackExample}</CodeBlock>
            </CardContent>
          </Card>
        </div>
      </section>

      <section className="space-y-4">
        <div>
          <h4 className="font-medium text-foreground">{t("pages.routeTemplates.jsonGuide.referenceTitle")}</h4>
          <p className="mt-1 text-xs text-muted-foreground">{t("pages.routeTemplates.jsonGuide.referenceHint")}</p>
        </div>
        {fieldGroups().map((group) => (
          <div key={group.title} className="overflow-hidden rounded-lg border border-border">
            <div className="border-b border-border bg-muted/40 px-4 py-3">
              <p className="font-medium text-foreground">{group.title}</p>
              <p className="mt-1 text-xs text-muted-foreground">{group.description}</p>
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("pages.routeTemplates.jsonGuide.field")}</TableHead>
                  <TableHead>{t("pages.routeTemplates.jsonGuide.type")}</TableHead>
                  <TableHead>{t("pages.routeTemplates.jsonGuide.defaultValue")}</TableHead>
                  <TableHead>{t("pages.routeTemplates.jsonGuide.support")}</TableHead>
                  <TableHead className="min-w-72">{t("pages.routeTemplates.jsonGuide.purpose")}</TableHead>
                  <TableHead>{t("pages.routeTemplates.jsonGuide.example")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {group.rows.map((row) => (
                  <TableRow key={row.name}>
                    <TableCell className="font-mono text-xs font-medium">{row.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{row.type}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{row.defaultValue}</TableCell>
                    <TableCell className="whitespace-nowrap text-xs font-medium">
                      {t(`pages.routeTemplates.jsonGuide.${row.support}`)}
                    </TableCell>
                    <TableCell className="whitespace-normal text-xs leading-5 text-muted-foreground">
                      {row.description}
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{row.example}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        ))}
      </section>

      <section className="space-y-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h4 className="font-medium text-foreground">{t("pages.routeTemplates.jsonGuide.fullExample")}</h4>
            <p className="mt-1 text-xs text-muted-foreground">{t("pages.routeTemplates.jsonGuide.fullExampleHint")}</p>
          </div>
          <Button type="button" size="sm" variant="outline" onClick={() => void copyExample()}>
            {copied ? <Check /> : <Copy />}
            {copied ? t("pages.routeTemplates.jsonGuide.copied") : t("pages.routeTemplates.jsonGuide.copy")}
          </Button>
        </div>
        <CodeBlock>{EXAMPLE_TEXT}</CodeBlock>
      </section>
    </div>
  );
};

export default RouteTemplateJsonGuide;
