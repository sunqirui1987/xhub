# 状态

| 状态 | UI |
|---|---|
| loading | 表骨架或行内 spinner，不闪空表。鉴权未决用全屏 LoadingScreen，不闪登录再闪壳。 |
| empty | EmptyState + 主按钮。view-only 只有说明、无创建 CTA。 |
| forbidden | 说明无权限（如 Playground Access Denied），不假装空列表。 |
| view-only | 同一列表/详情可读；Create / Save / Delete / Block / Regenerate / 发送 全部隐藏。API 仍会 403，界面不能假装已写入。 |
| 校验失败 | 字段下红字，可改后续提交。 |
| 429 / 超时 | Banner；写操作锁定原 payload。 |
| blocked / expired | 行徽章；行菜单仍可 Unblock（有写权限时）。 |
| 窄屏 | 侧栏抽屉；表横向滚动；主按钮保留。 |

桌面 ≥ 1024px 侧栏常驻；小于此宽度折叠。

**密钥明文只展示一次**仅适用于 Virtual Keys 的创建 / 轮换成功模态，见 [组件 SecretOnce](components.md)。其他页的 loading / empty / forbidden 各自写清，不要复用这句。
