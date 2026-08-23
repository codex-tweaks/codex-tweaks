import XCTest
@testable import CodexTweaks

final class TweakPackageBuilderTests: XCTestCase {
    func testDependencyInstallUsesLockedTreeWithoutLifecycleScripts() {
        XCTAssertEqual(
            TweakPackageBuilder.dependencyInstallArguments,
            ["ci", "--ignore-scripts", "--no-audit", "--no-fund"]
        )
    }

    func testEsbuildArgumentsPinCompilerAndBrowserCommonJSOutput() {
        let arguments = TweakPackageBuilder.esbuildArguments(
            entryURL: URL(fileURLWithPath: "/tmp/package/src/index.ts"),
            outputURL: URL(fileURLWithPath: "/tmp/build/bundle.js"),
            allowCompilerDownload: true
        )

        XCTAssertTrue(arguments.contains("esbuild@\(TweakPackageStore.compilerVersion)"))
        XCTAssertTrue(arguments.contains("--platform=browser"))
        XCTAssertTrue(arguments.contains("--format=cjs"))
        XCTAssertTrue(arguments.contains("--outfile=/tmp/build/bundle.js"))
        XCTAssertFalse(arguments.contains("--offline"))
    }

    func testDeveloperBuildUsesOnlyCachedCompiler() {
        let arguments = TweakPackageBuilder.esbuildArguments(
            entryURL: URL(fileURLWithPath: "/tmp/index.js"),
            outputURL: URL(fileURLWithPath: "/tmp/bundle.js"),
            allowCompilerDownload: false
        )

        XCTAssertTrue(arguments.contains("--offline"))
    }

    func testNodeCandidatesIncludePathAndRemoveDuplicates() {
        let home = URL(fileURLWithPath: "/tmp/codex-tweaks-home", isDirectory: true)
        let candidates = TweakPackageBuilder.nodeCandidates(
            environment: ["PATH": "/custom/bin:/custom/bin:/second/bin"],
            homeDirectoryURL: home
        )

        XCTAssertEqual(candidates.first?.path, "/custom/bin/node")
        XCTAssertEqual(candidates.filter { $0.path == "/custom/bin/node" }.count, 1)
        XCTAssertTrue(candidates.contains { $0.path == "/second/bin/node" })
        XCTAssertTrue(candidates.contains { $0.path == "/tmp/codex-tweaks-home/.volta/bin/node" })
    }
}
