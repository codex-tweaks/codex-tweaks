import Foundation
import XCTest

enum BuiltAppBundle {
    static func load(for testCase: AnyClass) throws -> Bundle {
        let productsDirectory = Bundle(for: testCase).bundleURL.deletingLastPathComponent()
        let appURL = productsDirectory.appendingPathComponent("Codex Tweaks.app", isDirectory: true)
        return try XCTUnwrap(
            Bundle(url: appURL),
            "未找到待验证的 Codex Tweaks.app：\(appURL.path)"
        )
    }
}
