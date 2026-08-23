import Foundation

enum PresentationText {
    static func resolve(
        _ key: PresentationTextKey,
        contract: BackendPresentationContract? = nil,
        replacements: [String: String] = [:]
    ) -> String {
        var value = contract?.text[key.rawValue] ?? GeneratedPresentationDefaults.text[key] ?? ""
        for (name, replacement) in replacements {
            value = value.replacingOccurrences(of: "{\(name)}", with: replacement)
        }
        return value
    }
}
