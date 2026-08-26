import Foundation
import XCTest

final class SparkleLocalizationTests: XCTestCase {
    func testHostAppAndSparkleExposeSupportedLocalizations() throws {
        let appBundle = try BuiltAppBundle.load(for: Self.self)
        XCTAssertEqual(appBundle.developmentLocalization, "en")
        for localization in ["en", "zh-Hans", "zh-Hant", "ja", "ko"] {
            XCTAssertTrue(appBundle.localizations.contains(localization), localization)
        }
        XCTAssertEqual(
            Bundle.preferredLocalizations(
                from: appBundle.localizations,
                forPreferences: ["zh-Hant-HK"]
            ).first,
            "zh-Hant"
        )
        XCTAssertEqual(
            Bundle.preferredLocalizations(
                from: appBundle.localizations,
                forPreferences: ["ja-JP"]
            ).first,
            "ja"
        )

        let sparkleURL = appBundle.bundleURL
            .appendingPathComponent("Contents/Frameworks/Sparkle.framework", isDirectory: true)
        let sparkleBundle = try XCTUnwrap(Bundle(url: sparkleURL))
        for localization in ["en", "zh_CN", "zh_TW", "ja", "ko"] {
            XCTAssertTrue(sparkleBundle.localizations.contains(localization), localization)
        }
        let baseURL = try XCTUnwrap(
            sparkleBundle.url(forResource: "Base", withExtension: "lproj")
        )
        let baseBundle = try XCTUnwrap(Bundle(url: baseURL))
        XCTAssertEqual(
            baseBundle.localizedString(
                forKey: "Install Update",
                value: nil,
                table: "Sparkle"
            ),
            "Install Update"
        )
    }
}
