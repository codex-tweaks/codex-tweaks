import Foundation
import XCTest
@testable import CodexTweaks

final class SemanticVersionTests: XCTestCase {
    func testSemVerPrecedenceUsesNumericPrereleaseOrdering() throws {
        let beta9 = try XCTUnwrap(SemanticVersion("v1.0.0-beta.9"))
        let beta10 = try XCTUnwrap(SemanticVersion("1.0.0-beta.10"))
        let release = try XCTUnwrap(SemanticVersion("1.0.0+build.7"))
        let nextBeta = try XCTUnwrap(SemanticVersion("1.0.1-beta.1"))

        XCTAssertLessThan(beta9, beta10)
        XCTAssertLessThan(beta10, release)
        XCTAssertLessThan(release, nextBeta)
        XCTAssertEqual(release, SemanticVersion("1.0.0+another-build"))
    }

    func testSemVerSupportsLargeNumericIdentifiers() throws {
        let smaller = try XCTUnwrap(SemanticVersion("99999999999999999999.0.0"))
        let larger = try XCTUnwrap(SemanticVersion("100000000000000000000.0.0"))

        XCTAssertLessThan(smaller, larger)
    }

    func testSemVerRejectsInvalidIdentifiers() {
        XCTAssertNil(SemanticVersion("1.0"))
        XCTAssertNil(SemanticVersion("01.0.0"))
        XCTAssertNil(SemanticVersion("1.0.0-beta.01"))
        XCTAssertNil(SemanticVersion("1.0.0-beta_1"))
        XCTAssertNil(SemanticVersion("1.0.0+"))
    }
}

final class UpdateReleaseSelectionTests: XCTestCase {
    func testStableChannelOnlySelectsPublishedStableVersions() {
        let releases = [
            release("v1.1.0"),
            release("v9.0.0", draft: true),
            release("v2.0.0-beta.1", prerelease: true),
            release("not-a-version")
        ]

        XCTAssertEqual(
            UpdateChecker.selectLatestRelease(from: releases, for: .stable)?.tagName,
            "v1.1.0"
        )
    }

    func testBetaChannelAcceptsStableBetaAndRCButExcludesAlpha() {
        let releases = [
            release("v1.1.0"),
            release("v1.2.0-beta.10", prerelease: true),
            release("v1.2.0-rc.1", prerelease: true),
            release("v9.0.0-alpha.1", prerelease: true)
        ]

        XCTAssertEqual(
            UpdateChecker.selectLatestRelease(from: releases, for: .beta)?.tagName,
            "v1.2.0-rc.1"
        )
    }

    func testHigherStableReleaseWinsOnBetaChannel() {
        let releases = [
            release("v1.4.0"),
            release("v1.4.0-rc.2", prerelease: true)
        ]

        XCTAssertEqual(
            UpdateChecker.selectLatestRelease(from: releases, for: .beta)?.tagName,
            "v1.4.0"
        )
    }

    func testDownloadPrefersArchitectureThenUniversalThenReleasePage() throws {
        let releasePage = try XCTUnwrap(URL(string: "https://example.com/release"))
        let universal = try XCTUnwrap(URL(string: "https://example.com/universal.dmg"))
        let arm64 = try XCTUnwrap(URL(string: "https://example.com/arm64.dmg"))
        let x86 = try XCTUnwrap(URL(string: "https://example.com/x86.dmg"))
        let candidate = GitHubRelease(
            tagName: "v1.0.0",
            draft: false,
            prerelease: false,
            publishedAt: nil,
            htmlURL: releasePage,
            assets: [
                GitHubAsset(name: "Codex-Tweaks-v1.0.0.dmg", browserDownloadURL: universal),
                GitHubAsset(name: "Codex-Tweaks-v1.0.0-arm64.dmg", browserDownloadURL: arm64),
                GitHubAsset(name: "Codex-Tweaks-v1.0.0-x86_64.dmg", browserDownloadURL: x86)
            ]
        )

        XCTAssertEqual(
            UpdateChecker.preferredDownloadURL(for: candidate, architecture: "arm64"),
            arm64
        )
        XCTAssertEqual(
            UpdateChecker.preferredDownloadURL(for: candidate, architecture: "x86_64"),
            x86
        )

        let universalOnly = GitHubRelease(
            tagName: candidate.tagName,
            draft: false,
            prerelease: false,
            publishedAt: nil,
            htmlURL: releasePage,
            assets: [candidate.assets[0]]
        )
        XCTAssertEqual(
            UpdateChecker.preferredDownloadURL(for: universalOnly, architecture: "arm64"),
            universal
        )

        let noDMG = GitHubRelease(
            tagName: candidate.tagName,
            draft: false,
            prerelease: false,
            publishedAt: nil,
            htmlURL: releasePage,
            assets: []
        )
        XCTAssertEqual(
            UpdateChecker.preferredDownloadURL(for: noDMG, architecture: "arm64"),
            releasePage
        )
    }

