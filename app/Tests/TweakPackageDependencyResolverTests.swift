import XCTest
@testable import CodexTweaks

final class TweakPackageDependencyResolverTests: XCTestCase {
    func testSupportedSemanticVersionRequirements() throws {
        XCTAssertTrue(try XCTUnwrap(SemanticVersionRequirement("^1.2.3")).contains("1.9.0"))
        XCTAssertFalse(try XCTUnwrap(SemanticVersionRequirement("^1.2.3")).contains("2.0.0"))
        XCTAssertTrue(try XCTUnwrap(SemanticVersionRequirement("~1.2.3")).contains("1.2.99"))
        XCTAssertFalse(try XCTUnwrap(SemanticVersionRequirement("~1.2.3")).contains("1.3.0"))
        XCTAssertTrue(try XCTUnwrap(SemanticVersionRequirement("1.x")).contains("1.8.0"))
        XCTAssertTrue(try XCTUnwrap(SemanticVersionRequirement(">=1.2.3")).contains("2.0.0"))
        XCTAssertTrue(try XCTUnwrap(SemanticVersionRequirement("1.2.3")).contains("v1.2.3"))
    }

    func testDependencyTopologyOverridesUserPriority() {
        let dependency = makePackage(
            id: "foundation",
            priority: 100,
            priorityOverride: 500
        )
        let consumer = makePackage(
            id: "consumer",
            priority: 0,
            priorityOverride: -500,
            packageDependencies: ["foundation": "^1.0.0"]
        )

        let result = TweakPackageDependencyResolver.resolve(
            packages: [consumer, dependency],
            disabledPackageIDs: []
        )

        XCTAssertEqual(result.orderedPackages.map(\.id), ["foundation", "consumer"])
        XCTAssertEqual(result.loadablePackages.map(\.id), ["foundation", "consumer"])
        XCTAssertTrue(result.issuesByPackageID.isEmpty)
        XCTAssertEqual(
            result.priorityConstraintsByPackageID["foundation"]?.mustLoadBeforePackageIDs,
            ["consumer"]
        )
        XCTAssertEqual(
            result.priorityConstraintsByPackageID["consumer"]?.mustLoadAfterPackageIDs,
            ["foundation"]
        )
        XCTAssertEqual(
            result.priorityConstraintsByPackageID["consumer"]?.actualLoadPosition,
            2
        )
    }

    func testMissingDependencyOnlyBlocksAffectedPackage() {
        let affected = makePackage(
            id: "affected",
            packageDependencies: ["missing": "^1.0.0"]
        )
        let healthy = makePackage(id: "healthy", priority: 10)

        let result = TweakPackageDependencyResolver.resolve(
            packages: [affected, healthy],
            disabledPackageIDs: []
        )

        XCTAssertEqual(result.loadablePackages.map(\.id), ["healthy"])
        XCTAssertTrue(result.issuesByPackageID["affected"]?.contains {
            $0.contains("缺少依赖 missing")
        } == true)
        XCTAssertNil(result.issuesByPackageID["healthy"])
    }

    func testCycleIsIsolatedFromUnrelatedPackages() {
        let first = makePackage(
            id: "first",
            packageDependencies: ["second": "1.x"]
        )
        let second = makePackage(
            id: "second",
            packageDependencies: ["first": "1.x"]
        )
        let unrelated = makePackage(id: "unrelated")

        let result = TweakPackageDependencyResolver.resolve(
            packages: [first, second, unrelated],
            disabledPackageIDs: []
        )

        XCTAssertEqual(result.cyclePackageIDs, ["first", "second"])
        XCTAssertEqual(result.loadablePackages.map(\.id), ["unrelated"])
    }

    func testDisabledOrUnbuiltDependencyBlocksConsumer() {
        let dependency = makePackage(id: "dependency")
        let consumer = makePackage(
            id: "consumer",
            packageDependencies: ["dependency": "^1.0.0"]
        )

        let disabled = TweakPackageDependencyResolver.resolve(
            packages: [dependency, consumer],
            disabledPackageIDs: ["dependency"]
        )
        XCTAssertTrue(disabled.loadablePackages.isEmpty)
        XCTAssertTrue(disabled.issuesByPackageID["consumer"]?.contains {
            $0.contains("已停用")
        } == true)

        let unbuiltDependency = makePackage(id: "dependency", built: false)
        let unbuilt = TweakPackageDependencyResolver.resolve(
            packages: [unbuiltDependency, consumer],
            disabledPackageIDs: []
        )
        XCTAssertEqual(unbuilt.loadablePackages.map(\.id), [])
        XCTAssertTrue(unbuilt.issuesByPackageID["consumer"]?.contains {
            $0.contains("尚未编译")
        } == true)
    }

