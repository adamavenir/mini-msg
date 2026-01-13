import SwiftUI

struct ActivityPanelView: View {
    @Environment(AgentsViewModel.self) private var agentsVM
    @State private var showOffline = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text("Activity")
                .font(FrayTypography.headline)
                .padding(.horizontal, FraySpacing.md)
                .padding(.vertical, FraySpacing.sm)

            ScrollView {
                LazyVStack(alignment: .leading, spacing: FraySpacing.xs) {
                    ForEach(agentsVM.activeAgents) { agent in
                        AgentActivityRow(agent: agent, usage: agentsVM.usage(for: agent.agentId))
                            .padding(.horizontal, FraySpacing.sm)
                    }

                    if agentsVM.offlineCount > 0 {
                        Button(action: { showOffline.toggle() }) {
                            HStack {
                                Image(systemName: showOffline ? "chevron.down" : "chevron.right")
                                    .font(.caption2)
                                Text("\(agentsVM.offlineCount) offline")
                                    .font(FrayTypography.caption)
                                Spacer()
                            }
                            .foregroundStyle(.secondary)
                        }
                        .buttonStyle(.plain)
                        .padding(.horizontal, FraySpacing.sm)
                        .padding(.top, FraySpacing.sm)

                        if showOffline {
                            ForEach(agentsVM.offlineAgents) { agent in
                                AgentActivityRow(agent: agent)
                                    .padding(.horizontal, FraySpacing.sm)
                                    .opacity(0.6)
                            }
                        }
                    }

                    if agentsVM.activeAgents.isEmpty && agentsVM.offlineCount == 0 {
                        Text("No agents")
                            .font(FrayTypography.caption)
                            .foregroundStyle(.secondary)
                            .padding(FraySpacing.md)
                    }
                }
                .padding(.vertical, FraySpacing.sm)
            }
        }
        .task {
            await agentsVM.loadAgents()
            agentsVM.startPresenceRefresh(interval: 3.0)
        }
        .onDisappear {
            agentsVM.stopPresenceRefresh()
        }
    }
}

#Preview {
    ActivityPanelView()
        .environment(AgentsViewModel(bridge: FrayBridge()))
        .frame(width: 250, height: 400)
}
