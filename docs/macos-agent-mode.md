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
