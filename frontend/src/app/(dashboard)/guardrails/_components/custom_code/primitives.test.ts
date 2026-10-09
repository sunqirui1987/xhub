import { describe, expect, it } from "vitest";
import { parseTestInput } from "./primitives";
describe("XGo tester input", () => {
  it("accepts empty text arrays and optional model/metadata", () => {
    expect(parseTestInput('{"texts":[]}')).toEqual({ texts: [] });
    expect(parseTestInput('{"texts":["", "hello"],"model":"demo","metadata":{"tier":"test"}}')).toEqual({
      texts: ["", "hello"],
      model: "demo",
      metadata: { tier: "test" },
    });
  });
  it.each([
    "null",
    "[]",
    "1",
    '{"texts":null}',
    '{"texts":[1]}',
    '{"texts":[],"model":1}',
    '{"texts":[],"metadata":null}',
    '{"texts":[],"metadata":[]}',
    '{"texts":[],"images":[]}',
  ])("rejects unsupported input: %s", (input) => {
    expect(() => parseTestInput(input)).toThrow();
  });
});
