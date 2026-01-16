import SwiftUI

struct CommandPalette: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(FrayBridge.self) private var bridge
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.colorScheme) private var colorScheme

    @State private var query: String = ""
    @State private var results: [CommandResult] = []
    @State private var selectedIndex: Int = 0
    @FocusState private var isSearchFocused: Bool

    let onSelect: (CommandResult) -> Void

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: FraySpacing.sm) {
                Image(systemName: "magnifyingglass")
                    .foregroundStyle(.secondary)

                TextField("Search threads, agents, commands...", text: $query)
                    .textFieldStyle(.plain)
                    .font(.title3)
                    .focused($isSearchFocused)
                    .onSubmit {
                        if let result = selectedResult {
                            onSelect(result)
                            dismiss()
                        }
                    }

                if !query.isEmpty {
                    Button(action: { query = "" }) {
                        Image(systemName: "xmark.circle.fill")
                            .foregroundStyle(.secondary)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, FraySpacing.md)
            .padding(.vertical, FraySpacing.sm)

            Divider()

            if results.isEmpty && !query.isEmpty {
                VStack(spacing: FraySpacing.sm) {
                    Image(systemName: "magnifyingglass")
                        .font(.title)
                        .foregroundStyle(.tertiary)
                    Text("No results for \"\(query)\"")
                        .foregroundStyle(.secondary)
                }
                .frame(maxHeight: .infinity)
                .padding()
            } else {
                ScrollViewReader { proxy in
                    List(selection: Binding(
                        get: { selectedResult?.id },
                        set: { newId in
                            if let idx = results.firstIndex(where: { $0.id == newId }) {
                                selectedIndex = idx
                            }
                        }
                    )) {
                        ForEach(groupedResults, id: \.key) { group, items in
                            Section(group) {
                                ForEach(items) { result in
                                    CommandResultRow(result: result, isSelected: result.id == selectedResult?.id)
                                        .id(result.id)
                                        .tag(result.id)
                                        .onTapGesture {
                                            onSelect(result)
                                            dismiss()
                                        }
                                }
                            }
                        }
                    }
                    .listStyle(.plain)
                    .onChange(of: selectedIndex) { _, newIndex in
                        if let result = results[safe: newIndex] {
                            if reduceMotion {
                                proxy.scrollTo(result.id, anchor: .center)
                            } else {
                                withAnimation {
                                    proxy.scrollTo(result.id, anchor: .center)
                                }
                            }
                        }
                    }
                }
            }
        }
        .frame(width: FraySpacing.commandPaletteWidth, height: FraySpacing.commandPaletteHeight)
        .background {
            RoundedRectangle(cornerRadius: 16)
                .fill(colorScheme == .dark
                    ? AnyShapeStyle(FrayColors.commandPaletteBackground.dark)
                    : AnyShapeStyle(.ultraThinMaterial))
        }
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .shadow(
            color: colorScheme == .dark ? .clear : .black.opacity(0.2),
            radius: colorScheme == .dark ? 0 : 20,
            y: colorScheme == .dark ? 0 : 10
        )
        .accessibilityAddTraits(.isModal)
        .accessibilityLabel("Command Palette")
        .onAppear {
            isSearchFocused = true
            performSearch()
        }
        .onChange(of: query) { _, _ in
            performSearch()
            selectedIndex = 0
        }
        .onKeyPress(.upArrow) {
            selectedIndex = max(0, selectedIndex - 1)
            return .handled
        }
        .onKeyPress(.downArrow) {
            selectedIndex = min(results.count - 1, selectedIndex + 1)
            return .handled
        }
        .onKeyPress(.escape) {
            dismiss()
            return .handled
        }
    }

    private var selectedResult: CommandResult? {
        results[safe: selectedIndex]
    }

    private var groupedResults: [(key: String, value: [CommandResult])] {
        Dictionary(grouping: results, by: { $0.category })
            .sorted { $0.key < $1.key }
            .map { ($0.key, $0.value) }
    }

    private func performSearch() {
        var newResults: [CommandResult] = []

        if query.isEmpty {
            newResults.append(contentsOf: defaultCommands)
        } else {
            let lowercaseQuery = query.lowercased()

            do {
                let threads = try bridge.getThreads()
                newResults.append(contentsOf: threads.filter {
                    $0.name.lowercased().contains(lowercaseQuery)
                }.prefix(5).map {
                    CommandResult(
                        id: $0.guid,
                        title: $0.name,
                        subtitle: $0.type == .knowledge ? "Knowledge" : "Thread",
                        icon: $0.status == .archived ? "archivebox" : "bubble.left",
                        category: "Threads",
                        action: .openThread($0.guid)
                    )
                })

                let agents = try bridge.getAgents()
                newResults.append(contentsOf: agents.filter {
                    $0.agentId.lowercased().contains(lowercaseQuery)
                }.prefix(5).map {
                    CommandResult(
                        id: $0.guid,
                        title: "@\($0.agentId)",
                        subtitle: $0.status ?? ($0.managed == true ? "Managed" : "Agent"),
                        icon: "person.fill",
                        category: "Agents",
                        action: .viewAgent($0.agentId)
                    )
                })
            } catch {
                print("Search error: \(error)")
            }

            newResults.append(contentsOf: defaultCommands.filter {
                $0.title.lowercased().contains(lowercaseQuery)
            })
        }

        results = newResults
    }

    private var defaultCommands: [CommandResult] {
        [
            CommandResult(
                id: "cmd-new-message",
                title: "New Message",
                subtitle: "⌘N",
                icon: "square.and.pencil",
                category: "Commands",
                action: .focusInput
            ),
            CommandResult(
                id: "cmd-toggle-sidebar",
                title: "Toggle Sidebar",
                subtitle: "⌘0",
                icon: "sidebar.left",
                category: "Commands",
                action: .toggleSidebar
            ),
            CommandResult(
                id: "cmd-toggle-activity",
                title: "Toggle Activity Panel",
                subtitle: "⌘I",
                icon: "sidebar.right",
                category: "Commands",
                action: .toggleActivity
            ),
            CommandResult(
                id: "cmd-room",
                title: "Go to Room",
                subtitle: "Main conversation",
                icon: "bubble.left.and.bubble.right",
                category: "Navigation",
                action: .openRoom
            )
        ]
    }
}

