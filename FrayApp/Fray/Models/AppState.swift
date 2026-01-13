import SwiftUI
import Observation

@Observable
final class AppState {
    var currentAgentId: String?
    var currentChannel: String?

    var showTimestamps: Bool = true
    var compactMode: Bool = false
    var showActivityPanel: Bool = false

    var isConnected: Bool = false
    var connectionError: String?

    init() {
        loadPreferences()
    }

    func setAgent(_ agentId: String?) {
        currentAgentId = agentId
        if let agentId = agentId {
            UserDefaults.standard.set(agentId, forKey: "lastAgentId")
        }
    }

    func setChannel(_ channel: String?) {
        currentChannel = channel
    }

    private func loadPreferences() {
        showTimestamps = UserDefaults.standard.bool(forKey: "showTimestamps")
        compactMode = UserDefaults.standard.bool(forKey: "compactMode")
        currentAgentId = UserDefaults.standard.string(forKey: "lastAgentId")

        if !UserDefaults.standard.bool(forKey: "preferencesInitialized") {
            showTimestamps = true
            compactMode = false
            UserDefaults.standard.set(true, forKey: "preferencesInitialized")
        }
    }

    func savePreferences() {
        UserDefaults.standard.set(showTimestamps, forKey: "showTimestamps")
        UserDefaults.standard.set(compactMode, forKey: "compactMode")
    }
}
