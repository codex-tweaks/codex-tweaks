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

    func testLoadsVendorEntryAndFeatureSourcesInDeterministicOrder() throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }

        try write("vendor-b", to: fixture.store.vendorDirectoryURL.appendingPathComponent("20-b.js"))
        try write("vendor-a", to: fixture.store.vendorDirectoryURL.appendingPathComponent("10-a.js"))
        try write("feature-b", to: fixture.store.scriptsDirectoryURL.appendingPathComponent("20-b.js"))
        try write("feature-a", to: fixture.store.scriptsDirectoryURL.appendingPathComponent("10-a.js"))
        try write("ignored", to: fixture.store.scriptsDirectoryURL.appendingPathComponent("notes.txt"))

        try write("vendor-css", to: fixture.store.vendorDirectoryURL.appendingPathComponent("10-lib.css"))
        try write("feature-css", to: fixture.store.stylesDirectoryURL.appendingPathComponent("10-feature.css"))

        let payload = try fixture.store.loadPayload(
            bundle: Bundle(for: Self.self)
        )

        let orderedMarkers = [
            "api.runModule(\"vendor/10-a.js\"",
            "api.runModule(\"vendor/20-b.js\"",
            "api.runModule(\"ui.js\"",
            "api.runModule(\"scripts/10-a.js\"",
            "api.runModule(\"scripts/20-b.js\"",
        ]
        var previousIndex = payload.javascript.startIndex
        for marker in orderedMarkers {
            let range = try XCTUnwrap(
                payload.javascript.range(of: marker, range: previousIndex..<payload.javascript.endIndex)
            )
            previousIndex = range.upperBound
        }
        XCTAssertTrue(payload.javascript.contains("function (api, root)"))
        XCTAssertTrue(payload.javascript.contains("vendor-a"))
        XCTAssertTrue(payload.javascript.contains("entry-js"))
        XCTAssertTrue(payload.javascript.contains("feature-b"))
        XCTAssertEqual(payload.css, "vendor-css\nentry-css\nfeature-css")
        XCTAssertFalse(payload.javascript.contains("ignored"))
    }

    func testModuleContentChangesPayloadVersion() throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }
        let moduleURL = fixture.store.scriptsDirectoryURL
            .appendingPathComponent("10-feature.js")

        try write("before", to: moduleURL)
        let before = try fixture.store.loadPayload(bundle: Bundle(for: Self.self))

        try write("after", to: moduleURL)
        let after = try fixture.store.loadPayload(bundle: Bundle(for: Self.self))

        XCTAssertNotEqual(before.version, after.version)
        XCTAssertTrue(after.javascript.contains("after"))
    }

    func testLegacyEntryFilesStillLoadWithoutModules() throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }

        let payload = try fixture.store.loadPayload(bundle: Bundle(for: Self.self))

        XCTAssertEqual(payload.css, "entry-css")
        XCTAssertTrue(payload.javascript.contains("api.runModule(\"ui.js\""))
        XCTAssertTrue(payload.javascript.contains("entry-js"))
    }

    private func makeFixture() throws -> (rootURL: URL, store: TweakResourceStore) {
        let rootURL = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        let store = TweakResourceStore(applicationSupportURL: rootURL)

        try FileManager.default.createDirectory(
            at: store.tweaksDirectoryURL,
            withIntermediateDirectories: true
        )
        try write("entry-css", to: store.customCSSURL)
        try write("entry-js", to: store.customJavaScriptURL)

        return (rootURL, store)
    }

    private func write(_ value: String, to url: URL) throws {
        try FileManager.default.createDirectory(
            at: url.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        try value.write(to: url, atomically: true, encoding: .utf8)
    }

}
