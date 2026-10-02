import { describe, expect, it } from "vitest";
import { translate } from "@/i18n/translate";
import { enPhrases } from "@/i18n/messages/phrases.en";

describe("health check endpoint translations", () => {
  it("preserves endpoint paths including case in Chinese labels", () => {
    const labels = Object.keys(enPhrases).filter((key) => /^.+ - \/[a-z]/.test(key));
    expect(labels.length).toBeGreaterThanOrEqual(12);
    for (const label of labels) {
      expect(translate("zh-CN", label).split(" - ")[1]).toBe(label.split(" - ")[1]);
    }
  });
});