    private func release(
        _ tag: String,
        draft: Bool = false,
        prerelease: Bool = false
    ) -> GitHubRelease {
        GitHubRelease(
            tagName: tag,
            draft: draft,
            prerelease: prerelease,
            publishedAt: nil,
            htmlURL: nil,
            assets: []
        )
    }
}

@MainActor
final class UpdateCheckerStateTests: XCTestCase {
    private var suiteName: String!
    private var defaults: UserDefaults!
    private var session: URLSession!

    override func setUp() {
        super.setUp()
        suiteName = "UpdateCheckerTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)
        defaults.removePersistentDomain(forName: suiteName)

        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [UpdateURLProtocol.self]
        session = URLSession(configuration: configuration)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suiteName)
        UpdateURLProtocol.requestHandler = nil
        session.invalidateAndCancel()
        session = nil
        defaults = nil
        suiteName = nil
        super.tearDown()
    }

    func testCheckPersistsSkipAndSendsGitHubHeaders() async throws {
        UpdateURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.url?.path, "/repos/cr-zhichen/codex-tweaks/releases")
            XCTAssertEqual(
                URLComponents(url: try XCTUnwrap(request.url), resolvingAgainstBaseURL: false)?
                    .queryItems?.first(where: { $0.name == "per_page" })?.value,
                "100"
            )
            XCTAssertEqual(request.value(forHTTPHeaderField: "Accept"), "application/vnd.github+json")
            XCTAssertEqual(request.value(forHTTPHeaderField: "X-GitHub-Api-Version"), "2026-03-10")

            let response = try XCTUnwrap(
                HTTPURLResponse(
                    url: try XCTUnwrap(request.url),
                    statusCode: 200,
                    httpVersion: nil,
                    headerFields: nil
                )
            )
            let data = Data(
                """
                [{
                  "tag_name": "v1.0.1",
                  "draft": false,
                  "prerelease": false,
                  "published_at": "2026-08-22T01:02:03Z",
                  "html_url": "https://github.com/cr-zhichen/codex-tweaks/releases/tag/v1.0.1",
                  "assets": []
                }]
                """.utf8
            )
            return (response, data)
        }

        let checker = UpdateChecker(
            defaults: defaults,
            session: session,
            installedVersion: "1.0.0"
        )
        XCTAssertTrue(checker.autoCheck)

        await checker.check(prompt: true)

        XCTAssertEqual(checker.latestVersionString, "1.0.1")
        XCTAssertTrue(checker.updateAvailable)
        XCTAssertNotNil(checker.pendingUpdate)
        XCTAssertNotNil(checker.lastCheckDate)
        XCTAssertNil(checker.lastError)

        checker.skipUpdate(try XCTUnwrap(checker.latestRelease))
        XCTAssertFalse(checker.updateAvailable)
        XCTAssertTrue(checker.latestVersionIsSkipped)
        XCTAssertNil(checker.pendingUpdate)

        let restored = UpdateChecker(
            defaults: defaults,
            session: session,
            installedVersion: "1.0.0"
        )
        await restored.check(prompt: true)
        XCTAssertFalse(restored.updateAvailable)
        XCTAssertNil(restored.pendingUpdate)

        restored.unskipAndPrompt()
        XCTAssertTrue(restored.updateAvailable)
        XCTAssertNotNil(restored.pendingUpdate)
    }

    func testHTTPFailureIsPresentedWithoutReplacingLastSuccessfulCheck() async {
        UpdateURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 403,
                httpVersion: nil,
                headerFields: nil
            )!
            return (response, Data())
        }

        let checker = UpdateChecker(
            defaults: defaults,
            session: session,
            installedVersion: "1.0.0"
        )
        await checker.check()

        XCTAssertEqual(checker.lastError, "GitHub 暂时限制了更新请求，请稍后再试。")
        XCTAssertNil(checker.lastCheckDate)
        XCTAssertFalse(checker.checking)
    }
}

private final class UpdateURLProtocol: URLProtocol {
    static var requestHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

    override class func canInit(with request: URLRequest) -> Bool {
        true
    }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

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
