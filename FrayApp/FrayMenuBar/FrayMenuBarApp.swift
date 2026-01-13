import SwiftUI

@main
struct FrayMenuBarApp: App {
    @State private var bridge = FrayBridge()
    @State private var unreadCount: Int = 0
    @State private var activeAgents: [FrayAgent] = []

    var body: some Scene {
        MenuBarExtra {
            MenuBarContentView(
                bridge: bridge,
                unreadCount: $unreadCount,
                activeAgents: $activeAgents
            )
        } label: {
            HStack(spacing: 4) {
                Image(systemName: "bubble.left.and.bubble.right.fill")
                if unreadCount > 0 {
                    Text("\(unreadCount)")
                        .font(.caption2.monospacedDigit())
                }
            }
        }
        .menuBarExtraStyle(.window)
    }
}
