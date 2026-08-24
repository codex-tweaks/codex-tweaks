import Foundation
import XCTest
@testable import CodexTweaks

final class BackendBundleTests: XCTestCase {
    func testBundledBackendVersionMatchesTheHostApp() throws {
        let executable = try XCTUnwrap(backendExecutableURL())
        let output = try run(executable, arguments: ["--version"])
        let expected = Bundle.main.object(
            forInfoDictionaryKey: "CodexTweaksReleaseVersion"
        ) as? String
        XCTAssertEqual(output.trimmingCharacters(in: .whitespacesAndNewlines), expected)
    }

    func testBundledBackendIsExecutableAndAnswersPing() throws {
        let executable = try XCTUnwrap(backendExecutableURL())
        XCTAssertTrue(FileManager.default.isExecutableFile(atPath: executable.path))

        let process = Process()
        let input = Pipe()
        let output = Pipe()
        let error = Pipe()
        process.executableURL = executable
        process.standardInput = input
        process.standardOutput = output
        process.standardError = error

        try process.run()
        try input.fileHandleForWriting.write(
            contentsOf: Data(#"{"id":1,"method":"ping","params":{}}"#.utf8) + Data([0x0A])
        )
        try input.fileHandleForWriting.close()

        let responseData = output.fileHandleForReading.readDataToEndOfFile()
        let errorData = error.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()

        XCTAssertEqual(
            process.terminationStatus,
            0,
            String(data: errorData, encoding: .utf8) ?? "Go 后端异常退出"
        )
        let response = try JSONDecoder().decode(PingResponse.self, from: responseData)
        XCTAssertEqual(response.id, 1)
        XCTAssertEqual(response.result.protocolVersion, 3)
        XCTAssertEqual(response.result.backend, "go")
    }

    private func backendExecutableURL() -> URL? {
        if let value = Bundle.main.url(forResource: "codex-tweaks-backend", withExtension: nil) {
            return value
        }
        if let testHost = ProcessInfo.processInfo.environment["TEST_HOST"] {
            let appURL = URL(fileURLWithPath: testHost)
                .deletingLastPathComponent()
                .deletingLastPathComponent()
            return appURL
                .appendingPathComponent("Contents/Resources/codex-tweaks-backend")
        }
        return nil
    }

    private func run(_ executable: URL, arguments: [String]) throws -> String {
        let process = Process()
        let output = Pipe()
        let error = Pipe()
        process.executableURL = executable
        process.arguments = arguments
        process.standardOutput = output
        process.standardError = error
        try process.run()
        let outputData = output.fileHandleForReading.readDataToEndOfFile()
        let errorData = error.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        XCTAssertEqual(
            process.terminationStatus,
            0,
            String(data: errorData, encoding: .utf8) ?? "Go 后端异常退出"
        )
        return String(decoding: outputData, as: UTF8.self)
    }
}

private struct PingResponse: Decodable {
    struct Result: Decodable {
        let protocolVersion: Int
        let backend: String
    }

    let id: Int
    let result: Result
}
