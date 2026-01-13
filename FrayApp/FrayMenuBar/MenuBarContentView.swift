import SwiftUI

struct MenuBarContentView: View {
    let bridge: FrayBridge
    @Binding var unreadCount: Int
    @Binding var activeAgents: [FrayAgent]

    @State private var composeText: String = ""
    @State private var showCompose: Bool = false
    @State private var isConnected: Bool = false
    @State private var recentMessages: [FrayMessage] = []
    @State private var refreshTimer: Timer?

    var body: some View {
        VStack(spacing: 0) {
            headerSection

            Divider()

            if !isConnected {
                notConnectedView
            } else if showCompose {
                composeSection
            } else {
                mainContent
            }

            Divider()

            footerSection
        }
        .frame(width: 320)
        .task {
            await connect()
            startRefreshTimer()
        }
        .onDisappear {
            stopRefreshTimer()
        }
    }

    private var headerSection: some View {
        HStack {
            Text("Fray")
                .font(.headline)

            Spacer()

            if unreadCount > 0 {
                Text("\(unreadCount) unread")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 2)
                    .background(.secondary.opacity(0.15))
                    .clipShape(Capsule())
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
    }

    private var notConnectedView: some View {
        VStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle")
                .font(.title2)
                .foregroundStyle(.secondary)

            Text("Not connected")
                .font(.subheadline)
                .foregroundStyle(.secondary)

            Text("Open a Fray project to connect")
                .font(.caption)
                .foregroundStyle(.tertiary)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 24)
    }

    private var composeSection: some View {
        VStack(spacing: 8) {
            HStack {
                Button(action: { showCompose = false }) {
                    Image(systemName: "chevron.left")
                }
                .buttonStyle(.plain)

                Text("Quick Message")
                    .font(.subheadline.weight(.medium))

                Spacer()
            }
            .padding(.horizontal, 12)
            .padding(.top, 8)

            TextEditor(text: $composeText)
                .font(.body)
                .frame(height: 80)
                .padding(8)
                .background(.secondary.opacity(0.1))
                .clipShape(RoundedRectangle(cornerRadius: 8))
                .padding(.horizontal, 12)

            HStack {
                Spacer()

                Button("Cancel") {
                    composeText = ""
                    showCompose = false
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)

                Button("Send") {
                    sendMessage()
                }
                .buttonStyle(.borderedProminent)
                .disabled(composeText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
            .padding(.horizontal, 12)
            .padding(.bottom, 8)
        }
    }

    private var mainContent: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                if !activeAgents.isEmpty {
                    agentsSection
                }

                if !recentMessages.isEmpty {
                    messagesSection
                }

                if activeAgents.isEmpty && recentMessages.isEmpty {
                    emptyStateView
                }
            }
            .padding(.vertical, 8)
        }
        .frame(maxHeight: 300)
    }

    private var agentsSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Active Agents")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .padding(.horizontal, 12)

            ForEach(activeAgents.prefix(5)) { agent in
                MenuBarAgentRow(agent: agent)
            }

            if activeAgents.count > 5 {
                Text("+\(activeAgents.count - 5) more")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
                    .padding(.horizontal, 12)
            }
        }
    }

    private var messagesSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Recent Messages")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .padding(.horizontal, 12)

            ForEach(recentMessages.prefix(3)) { message in
                MenuBarMessageRow(message: message)
            }
        }
    }

    private var emptyStateView: some View {
        VStack(spacing: 4) {
            Text("All quiet")
                .font(.subheadline)
                .foregroundStyle(.secondary)

            Text("No active agents or recent messages")
                .font(.caption)
                .foregroundStyle(.tertiary)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 24)
    }

    private var footerSection: some View {
        HStack(spacing: 12) {
            Button(action: { showCompose = true }) {
                Label("Compose", systemImage: "square.and.pencil")
            }
            .buttonStyle(.plain)
            .disabled(!isConnected)

            Spacer()

            Button(action: openMainApp) {
                Label("Open Fray", systemImage: "arrow.up.forward.app")
            }
            .buttonStyle(.plain)

            Button(action: { NSApplication.shared.terminate(nil) }) {
                Image(systemName: "xmark")
            }
            .buttonStyle(.plain)
            .foregroundStyle(.secondary)
        }
        .font(.caption)
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
    }

    private func connect() async {
        guard let projectPath = FrayBridge.discoverProject(
            from: FileManager.default.currentDirectoryPath
        ) else {
            return
        }

        do {
            try bridge.connect(projectPath: projectPath)
            isConnected = true
            await refresh()
        } catch {
            print("Failed to connect: \(error)")
        }
    }

    private func refresh() async {
        guard isConnected else { return }

        do {
            let agents = try bridge.getAgents()
            activeAgents = agents.filter {
                $0.presence == .active ||
                $0.presence == .spawning ||
                $0.presence == .prompting
            }

            let page = try bridge.getMessages(limit: 10)
            recentMessages = page.messages

            unreadCount = 0
        } catch {
            print("Refresh failed: \(error)")
        }
    }

    private func sendMessage() {
        guard !composeText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return
        }

        Task {
            do {
                _ = try bridge.postMessage(
                    body: composeText,
                    from: "menubar"
                )
                composeText = ""
                showCompose = false
                await refresh()
            } catch {
                print("Failed to send: \(error)")
            }
        }
    }

    private func openMainApp() {
        NSWorkspace.shared.open(URL(string: "fray://open")!)
    }

    private func startRefreshTimer() {
        refreshTimer = Timer.scheduledTimer(withTimeInterval: 10, repeats: true) { _ in
            Task { await refresh() }
        }
    }

    private func stopRefreshTimer() {
        refreshTimer?.invalidate()
        refreshTimer = nil
    }
}

struct MenuBarAgentRow: View {
    let agent: FrayAgent

    var body: some View {
        HStack(spacing: 8) {
            Circle()
                .fill(presenceColor)
                .frame(width: 8, height: 8)

            Text("@\(agent.agentId)")
                .font(.caption)
                .lineLimit(1)

            Spacer()

            if let status = agent.status {
                Text(status)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 4)
    }

    private var presenceColor: Color {
        switch agent.presence {
        case .active: return .green
        case .spawning: return .yellow
        case .prompting, .prompted: return .orange
        case .idle: return .gray
        case .error: return .red
        case .brb: return .purple
        case .offline, nil: return .gray.opacity(0.5)
        }
    }
}

struct MenuBarMessageRow: View {
    let message: FrayMessage

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Text("@\(message.fromAgent)")
                    .font(.caption.weight(.medium))

                Spacer()

                Text(formatTime(message.ts))
                    .font(.caption2)
                    .foregroundStyle(.tertiary)
            }

            Text(message.body)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(2)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 4)
    }

    private func formatTime(_ ts: Int64) -> String {
        let date = Date(timeIntervalSince1970: Double(ts) / 1000.0)
        let formatter = DateFormatter()
        formatter.timeStyle = .short
        return formatter.string(from: date)
    }
}

#Preview {
    MenuBarContentView(
        bridge: FrayBridge(),
        unreadCount: .constant(3),
        activeAgents: .constant([])
    )
}
