import { copy, groups, integration, quickstart, repository } from "./content.mjs";
import { documentPath, escapeHTML as e, rootPrefix } from "./core.mjs";

/** 生成导航；参数为语言、资源前缀和切换地址，返回 HTML，供首页与文档布局复用。 */
export function header(lang, prefix, alternate) {
  const t = copy[lang];
  const home = `${prefix}${lang === "en" ? "en/" : ""}index.html`;
  return `<header class="header"><div class="nav-wrap"><a class="brand" href="${home}" aria-label="XHub ${lang === "zh" ? "首页" : "home"}"><span class="brand-mark" aria-hidden="true">X</span>XHub</a><button class="menu-button" aria-label="${t.menu}" aria-expanded="false" aria-controls="main-navigation">☰</button><nav id="main-navigation" aria-label="${lang === "zh" ? "主导航" : "Main navigation"}"><a href="${home}#capabilities">${t.features}</a><a href="${home}#use-cases">${t.useCases}</a><a href="${prefix}${documentPath(lang === "zh" ? "README.zh-CN.md" : "README.md")}">${t.docs}</a><button class="search-open">${t.search}<kbd>/</kbd></button></nav><div class="nav-actions"><a class="language-link" href="${alternate}" aria-label="${lang === "zh" ? "Switch to English" : "切换到简体中文"}">${t.switch}</a><a class="nav-github" href="${repository}">GitHub <span aria-hidden="true">↗</span></a></div></div></header>`;
}

/** 生成代码块与复制按钮；参数为代码、标签和语言，返回已转义 HTML，供首页调用。 */
export function codeBlock(code, label, lang) {
  return `<div class="code-panel"><div class="code-label"><span>${label}</span><button class="copy-code">${copy[lang].copy}</button></div><pre><code>${e(code)}</code></pre></div>`;
}

/** 生成完整 HTML 外壳；参数含页面正文、标题、语言及输出路径，返回字符串，构建器负责落盘。 */
export function shell({ lang, output, title, description, body, alternate, docs = false }) {
  const prefix = rootPrefix(output);
  const t = copy[lang];
  return `<!doctype html><html lang="${lang === "zh" ? "zh-CN" : "en"}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${e(title)} · XHub</title><meta name="description" content="${e(description)}"><meta name="theme-color" content="#146b50"><meta property="og:title" content="${e(title)} · XHub"><meta property="og:description" content="${e(description)}"><meta property="og:type" content="website"><link rel="icon" href="${prefix}assets/favicon.svg" type="image/svg+xml"><link rel="stylesheet" href="${prefix}assets/style.css"></head><body data-root="${prefix}" data-language="${lang}" class="${docs ? "docs-page" : "home-page"}"><a class="skip-link" href="#main">${lang === "zh" ? "跳到正文" : "Skip to content"}</a>${header(lang, prefix, alternate)}${body}<footer class="footer"><div><a class="brand" href="${prefix}${lang === "en" ? "en/" : ""}index.html"><span class="brand-mark" aria-hidden="true">X</span>XHub</a><p>${t.footer}</p></div><div><a href="${prefix}${documentPath(lang === "zh" ? "getting-started.zh-CN.md" : "getting-started.md")}">${t.start}</a><a href="${prefix}${documentPath("development/runtime.md")}">${t.status}</a><a href="${repository}">GitHub ↗</a><p class="footer-note">${t.statusText}</p></div></footer><dialog class="search-dialog" aria-label="${t.search}"><form method="dialog" class="search-heading"><label for="doc-search">${t.search}</label><button aria-label="${t.close}">✕</button></form><input id="doc-search" type="search" placeholder="${t.searchHint}" autocomplete="off"><p class="search-status" aria-live="polite">${t.searchHint}</p><div class="search-results"></div></dialog><div class="toast" role="status" aria-live="polite"></div><script type="module" src="${prefix}assets/app.js"></script></body></html>`;
}

