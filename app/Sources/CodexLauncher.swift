import AppKit
import Foundation

@MainActor
struct CodexLauncher {
    enum LauncherError: LocalizedError {
        case applicationNotFound
        case launchFailed(Int32)
        case terminationFailed

        var errorDescription: String? {
            switch self {
            case .applicationNotFound:
                return "没有找到 ChatGPT.app（Codex 桌面客户端）"
            case let .launchFailed(status):
                return "Codex 启动失败，open 退出码为 \(status)"
            case .terminationFailed:
                return "Codex 未能正常退出，请手动退出后重试"
            }
        }
    }

    static let bundleIdentifier = "com.openai.codex"
    static let debuggingArguments = [
        "--remote-debugging-address=127.0.0.1",
        "--remote-debugging-port=9335",
        "--remote-allow-origins=http://127.0.0.1:9335",
    ]

    var isRunning: Bool {
        !runningApplications.isEmpty
    }

    func activate() {
        runningApplications.first?.activate(options: [.activateAllWindows, .activateIgnoringOtherApps])
    }

    func launchWithCDP() throws {
        let applicationURL = try locateApplication()
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/open")
        process.arguments = ["-n", applicationURL.path, "--args"] + Self.debuggingArguments
        try process.run()
        process.waitUntilExit()
        guard process.terminationStatus == 0 else {
            throw LauncherError.launchFailed(process.terminationStatus)
        }
    }

    func restartWithCDP() async throws {
        let applications = runningApplications
        applications.forEach { $0.terminate() }

        for _ in 0 ..< 25 where isRunning {
            try await Task.sleep(nanoseconds: 200_000_000)
        }

        if isRunning {
            runningApplications.forEach { $0.forceTerminate() }
            for _ in 0 ..< 15 where isRunning {
                try await Task.sleep(nanoseconds: 200_000_000)
            }
        }

        guard !isRunning else {
            throw LauncherError.terminationFailed
        }

        try launchWithCDP()
    }

    private var runningApplications: [NSRunningApplication] {
        NSRunningApplication.runningApplications(withBundleIdentifier: Self.bundleIdentifier)
    }

    private func locateApplication() throws -> URL {
        if let installedURL = NSWorkspace.shared.urlForApplication(withBundleIdentifier: Self.bundleIdentifier) {
            return installedURL
        }

        let fallbackURL = URL(fileURLWithPath: "/Applications/ChatGPT.app", isDirectory: true)
        guard FileManager.default.fileExists(atPath: fallbackURL.path) else {
            throw LauncherError.applicationNotFound
        }
        return fallbackURL
    }
}
