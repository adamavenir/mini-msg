import Foundation

// MARK: - JSON Decoder Configuration

extension JSONDecoder {
    static let fray: JSONDecoder = {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return decoder
    }()
}

// MARK: - FFI Response Wrapper

struct FFIResponse<T: Decodable>: Decodable {
    let ok: Bool
    let data: T?
    let error: String?
}

// MARK: - Message Types

/// Message type - matches internal/types/types.go:86
struct FrayMessage: Identifiable, Equatable {
    let id: String
    let ts: Int64
    let channelId: String?
    let home: String?
    let fromAgent: String
    let sessionId: String?
    let body: String
    let mentions: [String]
    let forkSessions: [String: String]?
    let reactions: [String: [ReactionEntry]]
    let type: MessageType
    let references: String?
    let surfaceMessage: String?
    let replyTo: String?
    let quoteMessageGuid: String?
    let editedAt: Int64?
    let edited: Bool?
    let editCount: Int?
    let archivedAt: Int64?

    var shortId: String { String(id.dropFirst(4).prefix(8)) }

    enum MessageType: String, Codable {
        case agent, user, event, surface, tombstone
    }
}

extension FrayMessage: Codable {
    enum CodingKeys: String, CodingKey {
        case id, ts, channelId, home, fromAgent, sessionId, body, mentions
        case forkSessions, reactions, type, references, surfaceMessage
        case replyTo, quoteMessageGuid, editedAt, edited, editCount, archivedAt
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        ts = try container.decode(Int64.self, forKey: .ts)
        channelId = try container.decodeIfPresent(String.self, forKey: .channelId)
        home = try container.decodeIfPresent(String.self, forKey: .home)
        fromAgent = try container.decode(String.self, forKey: .fromAgent)
        sessionId = try container.decodeIfPresent(String.self, forKey: .sessionId)
        body = try container.decode(String.self, forKey: .body)
        // Handle null mentions as empty array
        mentions = (try? container.decodeIfPresent([String].self, forKey: .mentions)) ?? []
        forkSessions = try container.decodeIfPresent([String: String].self, forKey: .forkSessions)
        // Handle null reactions as empty dictionary
        reactions = (try? container.decodeIfPresent([String: [ReactionEntry]].self, forKey: .reactions)) ?? [:]
        type = try container.decode(MessageType.self, forKey: .type)
        references = try container.decodeIfPresent(String.self, forKey: .references)
        surfaceMessage = try container.decodeIfPresent(String.self, forKey: .surfaceMessage)
        replyTo = try container.decodeIfPresent(String.self, forKey: .replyTo)
        quoteMessageGuid = try container.decodeIfPresent(String.self, forKey: .quoteMessageGuid)
        editedAt = try container.decodeIfPresent(Int64.self, forKey: .editedAt)
        edited = try container.decodeIfPresent(Bool.self, forKey: .edited)
        editCount = try container.decodeIfPresent(Int.self, forKey: .editCount)
        archivedAt = try container.decodeIfPresent(Int64.self, forKey: .archivedAt)
    }
}

/// Reaction entry - matches internal/types/types.go:73
struct ReactionEntry: Codable, Equatable {
    let agentId: String
    let reactedAt: Int64
}

// MARK: - Agent Types

/// Agent - matches internal/types/types.go:50
struct FrayAgent: Codable, Identifiable, Equatable {
    let guid: String
    let agentId: String
    let status: String?
    let purpose: String?
    let avatar: String?
    let registeredAt: Int64
    let lastSeen: Int64
    let leftAt: Int64?
    let managed: Bool?
    let invoke: InvokeConfig?
    let presence: AgentPresence?
    let mentionWatermark: String?
    let reactionWatermark: Int64?
    let lastHeartbeat: Int64?
    let lastSessionId: String?
    let sessionMode: String?
    let jobId: String?
    let jobIdx: Int?
    let isEphemeral: Bool?

    var id: String { guid }

    enum AgentPresence: String, Codable {
        case active, spawning, prompting, prompted, idle, error, offline, brb
    }
}

/// Invoke config - matches internal/types/types.go:37
struct InvokeConfig: Codable, Equatable {
    let driver: String?
    let model: String?
    let trust: [String]?
    let config: [String: JSONValue]?
    let promptDelivery: String?
    let spawnTimeoutMs: Int64?
    let idleAfterMs: Int64?
    let minCheckinMs: Int64?
    let maxRuntimeMs: Int64?
}

// MARK: - Usage Types

/// Agent session token usage - matches internal/usage.SessionUsage
struct AgentUsage: Codable, Equatable {
    let sessionId: String
    let driver: String?
    let model: String?
    let inputTokens: Int64
    let outputTokens: Int64
    let cachedTokens: Int64?
    let contextLimit: Int64
    let contextPercent: Int
}

