import Foundation
import XCTest

final class TestIsolationTests: XCTestCase {
    func testTestsRunWithoutAnApplicationHost() {
        XCTAssertNil(ProcessInfo.processInfo.environment["TEST_HOST"])
        XCTAssertFalse(Bundle.main.bundleURL.path.contains("/Codex Tweaks.app"))
        XCTAssertFalse(
            CommandLine.arguments.first?.contains("/Codex Tweaks.app/") ?? false
        )
    }

    func testPackagePageAvoidsLazyStackGeometryTransactions() throws {
        let source = try source(named: "MainWindowView.swift")
        let packageStart = try XCTUnwrap(source.range(of: "private struct TweakPackagesView"))
        let packageEnd = try XCTUnwrap(
            source.range(
                of: "private struct GitPackageInstallView",
                range: packageStart.upperBound..<source.endIndex
            )
        )
        let packageSource = source[packageStart.lowerBound..<packageEnd.lowerBound]

        XCTAssertFalse(packageSource.contains("LazyVStack("))
        XCTAssertTrue(packageSource.contains("VStack(alignment: .leading, spacing: 0)"))
    }

    func testMainWindowCloseKeepsTheApplicationRunning() throws {
        let source = try source(named: "CodexTweaksApp.swift")
        let closeStart = try XCTUnwrap(
            source.range(of: "func windowShouldClose(_ sender: NSWindow) -> Bool")
        )
        let terminateStart = try XCTUnwrap(
            source.range(of: "func applicationShouldTerminateAfterLastWindowClosed")
        )
        let reopenStart = try XCTUnwrap(
            source.range(of: "func applicationShouldHandleReopen")
        )
        let closeSource = source[closeStart.lowerBound..<terminateStart.lowerBound]
        let terminateSource = source[terminateStart.lowerBound..<reopenStart.lowerBound]

        XCTAssertTrue(source.contains("NSWindowDelegate"))
        XCTAssertTrue(closeSource.contains("sender.orderOut(nil)"))
        XCTAssertTrue(closeSource.contains("return false"))
        XCTAssertTrue(terminateSource.contains("false"))
    }

    private func source(named name: String) throws -> String {
        let sourceURL = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent("Sources", isDirectory: true)
            .appendingPathComponent(name)
        return try String(contentsOf: sourceURL, encoding: .utf8)
    }
}
