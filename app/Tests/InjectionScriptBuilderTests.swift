import XCTest
@testable import CodexTweaks

final class InjectionScriptBuilderTests: XCTestCase {
    func testEscapesPackageCSSAndJavaScriptPayload() {
        let payload = TweakPayload(
            packages: [
                package(
                    id: "sample",
                    css: "body::before { content: \"</style>\\n你好\"; }",
                    javascript: "module.exports.activate = ({ root }) => { root.textContent = `ok`; };"
                ),
            ],
            version: "abc123"
        )

        let script = InjectionScriptBuilder.injectionScript(payload: payload, forceGeneration: 0)

        XCTAssertTrue(script.contains("abc123"))
        XCTAssertTrue(script.contains("\\n"))
        XCTAssertTrue(script.contains("root.textContent = `ok`;"))
        XCTAssertTrue(script.contains("__CODEX_TWEAKS__"))
        XCTAssertTrue(script.contains("host.style.display = \"contents\""))
        XCTAssertTrue(script.contains("Package entry must export activate(context)"))
    }

    func testCreatesAllPackageStylesBeforeExecutingAnyPackage() throws {
        let payload = TweakPayload(
            packages: [
                package(id: "first", css: "first-css", javascript: "first-js"),
                package(id: "second", css: "second-css", javascript: "second-js"),
            ],
            version: "order"
        )

        let script = InjectionScriptBuilder.injectionScript(payload: payload, forceGeneration: 0)
        let firstCSS = try XCTUnwrap(script.range(of: "first-css"))
        let secondCSS = try XCTUnwrap(script.range(of: "second-css"))
        let firstJS = try XCTUnwrap(script.range(of: "first-js"))
        let secondJS = try XCTUnwrap(script.range(of: "second-js"))

        XCTAssertLessThan(firstCSS.lowerBound, secondCSS.lowerBound)
        XCTAssertLessThan(secondCSS.lowerBound, firstJS.lowerBound)
        XCTAssertLessThan(firstJS.lowerBound, secondJS.lowerBound)
    }

    func testPackageRuntimeFailuresAreCaughtAndCleanedIndependently() {
        let payload = TweakPayload(
            packages: [package(id: "broken", css: "", javascript: "throw new Error('boom')")],
            version: "errors"
        )

        let script = InjectionScriptBuilder.injectionScript(payload: payload, forceGeneration: 0)

        XCTAssertTrue(script.contains("const packageErrors = []"))
        XCTAssertTrue(script.contains("cleanupPackageState(state)"))
        XCTAssertTrue(script.contains("packageStates.delete(packageID)"))
        XCTAssertTrue(script.contains("packageErrors.push({ id: packageID, name: packageName, message })"))
        XCTAssertTrue(script.contains("for (const state of [...packageStates.values()].reverse())"))
    }

    func testPackageContextExposesRootAndCleanupAPI() {
        let script = InjectionScriptBuilder.injectionScript(
            payload: TweakPayload(
                packages: [package(id: "sample", css: "", javascript: "sample-js")],
                version: "context"
            ),
            forceGeneration: 0
        )

        XCTAssertTrue(script.contains("registerCleanup(callback)"))
        XCTAssertTrue(script.contains("registerLibrary(name, value)"))
        XCTAssertTrue(script.contains("root: state.root"))
        XCTAssertTrue(script.contains("dependencies,"))
        XCTAssertTrue(script.contains("Package dependency unavailable"))
        XCTAssertTrue(script.contains("onCleanup: api.registerCleanup"))
        XCTAssertTrue(script.contains("const cleanup = await activate(context)"))
    }

    func testForceGenerationChangesEffectiveVersion() {
        let payload = TweakPayload(packages: [], version: "base")

        let normal = InjectionScriptBuilder.injectionScript(payload: payload, forceGeneration: 0)
        let forced = InjectionScriptBuilder.injectionScript(payload: payload, forceGeneration: 7)

        XCTAssertTrue(normal.contains("\"base\""))
        XCTAssertTrue(forced.contains("\"base-force-7\""))
        XCTAssertNotEqual(normal, forced)
    }

    func testCleanupRemovesGlobalMarkerAndInjectedRoot() {
        XCTAssertTrue(InjectionScriptBuilder.cleanupScript.contains("delete globalThis[key]"))
        XCTAssertTrue(InjectionScriptBuilder.cleanupScript.contains("codex-tweaks-root"))
    }

    private func package(
        id: String,
        css: String,
        javascript: String
    ) -> CompiledTweakPackage {
        CompiledTweakPackage(
            id: id,
            name: id,
            version: "1.0.0",
            buildFingerprint: "fingerprint-\(id)",
            dependencyIDs: [],
            css: css,
            javascript: javascript
        )
    }
}
