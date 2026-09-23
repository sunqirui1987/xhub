# UI

控制台视觉与交互。页面「有哪些按钮、打哪条 API」在 [frontend](../frontend/README.md)；这里规定**长什么样、怎么操作**。

默认落地 **`/api-keys`（Virtual Keys）**。登录后的管理壳是左栏五组导航 + 内容列顶栏；未登录走 `/login`，不进这套壳。

| 文 | 内容 |
|---|---|
| [设计原则](principles.md) | 信息密度、管理工具气质、明文只出现一次、权限在界面也在 API |
| [视觉](visual.md) | 色、字、间距、密度、顶栏与侧栏尺寸 |
| [组件](components.md) | 五组 Sidebar、Topbar、表、模态、SecretOnce、FilterBar |
| [页面模板](patterns.md) | 列表 / 详情 / 创建 / Playground / 设置；筛选写入 URL |
| [状态](states.md) | loading / empty / 错误 / view-only / 权限 / 窄屏 |

实现可用任意组件库，但交互与信息结构必须对得上。产品名 XHub。
