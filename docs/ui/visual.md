# 视觉

主题可换色（`/ui-theme`），下面是默认管理台约定。

| Token | 用途 |
|---|---|
| 背景 | 近白或近黑的中性底，不要大面积品牌色 |
| 主色 | 主按钮、侧栏选中、链接；`/ui-theme` 的 `primary_color` |
| 成功 / 警告 / 危险 | 状态徽章、Block、Delete |
| 边框 | 1px 浅线，表格用，不用厚阴影卡片 |
| 横幅 | 壳层警告贴在顶栏与主内容之间，全宽、无圆角、底边框 |

字体：界面用无衬线；`key`、`request_id`、token 前缀、版本号用等宽。

密度：表格行高紧凑（约 36–44px）。页边距 16–24px（列表主区约 `p-8`）。

## 壳尺寸

| 区域 | 约定 |
|---|---|
| 视口 | `h-screen overflow-hidden`。侧栏与内容列各自滚动，整页不跟着拖。 |
| 侧栏 | 展开约 240px；可收成图标。分组标签大写（`AI GATEWAY` 等）。头部与顶栏同高。 |
| 顶栏 | 高 56px（`h-14`），底边框。只盖住内容列，不盖侧栏。 |
| Logo | 网关模式在**侧栏头部**，不在顶栏。Chat / 公开 Hub / `/connect` 用独立 Navbar，Logo 在左。来源 `/get_logo_url`、`/get_image`、`/upload/logo`。缺省显示产品名 XHub。 |
| 版本徽章 | 侧栏 Logo 旁等宽 `v{version}`，数据来自 `GET /health/readiness/details`。 |

桌面 ≥ 1024px 侧栏常驻；小于此宽度折叠为抽屉。
