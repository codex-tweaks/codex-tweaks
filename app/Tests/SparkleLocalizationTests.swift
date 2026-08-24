import Foundation
import XCTest
@testable import CodexTweaks

final class SparkleLocalizationTests: XCTestCase {
    func testSparkleUpdateUIUsesSimplifiedChinese() throws {
        XCTAssertEqual(Bundle.main.developmentLocalization, "zh-Hans")
        XCTAssertEqual(Bundle.main.preferredLocalizations.first, "zh-Hans")

        let sparkleBundle = try XCTUnwrap(Bundle(identifier: "org.sparkle-project.Sparkle"))
        XCTAssertTrue(sparkleBundle.localizations.contains("zh_CN"))
        XCTAssertEqual(
            sparkleBundle.localizedString(
                forKey: "Install Update",
                value: nil,
                table: "Sparkle"
            ),
            "安装更新"
        )
    }
}