struct CommandResult: Identifiable {
    let id: String
    let title: String
    let subtitle: String
    let icon: String
    let category: String
    let action: CommandAction
}

enum CommandAction {
    case openThread(String)
    case viewAgent(String)
    case focusInput
    case toggleSidebar
    case toggleActivity
    case openRoom
}

struct CommandResultRow: View {
    let result: CommandResult
    var isSelected: Bool = false

    var body: some View {
        HStack(spacing: FraySpacing.sm) {
            Image(systemName: result.icon)
                .frame(width: 24)
                .foregroundStyle(isSelected ? .white : .secondary)

            VStack(alignment: .leading, spacing: 2) {
                Text(result.title)
                    .font(FrayTypography.body)
                    .foregroundStyle(isSelected ? .white : .primary)

                Text(result.subtitle)
                    .font(FrayTypography.caption)
                    .foregroundStyle(isSelected ? .white.opacity(0.8) : .secondary)
            }

            Spacer()
        }
        .padding(.vertical, 4)
        .padding(.horizontal, FraySpacing.sm)
        .background(isSelected ? Color.accentColor : Color.clear, in: RoundedRectangle(cornerRadius: 6))
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(result.title), \(result.subtitle)")
        .accessibilityAddTraits(isSelected ? [.isButton, .isSelected] : .isButton)
    }
}

extension Array {
    subscript(safe index: Int) -> Element? {
        indices.contains(index) ? self[index] : nil
    }
}

#Preview {
    CommandPalette { result in
        print("Selected: \(result.title)")
    }
    .environment(FrayBridge())
    .padding(40)
    .background(.gray.opacity(0.5))
}