    func testDependencyStatusesDistinguishLocalAndGitInstallableDependencies() throws {
        let source = TweakPackageSource(
            url: "https://github.com/example/remote-core.git",
            selector: TweakPackageRemoteSelector(type: .latestSemverTag)
        )
        let local = makePackage(id: "local-core")
        let consumer = makePackage(
            id: "consumer",
            packageDependencies: [
                "local-core": "^1.0.0",
                "remote-core": "^2.0.0",
            ],
            dependencySources: ["remote-core": source]
        )

        let result = TweakPackageDependencyResolver.resolve(
            packages: [consumer, local],
            disabledPackageIDs: []
        )
        let statuses = try XCTUnwrap(result.dependenciesByPackageID["consumer"])
        let localStatus = try XCTUnwrap(statuses.first { $0.dependencyID == "local-core" })
        let remoteStatus = try XCTUnwrap(statuses.first { $0.dependencyID == "remote-core" })

        XCTAssertEqual(localStatus.state, .satisfied)
        XCTAssertEqual(localStatus.resolvedOrigin, TweakPackageOrigin.local)
        XCTAssertNil(localStatus.declaredSource)
        XCTAssertEqual(remoteStatus.state, .missingInstallable)
        XCTAssertEqual(remoteStatus.declaredSource, source)
        XCTAssertTrue(result.issuesByPackageID["consumer"]?.contains {
            $0.contains("可从声明的 Git 来源安装")
        } == true)
    }

    func testManagedDependencySourceConflictIsStructuredAndBlocking() throws {
        let installedSource = TweakPackageSource(
            url: "https://github.com/example/installed.git",
            selector: TweakPackageRemoteSelector(type: .branch, value: "main")
        )
        let declaredSource = TweakPackageSource(
            url: "https://github.com/example/declared.git",
            selector: TweakPackageRemoteSelector(type: .branch, value: "main")
        )
        let dependency = makePackage(
            id: "dependency",
            origin: .managed(
                TweakManagedPackageLock(
                    packageID: "dependency",
                    packageVersion: "1.0.0",
                    source: installedSource,
                    resolvedReference: "main",
                    resolvedCommit: String(repeating: "a", count: 40),
                    sourceRelativePath: "sources/dependency",
                    installedAt: Date(timeIntervalSince1970: 0)
                )
            )
        )
        let consumer = makePackage(
            id: "consumer",
            packageDependencies: ["dependency": "^1.0.0"],
            dependencySources: ["dependency": declaredSource]
        )

        let result = TweakPackageDependencyResolver.resolve(
            packages: [consumer, dependency],
            disabledPackageIDs: []
        )
        let status = try XCTUnwrap(
            result.dependenciesByPackageID["consumer"]?.first
        )

        XCTAssertEqual(
            status.state,
            .sourceConflict(installedURL: installedSource.url)
        )
        XCTAssertFalse(result.loadablePackages.map(\.id).contains("consumer"))
    }

    private func makePackage(
        id: String,
        version: String = "1.0.0",
        priority: Int = 0,
        priorityOverride: Int? = nil,
        packageDependencies: [String: String] = [:],
        dependencySources: [String: TweakPackageSource] = [:],
        origin: TweakPackageOrigin = .local,
        built: Bool = true
    ) -> TweakPackage {
        let manifestDependencies = Dictionary(uniqueKeysWithValues: packageDependencies.map {
            dependencyID, version in
            (
                dependencyID,
                TweakPackageDependency(
                    version: version,
                    source: dependencySources[dependencyID]
                )
            )
        })
        let manifest = TweakPackageManifest(
            name: id,
            version: version,
            description: id,
            type: "module",
            codexTweaks: .init(
                entry: "src/index.js",
                priority: priority,
                packageDependencies: manifestDependencies
            )
        )
        let activeBuild: ActiveTweakPackageBuild? = built
            ? ActiveTweakPackageBuild(
                record: TweakPackageBuildRecord(
                    packageID: id,
                    packageVersion: version,
                    packageDependencies: packageDependencies,
                    sourceFingerprint: "source-\(id)",
                    dependencyFingerprint: "dependencies-\(id)",
                    compilerVersion: TweakPackageStore.compilerVersion,
                    nodeVersion: "v24.0.0",
                    buildDirectoryName: "build",
                    hasCSS: false,
                    builtAt: Date(timeIntervalSince1970: 0)
                ),
                outputDirectoryURL: URL(fileURLWithPath: "/tmp/\(id)")
            )
            : nil

        return TweakPackage(
            id: id,
            directoryName: id,
            directoryURL: URL(fileURLWithPath: "/tmp/\(id)"),
            manifest: manifest,
            sourceFingerprint: "source-\(id)",
            dependencyFingerprint: "dependencies-\(id)",
            activeBuild: activeBuild,
            validationError: nil,
            priorityOverride: priorityOverride,
            origin: origin
        )
    }
}
