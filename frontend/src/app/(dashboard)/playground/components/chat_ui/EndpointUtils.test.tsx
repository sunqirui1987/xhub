import { describe, expect, it } from "vitest";
import type { ModelGroup, ModelEndpoint } from "@/components/llm_calls/fetch_models";
import { determineEndpointType, filterModelsForEndpoint, isModelCompatibleWithEndpoint } from "./EndpointUtils";
import { EndpointType } from "@/components/chat_ui/mode_endpoint_mapping";
const chat: ModelEndpoint = {
  endpoint_type: "chat",
  transport: "adapted",
  kind: "adapted",
  protocol: "adapted",
  family: "chat",
  method: "POST",
  path: "/v1/chat/completions",
};
const models: ModelGroup[] = [
  { model_group: "configured", mode: "image_generation", endpoints: [chat] },
  { model_group: "unbound", mode: "chat" },
];
describe("explicit endpoint contracts", () => {
  it("uses bindings when display category disagrees", () => {
    expect(determineEndpointType("configured", models)).toBe(EndpointType.CHAT);
  });
  it("does not invent support for unbound or missing models", () => {
    expect(determineEndpointType("unbound", models)).toBeNull();
    expect(determineEndpointType("missing", models)).toBeNull();
  });
  it("filters actual supported endpoints", () => {
    expect(filterModelsForEndpoint(models, EndpointType.CHAT)).toEqual([models[0]]);
    expect(isModelCompatibleWithEndpoint(models[0], EndpointType.IMAGE)).toBe(false);
  });
});
