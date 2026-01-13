import SwiftUI

struct SidebarView: View {
    @Binding var selectedThread: FrayThread?
    @Binding var currentChannel: FrayChannel?
    let currentAgentId: String?

    @Environment(FrayBridge.self) private var bridge

    @State private var threads: [FrayThread] = []
    @State private var channels: [FrayChannel] = []
    @State private var favedThreadGuids: Set<String> = []

    var favedThreads: [FrayThread] {
        threads.filter { favedThreadGuids.contains($0.guid) }
    }

    var unfavedRootThreads: [FrayThread] {
        threads.filter { $0.parentThread == nil && !favedThreadGuids.contains($0.guid) }
    }

    var body: some View {
        List(selection: $selectedThread) {
            channelPickerSection
            roomSection
            favedThreadsSection
            unfavedThreadsSection
        }
        .listStyle(.sidebar)
        .navigationTitle(currentChannel?.name ?? "Fray")
        .task {
            await loadData()
        }
        .onChange(of: bridge.projectPath) { _, _ in
            // Reload when bridge connects to a different project
            Task { await loadThreadsAndFaves() }
        }
    }

    @ViewBuilder
    private var channelPickerSection: some View {
        if channels.count > 1 {
            Picker("Channel", selection: $currentChannel) {
                ForEach(channels) { channel in
                    Text(channel.name)
                        .tag(channel as FrayChannel?)
                }
            }
            .pickerStyle(.menu)
            .labelsHidden()
            .padding(.bottom, FraySpacing.sm)
        }
    }

    @ViewBuilder
    private var roomSection: some View {
        HStack(spacing: FraySpacing.md) {
            Image(systemName: "bubble.left.and.bubble.right")
            Text(currentChannel?.name ?? "Room")
                .fontWeight(.semibold)
                .lineLimit(1)
            Spacer()
        }
        .contentShape(Rectangle())
        .onTapGesture {
            selectedThread = nil
        }
        .listRowBackground(selectedThread == nil ? Color.accentColor.opacity(0.15) : nil)
    }

    @ViewBuilder
    private var favedThreadsSection: some View {
        if !favedThreads.isEmpty {
            ForEach(favedThreads) { thread in
                FavedThreadRow(
                    thread: thread,
                    selectedThread: $selectedThread,
                    onUnfave: { unfaveThread(thread.guid) }
                )
            }
        }
    }

    @ViewBuilder
    private var unfavedThreadsSection: some View {
        ForEach(unfavedRootThreads) { thread in
            ThreadListItem(
                thread: thread,
                allThreads: threads,
                favedIds: favedThreadGuids,
                selectedThread: $selectedThread,
                onFave: { faveThread($0) }
            )
        }
    }

    private func loadData() async {
        // Load channels
        do {
            channels = try FrayBridge.listChannels()
            // If no current channel but we have channels, select first
            if currentChannel == nil, let first = channels.first {
                currentChannel = first
            }
        } catch {
            print("Failed to load channels: \(error)")
        }

        // Load threads
        do {
            threads = try bridge.getThreads()
        } catch {
            print("Failed to load threads: \(error)")
        }

        // Load faves from fray
        await loadFaves()
    }

    private func loadThreadsAndFaves() async {
        // Load threads for current channel
        do {
            threads = try bridge.getThreads()
        } catch {
            print("Failed to load threads: \(error)")
            threads = []
        }

        // Load faves
        await loadFaves()
    }

    private func loadFaves() async {
        guard let agentId = currentAgentId else {
            favedThreadGuids = []
            return
        }

        do {
            let faves = try bridge.getFaves(agentId: agentId, itemType: "thread")
            favedThreadGuids = Set(faves.map { $0.itemGuid })
        } catch {
            print("Failed to load faves: \(error)")
            favedThreadGuids = []
        }
    }

    private func faveThread(_ guid: String) {
        guard let agentId = currentAgentId else { return }
        Task {
            do {
                try bridge.faveItem(itemGuid: guid, agentId: agentId)
                favedThreadGuids.insert(guid)
            } catch {
                print("Failed to fave thread: \(error)")
            }
        }
    }

    private func unfaveThread(_ guid: String) {
        guard let agentId = currentAgentId else { return }
        Task {
            do {
                try bridge.unfaveItem(itemGuid: guid, agentId: agentId)
                favedThreadGuids.remove(guid)
            } catch {
                print("Failed to unfave thread: \(error)")
            }
        }
    }
}

struct FavedThreadRow: View {
    let thread: FrayThread
    @Binding var selectedThread: FrayThread?
    let onUnfave: () -> Void

    @State private var isHovering = false

