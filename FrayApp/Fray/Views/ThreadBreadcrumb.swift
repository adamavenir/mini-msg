import SwiftUI

struct ThreadBreadcrumb: View {
    let thread: FrayThread?
    let allThreads: [FrayThread]
    var channelName: String?
    let onNavigate: (FrayThread?) -> Void

    var body: some View {
        HStack(spacing: 4) {
            Button(action: { onNavigate(nil) }) {
                HStack(spacing: 4) {
                    Image(systemName: "house")
                        .font(.caption)
                    Text(channelName ?? "Room")
                        .font(FrayTypography.caption)
                }
            }
            .buttonStyle(.plain)
            .foregroundStyle(thread == nil ? .primary : .secondary)

            ForEach(breadcrumbPath, id: \.guid) { ancestor in
                Image(systemName: "chevron.right")
                    .font(.caption2)
                    .foregroundStyle(.tertiary)

                Button(action: { onNavigate(ancestor) }) {
                    Text(ancestor.name)
                        .font(FrayTypography.caption)
                        .lineLimit(1)
                }
                .buttonStyle(.plain)
                .foregroundStyle(ancestor.guid == thread?.guid ? .primary : .secondary)
            }
        }
        .padding(.horizontal, FraySpacing.md)
        .padding(.vertical, FraySpacing.xs)
        .background(.bar)
    }

    private var breadcrumbPath: [FrayThread] {
        guard let current = thread else { return [] }

        var path: [FrayThread] = [current]
        var checking: FrayThread? = current

        while let parentGuid = checking?.parentThread,
              let parent = allThreads.first(where: { $0.guid == parentGuid }) {
            path.insert(parent, at: 0)
            checking = parent
        }

        return path
    }
}

struct ThreadNavigationView: View {
    @Environment(FrayBridge.self) private var bridge
    @Binding var selectedThread: FrayThread?
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    @State private var allThreads: [FrayThread] = []
    @State private var navigationStack: [FrayThread] = []

    var body: some View {
        VStack(spacing: 0) {
            if selectedThread != nil || !navigationStack.isEmpty {
                ThreadBreadcrumb(
                    thread: selectedThread,
                    allThreads: allThreads,
                    onNavigate: { navigateTo($0) }
                )
            }

            ScrollView {
                LazyVStack(alignment: .leading, spacing: 0) {
                    ForEach(visibleThreads) { thread in
                        ThreadNavigationRow(
                            thread: thread,
                            depth: depthOf(thread),
                            hasChildren: hasChildren(thread),
                            onSelect: { navigateTo(thread) },
                            onDrillIn: { drillInto(thread) }
                        )
                    }
                }
            }
        }
        .task {
            await loadThreads()
        }
        .onKeyPress("h") {
            drillOut()
            return .handled
        }
        .onKeyPress("l") {
            if let current = selectedThread, hasChildren(current) {
                drillInto(current)
            }
            return .handled
        }
    }

    private var visibleThreads: [FrayThread] {
        if let current = selectedThread {
            return allThreads.filter { $0.parentThread == current.guid }
        } else if let parent = navigationStack.last {
            return allThreads.filter { $0.parentThread == parent.guid }
        } else {
            return allThreads.filter { $0.parentThread == nil }
        }
    }

    private func depthOf(_ thread: FrayThread) -> Int {
        var depth = 0
        var checking: FrayThread? = thread
        while let parentGuid = checking?.parentThread,
              let parent = allThreads.first(where: { $0.guid == parentGuid }) {
            depth += 1
            checking = parent
        }
        return depth
    }

    private func hasChildren(_ thread: FrayThread) -> Bool {
        allThreads.contains { $0.parentThread == thread.guid }
    }

    private func navigateTo(_ thread: FrayThread?) {
        if reduceMotion {
            selectedThread = thread
            if thread == nil {
                navigationStack = []
            }
        } else {
            withAnimation(.spring()) {
                selectedThread = thread
                if thread == nil {
                    navigationStack = []
                }
            }
        }
    }

    private func drillInto(_ thread: FrayThread) {
        if reduceMotion {
            if let current = selectedThread {
                navigationStack.append(current)
            }
            selectedThread = thread
        } else {
            withAnimation(.spring()) {
                if let current = selectedThread {
                    navigationStack.append(current)
                }
                selectedThread = thread
            }
        }
    }

    private func drillOut() {
        if reduceMotion {
            if let parent = navigationStack.popLast() {
                selectedThread = parent
            } else {
                selectedThread = nil
            }
        } else {
            withAnimation(.spring()) {
                if let parent = navigationStack.popLast() {
                    selectedThread = parent
                } else {
                    selectedThread = nil
                }
            }
        }
    }

    private func loadThreads() async {
        do {
            allThreads = try bridge.getThreads(includeArchived: false)
        } catch {
            print("Failed to load threads: \(error)")
        }
    }
}

struct ThreadNavigationRow: View {
    let thread: FrayThread
    let depth: Int
    let hasChildren: Bool
    let onSelect: () -> Void
    let onDrillIn: () -> Void

    @State private var isHovered = false
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        HStack(spacing: FraySpacing.sm) {
            Image(systemName: threadIcon)
                .frame(width: 20)
                .foregroundStyle(thread.status == .archived ? .secondary : .primary)

            VStack(alignment: .leading, spacing: 2) {
                Text(thread.name)
                    .font(FrayTypography.body)
                    .lineLimit(1)

                if let lastActivity = thread.lastActivityAt {
                    Text(FrayFormatters.compactRelativeTimestamp(lastActivity))
                        .font(FrayTypography.timestamp)
                        .foregroundStyle(.tertiary)
                }
            }

            Spacer()

            if thread.type == .knowledge {
                Image(systemName: "brain")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            if hasChildren {
                Button(action: onDrillIn) {
                    Image(systemName: "chevron.right")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
                .opacity(isHovered ? 1 : 0.5)
            }
        }
        .padding(.horizontal, FraySpacing.md)
        .padding(.vertical, FraySpacing.sm)
        .background(isHovered ? FrayColors.hoverFill.resolve(for: colorScheme) : Color.clear)
        .contentShape(Rectangle())
        .onTapGesture { onSelect() }
        .onHover { isHovered = $0 }
    }

    private var threadIcon: String {
        switch thread.type {
        case .knowledge: return "brain"
        case .system: return "gearshape"
        case .standard, nil: return thread.status == .archived ? "archivebox" : "bubble.left"
        }
    }
}

#Preview {
    ThreadNavigationView(selectedThread: .constant(nil))
        .environment(FrayBridge())
        .frame(width: 300, height: 400)
}
