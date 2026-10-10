import { describe, expect, it } from "vitest";

import { appPageWritePath, dashboardAppPath } from "./middleware";

const removed = [
  "/agents",
  "/workflows",
  "/memory",
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
  /** 目的：退役 MCP/Skills 管理入口不能作为 Next 页面；前置为真实路由解析器，根路径和 /ui 别名均交给网关，无持久化数据清理。 */
  it("leaves retired MCP and Skills surfaces to the gateway", () => {
    for (const path of ["/mcp-servers", "/skills"]) {
      expect(dashboardAppPath(path), path).toBeNull();
      expect(dashboardAppPath(`/ui${path}`), `/ui${path}`).toBeNull();
    }
  });

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

  it("renders the oauth callback in Next", () => {
    expect(dashboardAppPath("/mcp/oauth/callback")).toBe("/mcp/oauth/callback");
  });

  it("renders the audit page and leaves audit API requests for the gateway", () => {
    expect(dashboardAppPath("/audit-logs")).toBe("/audit-logs");
    expect(dashboardAppPath("/ui/audit-logs")).toBe("/audit-logs");
    expect(dashboardAppPath("/audit/logs")).toBeNull();
  });
});
