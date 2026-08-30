# macOS 应用入口显示

Codex Tweaks 在主窗口关闭后仍会继续管理 CDP 连接、功能包 Node 运行时与页面注入。macOS 可分别控制 Dock 和菜单栏中的应用入口。

## 使用方式

1. 在“概览 → 控制”中按需开启“隐藏 Dock 图标”
2. 开启后，应用会同时从 Dock 与 `Command-Tab` 应用切换器中隐藏
3. “隐藏菜单栏图标”是独立开关，两个选项默认都关闭
4. 关闭主窗口后，Codex Tweaks 与 Go 后端仍会继续运行

当 Dock 与菜单栏图标都隐藏时，可以从“应用程序”再次打开 Codex Tweaks，或执行：

```bash
open -a "Codex Tweaks"
```

现有进程会收到重新打开事件并恢复主窗口。退出应用仍会停止后台、Node 功能包与页面注入。
