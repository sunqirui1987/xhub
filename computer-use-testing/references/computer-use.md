# Computer Use 入口与操作

仅使用当前工具返回文档提供的 API。以下是当前 Codex 桌面入口参考；工具缺失时说明限制，不伪造或安装替代控制器。

## 首次调用

mcp__cua_repl__js 第一次调用或 reset 后只执行一种入口调用，可以赋给变量，不追加其他 API、等待或观察；读完返回文档再继续。

| 目标 | 入口示例 |
| --- | --- |
| 需要应用、浏览器及标签清单 | await cua.getState(); |
| 用户 @ 标签 | var testTab = await cua.getTab({ mention: 完整插件URL }); |
| 已知 URL 与浏览器 | var testTab = await cua.getTab({ url: 目标URL }, { browser: 浏览器ID }); |
| 已知标签 ID 与浏览器 | var testTab = await cua.getTab(标签ID, { browser: 浏览器ID }); |
| 内置浏览器打开 URL | var testTab = await cua.createBrowserTab("iab", 目标URL, { visible: true }); |
| 指定 Chrome/Edge | var testTab = await cua.createBrowserTab("chrome", 目标URL, { sessionName: "🧪 UI 测试" });，Edge 使用 "edge" |
| 已知 URL，未指定浏览器 | var testBrowser = await cua.getBrowser({ url: 目标URL }); |
| macOS 桌面应用 | var testApp = await cua.getApp("应用名"); |

表中的中文参数是说明占位，执行时替换成实际值。标签 @ 提及优先于 URL；指定浏览器时不要另选。绑定浏览器不会自动开页面，按返回文档继续选标签。MCP Apps 只能绑定现有标签。Linux/Windows 原生应用从清单选择确切窗口 ID，多窗口先核对标题。

## 持续操作

REPL 变量跨调用保留；reset 清空绑定。上下文压缩后继续任务，先调用 await cua.rewriteDocumentation(); 重新读取文档，不重置测试页面。

获取应用或标签已经自动显示初始状态。读到控件后，执行 await testApp.click(当前索引); await testApp.getAXState();，再基于新状态选择下一步。具体索引来自最近树；原生应用与标签页的键盘、输入及滚动签名可能不同。

DOM-only 标签不能使用原生输入包装方法，按返回浏览器文档使用 Playwright locator，优先角色、标签和可访问名称。不猜 DOM，不通过未记录的 evaluate、请求或内省绕过限制。原生模式可用当前树索引；坐标须有新截图且模式支持。

getAXState、getScreenshot、getAXStateAndScreenshot、入口和清单 API 自动输出，不重复 write 或 emitImage。自定义结果用 nodeRepl.write(...)。保存文件只使用当前文档明确提供的文件 API。终端可启动服务和管理产物，直接 UI 操作仍通过 Computer Use。

普通文本会话不能使用仅限活跃语音通话的 capture_screen_context 来截图。
