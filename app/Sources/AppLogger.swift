import Foundation
import OSLog

final class AppLogger: @unchecked Sendable {
    static let shared = AppLogger()

    let logFileURL: URL

    private let systemLogger = Logger(subsystem: "com.zgccrui.CodexTweaks", category: "app")
    private let queue = DispatchQueue(label: "com.zgccrui.CodexTweaks.log")
    private let fileManager: FileManager

    init(
        fileManager: FileManager = .default,
        applicationSupportURL: URL? = nil
    ) {
        self.fileManager = fileManager
        let applicationSupport = applicationSupportURL ?? fileManager.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first!
        let logDirectory = applicationSupport
            .appendingPathComponent("Codex Tweaks", isDirectory: true)
            .appendingPathComponent("Logs", isDirectory: true)
        try? fileManager.createDirectory(
            at: logDirectory,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        logFileURL = logDirectory.appendingPathComponent("codex-tweaks.log")
    }

    func info(_ message: String) {
        systemLogger.info("\(message, privacy: .public)")
        append(level: "INFO", message: message)
    }

    func error(_ message: String) {
        systemLogger.error("\(message, privacy: .public)")
        append(level: "ERROR", message: message)
    }

    func ensureLogFileExists() {
        queue.sync {
            try? ensureLogFileExistsLocked()
        }
    }

    func readContents() throws -> String {
        try queue.sync {
            try ensureLogFileExistsLocked()
            return try String(contentsOf: logFileURL, encoding: .utf8)
        }
    }

    func clear() throws {
        try queue.sync {
            try ensureLogFileExistsLocked()
            let handle = try FileHandle(forWritingTo: logFileURL)
            do {
                try handle.truncate(atOffset: 0)
                try handle.close()
            } catch {
                try? handle.close()
                throw error
            }
            try fileManager.setAttributes(
                [.posixPermissions: 0o600],
                ofItemAtPath: logFileURL.path
            )
        }
    }

    private func append(level: String, message: String) {
        queue.async { [self] in
            let timestamp = ISO8601DateFormatter().string(from: Date())
            let line = "\(timestamp) [\(level)] \(message)\n"
            let data = Data(line.utf8)
            do {
                try ensureLogFileExistsLocked()
                let handle = try FileHandle(forWritingTo: logFileURL)
                defer { try? handle.close() }
                try handle.seekToEnd()
                try handle.write(contentsOf: data)
            } catch {
                // 系统日志仍保留错误；文件日志失败不应影响注入流程。
            }
        }
    }

    private func ensureLogFileExistsLocked() throws {
        if !fileManager.fileExists(atPath: logFileURL.path) {
            guard fileManager.createFile(atPath: logFileURL.path, contents: nil) else {
                throw CocoaError(.fileWriteUnknown)
            }
        }
        try fileManager.setAttributes(
            [.posixPermissions: 0o600],
            ofItemAtPath: logFileURL.path
        )
    }
}

enum LogTextFormatter {
    static func newestFirst(_ contents: String) -> String {
        contents
            .split(whereSeparator: { $0.isNewline })
            .reversed()
            .joined(separator: "\n")
    }
}
