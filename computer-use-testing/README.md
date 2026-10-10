# Computer Use 自动测试技能

该目录随 xhub 仓库分发，让 Codex 根据真实界面自主操作、验证用户流程、记录证据并清理测试数据。入口为 SKILL.md，agents/openai.yaml 提供技能列表中的名称和默认提示，references/ 提供工具及 xhub 约定，assets/ 提供报告模板和连通性测试页面。

## clone 后安装

在仓库根目录执行以下命令（macOS/Linux）。使用绝对路径的符号链接，技能更新随 git pull 生效：

~~~sh
skill_install_root="${CODEX_HOME:-$HOME/.codex}/skills"
mkdir -p "$skill_install_root"
skill_install_target="$skill_install_root/computer-use-testing"
if [ -e "$skill_install_target" ] || [ -L "$skill_install_target" ]; then
  echo "技能位置已存在，请先确认来源：$skill_install_target"
else
  ln -s "$(pwd)/computer-use-testing" "$skill_install_target"
fi
~~~

命令保留已有目录和链接，不覆盖其他安装。仓库搬家后重新建立链接。

不支持符号链接时，可以将整个 computer-use-testing 目录复制到 Codex 的个人 skills 目录；复制方式需要在更新仓库后同步副本。Windows 用户同样可以复制到用户目录中的 .codex/skills/，自定义 CODEX_HOME 时使用该目录下的 skills/。

安装后在新建 Codex 会话的技能列表中检查 Computer Use 自动测试；若未发现，重启 Codex 后重新检查。直接提供项目内 SKILL.md 的路径也可以让 Codex 按文件执行。

## 使用

示例提示：

> 使用 $computer-use-testing，在本项目的隔离测试环境中模拟用户验证模型创建、编辑、刷新回读和删除流程，检查后台持久化结果，保存报告并清理本轮数据。

也可以明确指定网页地址、浏览器或桌面应用，例如让它复现一个表单问题并记录实际步骤。默认允许自动选择技能，未设置显式调用限制。

技能需要当前 Codex 环境提供 Computer Use 工具。安装这个目录不会增加浏览器控制能力；没有工具时会如实记录阻塞并区分已有命令行测试。真实项目 E2E 仍依赖项目后台、数据库和测试环境。付费供应商和共享环境的操作遵循任务范围及工具权限。

## 连通性验证

从仓库根目录启动仅监听本机的测试页服务：

~~~sh
python3 -m http.server 8765 --bind 127.0.0.1 --directory computer-use-testing/assets
~~~

让 Codex 使用该技能打开 http://127.0.0.1:8765/smoke.html，验证空值拒绝、输入提交、展开/折叠和清空草稿，结束后停止本次服务。该页面不创建账号、调用供应商或保存数据；通过只证明控制工具与浏览器表单可用，不代表 xhub 业务 E2E 已通过。

技能格式可通过 Codex 自带 skill-creator 的 scripts/quick_validate.py 校验。实际测试报告保存到项目忽略的 .e2e/computer-use/<run-id>/，报告模板在 assets/report-template.md。
