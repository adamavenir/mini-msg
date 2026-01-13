import SwiftUI

@main
struct FrayApp: App {
    @State private var bridge = FrayBridge()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(bridge)
        }
        .windowStyle(.automatic)
        .windowToolbarStyle(.unified(showsTitle: true))
    }
}
