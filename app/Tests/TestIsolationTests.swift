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
}
