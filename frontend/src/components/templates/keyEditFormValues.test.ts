import { describe, expect, it } from "vitest";
import { keyEditFormSchema } from "./keyEditFormValues";

const parse = (values: Record<string, unknown>) => keyEditFormSchema.safeParse(values);

describe("keyEditFormSchema", () => {
  it("accepts an empty form", () => {
    expect(parse({}).success).toBe(true);
  });

  it("rejects a fractional estimated output tokens value", () => {
    expect(parse({ default_estimated_output_tokens: "12.5" }).success).toBe(false);
  });

  it("rejects a zero or negative estimated output tokens value", () => {
    expect(parse({ default_estimated_output_tokens: "-5" }).success).toBe(false);
    expect(parse({ default_estimated_output_tokens: 0 }).success).toBe(false);
  });

  it("accepts a blank or absent estimated output tokens value", () => {
    expect(parse({ default_estimated_output_tokens: "" }).success).toBe(true);
    expect(parse({ default_estimated_output_tokens: null }).success).toBe(true);
  });

  it("rejects a per-model estimate that is not a JSON object of positive integers", () => {
    expect(parse({ default_estimated_output_tokens_per_model: "not json" }).success).toBe(false);
    expect(parse({ default_estimated_output_tokens_per_model: '{"gpt-4": 0}' }).success).toBe(false);
  });

  it("accepts a per-model estimate that is a JSON object of positive integers", () => {
    expect(parse({ default_estimated_output_tokens_per_model: '{"gpt-4": 4096}' }).success).toBe(true);
  });
});

/** 密钥编辑 RPM/TPM 的正常、空白、零和失败边界；纯 schema 无持久化，测试结束无需清理。 */
describe("key rate allocation boundaries", () => {
  it.each([undefined, null, "", 0, "0", 12, "2147483647"])("accepts %s", (value) => {
    expect(parse({ rpm_limit: value, tpm_limit: value }).success).toBe(true);
  });
  it.each([-1, "1.5", 2147483648, "wrong", true, {}, NaN, Infinity])("rejects %s", (value) => {
    expect(parse({ rpm_limit: value }).success).toBe(false);
    expect(parse({ tpm_limit: value }).success).toBe(false);
  });
});
