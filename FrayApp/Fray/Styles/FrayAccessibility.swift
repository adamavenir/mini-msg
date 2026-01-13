import SwiftUI

extension View {
    func accessibleMessageBubble(from agent: String, body: String, timestamp: Date) -> some View {
        self
            .accessibilityElement(children: .combine)
            .accessibilityLabel("Message from \(agent)")
            .accessibilityValue(body)
            .accessibilityHint("Double-tap to view actions")
    }

    func accessibleAgent(name: String, presence: FrayAgent.AgentPresence?) -> some View {
        self
            .accessibilityElement(children: .combine)
            .accessibilityLabel(agentAccessibilityLabel(name: name, presence: presence))
    }

    func accessibleThread(name: String, isArchived: Bool, isKnowledge: Bool) -> some View {
        self
            .accessibilityElement(children: .combine)
            .accessibilityLabel(threadAccessibilityLabel(name: name, isArchived: isArchived, isKnowledge: isKnowledge))
            .accessibilityAddTraits(.isButton)
    }

    func accessiblePresenceIndicator(_ presence: FrayAgent.AgentPresence) -> some View {
        self
            .accessibilityLabel(presenceAccessibilityLabel(presence))
    }

    @ViewBuilder
    func reduceMotionAnimation<V: Equatable>(_ value: V, reduceMotion: Bool) -> some View {
        if reduceMotion {
            self.animation(nil, value: value)
        } else {
            self.animation(.easeInOut(duration: 0.15), value: value)
        }
    }
}

private func agentAccessibilityLabel(name: String, presence: FrayAgent.AgentPresence?) -> String {
    guard let presence = presence else {
        return "Agent \(name)"
    }
    return "Agent \(name), \(presenceAccessibilityLabel(presence))"
}

private func threadAccessibilityLabel(name: String, isArchived: Bool, isKnowledge: Bool) -> String {
    var parts = ["Thread", name]
    if isArchived { parts.append("archived") }
    if isKnowledge { parts.append("knowledge type") }
    return parts.joined(separator: ", ")
}

private func presenceAccessibilityLabel(_ presence: FrayAgent.AgentPresence) -> String {
    switch presence {
    case .active: return "active"
    case .spawning: return "spawning"
    case .prompting: return "prompting"
    case .prompted: return "prompted"
    case .idle: return "idle"
    case .error: return "error"
    case .offline: return "offline"
    case .brb: return "will be right back"
    }
}

struct ReduceMotionEnvironmentKey: EnvironmentKey {
    static let defaultValue = false
}

extension EnvironmentValues {
    var frayReduceMotion: Bool {
        get { self[ReduceMotionEnvironmentKey.self] }
        set { self[ReduceMotionEnvironmentKey.self] = newValue }
    }
}
