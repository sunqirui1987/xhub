import { beforeEach, vi } from "vitest";
import "./remapLiteLLMProxyReads";
import { setActiveLocale } from "@/i18n/runtime";

beforeEach(() => {
  setActiveLocale("en");
});

vi.mock("@/lib/toast", () => ({
  toast: {
    success: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
    fromError: vi.fn(),
    dismiss: vi.fn(),
  },
}));
