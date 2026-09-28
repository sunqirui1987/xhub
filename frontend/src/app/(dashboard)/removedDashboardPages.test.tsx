import { describe, expect, it } from "vitest";

import AgentsPage from "./agents/page";
import WorkflowsPage from "./workflows/page";
import MemoryPage from "./memory/page";
import McpServersPage from "./mcp-servers/page";
import SkillsPage from "./skills/page";
import PoliciesPage from "./policies/page";
import SearchToolsPage from "./search-tools/page";
import VectorStoresPage from "./vector-stores/page";
import ToolPoliciesPage from "./tool-policies/page";
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

const removedPages = [
  ["agents", AgentsPage],
  ["workflows", WorkflowsPage],
  ["memory", MemoryPage],
  ["mcp-servers", McpServersPage],
  ["skills", SkillsPage],
  ["policies", PoliciesPage],
  ["search-tools", SearchToolsPage],
  ["vector-stores", VectorStoresPage],
  ["tool-policies", ToolPoliciesPage],
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

const removedPaths = [
  "/agents", "/workflows", "/memory", "/mcp-servers", "/skills", "/policies",
  "/search-tools", "/vector-stores", "/tool-policies", "/api-reference",
  "/model-hub-table", "/model_hub", "/model_hub_table", "/caching", "/prompts",
  "/transform-request", "/tag-management", "/old-usage", "/budgets", "/ui-theme",
];

describe("removed dashboard pages", () => {
  it.each(removedPages)("%s is a page component", (_name, Page) => {
    expect(typeof Page).toBe("function");
  });

  it("renders write pages and their /ui copies in Next", () => {
    for (const path of removedPaths) {
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
