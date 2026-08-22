import XCTest
@testable import CodexTweaks

final class TweakResourceStoreTests: XCTestCase {
    func testFingerprintIsStableAndContentSensitive() {
        XCTAssertEqual(
            TweakResourceStore.fingerprint("same"),
            TweakResourceStore.fingerprint("same")
        )
        XCTAssertNotEqual(
            TweakResourceStore.fingerprint("before"),
            TweakResourceStore.fingerprint("after")
        )
    }

}
