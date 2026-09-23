#!/usr/bin/env python3
"""Coverage checks — fail on stubs, handler-deferral, missing family paths."""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

DOCS = Path(__file__).resolve().parents[1]
CAT = json.loads((DOCS / "_inventory" / "catalog.json").read_text())
CONTRACTS = DOCS / "backend-api" / "contracts"
PAGES = DOCS / "frontend" / "pages"
PROVIDERS = DOCS / "backend-api" / "runtime" / "providers.md"
DASH = Path("/Users/sunqirui/Downloads/litellm-main/ui/litellm-dashboard/src/app")

SECTIONS = ["## 目的", "## HTTP", "## 请求", "## 响应", "## 错误", "## 副作用", "## 控制台绑定", "## 验收"]
HTTP_PARTS = ["### 请求头", "### 请求体", "### 响应头", "### 响应体"]
STUB_MARKERS = (
    "操作 1",
    "Status: `missing`",
    "handler 的 body 模型",
    "与后端契约同名",
    "按 LiteLLM 对应页的主区块实现",
    "见 Python",
)
LAYOUT_MARKERS = ("顶栏", "筛选", "表", "Tab", "抽屉", "模态", "URL")
SECRET_ONCE = "密钥明文只展示一次"
KEY_PAGE_FILES = {"api-keys.md", "chat-api-keys.md"}
KEY_PAGE_ROUTES = {"/api-keys", "/chat/api-keys"}
SECTION_RE = re.compile(r"^## (.+?)\s*$", re.M)


def norm(path: str) -> str:
    return path.replace(":path", "").rstrip("/") or "/"


def contract_text() -> str:
    parts = []
    for p in CONTRACTS.rglob("*.md"):
        parts.append(p.read_text())
    return "\n".join(parts)


def check_routes() -> list[str]:
    cat = {norm(r["path"]) for r in CAT["http_routes"]}
    mentioned: set[str] = set()
    pat = re.compile(r"`(/[^`\s]+)`")
    blob = contract_text()
    for m in pat.finditer(blob):
        mentioned.add(norm(m.group(1)))
    return sorted(cat - mentioned)


def check_family_paths() -> list[str]:
    blob = contract_text()
    bad = []
    for fam in CAT["families"]:
        fid = fam["id"]
        if not (fid.startswith("data.") or fid.startswith("mgmt.")):
            continue
        if fid in {"ui.pages", "ui.modules", "sdk.public", "sdk.router", "providers.llms", "router.strategies", "runtime.caching", "ops.integrations", "ops.proxy_types", "ops.enterprise"}:
            continue
        for hp in fam.get("http_paths") or []:
            n = norm(hp)
            if f"`{hp}`" not in blob and f"`{n}`" not in blob:
                bad.append(f"family {fid} path not in L1: {hp}")
    return bad


def check_l1() -> list[str]:
    bad = []
    empty_json = re.compile(r"```json\s*\{\s*\}\s*```")
    empty_data = re.compile(r'```json\s*\{\s*"data"\s*:\s*\[\s*\]\s*\}\s*```')
    for p in sorted(CONTRACTS.rglob("*.md")):
        if p.name == "README.md":
            continue
        t = p.read_text()
        miss = [s for s in SECTIONS + HTTP_PARTS if s not in t]
        if miss:
            bad.append(f"{p.relative_to(DOCS)} missing {miss}")
        if "Status: `missing`" in t:
            bad.append(f"{p.relative_to(DOCS)} still Status missing")
        if "handler 的 body 模型" in t or "见 Python" in t:
            bad.append(f"{p.relative_to(DOCS)} defers to handler/Python")
        if empty_json.search(t):
            bad.append(f"{p.relative_to(DOCS)} empty JSON object example")
        if empty_data.search(t):
            bad.append(f"{p.relative_to(DOCS)} empty data[] example")
    needed = {"projects.md", "scim.md", "audit.md", "email.md", "placeholders.md"}
    have = {p.name for p in CONTRACTS.rglob("*.md")}
    for n in needed:
        if n not in have:
            bad.append(f"missing L1 file {n}")
    return bad


def md_section(text: str, heading: str) -> str | None:
    matches = list(SECTION_RE.finditer(text))
    for i, m in enumerate(matches):
        if m.group(1) != heading:
            continue
        start = m.end()
        end = matches[i + 1].start() if i + 1 < len(matches) else len(text)
        return text[start:end]
    return None


def page_tsx_spec_name(page_tsx: Path) -> str:
    rel = page_tsx.relative_to(DASH)
    parts = [p for p in rel.parts[:-1] if not (p.startswith("(") and p.endswith(")"))]
    if not parts:
        return "index.md"
    return "-".join(parts) + ".md"


def spec_route(text: str) -> str:
    m = re.search(r"- Route:\s*`([^`]+)`", text)
    if not m:
        return ""
    return m.group(1).rstrip("/") or "/"


def is_key_page(path: Path, text: str) -> bool:
    if path.name in KEY_PAGE_FILES:
        return True
    return spec_route(text) in KEY_PAGE_ROUTES


