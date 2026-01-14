import SwiftUI

struct ContentView: View {
    @Environment(FrayBridge.self) private var bridge
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.colorScheme) private var colorScheme
    @State private var selectedThread: FrayThread?
    @State private var currentChannel: FrayChannel?
    @State private var columnVisibility: NavigationSplitViewVisibility = .all
    @State private var showActivityPanel = false
    @State private var showCommandPalette = false
    @State private var showIdentityPrompt = false
    @State private var currentAgentId: String?
    @State private var agentsVM: AgentsViewModel?
    @State private var allThreads: [FrayThread] = []
    @FocusState private var isInputFocused: Bool

    @SceneStorage("sidebarWidth") private var sidebarWidth: Double = 280
    @SceneStorage("selectedThreadId") private var selectedThreadId: String = ""
    @SceneStorage("activityPanelVisible") private var activityPanelVisible: Bool = false
    @SceneStorage("currentChannelId") private var currentChannelId: String = ""

    var body: some View {
        ZStack {
            NavigationSplitView(columnVisibility: $columnVisibility) {
                SidebarView(
                    selectedThread: $selectedThread,
                    currentChannel: $currentChannel,
                    currentAgentId: currentAgentId
                )
                .navigationSplitViewColumnWidth(min: 200, ideal: FraySpacing.sidebarWidth)
            } content: {
                VStack(spacing: 0) {
                    if selectedThread != nil {
                        ThreadBreadcrumb(
                            thread: selectedThread,
                            allThreads: allThreads,
                            channelName: currentChannel?.name,
                            onNavigate: { selectedThread = $0 }
                        )
                    }

                    if let thread = selectedThread {
                        MessageListView(
                            thread: thread,
                            currentAgentId: currentAgentId,
                            channelName: currentChannel?.name
                        )
                        .id(thread.guid)
                    } else {
                        RoomView(
                            currentAgentId: currentAgentId,
                            channelName: currentChannel?.name
                        )
                        .id("room-\(currentChannel?.id ?? "default")")
                    }
                }
            } detail: {
                if showActivityPanel, let vm = agentsVM {
                    ActivityPanelView()
                        .environment(vm)
                        .navigationSplitViewColumnWidth(
                            min: 150,
                            ideal: FraySpacing.activityPanelWidth
                        )
                } else {
                    EmptyView()
                        .navigationSplitViewColumnWidth(0)
                }
            }
            .task {
                await connectToProject()
                await checkIdentity()
                agentsVM = AgentsViewModel(bridge: bridge)
                await loadThreads()
                restoreState()
            }
            .onChange(of: selectedThread?.guid) { _, newValue in
                selectedThreadId = newValue ?? ""
            }
            .onChange(of: showActivityPanel) { _, newValue in
                activityPanelVisible = newValue
            }
            .onChange(of: currentChannel?.id) { _, newId in
                currentChannelId = newId ?? ""
                // Reconnect to new channel
                if let channel = currentChannel {
                    Task {
                        await reconnectToChannel(channel)
                    }
                }
            }
            .toolbar {
                ToolbarItem(placement: .navigation) {
                    Button(action: {
                        withAnimation {
                            columnVisibility = columnVisibility == .all ? .detailOnly : .all
                        }
                    }) {
                        Image(systemName: "sidebar.left")
                    }
                    .help("Toggle Sidebar (⌘0)")
                    .keyboardShortcut("0", modifiers: [.command])
                }

                ToolbarItem(placement: .navigation) {
                    Button(action: { showCommandPalette = true }) {
                        Image(systemName: "magnifyingglass")
                    }
                    .help("Command Palette (⌘K)")
                    .keyboardShortcut("k", modifiers: [.command])
                }

                ToolbarItem(placement: .primaryAction) {
                    Button(action: { showActivityPanel.toggle() }) {
                        Image(systemName: showActivityPanel ? "sidebar.trailing.fill" : "sidebar.trailing")
                    }
                    .help("Toggle Activity Panel (⌘I)")
                    .keyboardShortcut("i", modifiers: [.command])
                }
            }

            if showCommandPalette {
                FrayColors.modalOverlay.resolve(for: colorScheme)
                    .ignoresSafeArea()
                    .onTapGesture { showCommandPalette = false }

                CommandPalette { result in
                    handleCommandResult(result)
                }
            }
        }
        .sheet(isPresented: $showIdentityPrompt) {
            IdentityPromptView(isPresented: $showIdentityPrompt) { agentId in
                currentAgentId = agentId
            }
        }
    }

    private func handleCommandResult(_ result: CommandResult) {
        showCommandPalette = false

        switch result.action {
        case .openThread(let guid):
            if let thread = allThreads.first(where: { $0.guid == guid }) {
                selectedThread = thread
            }
        case .viewAgent:
            break
        case .focusInput:
            isInputFocused = true
        case .toggleSidebar:
            withAnimation {
                columnVisibility = columnVisibility == .all ? .detailOnly : .all
            }
        case .toggleActivity:
            showActivityPanel.toggle()
        case .openRoom:
            selectedThread = nil
        }
    }

    private func loadThreads() async {
        do {
            allThreads = try bridge.getThreads()
        } catch {
            print("Failed to load threads: \(error)")
        }
    }

    private func connectToProject() async {
        // Try multiple discovery paths since app launch directory varies
        let searchPaths = [
            FileManager.default.currentDirectoryPath,
            ProcessInfo.processInfo.environment["FRAY_PROJECT_PATH"],
            FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("dev/fray").path,
            "/Users/adam/dev/fray"
        ].compactMap { $0 }

        for startPath in searchPaths {
            if let projectPath = FrayBridge.discoverProject(from: startPath) {
                do {
                    try bridge.connect(projectPath: projectPath)
                    print("Connected to fray project at: \(projectPath)")
                    return
                } catch {
                    print("Failed to connect to \(projectPath): \(error)")
                }
            }
        }

        print("No fray project found. Searched: \(searchPaths)")
    }

    private func reconnectToChannel(_ channel: FrayChannel) async {
        bridge.disconnect()
        do {
            try bridge.connect(projectPath: channel.path)
            print("Switched to channel: \(channel.name) at \(channel.path)")
            selectedThread = nil
            await loadThreads()
        } catch {
            print("Failed to connect to channel \(channel.name): \(error)")
        }
    }

    private func restoreState() {
        showActivityPanel = activityPanelVisible

        // Restore channel
        if !currentChannelId.isEmpty {
            do {
                let channels = try FrayBridge.listChannels()
                if let channel = channels.first(where: { $0.id == currentChannelId }) {
                    currentChannel = channel
                }
            } catch {
                print("Failed to restore channel: \(error)")
            }
        }

        // Restore thread
        if !selectedThreadId.isEmpty,
           let thread = allThreads.first(where: { $0.guid == selectedThreadId }) {
            selectedThread = thread
        }
    }

    private func checkIdentity() async {
        guard bridge.isConnected else { return }

        do {
            if let username = try bridge.getConfig(key: "username"), !username.isEmpty {
                currentAgentId = username
            } else {
                showIdentityPrompt = true
            }
        } catch {
            showIdentityPrompt = true
        }
    }
}

#Preview {
    ContentView()
        .environment(FrayBridge())
        .frame(width: 1000, height: 600)
}