/** 生成营销首页；参数为 zh/en，返回 HTML，不读取源文件，链接使用相对地址适配 GitHub Pages。 */
export function homepage(lang) {
  const t = copy[lang];
  const output = lang === "zh" ? "index.html" : "en/index.html";
  const prefix = rootPrefix(output);
  const doc = (source) => `${prefix}${documentPath(source)}`;
  const start = doc(lang === "zh" ? "getting-started.zh-CN.md" : "getting-started.md");
  const body = `<main id="main">
    <section class="hero section"><div class="hero-copy"><p class="eyebrow"><span class="live-dot"></span>${t.tag}</p><h1>${t.hero}<br><span>${t.heroAccent}</span></h1><p class="hero-description">${t.intro}</p><div class="hero-buttons"><a class="button primary" href="${start}">${t.start} <span aria-hidden="true">→</span></a><a class="button secondary" href="${repository}">${t.github} ↗</a></div><p class="hero-note">${t.heroNote}</p></div>
    <div class="gateway-visual" aria-label="${lang === "zh" ? "应用经过 XHub 治理后连接供应商" : "Applications connect to providers through XHub governance"}"><div class="visual-top"><span class="mono">YOUR AI INFRASTRUCTURE</span><span class="visual-status">● ${lang === "zh" ? "统一管理" : "Unified control"}</span></div><div class="app-nodes"><span>Business apps</span><span>AI agents</span><span>Internal tools</span></div><div class="connection-line"></div><div class="gateway-node"><span class="brand-mark">X</span><strong>XHub</strong><span>Enterprise AI Gateway</span><div class="policy-tags"><span>Access</span><span>Budget</span><span>Guardrails</span><span>Routing</span></div></div><div class="connection-line"></div><div class="provider-nodes"><span>OpenAI</span><span>Anthropic</span><span>Gemini</span><span>${lang === "zh" ? "更多供应商" : "More providers"}</span></div><div class="visual-bottom"><span>PostgreSQL</span><span>Redis</span><span>Self-hosted</span></div></div></section>
    <div class="capability-strip">${t.strip.map((item) => `<span><span class="check">✓</span>${item}</span>`).join("")}</div>
    <section id="capabilities" class="section"><div class="section-heading"><p class="eyebrow">${t.eyebrow}</p><h2>${t.featureTitle}</h2><p>${t.featureIntro}</p></div><div class="capability-grid">${t.capabilities.map(([number, title, text, source, link]) => `<article class="capability-card"><span class="card-number">${number}</span><h3>${title}</h3><p>${text}</p><a href="${doc(`development/${source}`)}">${link} <span aria-hidden="true">↗</span></a></article>`).join("")}</div></section>
    <section class="difference-section"><div class="section"><p class="eyebrow">${t.differenceLabel}</p><h2>${t.differenceTitle}</h2><div class="difference-grid">${t.differences.map(([title, text, source, link]) => `<article><span class="difference-icon" aria-hidden="true">↗</span><h3>${title}</h3><p>${text}</p><a href="${doc(`development/${source}`)}">${link} →</a></article>`).join("")}</div></div></section>
    <section class="section console-section"><div class="section-heading"><p class="eyebrow">${t.consoleLabel}</p><h2>${t.consoleTitle}</h2><p>${t.consoleIntro}</p></div><div class="screenshot-frame"><div class="window-bar"><span></span><span></span><span></span><small>XHub / Model catalog</small></div><img src="${prefix}assets/console-models.png" alt="${t.screenshot}" loading="lazy" width="1440" height="900"></div><p class="caption">${t.screenshotNote}</p></section>
    <section class="section flow-section"><p class="eyebrow">${t.flowLabel}</p><h2>${t.flowTitle}</h2><div class="flow-grid">${t.flow.map(([title, text], index) => `<article><span class="step">0${index + 1}</span><h3>${title}</h3><p>${text}</p></article>`).join("")}</div></section>
    <section id="use-cases" class="section"><div class="section-heading"><h2>${t.casesTitle}</h2></div><div class="use-case-grid">${t.cases.map(([title, text]) => `<article><h3>${title}</h3><p>${text}</p></article>`).join("")}</div></section>
    <section id="install" class="install-section"><div class="section install-grid"><div><p class="eyebrow">${t.installLabel}</p><h2>${t.installTitle}</h2><p>${t.installIntro}</p><p class="requirements">${t.requirements}</p><a class="text-link" href="${start}">${t.installDocs} →</a><div class="ports"><span>${t.console} <code>:3000</code></span><span>${t.gateway} <code>:4000</code></span></div></div><div>${codeBlock(quickstart, "Docker Compose", lang)}${codeBlock(integration, "Python · OpenAI SDK", lang)}</div></div></section>
    <section class="section documentation-section"><p class="eyebrow">${t.docsLabel}</p><h2>${t.docsTitle}</h2><p>${t.docsIntro}</p><div class="docs-card-grid">${t.docCards.map(([title, text, source]) => `<a class="doc-card" href="${doc(source)}"><h3>${title}<span aria-hidden="true">↗</span></h3><p>${text}</p></a>`).join("")}</div><a class="button secondary" href="${doc(lang === "zh" ? "README.zh-CN.md" : "README.md")}">${t.allDocs} →</a></section></main>`;
  return shell({ lang, output, title: lang === "zh" ? "企业 AI 网关与治理平台" : "Enterprise AI gateway & governance", description: t.intro, body, alternate: lang === "zh" ? "./en/index.html" : "../index.html" });
}

