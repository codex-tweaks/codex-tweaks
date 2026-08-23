# Codex Tweaks 开发约定

## 架构边界

- Go 是唯一业务后端：进程发现与控制、CDP、注入、功能包、Git、更新、日志、校验和业务状态都写在 `backend/`；不要在 Swift 或 C# 中复制业务判断。
- macOS 使用 SwiftUI，Windows 使用 WinUI 3 + XAML。两端都是原生薄前端，只负责布局、窗口和平台交互，通过逐行 JSON RPC 使用同一个 Go sidecar。
- 文案、DTO、可用操作、状态和设计令牌以 Go Presentation Contract 为准。修改来源后运行 `mise run generate`；生成的 Swift、C#、XAML 与 JSON 文件禁止手改。
- 前后端协议可直接升级，不保留旧版兼容层，除非任务明确要求。

## 双端实现流程

1. 先在 Go 中完成业务与协议，并补齐 Go 测试。
2. 运行 `mise run generate`，再分别实现 SwiftUI 与 WinUI 3 的原生表现；两端保持相同的信息层级、页面顺序、状态语义和主要操作位置。
3. macOS 至少运行 `mise run test` 与 `mise run build`。Windows 依次运行 `scripts/build-windows.ps1`、`scripts/package-windows.ps1` 和 `scripts/verify-windows.ps1`，同时覆盖 x64、ARM64。
4. 合并或发布前执行 `mise run verify`，确认生成文件无漂移、双端 CI 全绿，再创建 Beta Tag 与 GitHub Release。

## 视觉与交互

- 遵循平台原生设计：macOS 使用系统 SwiftUI 控件与材质；Windows 使用 Windows 11 Fluent、WinUI 3 控件和 `ResourceDictionary`，不制作跨平台仿制皮肤。
- 内容在宽窗口中限制最大宽度并居中；窄窗口保持完整可用。每页只保留一个主要纵向滚动容器，避免同方向嵌套滚动，所有内容在最小窗口尺寸下都必须可达。
- 功能包使用紧凑单行排布；图标使用系统图标和统一小尺寸；标签应紧邻对应的开关或输入控件。
- 共用间距、圆角、颜色和尺寸优先来自 Presentation Contract；窗口最小/默认尺寸等平台令牌由 Go 按操作系统分别提供，调整一端时不得改变另一端。

## Windows 交付约束

- Windows 保持 self-contained、unpackaged WinUI 3 前端 + 无控制台窗口的 Go sidecar，并由 Velopack 按用户安装和整体替换。
- 发布产物必须包含 `.pri`、XAML 资源、Go sidecar 与运行时；安装、更新、卸载和应用启动都需在真实 Windows 环境验证。
