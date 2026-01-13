import SwiftUI

enum FrayColors {
    static let agentColors: [Color] = [
        Color(hex: "5AC8FA"), Color(hex: "34C759"), Color(hex: "FFD60A"),
        Color(hex: "FF9F0A"), Color(hex: "FF453A"), Color(hex: "FF2D55"),
        Color(hex: "BF5AF2"), Color(hex: "5856D6"), Color(hex: "007AFF"),
        Color(hex: "64D2FF"), Color(hex: "AC8E68"), Color(hex: "98989D"),
        Color(hex: "00C7BE"), Color(hex: "FF6961"), Color(hex: "77DD77"),
        Color(hex: "AEC6CF")
    ]

    static func colorForAgent(_ agentId: String) -> Color {
        let hash = agentId.utf8.reduce(0) { $0 &+ Int($1) }
        return agentColors[abs(hash) % agentColors.count]
    }

    static let presence: [FrayAgent.AgentPresence: Color] = [
        .active: .green,
        .spawning: .yellow,
        .prompting: .orange,
        .prompted: .orange,
        .idle: .gray,
        .error: .red,
        .offline: .gray.opacity(0.5),
        .brb: .purple
    ]

    static let background = Color(nsColor: .windowBackgroundColor)
    static let secondaryBackground = Color(nsColor: .controlBackgroundColor)
    static let tertiaryBackground = Color(nsColor: .underPageBackgroundColor)

    static let text = Color(nsColor: .labelColor)
    static let secondaryText = Color(nsColor: .secondaryLabelColor)
    static let tertiaryText = Color(nsColor: .tertiaryLabelColor)

    static let separator = Color(nsColor: .separatorColor)
    static let accent = Color.accentColor

    static let messageBubble = AdaptiveColor(
        light: Color(hex: "F5F5F7"),
        dark: Color(hex: "2C2C2E")
    )

    static let messageBubbleOwn = AdaptiveColor(
        light: Color(hex: "007AFF").opacity(0.15),
        dark: Color(hex: "0A84FF").opacity(0.2)
    )

    static let codeBackground = AdaptiveColor(
        light: Color(hex: "F5F5F7"),
        dark: Color(hex: "1C1C1E")
    )

    static let commandPaletteBackground = AdaptiveColor(
        light: Color.white.opacity(0.95),
        dark: Color(hex: "2C2C2E").opacity(0.95)
    )

    static let threadHover = AdaptiveColor(
        light: Color.black.opacity(0.05),
        dark: Color.white.opacity(0.08)
    )

    static let presenceBadgeBackground = AdaptiveColor(
        light: Color.black.opacity(0.08),
        dark: Color.white.opacity(0.12)
    )

    static let modalOverlay = AdaptiveColor(
        light: Color.black.opacity(0.2),
        dark: Color.black.opacity(0.4)
    )
}

struct AdaptiveColor {
    let light: Color
    let dark: Color

    func resolve(for colorScheme: ColorScheme) -> Color {
        colorScheme == .dark ? dark : light
    }
}

extension View {
    func adaptiveBackground(_ color: AdaptiveColor) -> some View {
        modifier(AdaptiveBackgroundModifier(color: color))
    }
}

struct AdaptiveBackgroundModifier: ViewModifier {
    let color: AdaptiveColor
    @Environment(\.colorScheme) private var colorScheme

    func body(content: Content) -> some View {
        content.background(color.resolve(for: colorScheme))
    }
}

extension Color {
    init(hex: String) {
        let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        var int: UInt64 = 0
        Scanner(string: hex).scanHexInt64(&int)
        let r = Double((int >> 16) & 0xFF) / 255
        let g = Double((int >> 8) & 0xFF) / 255
        let b = Double(int & 0xFF) / 255
        self.init(red: r, green: g, blue: b)
    }
}
