import Foundation
import XCTest
@testable import CodexTweaks

final class BackendProtocolTests: XCTestCase {
    func testDependencyStatesDecodeGoTaggedPayloads() throws {
        let mismatch = try decode(
            TweakPackageDependencyState.self,
            #"{"kind":"versionMismatch","activeVersion":"2.4.0"}"#
        )
        let conflict = try decode(
            TweakPackageDependencyState.self,
            #"{"kind":"sourceConflict","installedURL":"https://example.com/a.git"}"#
        )

        XCTAssertEqual(mismatch, .versionMismatch(activeVersion: "2.4.0"))
        XCTAssertEqual(conflict, .sourceConflict(installedURL: "https://example.com/a.git"))
    }

    func testReleaseDecodesGoDatesAndSnakeCaseFields() throws {
        let release = try decode(
            GitHubRelease.self,
            """
            {
              "tag_name": "v1.2.3-beta.1",
              "draft": false,
              "prerelease": true,
              "published_at": "2026-08-23T10:11:12.123456789Z",
              "html_url": "https://example.com/releases/v1.2.3-beta.1",
              "assets": [
                {
                  "name": "Codex-Tweaks-v1.2.3-beta.1-arm64.dmg",
                  "browser_download_url": "https://example.com/app.dmg"
                }
              ]
            }
            """
        )

        XCTAssertEqual(release.tagName, "v1.2.3-beta.1")
        XCTAssertEqual(release.assets.first?.browserDownloadURL?.absoluteString, "https://example.com/app.dmg")
        XCTAssertNotNil(release.publishedAt)
    }

    func testCompleteSnapshotAndPackageDecodeAcrossTheGoBoundary() throws {
        let presentationData = try JSONEncoder().encode(GeneratedPresentationDefaults.contract)
        let presentationJSON = try XCTUnwrap(String(data: presentationData, encoding: .utf8))
        let package = try decode(
            TweakPackage.self,
            """
            {
              "id": "ct-sample",
              "directoryName": "ct-sample",
              "directory": "/tmp/ct-sample",
              "exportFileName": "ct-sample-v1.0.0.zip",
              "manifest": {
                "name": "ct-sample",
                "version": "1.0.0",
                "description": "sample",
                "type": "module",
                "dependencies": {},
                "codexTweaks": {
                  "apiVersion": 2,
                  "entry": "src/index.js",
                  "priority": 100,
                  "packageDependencies": {}
                }
              },
              "sourceFingerprint": "source",
              "dependencyFingerprint": "dependencies",
              "origin": {"kind": "local"},
              "displayName": "ct-sample",
              "displayVersion": "1.0.0",
              "detail": "sample",
              "declaredPriority": 100,
              "priority": 100,
              "hasDependencies": false,
              "packageDependencies": {},
              "runtimePackageDependencies": {},
              "isManaged": false,
              "buildDisposition": "notBuilt",
              "buildRequestKey": "request",
              "canInstallMissingDependencies": false,
              "canEnableDependencies": false,
              "availableActions": {
                "setEnabled": true,
                "setPriority": true,
                "openDirectory": true,
                "export": true,
                "installMissingDependencies": false,
                "enableDependencies": false,
                "updateManagedPackage": false,
                "build": true
              },
              "presentation": {
                "statusTitle": "尚未编译",
                "statusDetail": "尚未生成可运行产物。",
                "statusTone": "warning",
                "isError": false,
                "isPending": true
              }
            }
            """
        )
        XCTAssertEqual(package.id, "ct-sample")
        XCTAssertEqual(package.buildDisposition, .notBuilt)
        XCTAssertEqual(package.origin, .local)

        let snapshot = try decode(
            BackendAppSnapshot.self,
            """
            {
              "protocolVersion": 3,
              "presentation": \(presentationJSON),
              "status": {"kind": "connected", "targetCount": 1},
              "enabled": true,
              "developerMode": false,
              "packages": [],
              "disabledPackageIDs": [],
              "buildingPackageIDs": [],
              "exportingPackageIDs": [],
              "packageBuildErrors": {},
              "packageRuntimeErrors": {},
              "packagePayloadErrors": {},
              "packageDependencyStatuses": {},
              "packageDependencyIssues": {},
              "packagePriorityConstraints": {},
              "checkingNode": false,
              "checkingGit": false,
              "checkingRemoteUpdates": false,
              "remotePackageUpdates": {},
              "remotePackageErrors": {},
              "installingPackageIDs": [],
              "installingRemotePackage": false,
              "installingLocalPackage": false,
              "tweaksDirectory": "/tmp/Tweaks",
              "packagesDirectory": "/tmp/Tweaks/packages",
              "logPath": "/tmp/app.log",
              "logText": "",
              "enabledPackageCount": 0,
              "activePackageCount": 0,
              "update": {
                "channel": "stable",
                "autoCheck": true,
                "checking": false,
                "currentVersion": "0.1.0",
                "buildNumber": "1",
                "hasNewerVersion": false,
                "updateAvailable": false,
                "latestVersionString": "-",
                "latestVersionIsSkipped": false
              }
            }
            """
        )
        XCTAssertEqual(snapshot.protocolVersion, 3)
        XCTAssertEqual(snapshot.presentation.version, 1)
        XCTAssertEqual(snapshot.status.targetCount, 1)
        XCTAssertEqual(snapshot.update.channel, .stable)
    }

    func testGeneratedPresentationContractOwnsSharedCopyTokensAndPlatformConstants() {
        let contract = GeneratedPresentationDefaults.contract
        XCTAssertEqual(contract.text[PresentationTextKey.appName.rawValue], "Codex Tweaks")
        XCTAssertEqual(contract.tokens.accentColor, "#0A84FF")
        XCTAssertEqual(contract.platform.cdpEndpoint, "127.0.0.1:9335")
        XCTAssertEqual(
            contract.platform.repositoryURL,
            "https://github.com/cr-zhichen/codex-tweaks"
        )
    }

    func testFrontendKeepsOnlyNativeStatusSymbolChoice() {
        XCTAssertEqual(AppModel.Status.restartRequired.symbol, "arrow.clockwise.circle")
        XCTAssertTrue(AppModel.Status.waitingForPage.isCDPAvailable)
    }

    private func decode<Value: Decodable>(_ type: Value.Type, _ json: String) throws -> Value {
        try BackendClient.makeDecoder().decode(type, from: Data(json.utf8))
    }
}
