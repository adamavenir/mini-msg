import SwiftUI

struct WindowState: Codable {
    var sidebarWidth: CGFloat
    var activityPanelWidth: CGFloat
    var selectedThreadGuid: String?
    var showActivityPanel: Bool
    var columnVisibility: ColumnVisibilityState

    enum ColumnVisibilityState: String, Codable {
        case all, detailOnly, doubleColumn, automatic
    }

    static let `default` = WindowState(
        sidebarWidth: FraySpacing.sidebarWidth,
        activityPanelWidth: FraySpacing.activityPanelWidth,
        selectedThreadGuid: nil,
        showActivityPanel: false,
        columnVisibility: .all
    )
}

@Observable
final class WindowStateService {
    private static let storageKey = "com.fray.windowState"

    var state: WindowState {
        didSet { save() }
    }

    init() {
        if let data = UserDefaults.standard.data(forKey: Self.storageKey),
           let decoded = try? JSONDecoder().decode(WindowState.self, from: data) {
            self.state = decoded
        } else {
            self.state = .default
        }
    }

    private func save() {
        if let data = try? JSONEncoder().encode(state) {
            UserDefaults.standard.set(data, forKey: Self.storageKey)
        }
    }

    func reset() {
        state = .default
    }
}

struct WindowStateModifier: ViewModifier {
    @Environment(WindowStateService.self) private var windowState

    func body(content: Content) -> some View {
        content
            .onAppear {
                restoreWindowFrame()
            }
    }

    private func restoreWindowFrame() {
        guard let screen = NSScreen.main else { return }

        if let frameString = UserDefaults.standard.string(forKey: "com.fray.windowFrame"),
           let frame = NSRectFromString(frameString) as NSRect? {
            if screen.visibleFrame.contains(frame.origin) {
                NSApplication.shared.windows.first?.setFrame(frame, display: true)
            }
        }
    }
}

extension View {
    func restoreWindowState() -> some View {
        modifier(WindowStateModifier())
    }
}

struct PersistentNavigationSplitView<Sidebar: View, Content: View, Detail: View>: View {
    @Environment(WindowStateService.self) private var windowState
    @Binding var columnVisibility: NavigationSplitViewVisibility

    let sidebar: () -> Sidebar
    let content: () -> Content
    let detail: () -> Detail

    var body: some View {
        NavigationSplitView(columnVisibility: $columnVisibility) {
            sidebar()
                .navigationSplitViewColumnWidth(
                    min: 180,
                    ideal: windowState.state.sidebarWidth,
                    max: 400
                )
        } content: {
            content()
        } detail: {
            detail()
                .navigationSplitViewColumnWidth(
                    min: 150,
                    ideal: windowState.state.activityPanelWidth,
                    max: 300
                )
        }
        .onChange(of: columnVisibility) { _, newValue in
            windowState.state.columnVisibility = mapVisibility(newValue)
        }
        .onAppear {
            columnVisibility = reverseMapVisibility(windowState.state.columnVisibility)
        }
    }

    private func mapVisibility(_ visibility: NavigationSplitViewVisibility) -> WindowState.ColumnVisibilityState {
        if visibility == .all { return .all }
        if visibility == .detailOnly { return .detailOnly }
        if visibility == .doubleColumn { return .doubleColumn }
        return .automatic
    }

    private func reverseMapVisibility(_ state: WindowState.ColumnVisibilityState) -> NavigationSplitViewVisibility {
        switch state {
        case .all: return .all
        case .detailOnly: return .detailOnly
        case .doubleColumn: return .doubleColumn
        case .automatic: return .automatic
        }
    }
}

class WindowDelegate: NSObject, NSWindowDelegate {
    func windowWillClose(_ notification: Notification) {
        guard let window = notification.object as? NSWindow else { return }
        let frameString = NSStringFromRect(window.frame)
        UserDefaults.standard.set(frameString, forKey: "com.fray.windowFrame")
    }

    func windowDidResize(_ notification: Notification) {
        guard let window = notification.object as? NSWindow else { return }
        let frameString = NSStringFromRect(window.frame)
        UserDefaults.standard.set(frameString, forKey: "com.fray.windowFrame")
    }

    func windowDidMove(_ notification: Notification) {
        guard let window = notification.object as? NSWindow else { return }
        let frameString = NSStringFromRect(window.frame)
        UserDefaults.standard.set(frameString, forKey: "com.fray.windowFrame")
    }
}
