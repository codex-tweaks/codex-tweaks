import Foundation

enum TweakPackageDependencyState: Codable, Equatable, Sendable {
    case satisfied
    case missingLocal
    case missingInstallable
    case disabled
    case notBuilt
    case versionMismatch(activeVersion: String)
    case sourceConflict(installedURL: String)
    case cycle
    case blocked
    case invalidRequirement
    case selfReference

    var isSatisfied: Bool { self == .satisfied }

    private enum CodingKeys: String, CodingKey { case kind, activeVersion, installedURL }
    private enum Kind: String, Codable {
        case satisfied, missingLocal, missingInstallable, disabled, notBuilt
        case versionMismatch, sourceConflict, cycle, blocked, invalidRequirement, selfReference
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        switch try container.decode(Kind.self, forKey: .kind) {
        case .satisfied: self = .satisfied
        case .missingLocal: self = .missingLocal
        case .missingInstallable: self = .missingInstallable
        case .disabled: self = .disabled
        case .notBuilt: self = .notBuilt
        case .versionMismatch:
            self = .versionMismatch(activeVersion: try container.decode(String.self, forKey: .activeVersion))
        case .sourceConflict:
            self = .sourceConflict(installedURL: try container.decode(String.self, forKey: .installedURL))
        case .cycle: self = .cycle
        case .blocked: self = .blocked
        case .invalidRequirement: self = .invalidRequirement
        case .selfReference: self = .selfReference
        }
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        switch self {
        case .satisfied: try container.encode(Kind.satisfied, forKey: .kind)
        case .missingLocal: try container.encode(Kind.missingLocal, forKey: .kind)
        case .missingInstallable: try container.encode(Kind.missingInstallable, forKey: .kind)
        case .disabled: try container.encode(Kind.disabled, forKey: .kind)
        case .notBuilt: try container.encode(Kind.notBuilt, forKey: .kind)
        case let .versionMismatch(version):
            try container.encode(Kind.versionMismatch, forKey: .kind)
            try container.encode(version, forKey: .activeVersion)
        case let .sourceConflict(url):
            try container.encode(Kind.sourceConflict, forKey: .kind)
            try container.encode(url, forKey: .installedURL)
        case .cycle: try container.encode(Kind.cycle, forKey: .kind)
        case .blocked: try container.encode(Kind.blocked, forKey: .kind)
        case .invalidRequirement: try container.encode(Kind.invalidRequirement, forKey: .kind)
        case .selfReference: try container.encode(Kind.selfReference, forKey: .kind)
        }
    }
}

struct TweakPackageDependencyStatus: Codable, Identifiable, Equatable, Sendable {
    let dependentPackageID: String
    let dependencyID: String
    let requirement: String
    let declaredSource: TweakPackageSource?
    let resolvedOrigin: TweakPackageOrigin?
    let installedVersion: String?
    let activeVersion: String?
    let state: TweakPackageDependencyState

    var id: String { dependencyID }
}

struct TweakPackagePriorityConstraint: Codable, Equatable, Sendable {
    let packageID: String
    let actualLoadPosition: Int
    let mustLoadAfterPackageIDs: [String]
    let mustLoadBeforePackageIDs: [String]
}
