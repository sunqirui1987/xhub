# XHub 官网与帮助文档：GitHub Pages 部署指南

将网站源码推送到 GitHub，在仓库设置中启用 **GitHub Actions** 作为 Pages 的发布来源，即可自动发布 XHub 官网与帮助文档。之后更新页面或文档并推送到 `main`，网站会自动重新构建和上线。

网站包括中英文产品首页、Docker 安装指南、企业管理与开发文档，支持全文搜索、文档分类、页内目录、复制代码和本地 Mermaid 架构图。构建后是纯静态文件，无需单独的网站服务器、数据库或模型供应商密钥。

> GitHub Pages 托管的是产品官网和公开帮助文档。XHub 网关、管理控制台与数据库的部署，请按 [XHub 安装与运行指南](../docs/getting-started.zh-CN.md)操作。Docker 安装 XHub 和发布官网是两个独立流程。

## 1. 发布前准备

- 一个 GitHub 仓库，以及修改仓库设置和推送代码的权限。
- 本地安装 Git、Node.js 22 或以上；推荐 Node.js 24，与自动发布环境一致。
- 仓库包含整个项目，尤其是 `website/`、`docs/`、模块 Markdown 和 [网站发布工作流](../.github/workflows/website.yml)。网站构建会读取这些文档和截图，不能只上传 `website/`。
- GitHub 仓库已启用 Actions。公开仓库可使用 GitHub Pages；私有仓库能否使用 Pages 取决于 GitHub 账户或组织套餐。

