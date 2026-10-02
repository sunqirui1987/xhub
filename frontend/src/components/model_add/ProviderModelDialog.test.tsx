/** @vitest-environment jsdom */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import { ProviderModelDialog } from "./ProviderModelDialog";

vi.mock("@/components/networking", () => ({ getProxyBaseUrl: () => "http://gateway.test" }));

describe("ProviderModelDialog", () => {
  beforeEach(() => setActiveLocale("en"));

  it("opens as a dialog, refreshes the provider, and adds one model with the selected credential", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      const body = JSON.parse(String(init?.body ?? "{}")) as { provider?: string; model_ids?: string[]; credential_name?: string };
      if (String(url).endsWith("/model/builtin/add")) {
        return new Response(JSON.stringify({ updated: true, api_base: "https://api.fenno.ai", wire_api: "responses" }), { status: 200 });
      }
      return new Response(
        JSON.stringify({
          provider: "fennoai",
          api_base: "https://api.fenno.ai",
          wire_api: "responses",
          models: [
            { id: "gpt-fenno", added: true },
            { id: "codex-mini", added: false },
          ],
        }),
        { status: 200 },
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    render(
      <ProviderModelDialog
        provider="fennoai"
        accessToken="token"
        initialCredentialName="fennoai-work"
        credentials={[
          { credential_name: "fennoai", credential_values: {}, credential_info: { builtin: "fennoai" } },
          { credential_name: "fennoai-work", credential_values: {}, credential_info: { builtin: "fennoai" } },
        ]}
        onClose={() => undefined}
        onAdded={() => undefined}
      />,
    );

    expect(await screen.findByRole("dialog")).toBeTruthy();
    expect(screen.getByText("Already added")).toBeTruthy();
    expect(screen.queryByText("https://api.fenno.ai")).toBeNull();
    expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith("/model/builtin/models"))).toBe(true);
    expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith("/model/builtin/add"))).toBe(false);

    expect(screen.getByRole("checkbox", { name: "gpt-fenno" })).toBeDisabled();

    await user.click(screen.getByRole("checkbox", { name: "codex-mini" }));
    await user.click(screen.getByRole("button", { name: "Add selected models" }));
    const addCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith("/model/builtin/add"));
    expect(JSON.parse(String(addCall?.[1]?.body))).toMatchObject({
      provider: "fennoai",
      model_ids: ["codex-mini"],
      credential_name: "fennoai-work",
    });
    const listCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith("/model/builtin/models"));
    expect(JSON.parse(String(listCall?.[1]?.body)).credential_name).toBe("fennoai-work");
    expect(JSON.parse(String(addCall?.[1]?.body))).not.toHaveProperty("mode");
    expect(JSON.parse(String(addCall?.[1]?.body)).model_ids).not.toContain("gpt-fenno");
  });
});
