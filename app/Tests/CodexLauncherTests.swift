import XCTest
@testable import CodexTweaks

@MainActor
final class CodexLauncherTests: XCTestCase {
    func testRemoteDebuggingIsRestrictedToLoopback() {
        XCTAssertTrue(
            CodexLauncher.debuggingArguments.contains(
                "--remote-debugging-address=127.0.0.1"
            )
        )
        XCTAssertTrue(
            CodexLauncher.debuggingArguments.contains(
                "--remote-debugging-port=9335"
            )
        )
        XCTAssertFalse(CodexLauncher.debuggingArguments.contains { $0.contains("0.0.0.0") })
    }
}
