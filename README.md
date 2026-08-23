<p align="center">
  <img alt="Codex Tweaks" src="app/Resources/Branding/CodexTweaks-AppIcon.png" width="128">
</p>

<h1 align="center">Codex Tweaks</h1>

<p align="center">
  用双原生薄前端与单一 Go 后端，按包管理、编译与加载 Codex 桌面客户端的本地界面增强。
</p>

<p align="center">
  <img alt="macOS" src="https://img.shields.io/badge/macOS-13.0%2B-black?logo=apple&logoColor=white">
  <img alt="Windows" src="https://img.shields.io/badge/Windows-10%202004%2B-0078D4?logo=windows&logoColor=white">
  <img alt="Go" src="https://img.shields.io/badge/backend-Go-00ADD8?logo=go&logoColor=white">
  <img alt="SwiftUI" src="https://img.shields.io/badge/frontend-SwiftUI-F05138?logo=swift&logoColor=white">
  <img alt="WinUI 3" src="https://img.shields.io/badge/frontend-WinUI%203-0078D4?logo=windows&logoColor=white">
  <img alt="Node.js" src="https://img.shields.io/badge/package_build-Node.js-5FA04E?logo=nodedotjs&logoColor=white">
  <img alt="CDP" src="https://img.shields.io/badge/CDP-127.0.0.1%3A9335-4A5568">
  <a href="https://github.com/cr-zhichen/codex-tweaks/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/cr-zhichen/codex-tweaks/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/cr-zhichen/codex-tweaks/releases"><img alt="Release" src="https://img.shields.io/github/v/release/cr-zhichen/codex-tweaks"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
</p>

## 简介

Codex Tweaks 同时提供 macOS SwiftUI 与 Windows WinUI 3 原生界面。两个前端都只负责原生控件布局、窗口、系统对话框、剪贴板和打开本机路径；包管理、验证、依赖解析、编译、Git、更新状态、日志、Codex 进程控制、CDP 和注入全部由随应用打包的 Go sidecar 负责。前后端通过标准输入输出上的逐行 JSON RPC 通信，不额外监听本地服务端口。

Go 同时拥有单一 Presentation Contract：业务状态、DTO、可用操作、界面文案、设计令牌、CDP 地址和更新仓库只在 Go 中定义。生成器据此输出 Swift 与 C# 类型；SwiftUI 和 WinUI 3 仅维护各自平台的原生布局与交互，不复制业务判断。

Windows 版本是 self-contained、unpackaged 的 WinUI 3 应用，Go sidecar、.NET 运行时和 Windows App SDK 运行时一起进入同一个 Velopack 版本包。安装、更新和回滚替换的是整个版本目录，因此前端与后端不会被拆开升级。

一个目录就是一个包。包可使用常见的 Node.js 模块写法、TypeScript、CSS import 和锁定的 npm 依赖；用户在 UI 中明确点击后，应用才会下载依赖并用固定版本 esbuild 编译成浏览器可执行产物。

项目不修改 `ChatGPT.app` 或 `app.asar`。Node.js 只用于功能包编译，不是页面注入时的常驻运行时。

> Codex Tweaks 是非官方本地定制工具。Codex 客户端升级后，页面结构或调试参数可能发生变化。

## 功能

| 模块 | 功能 |
| --- | --- |
| Codex 连接 | Codex 未运行时使用本机 CDP 参数启动；已运行但未开启 CDP 时由用户确认是否重启 |
| 包级开关 | 按功能包整体启用/停用；新发现的包默认停用，包内文件不再单独设置开关 |
| 可读元数据 | 在 UI 中显示包名、说明、源版本、当前激活版本和更新原因 |
| 本地优先级 | 用户可调整有效优先级或恢复作者默认值；覆盖值独立存储，与声明值一致时自动移除；依赖关系覆盖数值顺序时显示原因 |
| 包依赖 | 逐条显示本地/Git 来源、版本和解析状态；依赖拓扑优先于数值优先级，缺失、停用、来源冲突、版本冲突与循环依赖会明确提示 |
| 本地导入 | 选择功能包目录或 ZIP，完整校验后原子复制到本地 packages 目录 |
| Git 包管理 | 从 HTTPS/SSH Git 仓库按分支、最新 SemVer Tag、指定 Tag、GitHub Release 或 Commit 安装，并锁定到精确 Commit |
| 更新检查 | 定期只检查远端是否有更新；只有用户点击“更新并编译”后才拉取、编译并切换 |
| 手动更新 | 只有点击包的编译/更新按钮才会执行依赖同步与编译 |
| 原子激活 | 新产物完整编译成功后才切换；失败时保留上一个可用版本 |
| 开发者模式 | 自动编译已启用包的源码变化，但不自动下载新依赖或切换包版本 |
| 故障隔离 | 一个包编译或运行失败时，其他包继续加载；错误回显在对应包上 |
| 开发 Skill | 项目内置功能包开发 Skill；“复制给 AI”直接读取同一份 `SKILL.md`，避免两套说明漂移 |
| 软件更新 | macOS 按架构选择 GitHub Release DMG；Windows 使用 Velopack 正式版/测试版通道原子替换前端与 Go sidecar |
| 内置示例 | 只提供 `ct-sample`：在右下角显示 `codex_tweaks 已注入` 状态 |

