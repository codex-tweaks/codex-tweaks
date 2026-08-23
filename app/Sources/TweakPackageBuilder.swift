import Foundation

struct NodeEnvironment: Codable, Equatable, Sendable {
    let nodePath: String
    let npmPath: String
    let npxPath: String
    let version: String

    var nodeURL: URL { URL(fileURLWithPath: nodePath) }
    var npmURL: URL { URL(fileURLWithPath: npmPath) }
    var npxURL: URL { URL(fileURLWithPath: npxPath) }
}