/** 生成文档页面；参数包含已消毒正文、标题目录与文档清单，返回 HTML；不重新解析 Markdown。 */
export function documentPage({ source, title, html, headings, documents, lang, alternate }) {
  const output = documentPath(source);
  const prefix = rootPrefix(output);
  const t = copy[lang];
  const known = new Map(documents.map((item) => [item.source, item]));
  const used = new Set();
  const sidebar = groups.map((group) => {
    const files = lang === "en" && group.english ? group.english : group.files;
    return `<section class="nav-group"><h2>${group[lang]}</h2>${files.filter((file) => known.has(file)).map((file) => { used.add(file); return `<a href="${prefix}${documentPath(file)}" ${file === source ? 'aria-current="page"' : ""}>${e(known.get(file).title)}</a>`; }).join("")}</section>`;
  }).join("");
  const remaining = documents.filter((item) => !used.has(item.source) && !(lang === "zh" ? /^(README|getting-started|user-guide)\.md$/.test(item.source) : /zh-CN/.test(item.source)));
  const refs = remaining.map((item) => `<a href="${prefix}${documentPath(item.source)}" ${item.source === source ? 'aria-current="page"' : ""}>${e(item.title)}<small>${e(item.source.replace(/\/readme(?:_cn)?\.md$/i, ""))}</small></a>`).join("");
  const body = `<div class="docs-layout"><aside class="docs-sidebar" aria-label="${t.documentHome}"><button class="search-open sidebar-search">${t.search}<kbd>/</kbd></button>${sidebar}<details class="nav-group additional-docs" ${used.has(source) ? "" : "open"}><summary>${t.moduleDocs}</summary>${refs}</details></aside><main id="main" class="doc-main"><div class="doc-meta"><a href="${prefix}${documentPath(lang === "zh" ? "README.zh-CN.md" : "README.md")}">${t.documentHome}</a><span>/</span><span>${e(title)}</span></div>${lang === "zh" && /^development\//.test(source) ? `<p class="reference-note">中文参考文档 · <a href="${prefix}en/index.html">English overview</a></p>` : ""}<article class="prose">${html}</article><div class="doc-end"><a href="${repository}/blob/main/${source.startsWith("modules/") ? source.slice(8) : `docs/${source}`}">${t.source} ↗</a><a href="#main">↑ ${t.toc}</a></div></main><aside class="toc" aria-label="${t.toc}"><strong>${t.toc}</strong>${headings.filter((item) => item.depth === 2 || item.depth === 3).map((item) => `<a class="depth-${item.depth}" href="#${e(item.id)}">${e(item.text)}</a>`).join("")}</aside></div>`;
  return shell({ lang, output, title, description: `${title} — XHub ${t.docs}`, body, alternate: `${prefix}${documentPath(alternate)}`, docs: true });
}