## 工作方式

```text
macOS SwiftUI 薄前端          Windows WinUI 3 薄前端
          \                     /
           \  Presentation v1  /
            + stdio JSON-lines RPC
                      |
                      v
            codex-tweaks-backend (Go)
                |
本地包：Tweaks/packages/<package>/
远程包：ManagedPackages/sources/<package-hash>/<commit>/
  package.json + src + package-lock.json
                |
                | 依赖解析：拓扑顺序 > 用户有效优先级 > 包名
                | 用户在 UI 中点击编译或更新
                v
        npm ci --ignore-scripts
        esbuild 0.25.9 --platform=browser --format=cjs
                |
                | 成功后原子切换
                v
~/Library/Caches/Codex Tweaks/PackageBuilds/
                |
                v
CDP Runtime.evaluate -> Codex app:// 页面
```

Codex Tweaks 每 2 秒重新发现页面与包状态。注入使用产物指纹保证幂等；重新注入前会从后往前执行已注册的清理回调。

## 开始使用

### 运行环境

- macOS 13 或更新版本，或 Windows 10 2004（build 19041）及更新版本
- 已安装 Codex 桌面客户端（macOS 的 `ChatGPT.app` 或 Windows 的 `ChatGPT.exe`）
- Node.js（需同时提供 `node`、`npm` 和 `npx`），用于功能包的依赖下载与编译
- Git，用于安装和更新远程功能包；私有仓库可使用本机已有的 SSH 凭据，应用不会弹出交互式凭据提示

应用会从两个平台的常见位置检测 Codex、Node.js 和 Git，并在功能包页面显示实际结果。没有 Node.js 时，已编译的包仍可加载，但不能更新产物；没有 Git 时，本地包仍可使用，但不能安装或更新远程包。Windows 安装包已经包含应用所需的 .NET 与 Windows App SDK 运行时，不要求用户另外安装开发环境。

### 安装发行版

