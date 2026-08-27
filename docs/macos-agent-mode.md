# macOS Agent 模式

Agent 模式用于让 Codex Tweaks 在没有常驻窗口的情况下继续管理 CDP 连接、功能包 Node 运行时与页面注入。

## 使用方式

1. 在“概览 → 控制”中开启“macOS Agent 模式”
2. 应用会立即从 Dock 与 `Command-Tab` 应用切换器中隐藏
3. “隐藏菜单栏图标”默认开启；关闭它可保留状态栏入口
4. 关闭主窗口后，Codex Tweaks 与 Go 后端仍会继续运行

当 Dock 与菜单栏图标都隐藏时，可以从“应用程序”再次打开 Codex Tweaks，或执行：

```bash
open -a "Codex Tweaks"
```

现有进程会收到重新打开事件并恢复主窗口。退出应用仍会停止后台、Node 功能包与页面注入。

## 随 Codex 启动的可行性

Codex 的 CDP 端口只能通过进程启动参数开启，因此启动顺序决定了能否无中断连接。

| 启动方式 | 可行性 | 行为与限制 |
| --- | --- | --- |
| 先启动 Codex Tweaks | 已支持 | Codex 未运行时，Go 后端会使用 CDP 参数启动 Codex |
| Codex Tweaks 作为登录项 | 可实现 | 使用 `SMAppService` 即可注册，但现有后端随后会自动启动 Codex，而不是等待用户以后手动启动 |
| 用户先正常启动 Codex | 不能直接附加 | 已运行进程没有 CDP 端口，Codex Tweaks 必须提示并重启 Codex |
| 监听 Codex 启动事件 | 有条件可实现 | 监听器必须预先驻留；发现普通启动后仍需终止并使用 CDP 参数重启 Codex |
| 使用专用启动器或快捷方式 | 推荐 | 启动器先唤醒 Agent，再由现有后端启动带 CDP 参数的 Codex，不需要常驻轮询进程 |

真正实现“点击 Codex 后自动同时启动 Tweaks”需要改变用户的启动入口，例如创建“Open Codex with Tweaks”启动器、快捷指令或替换 Dock 中的 Codex 图标。macOS 没有提供让一个完全未运行的普通应用订阅另一个应用启动事件的机制；要监听该事件，必须先安装并运行登录项或 LaunchAgent。

本次 Agent 模式不注册登录项，也不修改 Codex 应用包。这样可以避免登录后意外启动 Codex、破坏 Codex 签名或在 Codex 更新后失效。

## 推荐的后续方案

如果需要无感启动，建议增加一个可选的 macOS 启动器：

1. 用户从启动器打开 Codex
2. 启动器先启动或唤醒隐藏的 Codex Tweaks Agent
3. Agent 的 Go 后端使用现有 CDP 参数启动 Codex
4. 连接成功后启动器退出，Codex Tweaks 继续后台注入

该方案复用现有启动和注入逻辑，不需要修改 Codex 安装文件，也不会依赖脆弱的进程轮询。
