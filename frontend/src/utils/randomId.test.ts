import { afterEach, describe, expect, it } from "vitest";
import { randomId } from "./randomId";

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

describe("randomId", () => {
  const original = globalThis.crypto;

  afterEach(() => {
    Object.defineProperty(globalThis, "crypto", { value: original, configurable: true });
  });

  it("returns a uuid when randomUUID is missing", () => {
    Object.defineProperty(globalThis, "crypto", {
      value: { getRandomValues: (bytes: Uint8Array) => bytes.fill(1) },
      configurable: true,
    });
    expect(randomId()).toMatch(uuid);
  });

  it("returns a uuid when crypto is unavailable", () => {
    Object.defineProperty(globalThis, "crypto", { value: undefined, configurable: true });
    expect(randomId()).toMatch(uuid);
  });
});
