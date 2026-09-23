/** Hide upstream LiteLLM / Berri branding from anything the console renders. */

export function scrubBrand(text: string): string {
  let out = text.replace(/litellm/gi, (match) => {
    if (match === "LITELLM") return "XHUB";
    if (match === "litellm") return "xhub";
    return "XHub";
  });
  out = out.replace(/[\w.+-]+@berri\.ai/gi, "");
  out = out.replace(/\bberri\.ai\b/gi, "");
  out = out.replace(/\ba XHub\b/g, "an XHub");
  out = out.replace(/[ \t]{2,}/g, " ");
  out = out.replace(/ +([。.,])/g, "$1");
  return out;
}

export function isBrandOutbound(href: string): boolean {
  try {
    const url = new URL(href, "https://placeholder.local");
    const host = url.hostname.toLowerCase();
    if (host === "litellm.ai" || host.endsWith(".litellm.ai")) return true;
    if (host === "berri.ai" || host.endsWith(".berri.ai")) return true;
    if (host === "github.com" && url.pathname.toLowerCase().startsWith("/berriai/")) return true;
  } catch {
    return false;
  }
  return false;
}