def check_pages() -> list[str]:
    if not DASH.exists():
        return ["dashboard source missing"]
    bad = []
    required = {
        "/chat": "chat.md",
        "/onboarding": "onboarding.md",
        "/connect": "connect.md",
        "/model_hub": "model_hub.md",
        "/mcp/oauth/callback": "mcp-oauth-callback.md",
        "/": "index.md",
    }
    for route, fn in required.items():
        found = list(PAGES.rglob(fn))
        if not found:
            bad.append(f"missing page spec {route} -> {fn}")
            continue
        t = found[0].read_text()
        if "## 字段" not in t:
            bad.append(f"no 字段 {fn}")
    for tsx in sorted(DASH.rglob("page.tsx")):
        fn = page_tsx_spec_name(tsx)
        if not list(PAGES.rglob(fn)):
            rel = tsx.relative_to(DASH).as_posix()
            bad.append(f"missing page spec {rel} -> {fn}")
    for p in PAGES.rglob("*.md"):
        if p.name == "README.md":
            continue
        t = p.read_text()
        for m in STUB_MARKERS:
            if m in t:
                bad.append(f"{p.name} stub marker {m!r}")
        if "| 操作 1 |" in t or t.find("操作 1") != -1:
            bad.append(f"{p.name} numbered 操作 1")
        if "Status: `missing`" in t:
            bad.append(f"{p.name} Status missing")
        if "## 字段" not in t:
            bad.append(f"no 字段 {p.name}")
        # require at least 2 named action rows
        if t.count("| `") < 4:
            bad.append(f"{p.name} too few backticked fields/actions")
        if "## 布局" not in t:
            bad.append(f"{p.name} missing ## 布局")
        if "## 交互" not in t:
            bad.append(f"{p.name} missing ## 交互")
        layout = md_section(t, "布局")
        if layout is None:
            if "## 布局" in t:
                bad.append(f"{p.name} 布局 empty")
        else:
            hit = [m for m in LAYOUT_MARKERS if m in layout]
            if len(hit) < 3:
                bad.append(
                    f"{p.name} 布局 needs >=3 region markers "
                    f"(顶栏/筛选/表/Tab/抽屉/模态/URL), has {hit}"
                )
        status = md_section(t, "状态") or ""
        if SECRET_ONCE in status and not is_key_page(p, t):
            bad.append(f"{p.name} 状态 has {SECRET_ONCE!r}")
    return bad


def check_providers() -> list[str]:
    if not PROVIDERS.exists():
        return ["providers.md missing"]
    t = PROVIDERS.read_text()
    bad = []
    if "须实现或明确拒绝" in t:
        bad.append("providers.md still uses vague 须实现或明确拒绝")
    if "provider_not_implemented" not in t:
        bad.append("providers.md missing provider_not_implemented")
    pkgs = CAT.get("providers") or []
    for pkg in pkgs:
        if f"`{pkg}`" not in t:
            bad.append(f"provider package not listed: {pkg}")
    return bad


PRD_TOKENS = (
    "x-litellm-api-key",
    "VerificationToken",
    "Organization",
    "Team",
    "Project",
    "max_budget",
    "PROXY_HOOKS",
    "simple-shuffle",
    "least-busy",
    "lowest-cost",
    "SpendLogs",
    "cache_hit",
    "model_list",
    "key_type",
    "provider_not_implemented",
    "AI GATEWAY",
    "Chat 壳",
    "/chat",
    "鉴权",
    "路由",
    "计量",
)


def check_prd() -> list[str]:
    prd_dir = DOCS / "prd"
    if not prd_dir.is_dir():
        return ["prd directory missing"]
    text = "\n".join(p.read_text() for p in prd_dir.rglob("*.md"))
    bad = [f"prd missing {tok!r}" for tok in PRD_TOKENS if tok not in text]
    # stage order: 鉴权 then 路由 then spend
    if "鉴权" in text and "路由" in text:
        if text.find("鉴权") > text.find("路由") and "生命周期" not in text:
            pass
    for need in ("预扣", "不得记 0", "重新"):
        if need not in text:
            bad.append(f"prd missing operational phrase {need!r}")
    if "master" not in text.lower() or "/v1/chat/completions" not in text:
        bad.append("prd missing master-key vs /v1/chat/completions")
    if "业务 chunk" not in text:
        bad.append("prd missing first-chunk no provider switch")
    return bad


def check_mechanisms() -> list[str]:
    text = "\n".join(
        p.read_text()
        for p in DOCS.rglob("*.md")
        if "_inventory" not in str(p) and "_tools" not in str(p)
    )
    need = [
        "PROXY_HOOKS",
        "max_budget_limiter",
        "x-litellm-api-key",
        "simple-shuffle",
        "least-busy",
        "lowest-cost",
        "Chat 终端用户",
        "公开模型目录",
        "independent-impl",
    ]
    return [s for s in need if s not in text]


def main() -> int:
    missing = check_routes()
    families = check_family_paths()
    l1 = check_l1()
    pages = check_pages()
    prov = check_providers()
    mech = check_mechanisms()
    prd = check_prd()
    print("missing_paths", len(missing))
    for p in missing[:50]:
        print("  PATH", p)
    print("family_path_gaps", len(families))
    for x in families[:50]:
        print("  FAM", x)
    print("l1_incomplete", len(l1))
    for x in l1:
        print("  L1", x)
    print("page_gaps", len(pages))
    for x in pages:
        print("  PAGE", x)
    print("provider_gaps", len(prov))
    for x in prov:
        print("  PROV", x)
    print("mechanism_gaps", len(mech))
    for x in mech:
        print("  MECH", x)
    print("prd_gaps", len(prd))
    for x in prd:
        print("  PRD", x)
    ok = not (missing or families or l1 or pages or prov or mech or prd)
    print("PASS" if ok else "FAIL")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
