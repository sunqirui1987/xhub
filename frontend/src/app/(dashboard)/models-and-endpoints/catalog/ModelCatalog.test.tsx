/** @vitest-environment jsdom */
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import { ModelCatalog } from "./ModelCatalog";

vi.mock("@/components/networking", () => ({ getProxyBaseUrl: () => "http://gateway.test" }));

const catalog = {
  models: [
    { id: "gpt-chat", category: "llm", input_price: 1.25, output_price: 10, added: false },
    { id: "eye-1", category: "vision", input_price: 2.5, output_price: 8, added: true },
    { id: "gpt-4o", category: "llm", input_price: 2.5, output_price: 10, added: false },
  ],
};

describe("ModelCatalog", () => {
  beforeEach(() => setActiveLocale("en"));

  it("shows prices by category and adds only the chosen card", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      const path = String(url);
      const body = JSON.parse(String(init?.body ?? "{}")) as { provider?: string };
      if (path.endsWith("/model/builtin/add")) {
        return new Response(JSON.stringify({ updated: true }), { status: 200 });
      }
      const models =
        body.provider === "qiniu"
          ? [
              { id: "qwen-vision", category: "vision", input_price: 3, output_price: 6, added: false },
              { id: "gpt-4o", category: "llm", input_price: 1, output_price: 2, added: false },
            ]
          : catalog.models;
      return new Response(JSON.stringify({ models }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<ModelCatalog provider="fennoai" accessToken="token" />);

    expect(await screen.findByText("Input $1.25")).toBeTruthy();
    expect(screen.getAllByText("Output $10").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Input $2.5").length).toBeGreaterThan(0);
    expect(screen.getByText("Output $8")).toBeTruthy();
    expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith("/model/builtin/add"))).toBe(false);

    expect(screen.getByTestId("catalog-card-fennoai-gpt-4o")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "qiniu" }));
    expect(screen.queryByText("gpt-chat")).toBeNull();
    expect(screen.getByText("qwen-vision")).toBeTruthy();
    await user.click(within(screen.getByTestId("catalog-card-qiniu-qwen-vision")).getByRole("button", { name: "Add this model" }));

    const addCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith("/model/builtin/add"));
    expect(JSON.parse(String(addCall?.[1]?.body))).toEqual({
      provider: "qiniu",
      model_ids: ["qwen-vision"],
      credential_name: "qiniu",
      models: [{ id: "qwen-vision", category: "vision", input_price: 3, output_price: 6 }],
    });

    await user.click(screen.getAllByRole("button", { name: "All" })[0]);
    expect(screen.getByTestId("catalog-card-fennoai-gpt-4o")).toBeTruthy();
    expect(screen.getByTestId("catalog-card-qiniu-gpt-4o")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "vision" }));
    expect(screen.queryByText("gpt-chat")).toBeNull();
    expect(screen.getByText("eye-1")).toBeTruthy();
  });
});
