/** @vitest-environment jsdom */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import { BuiltinModelPicker, selectedNewIds } from "./BuiltinModelPicker";

vi.mock("@/components/networking", () => ({ getProxyBaseUrl: () => "http://gateway.test" }));

describe("selectedNewIds", () => {
  it("skips models that are already added", () => {
    expect(
      selectedNewIds(
        [
          { id: "old", added: true },
          { id: "new", added: false },
        ],
        { old: true, new: true },
      ),
    ).toEqual(["new"]);
  });
});

describe("BuiltinModelPicker", () => {
  beforeEach(() => setActiveLocale("en"));

  it("grays out added models and posts only the checked new ids", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      const body = JSON.parse(String(init?.body ?? "{}")) as { provider?: string; model_ids?: string[] };
      if (String(url).endsWith("/model/builtin/add")) {
        return new Response(JSON.stringify({ updated: true, model_ids: body.model_ids, api_base: "https://api.fenno.ai", wire_api: "responses" }), { status: 200 });
      }
      if (String(url).endsWith("/model/builtin/refresh")) {
        return new Response(
          JSON.stringify({
            updated: true,
            provider: body.provider,
            api_base: body.provider === "qiniu" ? "https://api.qnaigc.com/bypass/openai/v1" : "https://api.fenno.ai",
            wire_api: "responses",
            model_ids: body.provider === "qiniu" ? ["qwen-turbo"] : ["gpt-fenno-2"],
          }),
          { status: 200 },
        );
      }
      const models =
        body.provider === "qiniu"
          ? [
              { id: "qwen-turbo", added: true },
              { id: "deepseek/deepseek-v3.2-exp", added: false },
            ]
          : [{ id: "gpt-fenno", added: false }];
      const apiBase = body.provider === "qiniu" ? "https://api.qnaigc.com/bypass/openai/v1" : "https://api.fenno.ai";
      return new Response(JSON.stringify({ provider: body.provider, api_base: apiBase, models }), {
        status: 200,
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<BuiltinModelPicker accessToken="token" onAdded={() => undefined} />);

    const added = await screen.findByRole("checkbox", { name: "qwen-turbo" });
    expect(added).toHaveAttribute("aria-disabled", "true");
    expect(added.closest("li")?.className).toContain("opacity-60");

    await user.click(await screen.findByRole("checkbox", { name: "deepseek/deepseek-v3.2-exp" }));
    const addButtons = screen.getAllByRole("button", { name: "Add selected models" });
    await user.click(addButtons[1]);

    const addCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith("/model/builtin/add"));
    expect(JSON.parse(String(addCall?.[1]?.body))).toMatchObject({
      provider: "qiniu",
      model_ids: ["deepseek/deepseek-v3.2-exp"],
    });

    await user.click(screen.getByRole("button", { name: "Fetch fennoai models" }));
    await user.click(screen.getByRole("button", { name: "Fetch qiniu models" }));

    const refreshCalls = fetchMock.mock.calls.filter((call) => String(call[0]).endsWith("/model/builtin/refresh"));
    expect(refreshCalls.map((call) => JSON.parse(String(call[1]?.body)))).toEqual([
      { provider: "fennoai" },
      { provider: "qiniu" },
    ]);
    expect(screen.getByText("https://api.fenno.ai")).toBeTruthy();
    expect(screen.getByText("https://api.qnaigc.com/bypass/openai/v1")).toBeTruthy();
  });
});
