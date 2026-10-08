import { describe, expect, it } from "vitest";
import { chainsFromGroups, groupsFromChains } from "./modelFallbackGroups";

describe("model fallback groups", () => {
  it("opens an empty chain on one blank group", () => {
    expect(groupsFromChains([])).toEqual([{ id: "1", primaryModel: null, fallbackModels: [] }]);
  });

  it("round-trips a primary and its ordered chain", () => {
    const groups = groupsFromChains([{ primary: "gpt-4o", targets: "claude, gemini" }]);
    expect(groups).toEqual([{ id: "1", primaryModel: "gpt-4o", fallbackModels: ["claude", "gemini"] }]);
    expect(chainsFromGroups(groups)).toEqual([{ primary: "gpt-4o", targets: "claude, gemini" }]);
  });

  it("keeps a group the operator has not given a primary yet out of the document", () => {
    expect(
      chainsFromGroups([
        { id: "1", primaryModel: "gpt-4o", fallbackModels: ["claude"] },
        { id: "2", primaryModel: null, fallbackModels: [] },
      ]),
    ).toEqual([{ primary: "gpt-4o", targets: "claude" }]);
  });

  it("stops a chain at ten models", () => {
    const many = Array.from({ length: 12 }, (_, index) => `m${index}`).join(", ");
    const [group] = groupsFromChains([{ primary: "gpt-4o", targets: many }]);
    expect(group.fallbackModels).toHaveLength(10);
  });
});
