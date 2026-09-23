# 界面主题

- Route: `/ui-theme`
- Nav: SETTINGS
- Status: `specified`

## 目的

给控制台换 Logo（亮/暗）和 Favicon。改输入即预览顶栏 Logo；Save 才持久化。无主色选择器。仅 admin。无 token 整页不渲染。

## 布局

登录后控制台壳。居中 `max-w-4xl px-6 py-8`。

### 顶栏

标题 `UI Theme Customization` + 说明「Customize your LiteLLM admin dashboard with a custom logo and favicon.」

### 筛选

无。

### 表

无。一张 Card 三个 URL 输入 + Save / Reset。

### Tab

无。

### 抽屉

无。

### 模态

无。无本地文件上传控件（源码无 `POST /upload/logo`）；只接受 URL。

### URL

`/ui-theme`。

## 交互

进入 `GET /get/ui_theme_settings`，填 `logo_url` / `logo_url_dark` / `favicon_url`，并写入 ThemeContext（顶栏立刻换图）。改 input 同步预览（空则回默认）。Save：`PATCH /update/ui_theme_settings`，body 空串当 `null`。Reset：清空三字段并 PATCH 全 `null`，toast `Theme settings reset to default!`。保存中两按钮 spinner + disabled。失败 toast，预览可已变但库未写，需再 Save。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 读取 | `GET /get/ui_theme_settings` | 填三个 URL |
| 保存 | `PATCH /update/ui_theme_settings` | toast，顶栏/favicon 保持新值 |
| 重置 | `PATCH /update/ui_theme_settings`（全 null） | 恢复默认 Logo |

## 字段

| 字段 | 含义 |
|---|---|
| `logo_url` | 亮色 Logo URL |
| `logo_url_dark` | 暗色 Logo；空则复用 `logo_url` |
| `favicon_url` | favicon（.ico / .png / .svg） |

无 `primary_color` 字段。

## 状态

loading：Save/Reset spinner。accessToken 空：`return null`。GET 失败：控制台 error，表单空。PATCH 失败：toast `Failed to update theme settings`。窄屏单列。

## 验收

改 URL 顶栏即时预览；Save 后刷新仍在；Reset 回默认；无上传按钮；桌面与窄屏。
