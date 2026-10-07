import { describe, expect, it } from "vitest";
import { existsSync } from "node:fs";
import { join } from "node:path";

import PoliciesPage from "./policies/page";
import SearchToolsPage from "./search-tools/page";
import VectorStoresPage from "./vector-stores/page";
import ApiReferencePage from "./api-reference/page";
import ModelHubPage from "./model-hub-table/page";
import CachingPage from "./caching/page";
import PromptsPage from "./prompts/page";
import TransformRequestPage from "./transform-request/page";
import TagManagementPage from "./tag-management/page";
import OldUsagePage from "./old-usage/page";
import BudgetsPage from "./budgets/page";
import UIThemePage from "./ui-theme/page";
import PublicModelHubPage from "../model_hub/page";
import { dashboardAppPath } from "@/middleware";

/**
 * Pages kept in the tree but unreachable from the sidebar.
 *
 * They are stubs over endpoints the gateway still answers from its generic
 * catalog store. They are not linked, so they are not offered; a request that
 * reaches them by URL renders whatever the store returns.
 */
const unreachablePages = [
  ["policies", PoliciesPage],
  ["search-tools", SearchToolsPage],
  ["vector-stores", VectorStoresPage],
  ["api-reference", ApiReferencePage],
  ["model-hub", ModelHubPage],
  ["model_hub", PublicModelHubPage],
  ["caching", CachingPage],
  ["prompts", PromptsPage],
  ["transform-request", TransformRequestPage],
  ["tag-management", TagManagementPage],
  ["old-usage", OldUsagePage],
  ["budgets", BudgetsPage],
  ["ui-theme", UIThemePage],
] as const;

/**
 * Pages deleted because their backing routes were retired.
 *
 * The gateway answers /v1/agents, /v1/skills, /v1/memory, /v1/workflows and
 * /v1/tool/* with 410. The store behind them was one shared key-value namespace
 * keyed by resource kind, with no owner and no team column, so serving them let
 * any signed-in member create records that every other principal could list.
 * The pages existed only to call those routes.
 *
 * The assertion is that the directories are gone, so re-adding a page that
 * writes to a retired route fails here rather than reaching a user.
 */
const retiredPageDirs = ["agents", "memory", "workflows", "tool-policies"];

/**
 * Pages removed with their feature.
 *
 * Access groups bundled MCP servers and skills for a team; with those two gone
 * the bundle had nothing to hold, so all three left together. The directories
 * must not come back on their own: a page that calls a route nobody serves
 * renders an empty list and looks like a working feature.
 */
const removedFeatureDirs = ["access-groups", "mcp-servers", "skills", "cost-tracking", "logging-and-alerts"];

const dashboardDir = join(__dirname);

describe("dashboard pages", () => {
  it.each(unreachablePages)("%s is a page component", (_name, Page) => {
    expect(typeof Page).toBe("function");
  });

  it.each(retiredPageDirs)("%s is not in the tree", (name) => {
    expect(existsSync(join(dashboardDir, name, "page.tsx")), name).toBe(false);
  });

  it.each(removedFeatureDirs)("%s is gone with its feature", (name) => {
    expect(existsSync(join(dashboardDir, name, "page.tsx")), name).toBe(false);
  });

  it("renders write pages and their /ui copies in Next", () => {
    const paths = [
          "/policies",
      "/search-tools",
      "/vector-stores",
      "/api-reference",
      "/caching",
      "/prompts",
      "/transform-request",
      "/tag-management",
      "/old-usage",
      "/budgets",
      "/ui-theme",
    ];
    for (const path of paths) {
      expect(dashboardAppPath(path), path).toBe(path);
      expect(dashboardAppPath(`/ui${path}`), `/ui${path}`).toBe(path);
    }
  });

  it("still renders /logs, /models-and-endpoints, and /admin-panel", () => {
    expect(dashboardAppPath("/logs")).toBe("/logs");
    expect(dashboardAppPath("/ui/logs")).toBe("/logs");
    expect(dashboardAppPath("/models-and-endpoints")).toBe("/models-and-endpoints");
    expect(dashboardAppPath("/ui/models-and-endpoints")).toBe("/models-and-endpoints");
    expect(dashboardAppPath("/admin-panel")).toBe("/admin-panel");
    expect(dashboardAppPath("/ui/admin-panel")).toBe("/admin-panel");
  });
});
