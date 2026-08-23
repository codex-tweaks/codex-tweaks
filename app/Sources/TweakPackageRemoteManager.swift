import Foundation

struct GitEnvironment: Codable, Equatable, Sendable {
    let gitPath: String
    let version: String

    var gitURL: URL { URL(fileURLWithPath: gitPath) }
}

enum TweakPackageRemoteUpdateStatus: String, Codable, Equatable, Sendable {
    case current
    case available
    case pinnedReferenceChanged
}

struct TweakPackageRemoteUpdate: Codable, Equatable, Sendable {
    let packageID: String
    let currentCommit: String
    let candidateReference: String
    let candidateCommit: String
    let checkedAt: Date
    let status: TweakPackageRemoteUpdateStatus

    var isInstallable: Bool { status == .available }
}
