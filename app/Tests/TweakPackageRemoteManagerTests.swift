import XCTest
@testable import CodexTweaks

final class TweakPackageRemoteManagerTests: XCTestCase {
    func testInstallsLatestSemverTagAndFindsNewerTag() async throws {
        let rootURL = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        let repositoryURL = rootURL.appendingPathComponent("repository", isDirectory: true)
        try FileManager.default.createDirectory(at: repositoryURL, withIntermediateDirectories: true)
        try runGit(["init", "--quiet", "--initial-branch", "main"], at: repositoryURL)
        try runGit(["config", "user.email", "tests@codex-tweaks.invalid"], at: repositoryURL)
        try runGit(["config", "user.name", "Codex Tweaks Tests"], at: repositoryURL)
        try writePackage(version: "1.0.0", to: repositoryURL)
        try commitAll("v1", at: repositoryURL)
        try runGit(["tag", "v1.0.0"], at: repositoryURL)

        let remoteURL = "https://example.test/codex-tweaks-package.git"
        let store = TweakPackageStore(
            applicationSupportURL: rootURL.appendingPathComponent("Application Support"),
            cachesURL: rootURL.appendingPathComponent("Caches")
        )
        var environment = ProcessInfo.processInfo.environment
        environment["GIT_CONFIG_COUNT"] = "1"
        environment["GIT_CONFIG_KEY_0"] = "url.file://\(repositoryURL.path)/.insteadOf"
        environment["GIT_CONFIG_VALUE_0"] = remoteURL
        environment["GIT_ALLOW_PROTOCOL"] = "file"
        let manager = TweakPackageRemoteManager(
            store: store,
            processEnvironment: environment
        )
        let source = TweakPackageSource(
            url: remoteURL,
            selector: TweakPackageRemoteSelector(type: .latestSemverTag)
        )

        let firstInstall = try await manager.install(source: source)
        XCTAssertEqual(firstInstall.packageID, "remote-sample")
        XCTAssertEqual(firstInstall.manifest.version, "1.0.0")
        XCTAssertEqual(firstInstall.lock.resolvedReference, "v1.0.0")
        XCTAssertTrue(FileManager.default.fileExists(
            atPath: store.managedPackagesDirectoryURL
                .appendingPathComponent(firstInstall.lock.sourceRelativePath)
                .appendingPathComponent("package.json")
                .path
        ))

        let loaded = try store.loadPackages(bundle: Bundle(for: Self.self))
        let managed = try XCTUnwrap(loaded.first { $0.id == "remote-sample" })
        XCTAssertTrue(managed.isManaged)
        XCTAssertEqual(managed.managedLock?.resolvedCommit, firstInstall.lock.resolvedCommit)

        try writePackage(version: "1.1.0", to: repositoryURL)
        try commitAll("v1.1", at: repositoryURL)
        try runGit(["tag", "v1.1.0"], at: repositoryURL)

        let update = try await manager.checkForUpdate(packageID: "remote-sample")
        XCTAssertEqual(update.status, .available)
        XCTAssertEqual(update.candidateReference, "v1.1.0")
        XCTAssertNotEqual(update.candidateCommit, update.currentCommit)

        let secondInstall = try await manager.install(source: source)
        XCTAssertEqual(secondInstall.manifest.version, "1.1.0")
        XCTAssertEqual(secondInstall.lock.resolvedReference, "v1.1.0")
        XCTAssertNotEqual(secondInstall.lock.resolvedCommit, firstInstall.lock.resolvedCommit)
    }

