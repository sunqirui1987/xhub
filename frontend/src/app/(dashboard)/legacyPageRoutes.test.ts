import { describe, expect, it } from "vitest";
import { menuGroups } from "@/components/leftnav";
import { legacyPageRedirectHref } from "./legacyPageRoutes";

const redirect = (query: string) => legacyPageRedirectHref(new URLSearchParams(query));

describe("legacyPageRedirectHref", () => {
  it("sends an old ?page= bookmark to the path route that replaced it", () => {
    expect(redirect("page=logs")).toBe("/ui/logs");
    expect(redirect("page=models")).toBe("/ui/models-and-endpoints");
    expect(redirect("page=llm-playground")).toBe("/ui/playground");
    expect(redirect("page=new_usage")).toBe("/ui/usage");
  });

  it("does not open removed columns from old bookmarks", () => {
    for (const page of [
      "agents",
      "workflows",
      "memory",
      "mcp-servers",
      "skills",
      "policies",
      "search-tools",
      "vector-stores",
      "tool-policies",
      "prompts",
      "tag-management",
      "transform-request",
      "caching",
      "api_ref",
      "api-reference",
      "model-hub-table",
      "usage",
      "claude-code-plugins",
      "budgets",
      "ui-theme",
    ]) {
      expect(redirect(`page=${page}`), page).toBeNull();
    }
  });

  it("returns null when there is no page param or the id is unknown", () => {
    expect(redirect("")).toBeNull();
    expect(redirect("login=success")).toBeNull();
    expect(redirect("page=does-not-exist")).toBeNull();
    expect(redirect("page=constructor")).toBeNull();
  });

  it("covers every sidebar page id with the route the sidebar itself links to", () => {
    const leaves = menuGroups
      .flatMap((group) => group.items.flatMap((item) => item.children ?? [item]))
      .filter((item) => !item.external_url);
    expect(leaves.length).toBeGreaterThan(10);
    for (const leaf of leaves) {
      expect(redirect(`page=${leaf.page}`), leaf.page).toBe(`/ui/${leaf.route ?? leaf.page}`);
    }
  });
});
