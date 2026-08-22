import AppKit
import XCTest
@testable import CodexTweaks

final class TweakAuthoringPromptBuilderTests: XCTestCase {
    func testPromptContainsDynamicPathAndCoreContracts() {
        let directoryURL = URL(fileURLWithPath: "/tmp/Custom Tweaks", isDirectory: true)

        let prompt = TweakAuthoringPromptBuilder.makePrompt(
            tweaksDirectoryURL: directoryURL
        )

        XCTAssertTrue(prompt.contains("/tmp/Custom Tweaks"))
        XCTAssertTrue(prompt.contains("vendor/**/*.js → ui.js → scripts/**/*.js"))
        XCTAssertTrue(prompt.contains("styles/NN-feature.css"))
        XCTAssertTrue(prompt.contains("api.registerCleanup(callback)"))
        XCTAssertTrue(prompt.contains("api.getLibrary(name)"))
        XCTAssertTrue(prompt.contains("IIFE/UMD"))
        XCTAssertTrue(prompt.contains("不要使用远程 CDN"))
        XCTAssertTrue(prompt.contains("node --check"))
        XCTAssertTrue(prompt.contains("我的具体需求"))
        XCTAssertFalse(prompt.contains("## 执行边界"))
        XCTAssertFalse(prompt.contains("如果你能访问本地文件"))
        XCTAssertFalse(prompt.contains("默认只允许修改"))
    }

    func testCopyPromptWritesCompletePromptToPasteboard() {
        let directoryURL = URL(fileURLWithPath: "/tmp/Tweaks", isDirectory: true)
        let pasteboard = NSPasteboard(
            name: NSPasteboard.Name("TweakAuthoringPromptBuilderTests-\(UUID())")
        )
        defer { pasteboard.clearContents() }

        XCTAssertTrue(
            TweakAuthoringPromptBuilder.copyPrompt(
                tweaksDirectoryURL: directoryURL,
                to: pasteboard
            )
        )
        XCTAssertEqual(
            pasteboard.string(forType: .string),
            TweakAuthoringPromptBuilder.makePrompt(
                tweaksDirectoryURL: directoryURL
            )
        )
    }
}
