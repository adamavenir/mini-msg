import SwiftUI

struct AgentActivityRow: View {
    let agent: FrayAgent
    var usage: AgentUsage?

    var body: some View {
        HStack(spacing: FraySpacing.sm) {
            PresenceIndicatorAnimated(presence: agent.presence ?? .offline)

            VStack(alignment: .leading, spacing: 2) {
                Text("@\(agent.agentId)")
                    .font(FrayTypography.agentName)
                    .foregroundStyle(FrayColors.colorForAgent(agent.agentId))

                if let status = agent.status, !status.isEmpty {
                    Text(status)
                        .font(FrayTypography.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }

                if let lastActivity = formattedLastActivity {
                    Text(lastActivity)
                        .font(FrayTypography.timestamp)
                        .foregroundStyle(.tertiary)
                }

                if let usage = usage, shouldShowUsage {
                    ContextUsageBar(usage: usage)
                }
            }

            Spacer()
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
        .accessibilityLabel(accessibilityLabel)
    }

    private var shouldShowUsage: Bool {
        guard let presence = agent.presence else { return false }
        return presence == .active || presence == .idle || presence == .prompting || presence == .prompted
    }

    private var accessibilityLabel: String {
        var parts = ["Agent \(agent.agentId)"]
        if let presence = agent.presence {
            parts.append(presenceDescription(presence))
        }
        if let status = agent.status, !status.isEmpty {
            parts.append(status)
        }
        if let lastActivity = formattedLastActivity {
            parts.append("Last active \(lastActivity)")
        }
        return parts.joined(separator: ", ")
    }

    private func presenceDescription(_ presence: FrayAgent.AgentPresence) -> String {
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

    private var formattedLastActivity: String? {
        let ts = agent.lastHeartbeat ?? agent.lastSeen
        guard ts > 0 else { return nil }

        let date = Date(timeIntervalSince1970: Double(ts))
        let now = Date()
        let interval = now.timeIntervalSince(date)

        if interval < 60 {
            return "just now"
        } else if interval < 3600 {
            let mins = Int(interval / 60)
            return "\(mins)m ago"
        } else if interval < 86400 {
            let hours = Int(interval / 3600)
            return "\(hours)h ago"
        } else {
            let days = Int(interval / 86400)
            return "\(days)d ago"
        }
    }

}

struct PresenceIndicatorAnimated: View {
    let presence: FrayAgent.AgentPresence
    @State private var isAnimating = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var iconName: String {
        switch presence {
        case .active: return "circle.fill"
        case .spawning: return "arrow.triangle.2.circlepath"
        case .prompting, .prompted: return "questionmark.circle"
        case .idle: return "circle"
        case .error: return "exclamationmark.triangle"
        case .offline: return "circle.dotted"
        case .brb: return "moon.fill"
        }
    }

    var body: some View {
        Image(systemName: iconName)
            .font(.system(size: FraySpacing.presenceIndicatorSizeLarge))
            .foregroundStyle(FrayColors.presence[presence] ?? .gray)
            .symbolEffect(.pulse, options: .repeating, isActive: presence == .spawning && !reduceMotion)
            .accessibilityLabel(presenceAccessibilityLabel)
    }

    private var presenceAccessibilityLabel: String {
        switch presence {
        case .active: return "Active"
        case .spawning: return "Spawning"
        case .prompting: return "Prompting"
        case .prompted: return "Prompted"
        case .idle: return "Idle"
        case .error: return "Error"
        case .offline: return "Offline"
        case .brb: return "Will be right back"
        }
    }
}

struct ContextUsageBar: View {
    let usage: AgentUsage

    var body: some View {
        HStack(spacing: 4) {
            GeometryReader { geometry in
                ZStack(alignment: .leading) {
                    RoundedRectangle(cornerRadius: 2)
                        .fill(Color.secondary.opacity(0.2))

                    RoundedRectangle(cornerRadius: 2)
                        .fill(barColor)
                        .frame(width: geometry.size.width * fillPercentage)
                }
            }
            .frame(height: 4)

            Text(formattedUsage)
                .font(.system(size: 9, weight: .medium, design: .monospaced))
                .foregroundStyle(barColor)
        }
        .accessibilityLabel("Context usage: \(usage.contextPercent) percent")
    }

    private var fillPercentage: CGFloat {
        min(1.0, CGFloat(usage.contextPercent) / 100.0)
    }

    private var barColor: Color {
        switch usage.contextPercent {
        case 0..<50: return .green
        case 50..<80: return .yellow
        default: return .red
        }
    }

    private var formattedUsage: String {
        let k = Double(usage.inputTokens) / 1000.0
        if k >= 1000 {
            return String(format: "%.0fM", k / 1000.0)
        } else if k >= 100 {
            return String(format: "%.0fk", k)
        } else {
            return String(format: "%.1fk", k)
        }
    }
}

#Preview {
    VStack(spacing: 16) {
        AgentActivityRow(
            agent: FrayAgent(
                guid: "usr-12345678",
                agentId: "opus",
                status: "Working on macOS client",
                purpose: nil,
                avatar: nil,
                registeredAt: 0,
                lastSeen: Int64(Date().timeIntervalSince1970 * 1000) - 300000,
                leftAt: nil,
                managed: true,
                invoke: nil,
                presence: .active,
                mentionWatermark: nil,
                reactionWatermark: nil,
                lastHeartbeat: Int64(Date().timeIntervalSince1970 * 1000) - 60000,
                lastSessionId: nil,
                sessionMode: nil,
                jobId: nil,
                jobIdx: nil,
                isEphemeral: nil
            ),
            usage: AgentUsage(
                sessionId: "abc-123",
                driver: "claude",
                model: "claude-sonnet-4",
                inputTokens: 85000,
                outputTokens: 12000,
                cachedTokens: 5000,
                contextLimit: 200000,
                contextPercent: 42
            )
        )

        AgentActivityRow(
            agent: FrayAgent(
                guid: "usr-23456789",
                agentId: "designer",
                status: nil,
                purpose: nil,
                avatar: nil,
                registeredAt: 0,
                lastSeen: Int64(Date().timeIntervalSince1970 * 1000) - 7200000,
                leftAt: nil,
                managed: true,
                invoke: nil,
                presence: .idle,
                mentionWatermark: nil,
                reactionWatermark: nil,
                lastHeartbeat: nil,
                lastSessionId: nil,
                sessionMode: nil,
                jobId: nil,
                jobIdx: nil,
                isEphemeral: nil
            ),
            usage: AgentUsage(
                sessionId: "xyz-456",
                driver: "codex",
                model: "gpt-5",
                inputTokens: 100000,
                outputTokens: 25000,
                cachedTokens: 8000,
                contextLimit: 128000,
                contextPercent: 78
            )
        )

        AgentActivityRow(agent: FrayAgent(
            guid: "usr-34567890",
            agentId: "reviewer",
            status: nil,
            purpose: nil,
            avatar: nil,
            registeredAt: 0,
            lastSeen: Int64(Date().timeIntervalSince1970 * 1000) - 86400000,
            leftAt: nil,
            managed: false,
            invoke: nil,
            presence: .offline,
            mentionWatermark: nil,
            reactionWatermark: nil,
            lastHeartbeat: nil,
            lastSessionId: nil,
            sessionMode: nil,
            jobId: nil,
            jobIdx: nil,
            isEphemeral: nil
        ))
    }
    .padding()
    .frame(width: 250)
}
