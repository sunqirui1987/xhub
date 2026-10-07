import { describe, expect, it } from "vitest";
import { BILLING_CATEGORIES, categoryOf, rateFieldsOf } from "./billing_categories";

/**
 * 单价表单按计费维度分类：按 token（含空闲/高峰/缓存）、按张、按秒、按次。
 *
 * 这些断言盯住的是一件具体的事：一条模型只会被**一种**方式计费，表单不该
 * 把所有格子一股脑show出来让运维猜该填哪个。分类错了，运维要么填了不读的
 * 格子（以为定过价，实际记零），要么看不到该填的格子。
 */
describe("单价表单的计费分类", () => {
  it("对话模型按 token 计费，带空闲和高峰两档", () => {
    const fields = rateFieldsOf("chat");
    expect(fields).toContain("input_cost_per_token");
    expect(fields).toContain("output_cost_per_token");
    // DeepSeek 那类模型白天和夜里的价差一倍，两个格子都要在。
    expect(fields).toContain("input_cost_per_token_peak");
    expect(fields).toContain("output_cost_per_token_peak");
    // 缓存读写的价通常和输入价差很远，漏了它一次带缓存的调用会按输入价扣。
    expect(fields).toContain("cache_read_input_token_cost");
    expect(fields).toContain("cache_creation_input_token_cost");
    // 对话模型不该出现按张/按秒的格子。
    expect(fields).not.toContain("output_cost_per_image");
    expect(fields).not.toContain("output_cost_per_second");
  });

  it("图像模型按张计费，同时保留 token 档", () => {
    const fields = rateFieldsOf("image");
    expect(fields).toContain("input_cost_per_image");
    expect(fields).toContain("output_cost_per_image");
    // gpt-image 那类模型输入图和输出图是两个价，两个都要在。
    expect(fields).not.toContain("output_cost_per_second");
  });

  it("视频模型按秒计费", () => {
    const fields = rateFieldsOf("video");
    expect(fields).toContain("output_cost_per_second");
    expect(fields).not.toContain("output_cost_per_image");
  });

  it("语音合成也按秒", () => {
    expect(rateFieldsOf("audio_speech")).toContain("output_cost_per_second");
  });

  it("联网搜索按次计费，每一类都有", () => {
    for (const capability of ["chat", "image", "video", "audio_speech"]) {
      expect(rateFieldsOf(capability)).toContain("search_context_cost_per_query");
    }
  });

  it("认不出的调用方式退回按 token 计费", () => {
    // 绝大多数模型就是对话模型，也是网关默认读的那组字段。
    const fields = rateFieldsOf("something-new");
    expect(fields).toContain("input_cost_per_token");
    expect(fields).toContain("output_cost_per_token");
    expect(fields).not.toContain("output_cost_per_image");
  });

  it("每一类都非空，且不含重复字段", () => {
    for (const category of BILLING_CATEGORIES) {
      const fields = category.fields.map((f) => f.name);
      expect(fields.length, `category ${category.id} has no fields`).toBeGreaterThan(0);
      expect(new Set(fields).size, `category ${category.id} repeats a field`).toBe(fields.length);
    }
  });

  it("高峰那一档的名字都带 _peak 后缀", () => {
    // 计费路径按这个后缀区分时段。名字不对，高峰的价会写进空闲那一格。
    const peak = BILLING_CATEGORIES.filter((c) => c.window === "peak");
    expect(peak.map((c) => c.id).sort()).toEqual(["token_peak"]);
    for (const category of peak) {
      for (const field of category.fields) {
        expect(field.name).toMatch(/_peak$/);
      }
    }
  });
});

describe("categoryOf", () => {
  it("会话能力认成 token", () => {
    expect(categoryOf("chat")).toBe("token");
  });

  it("图像认成 image，视频和语音认成 second", () => {
    expect(categoryOf("image")).toBe("image");
    expect(categoryOf("video")).toBe("second");
    expect(categoryOf("audio_speech")).toBe("second");
  });
});
