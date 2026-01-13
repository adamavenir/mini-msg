import Foundation
import UserNotifications

@Observable
final class NotificationService {
    private(set) var isAuthorized = false
    private(set) var authorizationStatus: UNAuthorizationStatus = .notDetermined

    static let shared = NotificationService()

    private init() {
        Task {
            await checkAuthorizationStatus()
        }
    }

    func requestPermission() async -> Bool {
        do {
            let granted = try await UNUserNotificationCenter.current()
                .requestAuthorization(options: [.alert, .sound, .badge])
            await checkAuthorizationStatus()
            return granted
        } catch {
            print("Notification permission error: \(error)")
            return false
        }
    }

    func checkAuthorizationStatus() async {
        let settings = await UNUserNotificationCenter.current().notificationSettings()
        authorizationStatus = settings.authorizationStatus
        isAuthorized = settings.authorizationStatus == .authorized
    }

    func showMentionNotification(message: FrayMessage, currentAgentId: String) {
        guard isAuthorized else { return }
        guard message.mentions.contains(currentAgentId) || message.mentions.contains("all") else {
            return
        }

        let content = UNMutableNotificationContent()
        content.title = "@\(message.fromAgent)"
        content.body = String(message.body.prefix(200))
        content.sound = .default
        content.categoryIdentifier = "MENTION"
        content.userInfo = [
            "messageId": message.id,
            "fromAgent": message.fromAgent,
            "home": message.home ?? ""
        ]

        let request = UNNotificationRequest(
            identifier: message.id,
            content: content,
            trigger: nil
        )

        UNUserNotificationCenter.current().add(request) { error in
            if let error = error {
                print("Failed to show notification: \(error)")
            }
        }
    }

    func showPresenceNotification(agent: FrayAgent, newPresence: FrayAgent.AgentPresence) {
        guard isAuthorized else { return }

        let content = UNMutableNotificationContent()
        content.title = "@\(agent.agentId)"

        switch newPresence {
        case .active:
            content.body = "is now active"
        case .spawning:
            content.body = "is starting up..."
        case .idle:
            content.body = "went idle"
        case .error:
            content.body = "encountered an error"
            content.sound = .defaultCritical
        case .offline:
            content.body = "went offline"
        case .brb:
            content.body = "will be right back"
        case .prompting, .prompted:
            return
        }

        content.categoryIdentifier = "PRESENCE"
        content.userInfo = [
            "agentId": agent.agentId,
            "presence": newPresence.rawValue
        ]

        let request = UNNotificationRequest(
            identifier: "presence-\(agent.agentId)-\(Date().timeIntervalSince1970)",
            content: content,
            trigger: nil
        )

        UNUserNotificationCenter.current().add(request)
    }

    func clearNotifications(for messageId: String? = nil) {
        if let id = messageId {
            UNUserNotificationCenter.current().removeDeliveredNotifications(withIdentifiers: [id])
        } else {
            UNUserNotificationCenter.current().removeAllDeliveredNotifications()
        }
    }

    func setBadgeCount(_ count: Int) {
        UNUserNotificationCenter.current().setBadgeCount(count)
    }

    func setupCategories() {
        let replyAction = UNNotificationAction(
            identifier: "REPLY",
            title: "Reply",
            options: [.foreground]
        )

        let markReadAction = UNNotificationAction(
            identifier: "MARK_READ",
            title: "Mark Read",
            options: []
        )

        let mentionCategory = UNNotificationCategory(
            identifier: "MENTION",
            actions: [replyAction, markReadAction],
            intentIdentifiers: [],
            options: .customDismissAction
        )

        let presenceCategory = UNNotificationCategory(
            identifier: "PRESENCE",
            actions: [],
            intentIdentifiers: [],
            options: []
        )

        UNUserNotificationCenter.current().setNotificationCategories([
            mentionCategory,
            presenceCategory
        ])
    }
}

class NotificationDelegate: NSObject, UNUserNotificationCenterDelegate {
    var onReply: ((String, String) -> Void)?
    var onMarkRead: ((String) -> Void)?
    var onOpen: ((String) -> Void)?

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .sound, .badge])
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let userInfo = response.notification.request.content.userInfo
        let messageId = userInfo["messageId"] as? String ?? ""

        switch response.actionIdentifier {
        case "REPLY":
            onReply?(messageId, "")
        case "MARK_READ":
            onMarkRead?(messageId)
        case UNNotificationDefaultActionIdentifier:
            onOpen?(messageId)
        default:
            break
        }

        completionHandler()
    }
}