    var body: some View {
        HStack(spacing: FraySpacing.md) {
            Image(systemName: "star.fill")
                .foregroundStyle(.yellow)
                .font(.caption)

            Text(thread.name)
                .lineLimit(1)

            Spacer()

            if isHovering {
                Button(action: onUnfave) {
                    Image(systemName: "star.slash")
                        .font(.caption)
                }
                .buttonStyle(.borderless)
                .help("Remove from favorites")
            }
        }
        .contentShape(Rectangle())
        .onTapGesture {
            selectedThread = thread
        }
        .onHover { isHovering = $0 }
        .tag(thread)
    }
}

struct AgentListSection: View {
    let agents: [FrayAgent]

    var body: some View {
        ForEach(agents) { agent in
            AgentListItem(agent: agent)
        }
    }
}

struct AgentListItem: View {
    let agent: FrayAgent

    var body: some View {
        HStack(spacing: FraySpacing.sm) {
            AgentAvatar(agentId: agent.agentId, size: 20)

            Text("@\(agent.agentId)")
                .font(FrayTypography.agentName)

            Spacer()

            if let presence = agent.presence {
                PresenceIndicator(presence: presence)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(agentAccessibilityLabel)
    }

    private var agentAccessibilityLabel: String {
        var label = "Agent \(agent.agentId)"
        if let presence = agent.presence {
            label += ", \(presenceText(presence))"
        }
        return label
    }

    private func presenceText(_ presence: FrayAgent.AgentPresence) -> String {
        switch presence {
        case .active: return "active"
        case .spawning: return "spawning"
        case .prompting, .prompted: return "prompting"
        case .idle: return "idle"
        case .error: return "error"
        case .offline: return "offline"
        case .brb: return "will be right back"
        }
    }
}

struct AgentAvatar: View {
    let agentId: String
    var size: CGFloat = FraySpacing.avatarSize

    var body: some View {
        Circle()
            .fill(FrayColors.colorForAgent(agentId))
            .frame(width: size, height: size)
            .overlay {
                Text(String(agentId.prefix(1)).uppercased())
                    .font(.system(size: size * 0.5, weight: .semibold))
                    .foregroundStyle(.white)
            }
            .accessibilityHidden(true)
    }
}

struct PresenceIndicator: View {
    let presence: FrayAgent.AgentPresence

    var body: some View {
        Circle()
            .fill(FrayColors.presence[presence] ?? .gray)
            .frame(width: 8, height: 8)
    }
}

struct ThreadListItem: View {
    let thread: FrayThread
    let allThreads: [FrayThread]
    let favedIds: Set<String>
    @Binding var selectedThread: FrayThread?
    let onFave: (String) -> Void

    @State private var isExpanded = false
    @State private var isHovering = false

    var childThreads: [FrayThread] {
        allThreads.filter { $0.parentThread == thread.guid && !favedIds.contains($0.guid) }
    }

    var hasChildren: Bool {
        !childThreads.isEmpty
    }

    var body: some View {
        if hasChildren {
            DisclosureGroup(isExpanded: $isExpanded) {
                ForEach(childThreads) { child in
                    ThreadListItem(
                        thread: child,
                        allThreads: allThreads,
                        favedIds: favedIds,
                        selectedThread: $selectedThread,
                        onFave: onFave
                    )
                }
            } label: {
                threadLabel
            }
            .tag(thread)
        } else {
            threadLabel
                .tag(thread)
        }
    }

    private var threadLabel: some View {
        HStack(spacing: FraySpacing.md) {
            Text(thread.name)
                .lineLimit(1)
                .foregroundStyle(thread.status == .archived ? .secondary : .primary)

            Spacer()

            if isHovering {
                Button(action: { onFave(thread.guid) }) {
                    Image(systemName: "star")
                        .font(.caption)
                }
                .buttonStyle(.borderless)
                .help("Add to favorites")
            }

            if thread.type == .knowledge {
                Image(systemName: "brain")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .contentShape(Rectangle())
        .onTapGesture {
            selectedThread = thread
        }
        .onHover { isHovering = $0 }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(threadAccessibilityLabel)
        .accessibilityAddTraits(.isButton)
    }

    private var threadAccessibilityLabel: String {
        var parts = ["Thread", thread.name]
        if thread.status == .archived {
            parts.append("archived")
        }
        if thread.type == .knowledge {
            parts.append("knowledge type")
        }
        if hasChildren {
            parts.append("\(childThreads.count) sub-threads")
        }
        return parts.joined(separator: ", ")
    }
}

#Preview {
    SidebarView(
        selectedThread: .constant(nil),
        currentChannel: .constant(nil),
        currentAgentId: "preview-user"
    )
    .environment(FrayBridge())
    .frame(width: 280)
}
