import XCTest
@testable import CodexTweaks

final class TweakPackageStoreTests: XCTestCase {
    func testEnablementReconciliationUsesFirstDiscoveryAsExistingBaseline() {
        let result = TweakPackageEnablementReconciliation.reconcile(
            discoveredPackageIDs: ["enabled", "disabled"],
            knownPackageIDs: nil,
            disabledPackageIDs: ["disabled"]
        )

        XCTAssertEqual(result.knownPackageIDs, ["enabled", "disabled"])
        XCTAssertEqual(result.disabledPackageIDs, ["disabled"])
        XCTAssertTrue(result.newlyDiscoveredPackageIDs.isEmpty)
    }

    func testEnablementReconciliationDisablesPackagesDiscoveredAfterBaseline() {
        let result = TweakPackageEnablementReconciliation.reconcile(
            discoveredPackageIDs: ["existing", "new-package"],
            knownPackageIDs: ["existing"],
            disabledPackageIDs: []
        )

        XCTAssertEqual(result.knownPackageIDs, ["existing", "new-package"])
        XCTAssertEqual(result.disabledPackageIDs, ["new-package"])
        XCTAssertEqual(result.newlyDiscoveredPackageIDs, ["new-package"])
    }

    func testFingerprintIsStableAndContentSensitive() {
        XCTAssertEqual(
            TweakPackageStore.fingerprint("same"),
            TweakPackageStore.fingerprint("same")
        )
        XCTAssertNotEqual(
            TweakPackageStore.fingerprint("before"),
            TweakPackageStore.fingerprint("after")
        )
    }

