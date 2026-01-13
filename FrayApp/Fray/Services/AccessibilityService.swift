import SwiftUI

enum AccessibilityIdentifiers {
    static let sidebar = "fray.sidebar"
    static let messageList = "fray.messageList"
    static let activityPanel = "fray.activityPanel"
    static let commandPalette = "fray.commandPalette"
    static let messageInput = "fray.messageInput"

    static func message(_ id: String) -> String { "fray.message.\(id)" }
    static func thread(_ id: String) -> String { "fray.thread.\(id)" }
    static func agent(_ id: String) -> String { "fray.agent.\(id)" }
}

struct AccessibleMessageView: ViewModifier {
    let message: FrayMessage
    let isSelected: Bool

    func body(content: Content) -> some View {
        content
            .accessibilityIdentifier(AccessibilityIdentifiers.message(message.id))
            .accessibilityLabel(accessibilityLabel)
            .accessibilityHint("Double tap to reply")
            .accessibilityAddTraits(isSelected ? .isSelected : [])
    }

    private var accessibilityLabel: String {
        var parts: [String] = []

        parts.append("Message from \(message.fromAgent)")

        if let replyTo = message.replyTo {
            parts.append("replying to message")
        }

        parts.append(message.body)

        if !message.reactions.isEmpty {
            let reactionCount = message.reactions.values.reduce(0) { $0 + $1.count }
            parts.append("\(reactionCount) reactions")
        }

        if message.edited == true {
            parts.append("edited")
        }

        return parts.joined(separator: ". ")
    }
}

struct AccessibleAgentView: ViewModifier {
    let agent: FrayAgent

    func body(content: Content) -> some View {
        content
            .accessibilityIdentifier(AccessibilityIdentifiers.agent(agent.agentId))
            .accessibilityLabel(accessibilityLabel)
    }

    private var accessibilityLabel: String {
        var parts: [String] = ["Agent \(agent.agentId)"]

        if let presence = agent.presence {
            parts.append(presenceDescription(presence))
        }

        if agent.managed == true {
            parts.append("managed")
        }

        if let status = agent.status {
            parts.append("status: \(status)")
        }

        return parts.joined(separator: ", ")
    }

    private func presenceDescription(_ presence: FrayAgent.AgentPresence) -> String {
        switch presence {
        case .active: return "active"
        case .spawning: return "starting up"
        case .prompting: return "being prompted"
        case .prompted: return "prompted"
        case .idle: return "idle"
        case .error: return "has an error"
        case .offline: return "offline"
        case .brb: return "will be right back"
        }
    }
}

struct AccessibleThreadView: ViewModifier {
    let thread: FrayThread
    let hasUnread: Bool

    func body(content: Content) -> some View {
        content
            .accessibilityIdentifier(AccessibilityIdentifiers.thread(thread.guid))
            .accessibilityLabel(accessibilityLabel)
            .accessibilityHint("Double tap to open thread")
            .accessibilityAddTraits(hasUnread ? .updatesFrequently : [])
    }

    private var accessibilityLabel: String {
        var parts: [String] = ["Thread: \(thread.name)"]

        if thread.status == .archived {
            parts.append("archived")
        }

        if thread.type == .knowledge {
            parts.append("knowledge thread")
        }

        if hasUnread {
            parts.append("has unread messages")
        }

        return parts.joined(separator: ", ")
    }
}

extension View {
    func accessibleMessage(_ message: FrayMessage, isSelected: Bool = false) -> some View {
        modifier(AccessibleMessageView(message: message, isSelected: isSelected))
    }

    func accessibleAgent(_ agent: FrayAgent) -> some View {
        modifier(AccessibleAgentView(agent: agent))
    }

    func accessibleThread(_ thread: FrayThread, hasUnread: Bool = false) -> some View {
        modifier(AccessibleThreadView(thread: thread, hasUnread: hasUnread))
    }
}

struct ReduceMotionModifier: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func body(content: Content) -> some View {
        content.transaction { transaction in
            if reduceMotion {
                transaction.animation = nil
            }
        }
    }
}

extension View {
    func respectsReduceMotion() -> some View {
        modifier(ReduceMotionModifier())
    }
}

struct DynamicTypeModifier: ViewModifier {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    let baseSize: CGFloat
    let minSize: CGFloat
    let maxSize: CGFloat

    func body(content: Content) -> some View {
        content.font(.system(size: scaledSize))
    }

    private var scaledSize: CGFloat {
        let scale = dynamicTypeScale
        let scaled = baseSize * scale
        return min(max(scaled, minSize), maxSize)
    }

    private var dynamicTypeScale: CGFloat {
        switch dynamicTypeSize {
        case .xSmall: return 0.8
        case .small: return 0.9
        case .medium: return 1.0
        case .large: return 1.1
        case .xLarge: return 1.2
        case .xxLarge: return 1.3
        case .xxxLarge: return 1.4
        case .accessibility1: return 1.5
        case .accessibility2: return 1.7
        case .accessibility3: return 1.9
        case .accessibility4: return 2.1
        case .accessibility5: return 2.3
        @unknown default: return 1.0
        }
    }
}

extension View {
    func dynamicTypeFont(base: CGFloat, min: CGFloat = 10, max: CGFloat = 32) -> some View {
        modifier(DynamicTypeModifier(baseSize: base, minSize: min, maxSize: max))
    }
}
