import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import RouteTemplateJsonGuide, { ROUTE_TEMPLATE_EXAMPLE } from "./RouteTemplateJsonGuide";

describe("RouteTemplateJsonGuide", () => {
  it("documents the advanced fallback, retry, and alias fields", () => {
    render(<RouteTemplateJsonGuide />);

    expect(screen.getByText("Complete route-template JSON guide")).toBeInTheDocument();
    for (const field of [
      "fallbacks",
      "context_window_fallbacks",
      "content_policy_fallbacks",
      "default_fallbacks",
      "fallback_causes",
      "retry_policy",
      "model_group_alias",
    ]) {
      expect(screen.getByText(field)).toBeInTheDocument();
    }
  });

  it("provides a valid complete example with routing groups and default fallbacks", () => {
    const reparsed = JSON.parse(JSON.stringify(ROUTE_TEMPLATE_EXAMPLE)) as Record<string, unknown>;

    expect(reparsed.routing_groups).toEqual(ROUTE_TEMPLATE_EXAMPLE.routing_groups);
    expect(reparsed.default_fallbacks).toEqual(["gpt-4o-mini"]);
  });
});
