<p align="center">
  <img alt="Codex Tweaks" src="app/Resources/Branding/CodexTweaks-AppIcon.png" width="128">
</p>

<h1 align="center">Codex Tweaks</h1>

<p align="center">
  Codex Tweaks 是一个面向 Codex 桌面客户端的本地界面定制工具。<br>
  它通过统一、简单的功能包管理方式，让用户无需直接修改 Codex 应用文件，也能调整 UI、扩展界面交互并进行个性化美化。
</p>

<p align="center">
  <img alt="macOS 13 或更新版本" src="https://img.shields.io/badge/macOS-13.0%2B-black?logo=apple&logoColor=white">
  <img alt="Windows 10 2004 或更新版本" src="https://img.shields.io/badge/Windows-10%202004%2B-0078D4?logo=windows&logoColor=white">
  <a href="https://github.com/cr-zhichen/codex-tweaks/releases"><img alt="最新版本" src="https://img.shields.io/github/v/release/cr-zhichen/codex-tweaks"></a>
  <a href="LICENSE"><img alt="MIT 许可证" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
</p>

<p align="center">
  <a href="https://github.com/cr-zhichen/codex-tweaks/releases"><strong>下载 Codex Tweaks</strong></a>
</p>

> Codex Tweaks 是非官方的本地定制工具，与 OpenAI 没有隶属或授权关系。Codex 更新后，部分功能包可能需要同步适配。

## 主要功能

使用 Codex Tweaks 可以：

- 改变 Codex 的界面样式和视觉表现，根据自己的偏好进行个性化美化
- 调整页面布局、界面组件和交互方式，让常用内容和操作更顺手
- 添加新的界面元素、状态提示或辅助功能，扩展 Codex 原有的使用体验
- 按需组合多个功能包，集中管理不同的 UI 修改和美化效果
- 随时启用、停用或更新单个功能包，也可以一键关闭全部界面增强并恢复原始界面
- 从本地目录、ZIP 或 Git 仓库安装功能包，由应用统一处理连接、依赖、编译、加载顺序和失败回退
- 将已安装的功能包导出为可再次安装的 ZIP，便于备份或分享源码

Codex Tweaks 不会修改 Codex 的应用文件。停用界面增强后，已加入页面的样式、组件和事件监听会被清理，Codex 本身可以继续使用。

## 开始使用

### 1. 准备环境