当前项目的发布分支是 `main`，仓库为 [sunqirui1987/xhub](https://github.com/sunqirui1987/xhub)。以下步骤按这个仓库说明。

## 2. 先在本地预览

在项目根目录执行：

```bash
cd website
npm ci
npm test
npm run build
npm run dev
```

打开 [本地官网](http://127.0.0.1:4321/)，检查首页、帮助文档和搜索。终端按 `Ctrl+C` 停止预览。修改内容后再次执行 `npm run build`，刷新浏览器查看结果。

构建结果在 `website/dist/`。该目录已被忽略，不需要提交到 Git，也不需要创建 `gh-pages` 分支；GitHub Actions 会负责构建和上传。

## 3. 在 GitHub 启用 Pages

1. 打开 [仓库设置](https://github.com/sunqirui1987/xhub/settings/pages)。
2. 在左侧选择 **Pages**。
3. 找到 **Build and deployment → Source**，选择 **GitHub Actions**。
4. 如果仓库的 Actions 被关闭，在 **Settings → Actions → General** 中启用工作流。组织仓库还需允许工作流使用 `actions/*` 官方操作。

工作流已为发布任务配置 `pages: write` 和 `id-token: write`，使用 GitHub 提供的令牌，不需要配置个人访问令牌或供应商 API Key。

## 4. 提交并推送网站源码

回到项目根目录，先确认仓库地址、分支和待提交文件：

```bash
git remote -v
git branch --show-current
git status --short
```

如果当前已经是 `main`，可以只提交本次官网和文档相关文件：

```bash
git add website .github/workflows/website.yml README.md README.zh-CN.md docs/user-guide.md docs/user-guide.zh-CN.md docs/development/model-marketplace.md
git diff --cached --stat
git commit -m "docs: add XHub website and GitHub Pages guide"
git push origin main
```

提交前检查暂存区，确认没有夹带其他开发工作。如果当前在功能分支，先推送该分支并通过 Pull Request 合并到 `main`。Pull Request 会运行网站检查，合并到 `main` 后才发布。

如果这是新建的 GitHub 仓库，先将完整 XHub 项目提交并推送到该仓库，再按本文配置 Pages。Fork 或更换仓库时，也请完成下文的地址调整。

## 5. 查看发布结果

打开仓库的 **Actions → XHub Website**，查看本次运行：

1. **build**：安装依赖，运行单元与 HTTP 服务测试，构建网站，安装测试浏览器，执行链接回归和真实浏览器验证。
2. **deploy**：上传并发布到 GitHub Pages。只有 `main` 的运行才执行发布。

两个任务都成功后，运行详情中的 **github-pages** 环境会显示实际访问地址。也可回到 **Settings → Pages** 查看 **Visit site**。当前仓库的预期地址如下：

| 内容 | 地址 |
| --- | --- |
| 中文官网 | <https://sunqirui1987.github.io/xhub/> |
| 英文官网 | <https://sunqirui1987.github.io/xhub/en/> |
| 中文文档入口 | <https://sunqirui1987.github.io/xhub/docs/README.zh-CN.html> |
| Docker 安装指南 | <https://sunqirui1987.github.io/xhub/docs/getting-started.zh-CN.html> |

以上是部署成功后的预期地址；本地构建成功不代表远端已经上线。首次发布或更新后，GitHub Pages 可能需要短暂时间传播新版本。

若代码已在 `main`，但尚未触发发布，在 **Actions → XHub Website → Run workflow** 中选择 `main` 并运行。工作流只对网站、文档、模块 Markdown 和自身配置的改动自动触发；仅修改根目录 README 时，可手动运行。

## 6. 更新官网和帮助文档

| 想修改的内容 | 源文件 |
| --- | --- |
| 产品定位、能力介绍、中英文文案、导航分类 | [src/content.mjs](src/content.mjs) |
| 首页布局、文档侧栏和页内目录 | [src/templates.mjs](src/templates.mjs) |
| 配色、排版、移动端样式 | [src/style.css](src/style.css) |
| 安装、用户和开发文档 | [项目 docs 目录](../docs/) |
| 模块说明 | `internal/`、`cmd/regression/` 中的正式 Markdown |
| 发布分支、构建和测试步骤 | [.github/workflows/website.yml](../.github/workflows/website.yml) |

修改 Markdown 后提交并推送到 `main`，网站页内目录和搜索索引会一起更新。文档里的相对 Markdown 链接会转换成网站页面链接，源码链接会指向 GitHub。

新增文档必须先纳入 Git 版本管理，构建器才会收录；本地验证新文档时，先执行 `git add docs/你的文档.md`，再构建。网站收录已跟踪的正式 Markdown，排除测试样本、代理指令和临时验证报告。新文档默认可搜索；若需要固定分类入口，再修改 `src/content.mjs` 的导航。

不要直接修改 `website/dist/`，下次构建会重新生成。

## 7. Fork、改名和自定义域名

普通项目仓库的默认地址为 `https://你的用户名.github.io/仓库名/`；名为 `你的用户名.github.io` 的站点仓库通常部署到域名根路径。网站资源使用相对路径，支持域名根路径和项目子路径。

Fork、迁移或改名后，更新以下地址，使客户能返回正确的项目：

- `website/src/content.mjs`、`website/src/templates.mjs`、`website/src/markdown.mjs` 中的 GitHub 仓库与源码链接。
- `website/src/build.mjs` 中 404 页的官网地址。
- 根目录中英文 README 以及本指南中的官网和仓库地址。

可以在项目根目录查找需要调整的引用：

```bash
rg -n 'sunqirui1987|sunqirui1987.github.io' website/src README.md README.zh-CN.md website/README.md
```

如果发布分支不是 `main`，同时修改工作流的 `push.branches` 与 `deploy.if` 分支条件，以及源码链接中的分支名。

绑定自定义域名时，在 **Settings → Pages → Custom domain** 填写域名，并按 GitHub 提示配置 DNS。子域名一般设置 CNAME 指向 `你的用户名.github.io`；域名根路径的记录请按 [GitHub 官方域名说明](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site)配置。DNS 生效后启用 **Enforce HTTPS**，再更新 README 和 404 页的官网地址。

## 8. 常见问题

| 现象 | 检查与处理 |
| --- | --- |
| 找不到 XHub Website 工作流 | 确认 `.github/workflows/website.yml` 已推送到仓库默认分支，且仓库启用了 Actions。 |
| build 成功，deploy 失败 | 确认 Pages 来源为 GitHub Actions，查看失败日志；若 github-pages 环境设置了审批或分支限制，在仓库环境设置中处理。 |
| 页面显示 404 | 从 Pages 设置中的 Visit site 打开，确认项目地址包含 `/仓库名/`，检查最近一次 deploy 是否成功。 |
| 页面仍是旧内容 | 确认更改已推送到 `main`、本次工作流已发布成功；等待传播后强制刷新。 |
| 新文档搜不到 | 确认 Markdown 已被 Git 跟踪、位于收录目录、未命中临时报告排除规则，并重新构建。 |
| npm ci 报错 | 使用 Node.js 22 或以上；依赖变更需同时提交 `package.json` 和 `package-lock.json`。 |
| 本地浏览器测试找不到 Chromium | 在 `website/` 执行 `npx playwright install chromium`；Linux 缺少系统依赖时用 `npx playwright install --with-deps chromium`。 |
| 图片或搜索在 GitHub 上打不开 | 检查 Actions 的测试报告及实际访问路径，确认整站由工作流发布，且没有将资源链接改成以 `/assets/` 开头的根路径。 |

## 9. 本地验证与报告

在项目根目录执行：

```bash
cd website
npm ci
npm test
npm run build
npx playwright install chromium
npm run test:e2e
```

单元测试验证路径映射、Markdown 安全处理、锚点、全文搜索和键盘交互；服务测试启动真实 HTTP 服务器检查状态码、文件类型、HEAD 和子路径访问。构建回归遍历生成页面的站内链接、图片、资源与锚点。

浏览器验证在 `/xhub/` 路径覆盖首页到 Docker 安装文档、刷新、全文搜索到护栏、代码复制、中英文切换、空结果与 Escape 关闭、索引加载失败及重试、Mermaid 渲染和移动端导航。测试读取真实构建文件和索引；失败场景仅中止索引请求，恢复后继续读取真实索引。

报告位于 `website/test-results/report.json`，截图位于同一目录。测试关闭独立服务器和浏览器，不创建网关业务数据。GitHub Actions 每次运行也会上传名为 **website-test-results** 的报告附件，可从运行详情下载。

网站验证不连接网关、数据库或付费模型服务，也不验证 GitHub 的实际发布权限和 DNS；这些状态以远端 Actions、Pages 设置和最终访问结果为准。
