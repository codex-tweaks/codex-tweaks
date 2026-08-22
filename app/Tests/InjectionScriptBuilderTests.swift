import XCTest
@testable import CodexTweaks

final class InjectionScriptBuilderTests: XCTestCase {
    func testEscapesCSSAndJavaScriptPayload() {
        let payload = TweakPayload(
            css: "body::before { content: \"</style>\\n你好\"; }",
            javascript: "root.textContent = `ok`;",
            version: "abc123"
        )

        let script = InjectionScriptBuilder.injectionScript(payload: payload, forceGeneration: 0)

        XCTAssertTrue(script.contains("abc123"))
        XCTAssertTrue(script.contains("\\n"))
        XCTAssertTrue(script.contains("root.textContent = `ok`;"))
        XCTAssertTrue(script.contains("__CODEX_TWEAKS__"))
    }

    func testForceGenerationChangesEffectiveVersion() {
        let payload = TweakPayload(css: "", javascript: "", version: "base")

        let normal = InjectionScriptBuilder.injectionScript(payload: payload, forceGeneration: 0)
        let forced = InjectionScriptBuilder.injectionScript(payload: payload, forceGeneration: 7)

        XCTAssertTrue(normal.contains("\"base\""))
        XCTAssertTrue(forced.contains("\"base-force-7\""))
        XCTAssertNotEqual(normal, forced)
    }

    func testCleanupRemovesGlobalMarkerAndInjectedNodes() {
        XCTAssertTrue(InjectionScriptBuilder.cleanupScript.contains("delete globalThis[key]"))
        XCTAssertTrue(InjectionScriptBuilder.cleanupScript.contains("codex-tweaks-style"))
        XCTAssertTrue(InjectionScriptBuilder.cleanupScript.contains("codex-tweaks-root"))
    }
}
