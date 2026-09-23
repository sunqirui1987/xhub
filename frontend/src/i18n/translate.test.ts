import fs from "fs";
import path from "path";
import { describe, expect, it } from "vitest";
import { findHitsInSource, findUserVisibleEnglish, isEnglishProse } from "./englishProseScan";
import { catalogs, collectKeys, interpolate, parseLocale, translate } from "./translate";

const SRC = path.resolve(__dirname, "..");

function productionFiles(dir: string, out: string[] = []): string[] {
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    if (ent.name.startsWith(".") || ent.name === "node_modules") continue;
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) {
      if (ent.name === "messages" && path.basename(dir) === "i18n") continue;
      productionFiles(p, out);
    } else if (/\.tsx?$/.test(ent.name) && !/\.(test|spec|cases)\./.test(ent.name) && !ent.name.endsWith(".d.ts")) {
      out.push(p);
    }
  }
  return out;
}

function readJsString(text: string, start: number): { value: string; end: number } | null {
  const quote = text[start];
  if (quote !== '"' && quote !== "'" && quote !== "`") return null;
  let value = "";
  for (let i = start + 1; i < text.length; i++) {
    const ch = text[i];
    if (ch === "\\") {
      const next = text[i + 1] ?? "";
      value += next === "n" ? "\n" : next === "t" ? "\t" : next;
      i += 1;
      continue;
    }
    if (ch === quote) return { value, end: i + 1 };
    if (quote === "`" && ch === "$") return null;
    value += ch;
  }
  return null;
}

