<p align="center">
  <img alt="Codex Tweaks" src="app/Resources/Branding/CodexTweaks-AppIcon.png" width="128">
</p>

<h1 align="center">Codex Tweaks</h1>

<p align="center">
  用一个原生 macOS App，管理 Codex 桌面客户端的本地 CSS 与 JavaScript 增强。
</p>

<p align="center">
  <img alt="macOS" src="https://img.shields.io/badge/macOS-13.0%2B-black?logo=apple&logoColor=white">
  <img alt="Swift" src="https://img.shields.io/badge/Swift-5-F05138?logo=swift&logoColor=white">
  <img alt="SwiftUI" src="https://img.shields.io/badge/UI-SwiftUI-0A84FF">
  <img alt="CDP" src="https://img.shields.io/badge/CDP-127.0.0.1%3A9335-4A5568">
  <img alt="mise" src="https://img.shields.io/badge/toolchain-mise-3FA7D6">
</p>

## 简介

Codex Tweaks 是一个带主窗口与菜单栏入口的原生 macOS 工具。它通过仅监听本机的 Chrome DevTools Protocol（CDP），把用户自己的 CSS 和 JavaScript 注入 Codex 的 `app://` 页面，并在文件变化、页面刷新或新窗口出现后自动恢复。

项目不会修改 `ChatGPT.app` 或 `app.asar`，也不需要常驻的 Node.js CLI。所有自定义资源都保存在用户目录中，可以直接检查、编辑、停用或删除。

> Codex Tweaks 是非官方本地定制工具。Codex 客户端升级后，页面结构或调试参数可能发生变化。

## 功能

| 模块 | 功能 |
| --- | --- |
| Codex 连接 | Codex 未运行时使用本机 CDP 参数启动；已运行但未开启 CDP 时提示确认重启；用户主动退出后不会反复拉起 |
| 自动注入 | 发现全部 `app://` 页面并幂等注入 CSS/JavaScript；每 2 秒检查窗口与资源变化 |
| 本地定制 | 从概览页直接用系统关联编辑器打开 CSS 或 JavaScript；保存后自动重新注入 |
| 原生界面 | SwiftUI 主窗口集中显示连接状态、注入控制、资源入口和运行日志；菜单栏保留常用快捷操作 |
| 日志 | 最新记录优先显示，支持刷新、打开日志文件，以及确认后清除现有日志 |
| 生命周期 | 停用增强或正常退出时清理样式、Shadow DOM 和已注册的事件监听器 |
| 默认示例 | Codex 页面右下角显示隔离在 Shadow DOM 中的 `CT` 按钮，点击后提示“CSS 与 JS 加载完成” |

## 界面预览

| 连接概览 | 运行日志 |
| --- | --- |
| ![连接概览](docs/screenshot-overview.png) | ![运行日志](docs/screenshot-logs.png) |

## 工作方式

```text
Codex Tweaks.app
  ├─ 启动或连接 ChatGPT.app
  ├─ GET 127.0.0.1:9335/json/list
  ├─ 筛选 type=page 且 URL 为 app:// 的目标
  ├─ WebSocket → Runtime.evaluate
  └─ 注入 ui.css + ui.js
```

注入脚本使用内容指纹判断资源是否变化，并在独立的 Shadow DOM 中承载默认组件。重复检查不会重复创建节点；重新注入前会先执行上一版本注册的清理回调。

## 开始使用

### 环境要求

- macOS 13 或更新版本
- Xcode
- [mise](https://mise.jdx.dev/)
- 已安装包含 Codex 桌面客户端的 `ChatGPT.app`

### 构建与启动

```sh
mise install
mise run build
open "dist/Codex Tweaks.app"
```

首次连接时：

1. Codex 未运行：Codex Tweaks 会使用仅监听 `127.0.0.1:9335` 的调试参数启动客户端。
2. Codex 已运行且 CDP 可用：直接连接并注入当前全部 `app://` 窗口。
3. Codex 已运行但没有 CDP：界面会说明原因，并在用户确认后重新启动 Codex。

## 自定义 CSS 与 JavaScript

首次启动会创建：

```text
~/Library/Application Support/Codex Tweaks/Tweaks/ui.css
~/Library/Application Support/Codex Tweaks/Tweaks/ui.js
```

在“概览”中点击“编辑 CSS”或“编辑 JavaScript”即可用系统关联的外部编辑器打开文件。保存后无需重启应用，下一轮检查会自动重新注入。

`ui.js` 在函数作用域中执行，并获得：

- `root`：隔离的 `ShadowRoot`
- `api.version`：当前资源版本
- `api.registerCleanup(callback)`：注册重新注入或停用时的清理逻辑

## 安全边界

- CDP 固定监听 `127.0.0.1:9335`，不会主动暴露到局域网。
- 调试端口开启期间，本机其他进程仍可能连接；不要把该端口转发或代理到外部网络。
- 退出 Codex Tweaks 会清理当前注入内容，但不会关闭 Codex 已开启的调试端口；完全退出并正常重开 Codex 后才会关闭。
- 自定义 JavaScript 与 Codex 页面拥有相同的渲染上下文权限，只运行自己审阅过的本地脚本。
- 目标筛选刻意排除普通 `https://` 页面，不会向 Codex 的应用内浏览器网页注入。

## 开发

项目使用 XcodeGen 生成工程，相关工具版本与任务由 `mise.toml` 统一管理。

```sh
mise run generate   # 生成 CodexTweaks.xcodeproj
mise run test       # 运行 macOS 单元测试
mise run build      # 构建 Release App 到 dist/
mise run launch     # 启动已构建的 App
mise run verify     # 测试、Release 构建与 diff 检查
mise run kill       # 停止 Codex Tweaks
mise run clean      # 清理 build/ 与 dist/
```

Release 构建输出：

```text
dist/Codex Tweaks.app
```

## 验证范围

自动测试覆盖 CDP 目标筛选、注入脚本生成、清理逻辑与内容指纹。`mise run verify` 会依次执行测试、Release 构建和 `git diff --check`；本地构建成功不代表所有未来 Codex 客户端版本都保持相同页面结构。

当前实现仅对 `app://` 页面生效。真实注入仍取决于 Codex 客户端允许的 CDP 启动参数、端口状态以及页面生命周期。

## 项目结构

```text
app/
├── Sources/                    # SwiftUI、启动器、CDP 与注入实现
├── Resources/Assets.xcassets/  # macOS AppIcon 资产目录
├── Resources/Branding/         # B1 原始图标母版
├── Resources/Tweaks/           # 首次启动时复制的默认 CSS/JS
└── Tests/                      # 目标筛选、脚本生成与内容指纹测试
docs/                           # README 界面截图
mise.toml                       # 固定依赖与统一任务入口
project.yml                     # XcodeGen 工程定义
```
