import XCTest
@testable import CodexTweaks

final class TweakPackageLocalInstallerTests: XCTestCase {
    func testInstallsDirectoryAsLocalPackageAndDropsGeneratedTrees() async throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }
        let sourceURL = fixture.rootURL.appendingPathComponent("source-package", isDirectory: true)
        try writePackage(name: "local-directory-sample", to: sourceURL)
        try write("ignored", to: sourceURL.appendingPathComponent(".git/config"))
        try write("ignored", to: sourceURL.appendingPathComponent("node_modules/demo/index.js"))

        let result = try await fixture.installer.install(from: sourceURL)

        XCTAssertEqual(result.packageID, "local-directory-sample")
        XCTAssertEqual(result.directoryURL.lastPathComponent, "local-directory-sample")
        XCTAssertTrue(FileManager.default.fileExists(
            atPath: result.directoryURL.appendingPathComponent("package.json").path
        ))
        XCTAssertFalse(FileManager.default.fileExists(
            atPath: result.directoryURL.appendingPathComponent(".git").path
        ))
        XCTAssertFalse(FileManager.default.fileExists(
            atPath: result.directoryURL.appendingPathComponent("node_modules").path
        ))

        let installed = try fixture.store.loadPackages(bundle: Bundle(for: Self.self))
            .first { $0.id == result.packageID }
        XCTAssertNotNil(installed)
        XCTAssertFalse(try XCTUnwrap(installed).isManaged)
    }

    func testInstallsZipContainingOneWrapperDirectory() async throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }
        let wrapperURL = fixture.rootURL
            .appendingPathComponent("archive-source", isDirectory: true)
            .appendingPathComponent("repository-main", isDirectory: true)
        try writePackage(name: "local-zip-sample", to: wrapperURL)
        let archiveURL = fixture.rootURL.appendingPathComponent("local-package.zip")
        try run(
            executableURL: URL(fileURLWithPath: "/usr/bin/ditto"),
            arguments: [
                "-c", "-k", "--sequesterRsrc", "--keepParent",
                wrapperURL.path, archiveURL.path,
            ]
        )

        let result = try await fixture.installer.install(from: archiveURL)

        XCTAssertEqual(result.packageID, "local-zip-sample")
        XCTAssertTrue(FileManager.default.fileExists(
            atPath: result.directoryURL.appendingPathComponent("src/index.js").path
        ))
    }

    func testRejectsInvalidPackageBeforeWritingDestination() async throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }
        let sourceURL = fixture.rootURL.appendingPathComponent("invalid-package", isDirectory: true)
        try writePackage(
            name: "missing-lock-sample",
            dependencies: ["demo": "1.0.0"],
            to: sourceURL
        )

        do {
            _ = try await fixture.installer.install(from: sourceURL)
            XCTFail("Expected invalid package error")
        } catch {
            XCTAssertTrue(error.localizedDescription.contains("package-lock.json"))
        }
        XCTAssertFalse(FileManager.default.fileExists(
            atPath: fixture.store.packagesDirectoryURL
                .appendingPathComponent("missing-lock-sample")
                .path
        ))
    }

    func testRejectsDuplicatePackageIDWithoutReplacingExistingSource() async throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }
        let firstSourceURL = fixture.rootURL.appendingPathComponent("first", isDirectory: true)
        let secondSourceURL = fixture.rootURL.appendingPathComponent("second", isDirectory: true)
        try writePackage(name: "duplicate-local-sample", version: "1.0.0", to: firstSourceURL)
        try writePackage(name: "duplicate-local-sample", version: "2.0.0", to: secondSourceURL)

        let first = try await fixture.installer.install(from: firstSourceURL)
        do {
            _ = try await fixture.installer.install(from: secondSourceURL)
            XCTFail("Expected duplicate package error")
        } catch {
            XCTAssertTrue(error.localizedDescription.contains("已经安装"))
        }

        let manifest = try JSONDecoder().decode(
            TweakPackageManifest.self,
            from: Data(contentsOf: first.directoryURL.appendingPathComponent("package.json"))
        )
        XCTAssertEqual(manifest.version, "1.0.0")
    }

    func testRejectsSymbolicLinksInSelectedDirectory() async throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }
        let sourceURL = fixture.rootURL.appendingPathComponent("linked-package", isDirectory: true)
        try writePackage(name: "linked-local-sample", to: sourceURL)
        try FileManager.default.createSymbolicLink(
            at: sourceURL.appendingPathComponent("linked.js"),
            withDestinationURL: sourceURL.appendingPathComponent("src/index.js")
        )

        do {
            _ = try await fixture.installer.install(from: sourceURL)
            XCTFail("Expected symbolic link rejection")
        } catch {
            XCTAssertTrue(error.localizedDescription.contains("符号链接"))
        }
    }

    func testRejectsArchiveTraversalBeforeExtraction() async throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.rootURL) }
        let archiveWorkURL = fixture.rootURL.appendingPathComponent("archive-work", isDirectory: true)
        try FileManager.default.createDirectory(at: archiveWorkURL, withIntermediateDirectories: true)
        try write("escape", to: fixture.rootURL.appendingPathComponent("escape.js"))
        let archiveURL = fixture.rootURL.appendingPathComponent("unsafe.zip")
        try run(
            executableURL: URL(fileURLWithPath: "/usr/bin/zip"),
            arguments: [archiveURL.path, "../escape.js"],
            currentDirectoryURL: archiveWorkURL
        )

        do {
            _ = try await fixture.installer.install(from: archiveURL)
            XCTFail("Expected archive traversal rejection")
        } catch {
            XCTAssertTrue(error.localizedDescription.contains("不安全的路径"))
        }
    }

    private func makeFixture() throws -> (
        rootURL: URL,
        store: TweakPackageStore,
        installer: TweakPackageLocalInstaller
    ) {
        let rootURL = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        let store = TweakPackageStore(
            applicationSupportURL: rootURL.appendingPathComponent("Application Support"),
            cachesURL: rootURL.appendingPathComponent("Caches")
        )
        let installer = TweakPackageLocalInstaller(store: store)
        return (rootURL, store, installer)
    }

    private func writePackage(
        name: String,
        version: String = "1.0.0",
        dependencies: [String: String] = [:],
        to packageURL: URL
    ) throws {
        try write(
            "export function activate() {}",
            to: packageURL.appendingPathComponent("src/index.js")
        )
        let manifest = TweakPackageManifest(
            name: name,
            version: version,
            description: "Local installer test package",
            type: "module",
            dependencies: dependencies,
            codexTweaks: .init(entry: "src/index.js", priority: 100)
        )
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        try encoder.encode(manifest).write(
            to: packageURL.appendingPathComponent("package.json"),
            options: .atomic
        )
    }

    private func write(_ value: String, to url: URL) throws {
        try FileManager.default.createDirectory(
            at: url.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        try value.write(to: url, atomically: true, encoding: .utf8)
    }

    private func run(
        executableURL: URL,
        arguments: [String],
        currentDirectoryURL: URL? = nil
    ) throws {
        let process = Process()
        let pipe = Pipe()
        process.executableURL = executableURL
        process.arguments = arguments
        process.currentDirectoryURL = currentDirectoryURL
        process.standardOutput = pipe
        process.standardError = pipe
        try process.run()
        let output = pipe.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        guard process.terminationStatus == 0 else {
            throw NSError(
                domain: "TweakPackageLocalInstallerTests",
                code: Int(process.terminationStatus),
                userInfo: [NSLocalizedDescriptionKey: String(decoding: output, as: UTF8.self)]
            )
        }
    }
}
