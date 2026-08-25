# Codex Tweaks Sample

在 Codex 页面右下角显示一行轻量状态文字，用于确认 Codex Tweaks 已成功连接并注入功能包。

## 功能

- 显示“Codex Tweaks 已注入”状态
- 跟随功能包的启用、停用和重新注入生命周期自动添加或清理

## 安装

这个功能包随 Codex Tweaks 一起提供。连接 Codex 后，在“功能包”页面找到 `ct-sample`，完成编译并启用即可。

## 权限与安全

- Renderer：只向当前 Codex 页面添加一个不可交互的状态元素
- Node：未使用
- 网络：未使用

## 兼容性

- Codex Tweaks API：v3
- 平台：macOS 与 Windows

## 开发

源码入口为 `src/index.js`，样式位于 `src/style.css`。修改后请在 Codex Tweaks 的功能包页面重新编译，并验证启用、停用与重新注入均能正确清理状态元素。

## 许可证

随 Codex Tweaks 项目使用 [MIT License](https://github.com/cr-zhichen/codex-tweaks/blob/main/LICENSE)。
