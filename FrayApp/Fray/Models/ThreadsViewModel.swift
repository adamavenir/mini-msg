import Foundation
import Observation

@Observable
@MainActor
final class ThreadsViewModel {
    private let bridge: FrayBridge

    private(set) var threads: [FrayThread] = []
    private(set) var threadTree: [String: [FrayThread]] = [:]
    private(set) var rootThreads: [FrayThread] = []
    private(set) var isLoading: Bool = false
    private(set) var error: String?

    private var faves: Set<String> = []
    private var subscriptions: Set<String> = []

    init(bridge: FrayBridge) {
        self.bridge = bridge
    }

    var favedThreads: [FrayThread] {
        threads.filter { faves.contains($0.guid) }
    }

    var followingThreads: [FrayThread] {
        threads.filter { subscriptions.contains($0.guid) }
    }

    func loadThreads(includeArchived: Bool = false) async {
        isLoading = true
        defer { isLoading = false }

        do {
            threads = try bridge.getThreads(parent: nil, includeArchived: includeArchived)
            buildTree()
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func buildTree() {
        threadTree = Dictionary(grouping: threads) { $0.parentThread ?? "" }
        rootThreads = threadTree[""] ?? []
    }

    func children(of thread: FrayThread) -> [FrayThread] {
        threadTree[thread.guid] ?? []
    }

    func thread(byGuid guid: String) -> FrayThread? {
        threads.first { $0.guid == guid }
    }

    func thread(byName name: String) -> FrayThread? {
        threads.first { $0.name == name }
    }

    func subscribeToThread(_ thread: FrayThread, as agentId: String) async throws {
        try bridge.subscribeToThread(threadGuid: thread.guid, agentId: agentId)
        subscriptions.insert(thread.guid)
    }

    func faveThread(_ thread: FrayThread, as agentId: String) async throws {
        try bridge.faveItem(itemGuid: thread.guid, agentId: agentId)
        faves.insert(thread.guid)
    }

    func unfaveThread(_ thread: FrayThread) {
        faves.remove(thread.guid)
    }

    func unsubscribeFromThread(_ thread: FrayThread) {
        subscriptions.remove(thread.guid)
    }
}
