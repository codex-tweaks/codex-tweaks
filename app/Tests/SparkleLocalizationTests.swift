import Foundation
import XCTest

final class SparkleLocalizationTests: XCTestCase {
    func testSparkleUpdateUIUsesSimplifiedChinese() throws {
        let appBundle = try BuiltAppBundle.load(for: Self.self)
        XCTAssertEqual(appBundle.developmentLocalization, "zh-Hans")
        XCTAssertEqual(appBundle.preferredLocalizations.first, "zh-Hans")

        let sparkleURL = appBundle.bundleURL
            .appendingPathComponent("Contents/Frameworks/Sparkle.framework", isDirectory: true)
        let sparkleBundle = try XCTUnwrap(Bundle(url: sparkleURL))
        XCTAssertTrue(sparkleBundle.localizations.contains("zh_CN"))
        let simplifiedChineseURL = try XCTUnwrap(
            sparkleBundle.url(forResource: "zh_CN", withExtension: "lproj")
        )
        let simplifiedChineseBundle = try XCTUnwrap(Bundle(url: simplifiedChineseURL))
        XCTAssertEqual(
            simplifiedChineseBundle.localizedString(
                forKey: "Install Update",
                value: nil,
                table: "Sparkle"
            ),
            "安装更新"
        )
    }
}