从 [GitHub Releases](https://github.com/cr-zhichen/codex-tweaks/releases) 下载对应平台与架构的文件：

- macOS：打开 universal、arm64 或 x86_64 DMG，将 Codex Tweaks 拖移到“应用程序”。当前使用 ad-hoc 签名且未经 Apple 公证；如果系统阻止首次打开，请在“系统设置 → 隐私与安全性”中确认。
- Windows：运行名称中包含 `win-x64` 或 `win-arm64` 的 `Setup.exe`。Velopack 默认安装到当前用户目录，不弹出管理员权限；可从“设置 → 应用 → 已安装的应用”卸载。测试版允许未签名发布，因此 Windows SmartScreen 可能要求用户确认运行。

Windows 应用会根据用户选择的正式版或测试版通道读取同一 GitHub Release 中的 Velopack feed，下载后整体替换 WinUI 前端、Go sidecar 和全部随包资源并重启。打包脚本预留 `VPK_AZURE_TRUSTED_SIGN_FILE` 与 `VPK_SIGN_TEMPLATE`：接入 Azure Trusted Signing 或其他托管签名命令后，无需修改应用或更新协议。

### 从源码构建

macOS 开发需要 Xcode 和 [mise](https://mise.jdx.dev/)；`mise install` 会安装固定版本的 Go、XcodeGen 和校验工具：

```sh
mise install
mise run build
open "dist/Codex Tweaks.app"
```

Windows 开发需要 Go 与 .NET 8 SDK。以下命令生成 x64、ARM64 两套 self-contained 目录、Velopack 安装器和更新 feed，并执行 PE 架构及 Go RPC 冒烟测试：

```powershell
./scripts/build-windows.ps1 -Version 3.0.0-beta.1 -BuildNumber 1
./scripts/package-windows.ps1 -Version 3.0.0-beta.1 -Channel beta
./scripts/verify-windows.ps1 -Version 3.0.0-beta.1 -Channel beta -RequirePackages
```

构建输出位于 `artifacts/windows/win-x64`、`artifacts/windows/win-arm64`，待发布文件统一暂存到 `artifacts/windows/release`。

## 功能包格式

用户功能包位于：

```text
~/Library/Application Support/Codex Tweaks/Tweaks/
└── packages/
    └── my-tweak/
        ├── package.json
        ├── package-lock.json   # 有 npm 依赖时必须提供
        └── src/
            ├── index.js
            └── style.css
```

旧的平铺 `ui.css` / `ui.js` / `vendor` / `scripts` / `styles` 格式不再加载。

应用会在首次使用新格式时记录当前包及其开关状态。此后新发现的包会默认保持停用，完成审阅和编译后需由用户在功能包页面手动启用。

### package.json

```json
{
  "name": "my-tweak",
  "version": "1.0.0",
  "description": "在这里写会显示在 UI 中的功能说明。",
  "type": "module",
  "dependencies": {},
  "codexTweaks": {
    "apiVersion": 2,
    "entry": "src/index.js",
    "priority": 100,
    "packageDependencies": {}
  }
}
```

- `name`：唯一包标识。同名包都会被标记为无效，不影响其他包。
- `version`：必须是有效 SemVer；当源版本高于当前产物时，UI 会继续显示当前激活版本，直到用户手动更新。
- `description`：显示在功能包页面。
- `dependencies`：运行时必须打包进浏览器产物的 npm 依赖；建议使用明确版本。
- `codexTweaks.entry`：esbuild 入口，必须位于包目录中。
- `codexTweaks.priority`：作者提供的默认优先级。用户在 UI 中设置的有效优先级保存在 `State/package-settings.json`，不会改写清单。
- `codexTweaks.packageDependencies`：其他功能包的版本要求与可选 Git 安装来源；它与 npm `dependencies` 是两套不同的依赖。

### 入口与生命周期

```js
import "./style.css";

export function activate({ root, onCleanup, api, dependencies }) {
  const button = document.createElement("button");
  button.textContent = "Hello";

  const handleClick = () => console.log("clicked");
  button.addEventListener("click", handleClick);
  root.append(button);

  onCleanup(() => {
    button.removeEventListener("click", handleClick);
  });
}
```

`activate(context)` 可以是同步或异步函数，也可直接返回清理函数。`context` 包含：

- `id` / `name` / `version`：当前激活包的信息
- `root`：当前包独立的 DOM 根节点
- `onCleanup(callback)` / `api.registerCleanup(callback)`：注册停用、重新注入或退出时的清理逻辑
- `api.registerLibrary` / `hasLibrary` / `getLibrary` / `listLibraries`：为依赖当前包的功能包注册具名能力
- `dependencies.has` / `get` / `list`：只访问清单中已声明且成功激活的功能包依赖及其能力

包可直接使用 `import` / `export` 组织多个 JS/TS/CSS 文件。包内顺序使用正常模块依赖图表达：先 import 底层模块，再由入口组合，不依赖文件名前缀。

### 跨包加载顺序

1. 先按功能包依赖图做拓扑排序，保证依赖在使用方之前；互不依赖的包再按用户有效优先级从小到大、同值按 `name` 排序。
2. 先按该顺序插入所有包的 CSS，使 JavaScript 启动时样式已就绪。
3. 再按同一顺序调用每个包的 `activate(context)`。
4. 单包失败时立即清理该包的 CSS、DOM、能力和回调；依赖它的包不会启动，其他无关包继续运行。

依赖关系是硬约束，数值优先级只处理没有依赖关系的包。作者默认优先级仍保留在包中，用户调整值只影响本机。

### 功能包依赖

```json
{
  "codexTweaks": {
    "apiVersion": 2,
    "entry": "src/index.js",
    "priority": 100,
    "packageDependencies": {
      "shared-core": {
        "version": "^1.2.0",
        "source": {
          "url": "https://github.com/example/shared-core.git",
          "selector": { "type": "latestSemverTag" }
        }
      }
    }
  }
}
```

- 版本要求支持精确 SemVer、`^`、`~`、`>=` / `>` / `<=` / `<`，以及 `x` / `*` 通配符。
- 包 ID 是离线构建依赖图所需的规范身份；版本范围始终需要明确声明。
- 没有 `source` 时是仅本地依赖，应用只提示缺失，不猜测仓库。
- 带有 `source` 时可从明确的 Git 来源安装；运行时仍使用本地锁定、编译后的产物。
- 只有 Git 地址的输入必须先读取远程 `package.json`，解析出包 ID 后再规范化为“包 ID + Git 来源”，不能把 URL 本身当作依赖身份。
- 应用可递归安装缺失依赖，并检查同一包 ID 的来源与版本约束是否冲突。
- 功能包依赖只提供加载顺序和运行时具名能力，不允许在编译时直接 import 另一个包的源码。需要共享编译代码时应使用 npm 包。

## 更新、编译与开发者模式

正常模式下：

1. Codex Tweaks 发现源码、版本、依赖锁文件或编译器版本变化。
2. UI 显示更新原因，页面继续使用上一个有效产物。
3. 用户点击按钮后，有依赖的包执行 `npm ci --ignore-scripts --no-audit --no-fund`，然后调用固定版本 esbuild。
4. 成功后原子更新激活记录并重新注入；失败则保留上一个产物。

开发者模式下：

- 已启用包的普通源码变化会自动编译和激活。
- 自动编译使用 `npx --offline`，不下载编译器或依赖。
- 新包含 npm 依赖、包版本变化、`package-lock.json` 变化或构建配置变化仍需手动点击。
- 同一次失败的源码状态不会每 2 秒重复尝试；继续修改源码或手动编译可再次触发。

### 从本地安装

功能包页面可以直接选择一个包目录或 ZIP 压缩包。`package.json` 可以位于所选内容的根目录，也可以位于 ZIP 中唯一的一级目录。程序先复制或解压到隐藏暂存目录，完成全部校验后才原子移动到 `Tweaks/packages/`；校验失败不会留下半安装目录，也不会覆盖同名包。

本地安装使用与 Git 安装相同的清单校验，包括 JSON 结构、包 ID、SemVer、API v2、入口文件范围、功能包依赖版本与 npm 锁文件。导入时不复制 `.git` 和 `node_modules`，并拒绝符号链接、特殊文件、危险 ZIP 路径、超量文件和过大的解压内容。安装结果属于普通本地包，不登记远程来源或检查 Git 更新。

新安装包默认保持停用；如果已检测到 Node.js，用户确认本地安装后会继续下载锁定依赖并编译，但不会自动启用。

### 从 Git 安装与更新

功能包页面可填写 HTTPS、`ssh://` 或 `git@host:path` 仓库地址，并选择：

- `branch`：跟踪指定分支。
- `latestSemverTag`：选择满足全部依赖版本约束的最新 SemVer Tag。
- `tag`：固定指定 Tag。
- `githubLatestRelease`：跟踪 github.com 仓库的最新 Release。
- `githubRelease`：固定指定 GitHub Release 的 Tag。
- `commit`：固定完整 40 位 Commit SHA。

检出首先发生在临时目录；只有仓库通过与本地安装相同的包格式校验后，程序才会写入 `ManagedPackages/registry.json` 和 `ManagedPackages/packages.lock.json`。实际运行永远使用锁文件中的精确 Commit。分支、最新 Tag 和最新 Release 可以报告有可安装更新；固定 Tag、固定 Release 或 Commit 不会因远端引用漂移而被静默替换。普通更新检查只读取远端引用，不会拉取源码、运行 npm 或切换当前产物。用户点击“更新并编译”后，应用才会检出新 Commit、编译并在成功后切换。

远程安装不执行 Git 凭据交互、不加载子模块，也不执行 npm lifecycle scripts。仓库源码与 npm 依赖仍属于用户选择运行的第三方代码，启用前应自行审阅。

## 依赖与安全边界

- `dependencies` 非空时必须提供 `package-lock.json`，手动更新使用 `npm ci`而不是可漂移的 `npm install`。
- 安装时固定使用 `--ignore-scripts`，不执行依赖声明的 lifecycle scripts。
- 功能包依赖使用 `codexTweaks.packageDependencies`，应用只会为带有明确 `source` 的缺失依赖执行 Git 安装。
- Git 安装锁定精确 Commit；仓库 URL 不允许在 HTTPS 地址中内嵌用户名或密码，Git 命令强制使用非交互模式。
- 页面不会获得 Node.js 内置 API；`fs`、`child_process` 等 Node 专用模块不能在浏览器产物中运行。
- 运行时不从 CDN 加载脚本或样式，不绕过 Codex CSP。
- 包 JavaScript 拥有与 Codex 页面相同的渲染上下文权限，只应启用自己审阅过的代码和依赖。
- CDP 固定监听 `127.0.0.1:9335`，但本机其他进程仍可能连接；不要把该端口转发到外部网络。
- 目标筛选仅允许 `app://` 页面，不向应用内普通 `https://` 网页注入。

## 功能包开发 Skill 与交给 AI

项目中的 [`Skills/develop-codex-tweaks-package/SKILL.md`](Skills/develop-codex-tweaks-package/SKILL.md) 是功能包开发的唯一指导源，覆盖 API v2 清单、生命周期、npm 与功能包依赖、Git 来源、浏览器约束及验证要求。

概览页和菜单栏的“复制给 AI”会直接读取 App 中打包的同一份 `SKILL.md`，不会维护第二份提示词。修改 Skill 后重新生成并构建工程，UI 复制内容会自然同步。

## 开发

项目使用 Go 生成 Presentation Contract，再由 XcodeGen 生成 macOS 工程；固定工具与任务由 `mise.toml` 统一管理。

```sh
mise run generate        # 生成 Swift/C# Contract 与 CodexTweaks.xcodeproj
mise run contract:check  # 确认生成文件无漂移
mise run test            # 运行 Go 后端与 Swift 前端测试
mise run build           # 构建 Release App 到 dist/
mise run launch          # 启动已构建的 App
mise run workflows:lint  # 检查 GitHub Actions 与 Release 脚本
mise run verify          # 工作流检查、测试、Release 构建与工程差异检查
mise run clean           # 清理 build/ 与 dist/
```

CI 在 `main`、Pull Request 和手动触发时并行验证两套原生应用：macOS 运行 Go race tests、Swift tests 和 Release App 构建；Windows 在 Windows runner 上运行 Go 原生测试、x64/ARM64 self-contained WinUI 发布、Velopack 打包、PE 架构校验和本机架构 sidecar RPC 冒烟测试。

推送 `v*` 标签后，Release 工作流会并行构建三套 macOS DMG 与两套 Windows Velopack 发行包，全部通过后才创建或更新同一个 GitHub Release。带预发布后缀的标签进入 `beta` feed，普通 SemVer 标签进入 `stable` feed。

## 验证范围

自动测试覆盖 Go RPC、Presentation Contract、平台发现、配置读写、CDP 目标筛选、包验证与排序、用户优先级独立持久化、依赖拓扑与故障隔离、本地目录/ZIP 安装、Git Tag 安装和更新检查、源码/依赖/版本更新识别、产物选择加载、固定编译参数、Skill 同源读取、SemVer 和软件更新策略。Swift 测试覆盖 Go 协议解码、原生状态展示，以及 App 包内 sidecar 的可执行性和真实 `ping` 往返；Windows 验证脚本检查双架构 PE、随包资源、原生 sidecar 初始化与 Velopack 安装/更新 feed。

单元测试与本地构建成功不等于所有未来 Codex 版本都保持相同 DOM。真实视觉效果仍需在对应 Codex 版本中确认。

## 项目结构

```text
app/
├── Sources/                     # SwiftUI 薄前端、Go RPC 客户端与协议 DTO
├── Resources/Assets.xcassets/   # macOS AppIcon
├── Resources/Branding/          # 图标母版
├── Resources/Tweaks/packages/   # 内置 ct-sample 源码包
└── Tests/                       # 前后端协议与 App 内 sidecar 测试
backend/
├── cmd/codex-tweaks-backend/    # stdio RPC sidecar 入口
├── cmd/contractgen/             # Swift/C# Presentation Contract 生成器
└── internal/                    # Go 业务核心、系统适配、Presentation、RPC 与测试
contract/
└── presentation-contract.json  # 可审阅的统一协议清单
windows/
└── CodexTweaks.Windows/         # WinUI 3 原生薄前端与 Velopack 接入
Skills/
└── develop-codex-tweaks-package/ # 功能包开发 Skill，也是 UI 复制内容的唯一来源
.github/workflows/               # CI 与标签发布工作流
scripts/                         # macOS/Windows 构建、打包与产物校验脚本
mise.toml                        # 固定工具与统一任务
project.yml                      # XcodeGen 工程定义
```

## 许可证

本项目基于 [MIT 许可证](LICENSE) 开源。
