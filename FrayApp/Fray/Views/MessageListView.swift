import SwiftUI

struct MessageListView: View {
    let thread: FrayThread?
    let currentAgentId: String?
    var channelName: String?

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

    // Limit initial load to prevent UI overwhelm
    private let initialLoadLimit = 50

    private var viewId: String {
        thread?.guid ?? "room"
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

                    // Only show messages if they match current view
                    if loadedForId == viewId {
                        ForEach(messages) { message in
                            MessageBubble(message: message, onReply: { replyTo = message })
                                .id(message.id)
                        }
                    }
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
                // Scroll to bottom after initial load with delay for layout
                if newId == viewId, let last = messages.last {
                    // Use longer delay to ensure LazyVStack has laid out content
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.15) {
                        withAnimation(.easeOut(duration: 0.1)) {
                            proxy.scrollTo(last.id, anchor: .bottom)
                        }
                    }
                }
            }
            .onChange(of: viewId) { oldId, newId in
                // Clear messages immediately on thread change to prevent showing stale content
                if oldId != newId {
                    messages = []
                    cursor = nil
                    hasInitialLoad = false
                    loadedForId = nil
                }
            }
        }
        .safeAreaInset(edge: .bottom) {
            MessageInputArea(
                text: $inputText,
                replyTo: $replyTo,
                onSubmit: handleSubmit
            )
            .padding()
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

    var body: some View {
        MessageListView(thread: nil, currentAgentId: currentAgentId, channelName: channelName)
    }
}

#Preview {
    MessageListView(thread: nil, currentAgentId: "preview-user", channelName: "fray")
        .environment(FrayBridge())
}
