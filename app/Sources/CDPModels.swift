import Foundation

struct CDPTarget: Decodable, Equatable {
    let id: String
    let type: String
    let title: String
    let url: String
    let webSocketDebuggerUrl: String?

    var debuggerURL: URL? {
        guard let webSocketDebuggerUrl else { return nil }
        return URL(string: webSocketDebuggerUrl)
    }

    var isInjectableCodexPage: Bool {
        type == "page" && url.lowercased().hasPrefix("app://") && debuggerURL != nil
    }
}
enum CDPTargetSelector {
    static func injectableTargets(from data: Data) throws -> [CDPTarget] {
        try JSONDecoder()
            .decode([CDPTarget].self, from: data)
            .filter(\.isInjectableCodexPage)
    }
}

struct CDPInjectionResult: Equatable {
    let targetCount: Int
    let successCount: Int
    let packageErrors: [String: String]

    var failureCount: Int {
        targetCount - successCount
    }
}
