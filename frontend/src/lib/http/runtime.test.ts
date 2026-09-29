import { afterEach, describe, expect, it, vi } from "vitest";
import { getAuthHeaderName, getRequestBaseUrl } from "./runtime";

describe("runtime request config defaults", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("resolves the default base URL from NEXT_PUBLIC_BASE_URL before a getter is registered", () => {
    vi.stubEnv("NEXT_PUBLIC_BASE_URL", "https://proxy.example.com/");
    expect(getRequestBaseUrl()).toBe("https://proxy.example.com");
  });

  it("defaults the base URL to the local gateway when NEXT_PUBLIC_BASE_URL is unset", () => {
    vi.stubEnv("NEXT_PUBLIC_BASE_URL", "");
    expect(getRequestBaseUrl()).toBe("http://localhost:4000");
  });

  it("defaults the auth header name to Authorization", () => {
    expect(getAuthHeaderName()).toBe("Authorization");
  });
});
