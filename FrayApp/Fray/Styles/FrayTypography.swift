import SwiftUI

enum FrayTypography {
    static let body = Font.body
    static let bodyMono = Font.body.monospaced()

    static let caption = Font.caption
    static let captionMono = Font.caption.monospaced()

    static let headline = Font.headline
    static let subheadline = Font.subheadline

    static let title = Font.title3
    static let title2 = Font.title2

    static let agentName = Font.system(.body, design: .monospaced, weight: .medium)
    static let timestamp = Font.caption.monospacedDigit()

    static let messageBody = Font.system(.body, design: .default)
    static let codeBlock = Font.system(.body, design: .monospaced)
    static let inlineCode = Font.system(.body, design: .monospaced)

    static let reactionCount = Font.caption.monospacedDigit()
    static let badge = Font.caption2.weight(.semibold)
}