function keysUsedInProduction(): string[] {
  const found = new Set<string>();
  const call = /\b(?:t|tRuntime|tDefault|useT)\(\s*/g;
  const translated = /\btranslate\(\s*/g;
  for (const file of productionFiles(SRC)) {
    const text = fs.readFileSync(file, "utf8");
    for (const re of [call, translated]) {
      re.lastIndex = 0;
      let m: RegExpExecArray | null;
      while ((m = re.exec(text))) {
        let cursor = m.index + m[0].length;
        if (re === translated) {
          const locale = readJsString(text, cursor);
          if (!locale) continue;
          cursor = locale.end;
          const comma = text.slice(cursor).match(/^\s*,\s*/);
          if (!comma) continue;
          cursor += comma[0].length;
        }
        const key = readJsString(text, cursor);
        if (key && !key.value.includes("${")) found.add(key.value);
      }
    }
  }
  return [...found];
}

describe("translate", () => {
  it("returns Simplified Chinese for the default locale", () => {
    expect(translate("zh-CN", "login.title")).toBe("登录");
    expect(translate("zh-CN", "nav.apiKeys")).toBe("虚拟密钥");
    expect(translate("zh-CN", "nav.groups.gateway")).toBe("AI 网关");
  });

  it("returns English when locale is en", () => {
    expect(translate("en", "login.title")).toBe("Login");
    expect(translate("en", "nav.apiKeys")).toBe("Virtual Keys");
    expect(translate("en", "nav.groups.gateway")).toBe("AI GATEWAY");
    expect(translate("en", "pages.apiKeys.create")).toBe("Create New Key");
  });

  it("interpolates named placeholders", () => {
    expect(translate("zh-CN", "connect.allowUse", { client: "app", server: "mcp" })).toBe("允许 app 使用 mcp");
    expect(translate("en", "connect.allowUse", { client: "app", server: "mcp" })).toBe("Allow app to use mcp");
  });

  it("falls back to the key when missing", () => {
    expect(translate("zh-CN", "does.not.exist")).toBe("does.not.exist");
  });
});

describe("interpolate", () => {
  it("leaves unknown placeholders intact", () => {
    expect(interpolate("Hello {name}", {})).toBe("Hello {name}");
  });
});

describe("catalogs", () => {
  it("keeps the same non-empty sentence keys in zh-CN and en", () => {
    const zhKeys = collectKeys(catalogs["zh-CN"]).sort();
    const enKeys = collectKeys(catalogs.en).sort();
    expect(zhKeys).toEqual(enKeys);
    expect(zhKeys.length).toBeGreaterThan(20);
    for (const key of zhKeys) {
      const zh = translate("zh-CN", key);
      const en = translate("en", key);
      expect(zh.length, key).toBeGreaterThan(0);
      expect(en.length, key).toBeGreaterThan(0);
      expect(zh === key && en === key, key).toBe(false);
    }
    expect(translate("zh-CN", "login.title")).not.toBe(translate("en", "login.title"));
    expect(translate("zh-CN", "pages.accessGroups.deleteTitle")).not.toBe(
      translate("en", "pages.accessGroups.deleteTitle"),
    );
  });
});

describe("production call sites", () => {
  it("resolves every catalog key used by the UI in both locales", () => {
    const used = keysUsedInProduction();
    expect(used.length).toBeGreaterThan(100);
    const unresolved = used.filter((key) => {
      const zh = translate("zh-CN", key);
      const en = translate("en", key);
      return zh.length === 0 || en.length === 0 || (zh === key && en === key);
    });
    expect(unresolved).toEqual([]);
    expect(translate("zh-CN", "pages.teams.title")).not.toBe(translate("en", "pages.teams.title"));
    expect(translate("en", "Edit Access Group")).toBe("Edit Access Group");
    expect(translate("zh-CN", "Edit Access Group")).not.toBe("Edit Access Group");
    expect(translate("zh-CN", "Edit Access Group")).toMatch(/[\u4e00-\u9fff]/);
    expect(translate("en", "LiteLLM Slack community")).toBe("XHub Slack community");
    expect(translate("zh-CN", "LiteLLM Slack community")).toBe("XHub Slack 社区");
    expect(translate("en", "Access group updated successfully")).toBe("Access group updated successfully");
    expect(translate("zh-CN", "Access group updated successfully")).not.toBe(translate("en", "Access group updated successfully"));
    expect(translate("zh-CN", "Access group updated successfully")).toMatch(/[\u4e00-\u9fff]/);
  });
});

describe("leftover user-visible English", () => {
  it("does not treat Select…from UI sentences as code", () => {
    expect(isEnglishProse("Select guardrails to remove (from inherited)")).toBe(true);
    expect(isEnglishProse("Auto-Rotation")).toBe(true);
    expect(isEnglishProse("all-proxy-models")).toBe(false);
    expect(isEnglishProse("least-busy")).toBe(false);
  });

  it("flags default placeholders, single-word buttons, and setError messages", () => {
    const source = `
      export function Widget({ placeholder = "Select a Model", labelText = "Select Model" }) {
        const message = "Please complete server URL and transport before starting OAuth.";
        setError("Missing admin token");
        setError(message);
        toast.error(message);
        return (
          <div>
            <button>Clear</button>
            <button>Save</button>
            <button>Delete</button>
            <button>Cancel</button>
            <MultiSelect placeholder="Select options" emptyText="No options found" />
            <input placeholder={loading ? "Loading..." : placeholder} />
            <Picker clearAllLabel="Clear all teams" placeholder="Search teams by alias..." />
            <Field placeholder="Select guardrails to remove (from inherited)" />
            <dt>Description</dt>
            <a>here</a>
          </div>
        );
      }
    `;
    const hits = findHitsInSource("widget.tsx", source).map((hit) => hit.template);
    for (const expected of [
      "Select a Model",
      "Select Model",
      "Please complete server URL and transport before starting OAuth.",
      "Missing admin token",
      "Clear",
      "Save",
      "Delete",
      "Cancel",
      "Select options",
      "No options found",
      "Loading...",
      "Clear all teams",
      "Search teams by alias...",
      "Select guardrails to remove (from inherited)",
      "Description",
      "here",
    ]) {
      expect(hits, expected).toContain(expected);
    }
  });

  it("flags export toasts, handler toasts, fallbacks, and Select placeholders", () => {
    expect(isEnglishProse("Failed to export to CloudZero")).toBe(true);
    expect(isEnglishProse("CSV export functionality coming soon!")).toBe(true);
    const source = `
      function Panel() {
        return <button onClick={() => toast.success("UI Access Control settings updated successfully")}>x</button>;
      }
      function save(error: { message?: string }) {
        toast.success("Copied to clipboard!");
        toast.success("Code Interpreter enabled!");
        toast.error(error.message || "Failed to update CloudZero integration");
        toast.error("Error creating search tool: " + error);
        toast.error("Error testing connection: " + String(error));
        toast.fromError("Failed to export to CloudZero");
        toast.info("CSV export functionality coming soon!");
        throw new Error(error.message || "Unknown error");
      }
      function SearchSelect({ placeholder = "Select…" }) {
        return <input placeholder={placeholder} />;
      }
    `;
    const hits = findHitsInSource("panel.tsx", source).map((hit) => hit.template);
    for (const expected of [
      "UI Access Control settings updated successfully",
      "Copied to clipboard!",
      "Code Interpreter enabled!",
      "Failed to update CloudZero integration",
      "Error creating search tool:",
      "Error testing connection:",
      "Failed to export to CloudZero",
      "CSV export functionality coming soon!",
      "Unknown error",
      "Select…",
    ]) {
      expect(hits, expected).toContain(expected);
    }
  });

  it("finds no English prose in production JSX, labels, toasts, or validation messages", () => {
    const hits = findUserVisibleEnglish(SRC);
    expect(hits, hits.slice(0, 20).map((hit) => `${hit.file}:${hit.line} ${hit.template}`).join("\n")).toEqual([]);
  });
});

describe("parseLocale", () => {
  it("accepts en and zh-CN and otherwise defaults to zh-CN", () => {
    expect(parseLocale("en")).toBe("en");
    expect(parseLocale("zh-CN")).toBe("zh-CN");
    expect(parseLocale("fr")).toBe("zh-CN");
    expect(parseLocale(undefined)).toBe("zh-CN");
  });
});
