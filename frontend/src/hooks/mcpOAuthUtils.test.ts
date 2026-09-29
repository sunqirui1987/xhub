import { afterEach, describe, expect, it } from "vitest";
import { buildCallbackUrl } from "./mcpOAuthUtils";

describe("buildCallbackUrl", () => {
  const originalLocation = window.location;

  afterEach(() => {
    Object.defineProperty(window, "location", {
      value: originalLocation,
      writable: true,
    });
  });

  it("stays on the console origin and does not prefix the gateway API base", () => {
    Object.defineProperty(window, "location", {
      value: {
        ...originalLocation,
        origin: "http://localhost:3000",
        pathname: "/mcp-servers",
      },
      writable: true,
    });
    const url = buildCallbackUrl();
    expect(url).toBe("http://localhost:3000/mcp/oauth/callback");
    expect(url).not.toContain(":4000");
    expect(url).not.toContain("localhost:4000/ui");
  });

  it("keeps a /ui prefix that is already on the console path", () => {
    Object.defineProperty(window, "location", {
      value: {
        ...originalLocation,
        origin: "http://localhost:3000",
        pathname: "/ui/mcp-servers",
      },
      writable: true,
    });
    expect(buildCallbackUrl()).toBe("http://localhost:3000/ui/mcp/oauth/callback");
  });
});
