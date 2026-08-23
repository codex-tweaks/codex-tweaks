import AppKit
import Foundation

enum TweakAuthoringPromptBuilder {
    static let skillDirectoryName = "develop-codex-tweaks-package"

    static var sourceSkillURL: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent("Skills", isDirectory: true)
            .appendingPathComponent(skillDirectoryName, isDirectory: true)
            .appendingPathComponent("SKILL.md")
    }

    static func skillContents(bundle: Bundle = .main) -> String? {
        let bundledURL = bundle.url(
            forResource: "SKILL",
            withExtension: "md",
            subdirectory: "Skills/\(skillDirectoryName)"
        )
        for url in [bundledURL, sourceSkillURL].compactMap({ $0 }) {
            if let contents = try? String(contentsOf: url, encoding: .utf8) {
                return contents
            }
        }
        return nil
    }

    static func makePrompt(
        tweaksDirectoryURL _: URL,
        bundle: Bundle = .main
    ) -> String {
        skillContents(bundle: bundle) ?? "无法读取 Codex Tweaks 功能包开发 Skill。"
    }

    @discardableResult
    static func copyPrompt(
        tweaksDirectoryURL _: URL,
        to pasteboard: NSPasteboard = .general,
        bundle: Bundle = .main
    ) -> Bool {
        guard let contents = skillContents(bundle: bundle) else { return false }
        pasteboard.clearContents()
        return pasteboard.setString(contents, forType: .string)
    }
}
