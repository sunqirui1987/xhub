import { describe, expect, it } from "vitest";

import { dashboardAppPath } from "./middleware";

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
  "/api-reference",
  "/model-hub-table",
  "/caching",
  "/prompts",
  "/transform-request",
  "/tag-management",
  "/old-usage",
];

describe("dashboardAppPath", () => {
  it("does not render removed columns, including /agents, as dashboard pages", () => {
    for (const path of removed) {
      expect(dashboardAppPath(path), path).toBeNull();
      expect(dashboardAppPath(`/ui${path}`), `/ui${path}`).toBeNull();
    }
  });

  it("still renders /logs and /models-and-endpoints", () => {
    expect(dashboardAppPath("/logs")).toBe("/logs");
    expect(dashboardAppPath("/ui/logs")).toBe("/logs");
    expect(dashboardAppPath("/models-and-endpoints")).toBe("/models-and-endpoints");
    expect(dashboardAppPath("/ui/models-and-endpoints")).toBe("/models-and-endpoints");
  });
});
