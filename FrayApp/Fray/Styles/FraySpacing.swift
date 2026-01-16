import SwiftUI

enum FraySpacing {
    static let xs: CGFloat = 4
    static let sm: CGFloat = 8
    static let md: CGFloat = 16
    static let lg: CGFloat = 24
    static let xl: CGFloat = 32
    static let xxl: CGFloat = 48

    static let messagePadding: CGFloat = 12
    static let messageSpacing: CGFloat = 16
    static let groupedMessageSpacing: CGFloat = 4
    static let avatarSize: CGFloat = 32
    static let sidebarWidth: CGFloat = 280
    static let activityPanelWidth: CGFloat = 200
    static let minContentWidth: CGFloat = 400

    static let cornerRadius: CGFloat = 12
    static let smallCornerRadius: CGFloat = 8
    static let badgeCornerRadius: CGFloat = 6

    // Input alignment: matches avatar column + gap so input aligns with message content
    static let inputLeadingPadding: CGFloat = avatarSize + sm

    // Component sizes
    static let presenceIndicatorSize: CGFloat = 10
    static let presenceIndicatorSizeLarge: CGFloat = 12
    static let commandPaletteWidth: CGFloat = 600
    static let commandPaletteHeight: CGFloat = 400
    static let inputMinHeight: CGFloat = 36
    static let inputMaxHeight: CGFloat = 120

    // Timing constants (seconds)
    static let hoverGracePeriod: Double = 0.2
    static let messageGroupTimeWindow: Int64 = 120
}
