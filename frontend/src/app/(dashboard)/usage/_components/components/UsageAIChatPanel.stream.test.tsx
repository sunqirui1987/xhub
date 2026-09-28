import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "@/../tests/test-utils";
import { setActiveLocale } from "@/i18n/runtime";
import UsageAIChatPanel from "./UsageAIChatPanel";

beforeAll(() => {
  if (typeof window !== "undefined" && !window.ResizeObserver) {
    window.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as any;
  }
});

describe("UsageAIChatPanel stream", () => {
  it("renders the usage AI route error instead of a blank reply", async () => {
    setActiveLocale("zh-CN");
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/usage/ai/chat")) {
          return new Response(
            JSON.stringify({ error: { message: "No model is configured for usage AI", type: "usage_ai_unavailable" } }),
            { status: 400, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(JSON.stringify({ data: [] }), { status: 200, headers: { "Content-Type": "application/json" } });
      }),
    );

    renderWithProviders(<UsageAIChatPanel open onClose={vi.fn()} accessToken="test-token" />);
    expect(screen.getByRole("heading", { name: "问 AI" })).toBeInTheDocument();
    expect(screen.getByText("询问支出、模型、密钥和趋势")).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("询问你的用量…"), "哪个模型最贵");
    await user.click(screen.getByRole("button", { name: "发送" }));

    expect(await screen.findByText(/No model is configured for usage AI/)).toBeInTheDocument();
    setActiveLocale("en");
    vi.unstubAllGlobals();
  });
});
