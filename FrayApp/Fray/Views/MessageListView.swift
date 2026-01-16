import SwiftUI

struct MessageListView: View {
    let thread: FrayThread?
    let currentAgentId: String?
    var channelName: String?
    @Binding var inputFocused: Bool

    @Environment(FrayBridge.self) private var bridge

    @State private var messages: [FrayMessage] = []
    @State private var cursor: MessageCursor?
    @State private var inputText: String = ""
    @State private var replyTo: FrayMessage?
    @State private var isLoading = false
    @State private var pollTimer: Timer?
    @State private var hasInitialLoad = false

    // Track the thread/room we loaded for to detect stale content
    @State private var loadedForId: String?

    // Scroll position restoration
    @AppStorage("scrollPositions") private var scrollPositionsData: Data = Data()
    @State private var lastVisibleMessageId: String?

    // Limit initial load to prevent UI overwhelm
    private let initialLoadLimit = 50

    private var viewId: String {
        thread?.guid ?? "room"
    }

    private var scrollPositions: [String: String] {
        (try? JSONDecoder().decode([String: String].self, from: scrollPositionsData)) ?? [:]
    }

    private func saveScrollPosition(for id: String, messageId: String) {
        var positions = scrollPositions
        positions[id] = messageId
        if let encoded = try? JSONEncoder().encode(positions) {
            scrollPositionsData = encoded
        }
    }

    private func getSavedScrollPosition(for id: String) -> String? {
        scrollPositions[id]
    }

    private struct MessageGroup: Identifiable {
        let id: String  // first message id
        let messages: [FrayMessage]
        var isGrouped: Bool { messages.count > 1 }
    }

    private var messageDict: [String: FrayMessage] {
        Dictionary(uniqueKeysWithValues: messages.map { ($0.id, $0) })
    }

    @ViewBuilder
    private func messageRow(msg: FrayMessage, showHeader: Bool) -> some View {
        if msg.type == .event,
           let event = parseInteractiveEvent(from: msg.body) {
            PermissionRequestBubble(message: msg, event: event)
                .id(msg.id)
                .onAppear { lastVisibleMessageId = msg.id }
        } else {
            MessageBubble(
                message: msg,
                onReply: { replyTo = msg },
                showHeader: showHeader,
                parentMessage: msg.replyTo.flatMap { messageDict[$0] }
            )
            .id(msg.id)
            .onAppear { lastVisibleMessageId = msg.id }
        }
    }

