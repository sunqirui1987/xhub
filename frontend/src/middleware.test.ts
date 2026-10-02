import { describe, expect, it } from "vitest";

import { appPageWritePath, dashboardAppPath } from "./middleware";

const removed = [
  "/agents",
  "/workflows",
  "/memory",
  "/mcp-servers",
  "/skills",
  "/policies",
  "/search-tools",
  "/vector-stores",
  "/tool-policies",
  "/budgets",
  "/caching",
  "/prompts",
  "/transform-request",
  "/tag-management",
  "/ui-theme",
  "/old-usage",
  "/api-reference",
  "/model-hub-table",
  "/model_hub",
  "/model_hub_table",
];

describe("dashboardAppPath", () => {
  it("renders write pages and their /ui copies in Next", () => {
    for (const path of removed) {
      expect(dashboardAppPath(path), path).toBe(path);
      expect(dashboardAppPath(`/ui${path}`), `/ui${path}`).toBe(path);
    }
  });

  it("still renders /logs and /models-and-endpoints", () => {
    expect(dashboardAppPath("/logs")).toBe("/logs");
    expect(dashboardAppPath("/ui/logs")).toBe("/logs");
    expect(dashboardAppPath("/models-and-endpoints")).toBe("/models-and-endpoints");
    expect(dashboardAppPath("/ui/models-and-endpoints")).toBe("/models-and-endpoints");
    expect(dashboardAppPath("/mine-models")).toBe("/mine-models");
    expect(dashboardAppPath("/ui/mine-models")).toBe("/mine-models");
    expect(dashboardAppPath("/price-data")).toBe("/price-data");
    expect(dashboardAppPath("/ui/price-data")).toBe("/price-data");
  });

  it("sends guardrail writes on the page path to the gateway and keeps the page GET", () => {
    expect(appPageWritePath("POST", "/guardrails")).toBe("/guardrails");
    expect(appPageWritePath("GET", "/guardrails")).toBeNull();
    expect(appPageWritePath("POST", "/ui/guardrails")).toBeNull();
    expect(appPageWritePath("POST", "/guardrails/list")).toBeNull();
  });

  it("renders onboarding and the oauth callback in Next", () => {
    expect(dashboardAppPath("/ui/onboarding")).toBe("/onboarding");
    expect(dashboardAppPath("/mcp/oauth/callback")).toBe("/mcp/oauth/callback");
  });
});