    func testInstallsGitHubLatestReleaseByResolvedTag() async throws {
        let rootURL = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        let repositoryURL = rootURL.appendingPathComponent("repository", isDirectory: true)
        try FileManager.default.createDirectory(at: repositoryURL, withIntermediateDirectories: true)
        try runGit(["init", "--quiet", "--initial-branch", "main"], at: repositoryURL)
        try runGit(["config", "user.email", "tests@codex-tweaks.invalid"], at: repositoryURL)
        try runGit(["config", "user.name", "Codex Tweaks Tests"], at: repositoryURL)
        try writePackage(version: "2.0.0", to: repositoryURL)
        try commitAll("release", at: repositoryURL)
        try runGit(["tag", "v2.0.0"], at: repositoryURL)

        let remoteURL = "https://github.com/example/remote-sample.git"
        var environment = ProcessInfo.processInfo.environment
        environment["GIT_CONFIG_COUNT"] = "1"
        environment["GIT_CONFIG_KEY_0"] = "url.file://\(repositoryURL.path)/.insteadOf"
        environment["GIT_CONFIG_VALUE_0"] = remoteURL
        environment["GIT_ALLOW_PROTOCOL"] = "file"
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [RemotePackageURLProtocol.self]
        let session = URLSession(configuration: configuration)
        RemotePackageURLProtocol.requestHandler = { request in
            XCTAssertEqual(
                request.url?.absoluteString,
                "https://api.github.com/repos/example/remote-sample/releases/latest"
            )
            XCTAssertEqual(request.value(forHTTPHeaderField: "Accept"), "application/vnd.github+json")
            let response = HTTPURLResponse(
                url: try XCTUnwrap(request.url),
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["ETag": "release-v2"]
            )!
            return (response, Data(#"{"tag_name":"v2.0.0"}"#.utf8))
        }
        defer { RemotePackageURLProtocol.requestHandler = nil }

        let store = TweakPackageStore(
            applicationSupportURL: rootURL.appendingPathComponent("Application Support"),
            cachesURL: rootURL.appendingPathComponent("Caches")
        )
        let manager = TweakPackageRemoteManager(
            store: store,
            session: session,
            processEnvironment: environment
        )
        let result = try await manager.install(
            source: TweakPackageSource(
                url: remoteURL,
                selector: TweakPackageRemoteSelector(type: .githubLatestRelease)
            )
        )

        XCTAssertEqual(result.manifest.version, "2.0.0")
        XCTAssertEqual(result.lock.resolvedReference, "v2.0.0")
        let registration = try await manager.registration(for: "remote-sample")
        XCTAssertEqual(registration?.remoteETag, "release-v2")
    }

    func testMovedPinnedTagIsReportedButNotInstallableAsOrdinaryUpdate() async throws {
        let rootURL = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        let repositoryURL = rootURL.appendingPathComponent("repository", isDirectory: true)
        try FileManager.default.createDirectory(at: repositoryURL, withIntermediateDirectories: true)
        try runGit(["init", "--quiet", "--initial-branch", "main"], at: repositoryURL)
        try runGit(["config", "user.email", "tests@codex-tweaks.invalid"], at: repositoryURL)
        try runGit(["config", "user.name", "Codex Tweaks Tests"], at: repositoryURL)
        try writePackage(version: "1.0.0", to: repositoryURL)
        try commitAll("original", at: repositoryURL)
        try runGit(["tag", "v1.0.0"], at: repositoryURL)

        let remoteURL = "https://example.test/pinned-package.git"
        var environment = ProcessInfo.processInfo.environment
        environment["GIT_CONFIG_COUNT"] = "1"
        environment["GIT_CONFIG_KEY_0"] = "url.file://\(repositoryURL.path)/.insteadOf"
        environment["GIT_CONFIG_VALUE_0"] = remoteURL
        environment["GIT_ALLOW_PROTOCOL"] = "file"
        let store = TweakPackageStore(
            applicationSupportURL: rootURL.appendingPathComponent("Application Support"),
            cachesURL: rootURL.appendingPathComponent("Caches")
        )
        let manager = TweakPackageRemoteManager(
            store: store,
            processEnvironment: environment
        )
        _ = try await manager.install(
            source: TweakPackageSource(
                url: remoteURL,
                selector: TweakPackageRemoteSelector(type: .tag, value: "v1.0.0")
            )
        )

        try writePackage(version: "1.0.1", to: repositoryURL)
        try commitAll("moved tag", at: repositoryURL)
        try runGit(["tag", "--force", "v1.0.0"], at: repositoryURL)

        let update = try await manager.checkForUpdate(packageID: "remote-sample")
        XCTAssertEqual(update.status, .pinnedReferenceChanged)
        XCTAssertFalse(update.isInstallable)
        XCTAssertNotEqual(update.currentCommit, update.candidateCommit)
    }

    func testRejectsRemotePackageWithDependenciesButNoLockfile() async throws {
        let rootURL = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        let repositoryURL = rootURL.appendingPathComponent("repository", isDirectory: true)
        try FileManager.default.createDirectory(at: repositoryURL, withIntermediateDirectories: true)
        try runGit(["init", "--quiet", "--initial-branch", "main"], at: repositoryURL)
        try runGit(["config", "user.email", "tests@codex-tweaks.invalid"], at: repositoryURL)
        try runGit(["config", "user.name", "Codex Tweaks Tests"], at: repositoryURL)
        try writePackage(
            version: "1.0.0",
            dependencies: ["demo": "1.0.0"],
            to: repositoryURL
        )
        try commitAll("missing lockfile", at: repositoryURL)

        let remoteURL = "https://example.test/invalid-package.git"
        var environment = ProcessInfo.processInfo.environment
        environment["GIT_CONFIG_COUNT"] = "1"
        environment["GIT_CONFIG_KEY_0"] = "url.file://\(repositoryURL.path)/.insteadOf"
        environment["GIT_CONFIG_VALUE_0"] = remoteURL
        environment["GIT_ALLOW_PROTOCOL"] = "file"
        let store = TweakPackageStore(
            applicationSupportURL: rootURL.appendingPathComponent("Application Support"),
            cachesURL: rootURL.appendingPathComponent("Caches")
        )
        let manager = TweakPackageRemoteManager(
            store: store,
            processEnvironment: environment
        )

        do {
            _ = try await manager.install(
                source: TweakPackageSource(
                    url: remoteURL,
                    selector: TweakPackageRemoteSelector(type: .branch, value: "main")
                )
            )
            XCTFail("Expected remote package validation to fail")
        } catch {
            XCTAssertTrue(error.localizedDescription.contains("package-lock.json"))
        }
        let registration = try await manager.registration(for: "remote-sample")
        XCTAssertNil(registration)
        XCTAssertNil(try store.loadManagedLockfile().packages["remote-sample"])
    }

    private func writePackage(
        version: String,
        dependencies: [String: String] = [:],
        to repositoryURL: URL
    ) throws {
        let sourceURL = repositoryURL
            .appendingPathComponent("src", isDirectory: true)
            .appendingPathComponent("index.js")
        try FileManager.default.createDirectory(
            at: sourceURL.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        try "export function activate() {}\n".write(
            to: sourceURL,
            atomically: true,
            encoding: .utf8
        )
        let manifest = TweakPackageManifest(
            name: "remote-sample",
            version: version,
            description: "Remote integration fixture",
            type: "module",
            dependencies: dependencies,
            codexTweaks: .init(entry: "src/index.js")
        )
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        try encoder.encode(manifest).write(
            to: repositoryURL.appendingPathComponent("package.json"),
            options: .atomic
        )
    }

    private func commitAll(_ message: String, at repositoryURL: URL) throws {
        try runGit(["add", "."], at: repositoryURL)
        try runGit(["commit", "--quiet", "-m", message], at: repositoryURL)
    }

    @discardableResult
    private func runGit(_ arguments: [String], at directoryURL: URL) throws -> String {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/git")
        process.arguments = arguments
        process.currentDirectoryURL = directoryURL
        let output = Pipe()
        process.standardOutput = output
        process.standardError = output
        try process.run()
        process.waitUntilExit()
        let text = String(
            decoding: output.fileHandleForReading.readDataToEndOfFile(),
            as: UTF8.self
        )
        guard process.terminationStatus == 0 else {
            throw NSError(
                domain: "TweakPackageRemoteManagerTests",
                code: Int(process.terminationStatus),
                userInfo: [NSLocalizedDescriptionKey: text]
            )
        }
        return text
    }
}

private final class RemotePackageURLProtocol: URLProtocol {
    static var requestHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

    override class func canInit(with request: URLRequest) -> Bool { true }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        guard let handler = Self.requestHandler else {
            client?.urlProtocol(self, didFailWithError: URLError(.unknown))
            return
        }
        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}
