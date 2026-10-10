import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { translate } from "@/i18n/translate";
import { QuotaGuide } from "./QuotaGuide";

let locale: "en" | "zh-CN" = "en";
vi.mock("@/i18n", async () => {
  const original = await vi.importActual<typeof import("@/i18n")>("@/i18n");
  return { ...original, useT: () => (key: string) => translate(locale, key) };
});
afterEach(cleanup);

/** 验证各额度层帮助在中英文完整渲染；前置真实字典，断言层级、共享和唯一团队，结束清理 DOM。 */
describe("QuotaGuide", () => {
  for (const language of ["en", "zh-CN"] as const) {
    for (const scope of ["organization", "team", "person", "key"] as const) {
      it(language + " " + scope, () => {
        locale = language;
        render(<QuotaGuide scope={scope} />);
        expect(screen.getByRole("complementary", { name: translate(locale, "quotaGuide.title") })).toBeVisible();
        for (const key of ["chain", scope, "shared", "singleTeam", "business", "rates"]) {
          expect(screen.getByText(translate(locale, "quotaGuide." + key))).toBeVisible();
        }
      });
    }
  }
});
