import Foundation

enum AppStatus: Equatable {
    case starting
    case launchingCodex
    case codexNotRunning
    case waitingForCDP
    case restartRequired
    case waitingForPage
    case connected(targetCount: Int)
    case disabled
    case error(String)

    var symbol: String {
        switch self {
        case .connected: return "checkmark.circle.fill"
        case .disabled: return "pause.circle"
        case .codexNotRunning: return "circle"
        case .restartRequired: return "arrow.clockwise.circle"
        case .error: return "exclamationmark.triangle.fill"
        default: return "circle.dotted"
        }
    }

    var isCDPAvailable: Bool {
        switch self {
        case .connected, .waitingForPage: return true
        default: return false
        }
    }

    init(_ status: BackendAppStatus) {
        switch status.kind {
        case .starting: self = .starting
        case .launchingCodex: self = .launchingCodex
        case .codexNotRunning: self = .codexNotRunning
        case .waitingForCDP: self = .waitingForCDP
        case .restartRequired: self = .restartRequired
        case .waitingForPage: self = .waitingForPage
        case .connected: self = .connected(targetCount: status.targetCount ?? 0)
        case .disabled: self = .disabled
        case .error: self = .error(status.message ?? PresentationText.resolve(.statusErrorTitle))
        }
    }
}
