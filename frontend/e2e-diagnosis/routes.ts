import fs from "fs";
import path from "path";

export interface DiscoveredPage {
  route: string;
  removed: boolean;
}

/** Every App Router page, including route groups that do not appear in the URL. */
export function discoverPages(appDir = path.resolve(__dirname, "../src/app")): DiscoveredPage[] {
  const pages: DiscoveredPage[] = [];
  const walk = (dir: string) => {
    for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, ent.name);
      if (ent.isDirectory()) {
        walk(full);
        continue;
      }
      if (ent.name !== "page.tsx") continue;
      const rel = path.relative(appDir, dir);
      const route =
        "/" +
        rel
          .split(path.sep)
          .filter((seg) => seg && !(seg.startsWith("(") && seg.endsWith(")")))
          .join("/");
      const source = fs.readFileSync(full, "utf8");
      pages.push({
        route: route === "/" ? "/" : route.replace(/\/$/, ""),
        removed: source.includes("notFound("),
      });
    }
  };
  walk(appDir);
  pages.sort((a, b) => a.route.localeCompare(b.route));
  return pages;
}