- macOS 13 或更新版本，或 Windows 10 2004（build 19041）及更新版本
- 已安装 Codex 桌面客户端
- [Node.js](https://nodejs.org/)：安装、编译或更新功能包时需要，同时应包含 `node`、`npm` 和 `npx`
- [Git](https://git-scm.com/)：仅从 Git 仓库安装或更新功能包时需要

应用会自动检测 Node.js 和 Git，并在“功能包”页面显示检测结果。缺少 Git 不影响本地功能包；缺少 Node.js 时，已有编译结果仍可运行，但无法重新编译或更新。

### 2. 下载并安装

前往 [GitHub Releases](https://github.com/cr-zhichen/codex-tweaks/releases)，按自己的设备下载一个安装文件：

| 系统 | 下载文件 | 适用设备 |
| --- | --- | --- |
| macOS 通用版（推荐） | 不带架构后缀的 `.dmg` | Apple Silicon 与 Intel Mac |
| macOS Apple Silicon | `-arm64.dmg` | M 系列 Mac，文件更小 |
| macOS Intel | `-x86_64.dmg` | Intel Mac，文件更小 |
| Windows x64 | `-windows-Setup-x86_64.exe` | Intel 或 AMD 64 位电脑 |
| Windows ARM64 | `-windows-Setup-arm64.exe` | Snapdragon 等 ARM64 电脑 |

不确定 Mac 架构时直接选择通用版。Windows 用户可在“设置 → 系统 → 系统信息/关于”中查看系统类型。

普通用户不需要下载 Source code、Sparkle ZIP、`.nupkg`、`appcast.xml` 或 `releases.*.json` 等更新文件。

#### macOS

1. 打开 DMG
2. 将 Codex Tweaks 拖入“应用程序”文件夹
3. 从“应用程序”中启动 Codex Tweaks

当前版本未经 Apple 公证。如果 macOS 阻止首次打开，请确认文件来自本项目的 GitHub Releases，再前往“系统设置 → 隐私与安全性”允许打开。

#### Windows

运行与设备架构对应的 Setup EXE。应用会安装到当前用户目录，不需要管理员权限，也可以从“设置 → 应用 → 已安装的应用”中卸载。

当前版本使用自签名证书，SmartScreen 可能显示提醒。请先确认文件来自本项目的 GitHub Releases，再决定是否继续运行。

### 3. 首次连接 Codex

1. 打开 Codex Tweaks
2. 如果 Codex 尚未运行，Codex Tweaks 会自动使用本地连接参数启动它
3. 如果 Codex 已在运行但无法连接，请先保存正在进行的工作，再按提示选择“重启并连接”
4. 概览页显示“已连接 Codex”后，即可进入“功能包”页面

重启只用于为本次 Codex 进程开启本地连接，不会修改 Codex 的安装文件。

### 4. 安装功能包

在“功能包”页面选择一种方式：

- **本地安装**：选择功能包目录或 ZIP 文件
- **从 Git 安装**：填写功能包仓库地址并选择版本

安装完成后，新功能包默认保持停用。检查名称、来源和说明，确认可信后再启用；如果卡片提示待编译，请先完成编译。你也可以先启用内置的 `ct-sample`，连接成功后它会在 Codex 右下角显示注入状态。

## 日常使用

- **启用或停用**：使用每个功能包旁的开关；存在依赖关系时，界面会提示需要同时启用的包
- **更新功能包**：先检查远程更新，再手动选择“更新并编译”；检查更新不会自动替换当前版本
- **导出功能包**：点击功能包旁的导出按钮并选择保存位置，应用会生成可再次安装的 ZIP
- **处理异常**：在功能包卡片中查看原因，或前往“运行日志”获取详细信息
- **更新应用**：在“关于与更新”中选择正式版或测试版通道；发现新版本后，应用会先询问，再下载、安装并重启
- **临时恢复原始界面**：在概览页关闭“启用界面增强”，无需退出 Codex

## 安全说明

- 功能包会在 Codex 页面中运行，可能接触页面内容。只安装自己信任并愿意运行的功能包及其依赖
- Git 功能包会锁定到明确版本；更新和编译需要由用户主动触发
- 安装依赖时不会执行依赖附带的安装脚本，但功能包本身仍属于第三方代码
- Codex Tweaks 只向 Codex 的本地界面应用增强，不向普通网页注入
- 本地连接只监听本机；不要通过代理或端口转发将它暴露到外部网络

## 常见问题

### 安装功能包后为什么没有变化？

新功能包默认停用。请确认 Codex 已连接、功能包已成功编译，并在“功能包”页面将它启用。

### 为什么提示需要重启 Codex？

Codex 只有在启动时才能开启所需的本地连接。保存当前工作后使用“重启并连接”即可，Codex Tweaks 不会修改 Codex 的应用文件。

### 为什么找不到 Node.js 或 Git？

先完成对应工具的安装，再回到“功能包”页面重新扫描。Node.js 是编译功能包所必需的；Git 只影响 Git 来源的安装和更新。

### 编译或更新失败会影响正在使用的版本吗？

不会。Codex Tweaks 只在新版本完整编译成功后切换；失败时继续使用上一个可用版本。详细原因可在功能包卡片和“运行日志”中查看。

### Codex 更新后功能包失效怎么办？

先停用异常功能包并查看运行日志，再检查该功能包是否已有更新。Codex 的页面结构发生变化时，功能包作者可能需要发布适配版本。

<details>
<summary>制作自己的功能包</summary>

如果你想制作自己的功能包，可在应用概览页使用“复制 Skill”，将完整开发说明交给 Codex 或其他智能体。规范原文位于 [`Skills/develop-codex-tweaks-package/SKILL.md`](Skills/develop-codex-tweaks-package/SKILL.md)。

</details>

## 许可证

本项目基于 [MIT 许可证](LICENSE) 开源。