// MARK: - Thread Types

/// Thread - matches internal/types/types.go:291
struct FrayThread: Codable, Identifiable, Equatable {
    let guid: String
    let name: String
    let parentThread: String?
    let status: ThreadStatus
    let type: ThreadType?
    let createdAt: Int64
    let createdBy: String?
    let ownerAgent: String?
    let anchorMessageGuid: String?
    let anchorHidden: Bool?
    let lastActivityAt: Int64?

    var id: String { guid }

    enum ThreadStatus: String, Codable, Hashable {
        case open, archived
    }

    enum ThreadType: String, Codable, Hashable {
        case standard, knowledge, system
    }
}

extension FrayThread: Hashable {
    func hash(into hasher: inout Hasher) {
        hasher.combine(guid)
    }

    static func == (lhs: FrayThread, rhs: FrayThread) -> Bool {
        lhs.guid == rhs.guid
    }
}

// MARK: - Pagination Types

/// Message cursor - matches internal/types/types.go:217
struct MessageCursor: Codable, Equatable {
    let guid: String
    let ts: Int64

    var encoded: String { "\(guid):\(ts)" }

    init(guid: String, ts: Int64) {
        self.guid = guid
        self.ts = ts
    }

    init?(from string: String) {
        let parts = string.split(separator: ":")
        guard parts.count == 2,
              let ts = Int64(parts[1]) else { return nil }
        self.guid = String(parts[0])
        self.ts = ts
    }
}

/// Paginated message response from FFI
struct MessagePage: Codable {
    let messages: [FrayMessage]
    let cursor: MessageCursor?
}

// MARK: - Channel Types

/// Channel info from global config
struct FrayChannel: Codable, Identifiable, Equatable, Hashable {
    let id: String
    let name: String
    let path: String
}

// MARK: - Fave Types

/// Fave entry - matches internal/db/queries_faves.go:Fave
struct FrayFave: Codable, Equatable {
    let agentId: String
    let itemType: String
    let itemGuid: String
    let favedAt: Int64
}

// MARK: - Interactive Event Types

/// Interactive action - matches internal/types/types.go:320
struct InteractiveAction: Codable, Equatable {
    let id: String
    let label: String
    let command: String
    let style: String?
    let confirm: Bool?
}

/// Interactive event - matches internal/types/types.go:328
struct InteractiveEvent: Codable, Equatable {
    let kind: String
    let targetGuid: String
    let title: String
    let body: String
    let actions: [InteractiveAction]
    let status: String?
    let resolvedBy: String?
}

/// Parse interactive event from message body.
/// Looks for <!--interactive:{...}--> marker.
func parseInteractiveEvent(from body: String) -> InteractiveEvent? {
    guard let startRange = body.range(of: "<!--interactive:") else { return nil }
    let afterStart = body[startRange.upperBound...]
    guard let endRange = afterStart.range(of: "-->") else { return nil }

    let jsonStr = String(afterStart[..<endRange.lowerBound])
    guard let jsonData = jsonStr.data(using: .utf8) else { return nil }

    return try? JSONDecoder.fray.decode(InteractiveEvent.self, from: jsonData)
}

// MARK: - JSON Value (for arbitrary nested structures)

/// Recursive JSON value wrapper for arbitrary config structures
enum JSONValue: Codable, Equatable {
    case null
    case bool(Bool)
    case int(Int)
    case double(Double)
    case string(String)
    case array([JSONValue])
    case object([String: JSONValue])

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()

        if container.decodeNil() {
            self = .null
        } else if let bool = try? container.decode(Bool.self) {
            self = .bool(bool)
        } else if let int = try? container.decode(Int.self) {
            self = .int(int)
        } else if let double = try? container.decode(Double.self) {
            self = .double(double)
        } else if let string = try? container.decode(String.self) {
            self = .string(string)
        } else if let array = try? container.decode([JSONValue].self) {
            self = .array(array)
        } else if let object = try? container.decode([String: JSONValue].self) {
            self = .object(object)
        } else {
            throw DecodingError.typeMismatch(
                JSONValue.self,
                DecodingError.Context(
                    codingPath: decoder.codingPath,
                    debugDescription: "Unable to decode JSON value"
                )
            )
        }
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .null: try container.encodeNil()
        case .bool(let v): try container.encode(v)
        case .int(let v): try container.encode(v)
        case .double(let v): try container.encode(v)
        case .string(let v): try container.encode(v)
        case .array(let v): try container.encode(v)
        case .object(let v): try container.encode(v)
        }
    }
}
