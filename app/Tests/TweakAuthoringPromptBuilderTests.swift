import AppKit
import XCTest
@testable import CodexTweaks

final class TweakAuthoringPromptBuilderTests: XCTestCase {
    func testPromptIsExactlyTheProjectSkill() throws {
        let directoryURL = URL(fileURLWithPath: "/tmp/Custom Tweaks", isDirectory: true)
        let prompt = TweakAuthoringPromptBuilder.makePrompt(
            tweaksDirectoryURL: directoryURL
        )
        let skill = try XCTUnwrap(TweakAuthoringPromptBuilder.skillContents())

        XCTAssertEqual(prompt, skill)
        XCTAssertTrue(prompt.contains("apiVersion\": 2"))
        XCTAssertTrue(prompt.contains("packageDependencies"))
        XCTAssertTrue(prompt.contains("dependencies.get"))
        XCTAssertTrue(prompt.contains("Omitting `source` declares a local-only dependency"))
        XCTAssertTrue(prompt.contains("A Git URL alone is not a valid canonical dependency declaration"))
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
            TweakAuthoringPromptBuilder.skillContents()
        )
    }
}
