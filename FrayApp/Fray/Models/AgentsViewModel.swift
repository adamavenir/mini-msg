import Foundation
import Observation

@Observable
@MainActor
final class AgentsViewModel {
    private let bridge: FrayBridge
    private var refreshTimer: Timer?

    private(set) var agents: [FrayAgent] = []
    private(set) var agentUsage: [String: AgentUsage] = [:]
    private(set) var isLoading: Bool = false
    private(set) var error: String?

    private let staleThresholdSeconds: Int64 = 3600

    init(bridge: FrayBridge) {
        self.bridge = bridge
    }

    private func isRecentlyActive(_ agent: FrayAgent) -> Bool {
        guard agent.presence == .offline || agent.presence == nil else {
            return true
        }
        let now = Int64(Date().timeIntervalSince1970)
        let lastActivityTs = agent.lastHeartbeat ?? agent.lastSeen
        return (now - lastActivityTs) <= staleThresholdSeconds
    }

    var activeAgents: [FrayAgent] {
        agents.filter { agent in
            let presence = agent.presence ?? .offline
            switch presence {
            case .active, .spawning, .prompting, .prompted, .idle, .error, .brb:
                return true
            case .offline:
                return isRecentlyActive(agent)
            }
        }
        .sorted { a, b in
            let aActive = a.presence == .active || a.presence == .spawning
            let bActive = b.presence == .active || b.presence == .spawning
            if aActive != bActive { return aActive }

            let aTs = a.lastHeartbeat ?? a.lastSeen
            let bTs = b.lastHeartbeat ?? b.lastSeen
            return aTs > bTs
        }
    }

    var managedAgents: [FrayAgent] {
        agents.filter { $0.managed == true }
    }

    var idleAgents: [FrayAgent] {
        agents.filter { $0.presence == .idle }
    }

    var offlineAgents: [FrayAgent] {
        agents.filter { agent in
            (agent.presence == .offline || agent.presence == nil) && !isRecentlyActive(agent)
        }
    }

    var offlineCount: Int {
        offlineAgents.count
    }

    func loadAgents(managedOnly: Bool = false) async {
        isLoading = true
        defer { isLoading = false }

        do {
            agents = try bridge.getAgents(managedOnly: managedOnly)
            error = nil

            // Load usage for active agents
            await loadUsageForActiveAgents()
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func loadUsageForActiveAgents() async {
        for agent in activeAgents {
            guard let presence = agent.presence,
                  presence == .active || presence == .idle || presence == .prompting || presence == .prompted else {
                continue
            }

            do {
                let usage = try bridge.getAgentUsage(agentId: agent.agentId)
                if usage.inputTokens > 0 {
                    agentUsage[agent.agentId] = usage
                }
            } catch {
                // Usage fetch failures are non-fatal, just skip
            }
        }
    }

    func usage(for agentId: String) -> AgentUsage? {
        agentUsage[agentId]
    }

    func agent(byId agentId: String) -> FrayAgent? {
        agents.first { $0.agentId == agentId }
    }

    func agent(byGuid guid: String) -> FrayAgent? {
        agents.first { $0.guid == guid }
    }

    func startPresenceRefresh(interval: TimeInterval = 5.0) {
        stopPresenceRefresh()
        refreshTimer = Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { [weak self] _ in
            Task { @MainActor in
                await self?.loadAgents()
            }
        }
    }

    func stopPresenceRefresh() {
        refreshTimer?.invalidate()
        refreshTimer = nil
    }

    func agentsByPresence() -> [FrayAgent.AgentPresence: [FrayAgent]] {
        var result: [FrayAgent.AgentPresence: [FrayAgent]] = [:]
        for agent in agents {
            let presence = agent.presence ?? .offline
            result[presence, default: []].append(agent)
        }
        return result
    }
}
