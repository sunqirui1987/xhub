import { beforeEach, describe, expect, it, vi } from "vitest";
import { modelAvailableCall, userAvailableModelsCall } from "@/components/networking";
import { fetchAvailableModels, fetchAvailableModelsForTeam } from "./fetch_models";

vi.mock("@/components/networking", () => ({
  modelAvailableCall: vi.fn(),
  userAvailableModelsCall: vi.fn(),
}));

const modelAvailableCallMock = vi.mocked(modelAvailableCall);
const availableMock = vi.mocked(userAvailableModelsCall);

describe("fetchAvailableModelsForTeam", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("requests the models scoped to the team so team-only BYOK models are included", async () => {
    modelAvailableCallMock.mockResolvedValue({
      data: [{ id: "all-proxy-models" }, { id: "openai/*" }, { id: "gpt-5-mini" }, { id: "openai/*" }],
    });

    const models = await fetchAvailableModelsForTeam("token", "team-123");

    expect(modelAvailableCallMock).toHaveBeenCalledWith("token", "", "", false, "team-123");
    expect(models).toEqual([{ model_group: "gpt-5-mini" }, { model_group: "openai/*" }]);
  });

  it("returns an empty list when the team has no models", async () => {
    modelAvailableCallMock.mockResolvedValue({ data: [] });

    expect(await fetchAvailableModelsForTeam("token", "team-123")).toEqual([]);
  });
});

describe("fetchAvailableModels", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("reads the models a signed-in caller may use, including the endpoint type", async () => {
    availableMock.mockResolvedValue({
      data: [
        { id: "kimi-k2", category: "chat" },
        { id: "embed-small", category: "embedding", capabilities: ["reasoning"] },
      ],
    });

    expect(await fetchAvailableModels("token")).toEqual([
      { model_group: "embed-small", endpoints: [], mode: "embedding", supports_reasoning: true },
      { model_group: "kimi-k2", endpoints: [], mode: "chat" },
    ]);
    expect(availableMock).toHaveBeenCalledWith("token");
  });

  it("does not expose provider shells as selectable models", async () => {
    availableMock.mockResolvedValue({
      data: [
        { id: "fennoai", role: "provider", category: "chat" },
        { id: "qiniu", model_info: { role: "provider" } },
        { id: "gpt-5.6-sol", category: "chat" },
      ],
    });

    expect(await fetchAvailableModels("token")).toEqual([{ model_group: "gpt-5.6-sol", endpoints: [], mode: "chat" }]);
  });

  it("keeps a model without endpoint metadata unbound", async () => {
    availableMock.mockResolvedValue({ data: [{ id: "plain" }] });
    expect(await fetchAvailableModels("token")).toEqual([{ model_group: "plain", endpoints: [] }]);
  });

  it.each([
    ["an error payload in place of the list", { data: { error: "no access" } }],
    ["a missing data key", {}],
    ["no body at all", undefined],
  ])("returns an empty list on %s rather than throwing", async (_label, response) => {
    availableMock.mockResolvedValue(response);
    expect(await fetchAvailableModels("token")).toEqual([]);
  });
});