    func testLoadsPackagesByPriorityThenNameAndKeepsInvalidPackageIsolated() throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }

        try makePackage(in: fixture.store, directory: "zeta", name: "zeta", priority: 20)
        try makePackage(in: fixture.store, directory: "beta", name: "beta", priority: 10)
        try makePackage(in: fixture.store, directory: "alpha", name: "alpha", priority: 10)
        let invalidURL = fixture.store.packagesDirectoryURL
            .appendingPathComponent("broken", isDirectory: true)
        try FileManager.default.createDirectory(at: invalidURL, withIntermediateDirectories: true)
        try write("{not json", to: invalidURL.appendingPathComponent("package.json"))

        let packages = try fixture.store.loadPackages(bundle: Bundle(for: Self.self))

        XCTAssertEqual(packages.filter { $0.validationError == nil }.map(\.id), ["alpha", "beta", "zeta"])
        XCTAssertEqual(packages.filter { $0.validationError != nil }.map(\.directoryName), ["broken"])
    }

    func testDuplicatePackageNamesAreInvalidWithoutHidingOtherPackages() throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }

        try makePackage(in: fixture.store, directory: "first", name: "duplicate")
        try makePackage(in: fixture.store, directory: "second", name: "duplicate")
        try makePackage(in: fixture.store, directory: "healthy", name: "healthy")

        let packages = try fixture.store.loadPackages(bundle: Bundle(for: Self.self))
        let duplicates = packages.filter { $0.manifest?.name == "duplicate" }

        XCTAssertEqual(duplicates.count, 2)
        XCTAssertTrue(duplicates.allSatisfy { $0.validationError?.contains("重复") == true })
        XCTAssertNil(packages.first { $0.id == "healthy" }?.validationError)
    }

    func testBuildDispositionSeparatesSourceVersionAndDependencyUpdates() throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }

        let packageURL = try makePackage(
            in: fixture.store,
            directory: "demo",
            name: "demo",
            dependencies: ["example": "1.0.0"],
            lockfile: "lock-v1"
        )
        var package = try XCTUnwrap(
            try fixture.store.loadPackages(bundle: Bundle(for: Self.self)).first
        )
        XCTAssertEqual(
            package.buildDisposition(compilerVersion: TweakPackageStore.compilerVersion),
            .notBuilt
        )

        try activate(package, in: fixture.store, javascript: "module.exports = () => {}")
        package = try XCTUnwrap(
            try fixture.store.loadPackages(bundle: Bundle(for: Self.self)).first
        )
        XCTAssertEqual(
            package.buildDisposition(compilerVersion: TweakPackageStore.compilerVersion),
            .current
        )

        try write("export const changed = true;", to: packageURL.appendingPathComponent("src/index.js"))
        package = try XCTUnwrap(
            try fixture.store.loadPackages(bundle: Bundle(for: Self.self)).first
        )
        XCTAssertEqual(
            package.buildDisposition(compilerVersion: TweakPackageStore.compilerVersion),
            .sourceChanged
        )

        try write("lock-v2", to: packageURL.appendingPathComponent("package-lock.json"))
        package = try XCTUnwrap(
            try fixture.store.loadPackages(bundle: Bundle(for: Self.self)).first
        )
        XCTAssertEqual(
            package.buildDisposition(compilerVersion: TweakPackageStore.compilerVersion),
            .dependencyUpdate
        )

        try writeManifest(
            to: packageURL,
            name: "demo",
            version: "2.0.0",
            priority: 0,
            dependencies: ["example": "1.0.0"]
        )
        package = try XCTUnwrap(
            try fixture.store.loadPackages(bundle: Bundle(for: Self.self)).first
        )
        XCTAssertEqual(
            package.buildDisposition(compilerVersion: TweakPackageStore.compilerVersion),
            .versionUpdate
        )
        XCTAssertEqual(package.activeBuild?.record.packageVersion, "1.0.0")
    }

    func testPayloadUsesOnlyEnabledActiveBuildsInPackageOrder() throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }

        try makePackage(in: fixture.store, directory: "later", name: "later", priority: 20)
        try makePackage(in: fixture.store, directory: "earlier", name: "earlier", priority: 10)
        var packages = try fixture.store.loadPackages(bundle: Bundle(for: Self.self))
        try activate(try XCTUnwrap(packages.first { $0.id == "earlier" }), in: fixture.store, javascript: "earlier-js", css: "earlier-css")
        try activate(try XCTUnwrap(packages.first { $0.id == "later" }), in: fixture.store, javascript: "later-js", css: "later-css")
        packages = try fixture.store.loadPackages(bundle: Bundle(for: Self.self))

        let all = fixture.store.loadPayload(from: packages, disabledPackageIDs: [])
        XCTAssertEqual(all.payload.packages.map(\.id), ["earlier", "later"])
        XCTAssertEqual(all.payload.packages.map(\.javascript), ["earlier-js", "later-js"])
        XCTAssertEqual(all.payload.packages.map(\.css), ["earlier-css", "later-css"])

        let filtered = fixture.store.loadPayload(
            from: packages,
            disabledPackageIDs: ["earlier"]
        )
        XCTAssertEqual(filtered.payload.packages.map(\.id), ["later"])
        XCTAssertNotEqual(filtered.payload.version, all.payload.version)
    }

    func testUnreadableBuildDoesNotPreventOtherPackagePayload() throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }

        try makePackage(in: fixture.store, directory: "broken", name: "broken", priority: 10)
        try makePackage(in: fixture.store, directory: "healthy", name: "healthy", priority: 20)
        var packages = try fixture.store.loadPackages(bundle: Bundle(for: Self.self))
        try activate(try XCTUnwrap(packages.first { $0.id == "broken" }), in: fixture.store, javascript: "broken")
        try activate(try XCTUnwrap(packages.first { $0.id == "healthy" }), in: fixture.store, javascript: "healthy")
        packages = try fixture.store.loadPackages(bundle: Bundle(for: Self.self))
        let broken = try XCTUnwrap(packages.first { $0.id == "broken" })
        try FileManager.default.removeItem(at: try XCTUnwrap(broken.activeBuild).javaScriptURL)

        let result = fixture.store.loadPayload(from: packages, disabledPackageIDs: [])

        XCTAssertEqual(result.payload.packages.map(\.id), ["healthy"])
        XCTAssertNotNil(result.packageErrors["broken"])
    }

    func testPriorityOverridePersistsSeparatelyWithoutChangingManifest() throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }

        let packageURL = try makePackage(
            in: fixture.store,
            directory: "priority-demo",
            name: "priority-demo",
            priority: 42
        )
        try fixture.store.setPriorityOverride(-7, forPackageID: "priority-demo")

        var package = try XCTUnwrap(
            try fixture.store.loadPackages(bundle: Bundle(for: Self.self))
                .first { $0.id == "priority-demo" }
        )
        XCTAssertEqual(package.declaredPriority, 42)
        XCTAssertEqual(package.priorityOverride, -7)
        XCTAssertEqual(package.priority, -7)

        let manifest = try JSONDecoder().decode(
            TweakPackageManifest.self,
            from: Data(contentsOf: packageURL.appendingPathComponent("package.json"))
        )
        XCTAssertEqual(manifest.codexTweaks.priority, 42)
        XCTAssertEqual(
            try fixture.store.loadUserSettings().packages["priority-demo"]?.priorityOverride,
            -7
        )

        try fixture.store.setPriorityOverride(nil, forPackageID: "priority-demo")
        package = try XCTUnwrap(
            try fixture.store.loadPackages(bundle: Bundle(for: Self.self))
                .first { $0.id == "priority-demo" }
        )
        XCTAssertNil(package.priorityOverride)
        XCTAssertEqual(package.priority, 42)
    }

    func testPriorityOverrideMatchingManifestIsRemovedWhenPackagesLoad() throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }

        try makePackage(
            in: fixture.store,
            directory: "priority-demo",
            name: "priority-demo",
            priority: 42
        )
        try fixture.store.setPriorityOverride(42, forPackageID: "priority-demo")

        let package = try XCTUnwrap(
            try fixture.store.loadPackages(bundle: Bundle(for: Self.self))
                .first { $0.id == "priority-demo" }
        )

        XCTAssertNil(package.priorityOverride)
        XCTAssertEqual(package.priority, 42)
        XCTAssertNil(
            try fixture.store.loadUserSettings().packages["priority-demo"]
        )
    }

    private func makeFixture() throws -> (rootURL: URL, store: TweakPackageStore) {
        let rootURL = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        let store = TweakPackageStore(
            applicationSupportURL: rootURL.appendingPathComponent("Application Support"),
            cachesURL: rootURL.appendingPathComponent("Caches")
        )
        try store.prepareUserPackages(bundle: Bundle(for: Self.self))
        return (rootURL, store)
    }

    @discardableResult
    private func makePackage(
        in store: TweakPackageStore,
        directory: String,
        name: String,
        version: String = "1.0.0",
        priority: Int = 0,
        dependencies: [String: String] = [:],
        lockfile: String? = nil
    ) throws -> URL {
        let packageURL = store.packagesDirectoryURL
            .appendingPathComponent(directory, isDirectory: true)
        try FileManager.default.createDirectory(
            at: packageURL.appendingPathComponent("src", isDirectory: true),
            withIntermediateDirectories: true
        )
        try write("export function activate() {}", to: packageURL.appendingPathComponent("src/index.js"))
        try writeManifest(
            to: packageURL,
            name: name,
            version: version,
            priority: priority,
            dependencies: dependencies
        )
        if let lockfile {
            try write(lockfile, to: packageURL.appendingPathComponent("package-lock.json"))
        }
        return packageURL
    }

    private func writeManifest(
        to packageURL: URL,
        name: String,
        version: String,
        priority: Int,
        dependencies: [String: String]
    ) throws {
        let manifest = TweakPackageManifest(
            name: name,
            version: version,
            description: "\(name) description",
            type: "module",
            dependencies: dependencies,
            codexTweaks: .init(entry: "src/index.js", priority: priority)
        )
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        try encoder.encode(manifest).write(
            to: packageURL.appendingPathComponent("package.json"),
            options: .atomic
        )
    }

    private func activate(
        _ package: TweakPackage,
        in store: TweakPackageStore,
        javascript: String,
        css: String? = nil
    ) throws {
        let buildName = "test-\(UUID().uuidString)"
        let buildURL = try store.buildsDirectoryURL(forPackageID: package.id)
            .appendingPathComponent(buildName, isDirectory: true)
        try FileManager.default.createDirectory(at: buildURL, withIntermediateDirectories: true)
        try write(javascript, to: buildURL.appendingPathComponent("bundle.js"))
        if let css { try write(css, to: buildURL.appendingPathComponent("bundle.css")) }
        try store.activateBuild(
            TweakPackageBuildRecord(
                packageID: package.id,
                packageVersion: try XCTUnwrap(package.manifest).version,
                sourceFingerprint: try XCTUnwrap(package.sourceFingerprint),
                dependencyFingerprint: try XCTUnwrap(package.dependencyFingerprint),
                compilerVersion: TweakPackageStore.compilerVersion,
                nodeVersion: "v24.0.0",
                buildDirectoryName: buildName,
                hasCSS: css != nil,
                builtAt: Date()
            )
        )
    }

    private func write(_ value: String, to url: URL) throws {
        try FileManager.default.createDirectory(
            at: url.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        try value.write(to: url, atomically: true, encoding: .utf8)
    }
}
