/* @vitest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AuthProvider, useAuth } from "./AuthContext";

const path = { current: "/login" };

vi.mock("next/navigation", () => ({
  usePathname: () => path.current,
}));

vi.mock("@/components/networking", () => ({
  getUiConfig: vi.fn().mockResolvedValue({}),
  setGlobalLitellmHeaderName: vi.fn(),
}));

function sessionJwt() {
  const payload = btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 3600, key: "sess-1", user_role: "proxy_admin" }))
    .replace(/=+$/g, "")
    .replace(/\+/g, "-")
    .replace(/\//g, "_");
  return `eyJhbGciOiJub25lIn0.${payload}.sig`;
}

function Probe() {
  const { authLoading, token } = useAuth();
  useEffect(() => {
    if (!authLoading && !token) {
      window.location.replace("/ui/login/");
    }
  }, [authLoading, token]);
  return <div data-testid="session">{authLoading ? "loading" : (token ?? "none")}</div>;
}

describe("AuthProvider session cookie", () => {
  const realLocation = window.location;

  beforeEach(() => {
    path.current = "/login";
    document.cookie = "token=; Max-Age=0; Path=/";
    sessionStorage.clear();
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { ...realLocation, href: "http://localhost:3000/login", replace: vi.fn() },
    });
  });

  afterEach(() => {
    Object.defineProperty(window, "location", { configurable: true, value: realLocation });
  });

  it("reads a cookie written after the login page mounted when the route changes", async () => {
    const { rerender } = render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("session")).toHaveTextContent("none"));
    const replace = vi.mocked(window.location.replace);
    replace.mockClear();

    const jwt = sessionJwt();
    document.cookie = `token=${jwt}`;
    path.current = "/ui";
    rerender(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("session").textContent).toContain(jwt));
    expect(replace).not.toHaveBeenCalled();
  });
});
