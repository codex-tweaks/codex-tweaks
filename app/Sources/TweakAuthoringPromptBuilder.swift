import AppKit
import Foundation

enum TweakAuthoringPromptBuilder {
    static func makePrompt(tweaksDirectoryURL: URL) -> String {
        let directoryPath = tweaksDirectoryURL.standardizedFileURL.path

        return """
        # Codex Tweaks 界面增强任务

        请根据我在文末填写的需求，完成 Codex Tweaks 自定义资源的实际编写。

        本地资源目录：
        \(directoryPath)

        ## 资源契约

        - CSS 加载顺序：vendor/**/*.css → ui.css → styles/**/*.css。
        - JavaScript 加载顺序：vendor/**/*.js → ui.js → scripts/**/*.js。
        - 同一目录按相对路径升序递归加载；使用 10-、20- 这样的数字前缀表达依赖顺序。
        - ui.css 和 ui.js 只作为兼容入口。新增功能应拆分为 styles/NN-feature.css 与 scripts/NN-feature.js，不要重新把所有内容堆进入口文件。
        - 每个 JavaScript 文件在独立函数作用域中执行，可直接使用 root 和 api；不要依赖其他文件的局部变量。

        ## JavaScript 与生命周期规则

        - 跨模块共享第三方库时使用 api.registerLibrary(name, value)、api.hasLibrary(name) 和 api.getLibrary(name)。
        - 用唯一的 data-codex-tweaks-* 属性标记和限定自己管理的 DOM/CSS，避免污染 Codex 的其他界面。
        - 实现必须可重复执行，并能处理目标元素暂时不存在、页面刷新和 Codex DOM 变化。
        - 优先使用 role、aria 属性和稳定结构定位元素，不要只依赖易变化的哈希 class。
        - 所有 MutationObserver、事件监听器、定时器和额外插入的节点都必须通过 api.registerCleanup(callback) 清理。
        - 尽量复用 Codex 原生控件与事件链，不要复制或替换原生业务逻辑。

        ## 第三方库规则

        - 先复用已经通过 api 注册的库。
        - 新依赖必须是固定版本、经过审阅的本地浏览器 IIFE/UMD bundle，放入 vendor；保留许可证和版本说明。
        - Codex CSP 下不要使用远程 CDN、data: 动态模块、静态 import/export 或未经打包的 ESM/CJS。
        - 第三方库与适配注册、实际功能代码应分文件，不要把压缩库源码混入功能模块。

        ## 样式与验证

        - 样式限定在功能标记下，兼顾浅色/深色、视口边界、长文本和 prefers-reduced-motion；不要无意截断重要内容。
        - 对修改过的 JavaScript 至少运行 node --check；有相关项目测试时运行对应的非破坏性测试。
        - 没有实际视觉证据时不要声称界面已经视觉正确。完成后说明修改了哪些文件、验证结果和仍需用户确认的界面表现。

        ## 我的具体需求

        【请在粘贴后把这里替换为你想实现或修复的内容】
        """
    }

    @discardableResult
    static func copyPrompt(
        tweaksDirectoryURL: URL,
        to pasteboard: NSPasteboard = .general
    ) -> Bool {
        pasteboard.clearContents()
        return pasteboard.setString(
            makePrompt(tweaksDirectoryURL: tweaksDirectoryURL),
            forType: .string
        )
    }
}
