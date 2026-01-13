import Foundation
import Observation

@Observable
@MainActor
final class MessagesViewModel {
    private let bridge: FrayBridge
    private var currentThread: FrayThread?
    private var pollTimer: Timer?
    private var cursor: MessageCursor?

    private(set) var messages: [FrayMessage] = []
    private(set) var isLoading: Bool = false
    private(set) var error: String?

    init(bridge: FrayBridge) {
        self.bridge = bridge
    }

    func loadMessages(thread: FrayThread?, limit: Int = 100) async {
        isLoading = true
        defer { isLoading = false }

        do {
            currentThread = thread
            cursor = nil

            if let thread = thread {
                let result = try bridge.getThreadMessages(threadGuid: thread.guid, limit: limit)
                messages = result.messages
                cursor = result.cursor
            } else {
                let result = try bridge.getMessages(home: nil, limit: limit, since: nil)
                messages = result.messages
                cursor = result.cursor
            }
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    func startPolling(thread: FrayThread?, interval: TimeInterval = 1.0) {
        stopPolling()
        currentThread = thread

        pollTimer = Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { [weak self] _ in
            Task { @MainActor in
                await self?.pollNewMessages()
            }
        }
    }

    func stopPolling() {
        pollTimer?.invalidate()
        pollTimer = nil
    }

    private func pollNewMessages() async {
        guard !isLoading else { return }

        do {
            let result: MessagePage
            if let thread = currentThread {
                result = try bridge.getThreadMessages(threadGuid: thread.guid, limit: 50, since: cursor)
            } else {
                result = try bridge.getMessages(home: nil, limit: 50, since: cursor)
            }

            if !result.messages.isEmpty {
                let existingIds = Set(messages.map { $0.id })
                let newMessages = result.messages.filter { !existingIds.contains($0.id) }
                messages.append(contentsOf: newMessages)
            }

            if let newCursor = result.cursor {
                self.cursor = newCursor
            }
        } catch {
            // Silent failure for polling
        }
    }

    func postMessage(body: String, from agent: String, replyTo: String? = nil) async throws {
        let home = currentThread?.name
        let message = try bridge.postMessage(
            body: body,
            from: agent,
            in: home,
            replyTo: replyTo
        )
        messages.append(message)
        cursor = MessageCursor(guid: message.id, ts: message.ts)
    }

    func editMessage(messageId: String, newBody: String, reason: String? = nil) async throws {
        let updated = try bridge.editMessage(msgId: messageId, newBody: newBody, reason: reason)
        if let index = messages.firstIndex(where: { $0.id == messageId }) {
            messages[index] = updated
        }
    }

    func addReaction(to messageId: String, emoji: String, from agent: String) async throws {
        try bridge.addReaction(msgId: messageId, emoji: emoji, agent: agent)
        await refreshMessage(messageId)
    }

    private func refreshMessage(_ messageId: String) async {
        // Re-fetch messages to get updated state
        // In a real implementation, we'd have a single-message fetch
        await loadMessages(thread: currentThread)
    }
}
