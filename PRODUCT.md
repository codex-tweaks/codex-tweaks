# Product

<!-- impeccable:product-schema 1 -->

## Platform

adaptive

## Users

当前可确认的主要使用者，是在自己的 macOS 或 Windows 电脑上运行 Codex，并希望集中查看连接状态、管理本地界面增强与处理更新的桌面用户和开发者。是否进一步面向完全非技术用户尚未决定。

## Product Purpose

Codex Tweaks 让用户无需直接编辑 Codex 应用文件，即可按功能包安装、校验、编译、启停和更新本地界面增强，并观察 Codex 连接、注入和运行状态。成功意味着同一套业务行为在 macOS 与 Windows 上保持一致，同时保留各平台的原生交互体验。

## Positioning

产品以一个 Go sidecar 同时拥有包管理、构建、依赖解析、CDP 注入、更新状态和 Presentation Contract；macOS 与 Windows 前端只翻译统一状态和动作，不复制业务判断，也不修改 Codex 的应用包或 `app.asar`。

## Operating Context

- 用户在自己的桌面会话中同时运行 Codex Tweaks 与 Codex。
- Node.js 只用于功能包构建；Git 用于远程包安装和更新。
- 功能包从本地目录、ZIP 或 Git 来源进入，编译成功后原子切换。
- Windows 版本通过 Velopack 在当前用户目录安装、卸载和整体更新；macOS 使用原生应用与 DMG。

## Capabilities and Constraints

- 所有业务逻辑使用 Go，不使用 Swift 或 C# 实现业务判断。
- 单一 Presentation Contract 提供业务状态、DTO、可用操作、文案和跨平台设计令牌，并生成 Swift 与 C# 类型。
- macOS 使用 SwiftUI 原生薄前端。
- Windows 使用 WinUI 3 原生薄前端、独立 XAML 页面和 ResourceDictionary 设计系统；C# 仅负责视图绑定、Windows 平台交互和协议调用。
- Windows 应用是 self-contained、unpackaged 构建，Go sidecar 与前端进入同一个 Velopack 版本包。
- Windows 安装、卸载和更新不要求管理员权限；初期允许未签名，后续能够接入托管代码签名而不改变应用协议。
- 不要求兼容迁移前的内部实现或旧包格式。

## Brand Commitments

产品名称为 Codex Tweaks。现有应用图标和中文界面文案是当前可用资产；产品明确说明自己是非官方本地定制工具，不伪造与 OpenAI 或 Codex 的官方关系。

macOS 与 Windows 共享相同的信息架构和大致内容分区，但不共享一套跨平台控件皮肤：macOS 跟随当前系统的 SwiftUI 原生设计，Windows 跟随当前 Windows 11 / WinUI 3 Fluent 原生设计。两端不应出现明显不同的页面顺序、功能分组或主要操作位置。

## Evidence on Hand

- `README.md` 记录现有功能、架构、安全边界与构建方式。
- `app/Resources/Branding/CodexTweaks-AppIcon.png` 是当前应用图标源。
- `contract/presentation-contract.json` 及生成的 Swift/C# 文件是跨平台协议证据。
- 仓库没有客户评价、使用量、性能基准或商业承诺，未来界面不得虚构这些内容。

## Product Principles

- 一套业务事实，两套真正原生的界面。
- 明确展示实际状态和可执行动作，不用前端猜测后端能力。
- 下载、编译和切换均由用户意图触发，并保留上一个可用结果。
- 本地优先、权限克制，不修改 Codex 应用本体。
- 前端和 sidecar 作为一个版本整体交付与验证。

## Accessibility & Inclusion

原生界面应支持键盘导航、系统缩放、高对比度、浅色/深色主题以及 Windows 的减少动画设置；状态不能只依赖颜色表达。
