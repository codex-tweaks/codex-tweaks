import XCTest
@testable import CodexTweaks

final class AppLoggerTests: XCTestCase {
    func testNewestFirstReversesLogLines() {
        let chronological = "oldest\nmiddle\nnewest\n"

        XCTAssertEqual(
            LogTextFormatter.newestFirst(chronological),
            "newest\nmiddle\noldest"
        )
        XCTAssertEqual(LogTextFormatter.newestFirst(""), "")
    }

    func testClearWaitsForQueuedWritesAndLeavesLogWritable() throws {
        let fileManager = FileManager.default
        let root = fileManager.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? fileManager.removeItem(at: root) }

        let logger = AppLogger(
            fileManager: fileManager,
            applicationSupportURL: root
        )

        logger.info("before-clear")
        XCTAssertTrue(try logger.readContents().contains("before-clear"))

        try logger.clear()
        XCTAssertEqual(try logger.readContents(), "")

        logger.error("after-clear")
        XCTAssertTrue(try logger.readContents().contains("after-clear"))

        let permissions = try fileManager.attributesOfItem(
            atPath: logger.logFileURL.path
        )[.posixPermissions] as? NSNumber
        XCTAssertEqual(permissions?.intValue, 0o600)
    }
}
