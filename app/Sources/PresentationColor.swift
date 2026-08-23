import SwiftUI

extension BackendPresentationTokens {
    var accentColorValue: Color {
        Color(presentationHex: accentColor) ?? .accentColor
    }

    var successColorValue: Color {
        Color(presentationHex: successColor) ?? .green
    }

    var warningColorValue: Color {
        Color(presentationHex: warningColor) ?? .orange
    }

    var dangerColorValue: Color {
        Color(presentationHex: dangerColor) ?? .red
    }
}

private extension Color {
    init?(presentationHex: String) {
        let value = presentationHex.trimmingCharacters(in: CharacterSet(charactersIn: "#"))
        guard value.count == 6, let rgb = UInt64(value, radix: 16) else { return nil }
        self.init(
            .sRGB,
            red: Double((rgb >> 16) & 0xff) / 255,
            green: Double((rgb >> 8) & 0xff) / 255,
            blue: Double(rgb & 0xff) / 255,
            opacity: 1
        )
    }
}