    @ViewBuilder
    private var messageContent: some View {
        if loadedForId == viewId {
            ForEach(groupMessages(messages)) { group in
                VStack(spacing: group.isGrouped ? FraySpacing.groupedMessageSpacing : 0) {
                    ForEach(Array(group.messages.enumerated()), id: \.element.id) { idx, msg in
                        messageRow(msg: msg, showHeader: idx == 0)
                    }
                }
            }
        }
    }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: FraySpacing.messageSpacing) {
                    if isLoading && !hasInitialLoad {
                        ProgressView()
                            .frame(maxWidth: .infinity)
                            .padding()
                    }

                    messageContent
                }
                .padding(.horizontal)
                .padding(.top)
                .padding(.bottom, 8)
            }
            .defaultScrollAnchor(.bottom)
            .onChange(of: messages.count) { oldCount, newCount in
                // Only auto-scroll when new messages arrive (not on initial load)
                if oldCount > 0 && newCount > oldCount, let last = messages.last {
                    withAnimation(.easeOut(duration: 0.2)) {
                        proxy.scrollTo(last.id, anchor: .bottom)
                    }
                }
            }
            .onChange(of: loadedForId) { _, newId in
                // Restore scroll position or scroll to bottom after initial load
                guard let newId = newId, newId == viewId else { return }
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.15) {
                    if let savedId = getSavedScrollPosition(for: newId),
                       messages.contains(where: { $0.id == savedId }) {
                        // Restore to saved position
                        withAnimation(.easeOut(duration: 0.1)) {
                            proxy.scrollTo(savedId, anchor: .center)
                        }
                    } else if let last = messages.last {
                        // Default to bottom
                        withAnimation(.easeOut(duration: 0.1)) {
                            proxy.scrollTo(last.id, anchor: .bottom)
                        }
                    }
                }
            }
            .onChange(of: viewId) { oldId, newId in
                // Save scroll position before switching
                if oldId != newId {
                    if let lastVisible = lastVisibleMessageId {
                        saveScrollPosition(for: oldId, messageId: lastVisible)
                    }
                    // Clear messages immediately on thread change
                    messages = []
                    cursor = nil
                    hasInitialLoad = false
                    loadedForId = nil
                    lastVisibleMessageId = nil
                }
            }
        }
        .safeAreaInset(edge: .bottom) {
            MessageInputArea(
                text: $inputText,
                replyTo: $replyTo,
                onSubmit: handleSubmit,
                focused: $inputFocused,
                contextName: thread?.name ?? (channelName.map { "#\($0)" })
            )
            .padding(.horizontal, FraySpacing.md)
            .background(.regularMaterial)
        }
        .navigationTitle(navigationTitle)
        .task(id: viewId) {
            await loadMessages()
            startPolling()
        }
        .onDisappear {
            stopPolling()
        }
    }

    private var navigationTitle: String {
        if let thread = thread {
            return thread.name
        } else {
            return channelName ?? "Room"
        }
    }

    private func groupMessages(_ messages: [FrayMessage]) -> [MessageGroup] {
        var groups: [MessageGroup] = []
        var currentGroup: [FrayMessage] = []
        var lastAgent: String?
        var lastTimestamp: Int64?

        for message in messages {
            let shouldGroup = message.fromAgent == lastAgent &&
                             message.type == .agent &&
                             lastTimestamp != nil &&
                             (message.ts - lastTimestamp!) < FraySpacing.messageGroupTimeWindow

            if shouldGroup {
                currentGroup.append(message)
            } else {
                if !currentGroup.isEmpty {
                    groups.append(MessageGroup(id: currentGroup[0].id, messages: currentGroup))
                }
                currentGroup = [message]
            }

            lastAgent = message.fromAgent
            lastTimestamp = message.ts
        }

        if !currentGroup.isEmpty {
            groups.append(MessageGroup(id: currentGroup[0].id, messages: currentGroup))
        }

        return groups
    }

    private func startPolling() {
        stopPolling()
        pollTimer = Timer.scheduledTimer(withTimeInterval: 1.0, repeats: true) { _ in
            Task { @MainActor in
                await pollNewMessages()
            }
        }
    }

    private func stopPolling() {
        pollTimer?.invalidate()
        pollTimer = nil
    }

    private func pollNewMessages() async {
        guard !isLoading, loadedForId == viewId else { return }

        do {
            // For threads, query messages with home = threadGuid (not thread members)
            // For room (thread = nil), pass nil to get main room messages
            let homeParam = thread?.guid
            let result = try bridge.getMessages(home: homeParam, limit: 50, since: cursor)

            if !result.messages.isEmpty {
                let existingIds = Set(messages.map { $0.id })
                let newMessages = result.messages.filter { !existingIds.contains($0.id) }
                if !newMessages.isEmpty {
                    messages.append(contentsOf: newMessages)
                }
            }

            if let newCursor = result.cursor {
                cursor = newCursor
            }
        } catch {
            // Silent failure for polling
        }
    }

    private func loadMessages() async {
        isLoading = true
        defer { isLoading = false }

        let targetId = viewId

        do {
            // For threads, query messages with home = threadGuid (not thread members)
            // For room (thread = nil), pass nil to get main room messages
            let homeParam = thread?.guid
            let page = try bridge.getMessages(home: homeParam, limit: initialLoadLimit)

            // Only update if we're still viewing the same thread/room
            if viewId == targetId {
                messages = page.messages
                cursor = page.cursor
                loadedForId = targetId
                hasInitialLoad = true
            }
        } catch {
            print("Failed to load messages: \(error)")
        }
    }

    private func handleSubmit(_ body: String) {
        guard let agentId = currentAgentId else {
            print("No agent ID set, cannot post")
            return
        }

        Task {
            do {
                let message = try bridge.postMessage(
                    body: body,
                    from: agentId,
                    in: thread?.name,
                    replyTo: replyTo?.id
                )
                messages.append(message)
                inputText = ""
                replyTo = nil
            } catch {
                print("Failed to post message: \(error)")
            }
        }
    }
}

struct RoomView: View {
    let currentAgentId: String?
    var channelName: String?
    @Binding var inputFocused: Bool

    var body: some View {
        MessageListView(thread: nil, currentAgentId: currentAgentId, channelName: channelName, inputFocused: $inputFocused)
    }
}

#Preview {
    MessageListView(thread: nil, currentAgentId: "preview-user", channelName: "fray", inputFocused: .constant(false))
        .environment(FrayBridge())
}
