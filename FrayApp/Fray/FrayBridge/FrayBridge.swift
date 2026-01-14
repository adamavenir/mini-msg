import Foundation
import Observation

/// Swift bridge to the Fray Go FFI library
@Observable
final class FrayBridge {
    private var handle: UInt64 = 0
    private(set) var projectPath: String?
    private(set) var isConnected: Bool = false

    /// Path to the fray binary (discovered on access)
    var frayPath: String {
        // Try common locations
        let candidates = [
            "/opt/homebrew/bin/fray",
            "/usr/local/bin/fray",
            NSHomeDirectory() + "/go/bin/fray"
        ]
        for path in candidates {
            if FileManager.default.isExecutableFile(atPath: path) {
                return path
            }
        }
        return "fray" // Fall back to PATH
    }

    /// Project root directory (for running commands)
    var projectRoot: String {
        projectPath ?? FileManager.default.currentDirectoryPath
    }

    // MARK: - Connection

    /// Discover a Fray project by walking up from the given directory
    static func discoverProject(from startDir: String) -> String? {
        startDir.withCString { cPath in
            guard let result = FrayDiscoverProject(UnsafeMutablePointer(mutating: cPath)) else {
                return nil
            }
            defer { FrayFreeString(result) }
            let json = String(cString: result)

            // Parse the JSON response to extract the root path
            guard let data = json.data(using: .utf8),
                  let response = try? JSONDecoder.fray.decode(FFIResponse<ProjectResponse>.self, from: data),
                  response.ok,
                  let project = response.data else {
                return nil
            }
            return project.root
        }
    }

    private struct ProjectResponse: Decodable {
        let root: String
        let dbPath: String
    }

    /// Connect to a Fray project database
    func connect(projectPath: String) throws {
        guard handle == 0 else { return }

        let newHandle = projectPath.withCString { cPath in
            FrayOpenDatabase(UnsafeMutablePointer(mutating: cPath))
        }

        guard newHandle != 0 else {
            throw FrayError.connectionFailed
        }

        handle = newHandle
        self.projectPath = projectPath
        isConnected = true
    }

    /// Disconnect from the current project
    func disconnect() {
        guard handle != 0 else { return }
        FrayCloseDatabase(handle)
        handle = 0
        projectPath = nil
        isConnected = false
    }

    deinit {
        disconnect()
    }

    // MARK: - Messages

    /// Fetch messages from room or thread
    /// - Parameters:
    ///   - home: Thread/room identifier. NULL → main room, empty string → all homes
    ///   - limit: Maximum messages to return
    ///   - since: Cursor for pagination
    func getMessages(home: String? = nil, limit: Int = 100, since: MessageCursor? = nil) throws -> MessagePage {
        try ensureConnected()

        let result = withOptionalCString(home) { cHome in
            withOptionalCString(since?.encoded) { cCursor in
                callFFI { FrayGetMessages(handle, cHome, Int32(limit), cCursor) }
            }
        }
        return try decode(MessagePage.self, from: result)
    }

    /// Fetch messages from a specific thread (playlist members)
    func getThreadMessages(threadGuid: String, limit: Int = 100, since: MessageCursor? = nil) throws -> MessagePage {
        try ensureConnected()

        let result = threadGuid.withCString { cThreadGuid in
            withOptionalCString(since?.encoded) { cCursor in
                callFFI {
                    FrayGetThreadMessages(
                        handle,
                        UnsafeMutablePointer(mutating: cThreadGuid),
                        Int32(limit),
                        cCursor
                    )
                }
            }
        }
        return try decode(MessagePage.self, from: result)
    }

    /// Post a new message
    func postMessage(
        body: String,
        from agent: String,
        in home: String? = nil,
        replyTo: String? = nil
    ) throws -> FrayMessage {
        try ensureConnected()

        let result = body.withCString { cBody in
            agent.withCString { cAgent in
                withOptionalCString(home) { cHome in
                    withOptionalCString(replyTo) { cReplyTo in
                        callFFI {
                            FrayPostMessage(
                                handle,
                                UnsafeMutablePointer(mutating: cBody),
                                UnsafeMutablePointer(mutating: cAgent),
                                cHome,
                                cReplyTo
                            )
                        }
                    }
                }
            }
        }
        return try decode(FrayMessage.self, from: result)
    }

    /// Edit an existing message
    func editMessage(msgId: String, newBody: String, reason: String? = nil) throws -> FrayMessage {
        try ensureConnected()

        let result = msgId.withCString { cMsgId in
            newBody.withCString { cNewBody in
                withOptionalCString(reason) { cReason in
                    callFFI {
                        FrayEditMessage(
                            handle,
                            UnsafeMutablePointer(mutating: cMsgId),
                            UnsafeMutablePointer(mutating: cNewBody),
                            cReason
                        )
                    }
                }
            }
        }
        return try decode(FrayMessage.self, from: result)
    }

    /// Add a reaction to a message
    func addReaction(msgId: String, emoji: String, agent: String) throws {
        try ensureConnected()

        let result = msgId.withCString { cMsgId in
            emoji.withCString { cEmoji in
                agent.withCString { cAgent in
                    callFFI {
                        FrayAddReaction(
                            handle,
                            UnsafeMutablePointer(mutating: cMsgId),
                            UnsafeMutablePointer(mutating: cEmoji),
                            UnsafeMutablePointer(mutating: cAgent)
                        )
                    }
                }
            }
        }

        // Just check for success, no data returned
        let response = try JSONDecoder.fray.decode(
            FFIResponse<EmptyData>.self,
            from: result.data(using: .utf8)!
        )
        if !response.ok {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
    }

    // MARK: - Agents

    /// Get all agents
    func getAgents(managedOnly: Bool = false) throws -> [FrayAgent] {
        try ensureConnected()

        let result = callFFI {
            FrayGetAgents(handle, managedOnly ? 1 : 0)
        }
        return try decode([FrayAgent].self, from: result)
    }

    /// Get a specific agent by ID
    func getAgent(agentId: String) throws -> FrayAgent? {
        try ensureConnected()

        let result = agentId.withCString { cAgentId in
            callFFI {
                FrayGetAgent(handle, UnsafeMutablePointer(mutating: cAgentId))
            }
        }

        // Agent might not exist - check response before decoding
        guard let data = result.data(using: .utf8) else {
            throw FrayError.decodingFailed("Invalid UTF-8 response")
        }

        let response = try JSONDecoder.fray.decode(FFIResponse<FrayAgent?>.self, from: data)
        if !response.ok {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
        return response.data ?? nil
    }

    /// Get token usage for an agent's current session
    func getAgentUsage(agentId: String) throws -> AgentUsage {
        try ensureConnected()

        let result = agentId.withCString { cAgentId in
            callFFI {
                FrayGetAgentUsage(handle, UnsafeMutablePointer(mutating: cAgentId))
            }
        }
        return try decode(AgentUsage.self, from: result)
    }

    // MARK: - Threads

    /// Get threads with optional parent filter
    func getThreads(parent: String? = nil, includeArchived: Bool = false) throws -> [FrayThread] {
        try ensureConnected()

        let result = withOptionalCString(parent) { cParent in
            callFFI {
                FrayGetThreads(handle, cParent, includeArchived ? 1 : 0)
            }
        }
        return try decode([FrayThread].self, from: result)
    }

    /// Get a specific thread by GUID or name
    func getThread(ref: String) throws -> FrayThread? {
        try ensureConnected()

        let result = ref.withCString { cRef in
            callFFI {
                FrayGetThread(handle, UnsafeMutablePointer(mutating: cRef))
            }
        }

        guard let data = result.data(using: .utf8) else {
            throw FrayError.decodingFailed("Invalid UTF-8 response")
        }

        let response = try JSONDecoder.fray.decode(FFIResponse<FrayThread?>.self, from: data)
        if !response.ok {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
        return response.data ?? nil
    }

    /// Subscribe to a thread
    func subscribeToThread(threadGuid: String, agentId: String) throws {
        try ensureConnected()

        let result = threadGuid.withCString { cThreadGuid in
            agentId.withCString { cAgentId in
                callFFI {
                    FraySubscribeToThread(
                        handle,
                        UnsafeMutablePointer(mutating: cThreadGuid),
                        UnsafeMutablePointer(mutating: cAgentId)
                    )
                }
            }
        }

        let response = try JSONDecoder.fray.decode(
            FFIResponse<EmptyData>.self,
            from: result.data(using: .utf8)!
        )
        if !response.ok {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
    }

    // MARK: - Faves

    /// Fave an item (message or thread)
    func faveItem(itemGuid: String, agentId: String) throws {
        try ensureConnected()

        let result = itemGuid.withCString { cItemGuid in
            agentId.withCString { cAgentId in
                callFFI {
                    FrayFaveItem(
                        handle,
                        UnsafeMutablePointer(mutating: cItemGuid),
                        UnsafeMutablePointer(mutating: cAgentId)
                    )
                }
            }
        }

        let response = try JSONDecoder.fray.decode(
            FFIResponse<EmptyData>.self,
            from: result.data(using: .utf8)!
        )
        if !response.ok {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
    }

    // MARK: - Read State

    /// Get read position for an agent in a home
    func getReadTo(agentId: String, home: String?) throws -> String? {
        try ensureConnected()

        let result = agentId.withCString { cAgentId in
            withOptionalCString(home) { cHome in
                callFFI {
                    FrayGetReadTo(
                        handle,
                        UnsafeMutablePointer(mutating: cAgentId),
                        cHome
                    )
                }
            }
        }

        guard let data = result.data(using: .utf8) else {
            throw FrayError.decodingFailed("Invalid UTF-8 response")
        }

        let response = try JSONDecoder.fray.decode(FFIResponse<String?>.self, from: data)
        if !response.ok {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
        return response.data ?? nil
    }

    /// Set read position for an agent in a home
    func setReadTo(agentId: String, home: String?, msgId: String) throws {
        try ensureConnected()

        let result = agentId.withCString { cAgentId in
            msgId.withCString { cMsgId in
                withOptionalCString(home) { cHome in
                    callFFI {
                        FraySetReadTo(
                            handle,
                            UnsafeMutablePointer(mutating: cAgentId),
                            cHome,
                            UnsafeMutablePointer(mutating: cMsgId)
                        )
                    }
                }
            }
        }

        let response = try JSONDecoder.fray.decode(
            FFIResponse<EmptyData>.self,
            from: result.data(using: .utf8)!
        )
        if !response.ok {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
    }

    // MARK: - Agent Registration

    /// Register a new agent or return existing one
    func registerAgent(agentId: String) throws -> FrayAgent {
        try ensureConnected()

        let result = agentId.withCString { cAgentId in
            callFFI {
                FrayRegisterAgent(handle, UnsafeMutablePointer(mutating: cAgentId))
            }
        }
        return try decode(FrayAgent.self, from: result)
    }

    // MARK: - Channels

    /// List all registered channels (reads from global config, no handle needed)
    static func listChannels() throws -> [FrayChannel] {
        let result = callFFI { FrayListChannels() }
        guard let data = result.data(using: .utf8) else {
            throw FrayError.decodingFailed("Invalid UTF-8 response")
        }
        let response = try JSONDecoder.fray.decode(FFIResponse<[FrayChannel]>.self, from: data)
        guard response.ok, let channels = response.data else {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
        return channels
    }

    // MARK: - Faves

    /// Get faves for an agent
    func getFaves(agentId: String, itemType: String? = nil) throws -> [FrayFave] {
        try ensureConnected()

        let result = agentId.withCString { cAgentId in
            withOptionalCString(itemType) { cItemType in
                callFFI {
                    FrayGetFaves(
                        handle,
                        UnsafeMutablePointer(mutating: cAgentId),
                        cItemType
                    )
                }
            }
        }
        return try decode([FrayFave].self, from: result)
    }

    /// Unfave an item (message or thread)
    func unfaveItem(itemGuid: String, agentId: String) throws {
        try ensureConnected()

        let result = itemGuid.withCString { cItemGuid in
            agentId.withCString { cAgentId in
                callFFI {
                    FrayUnfaveItem(
                        handle,
                        UnsafeMutablePointer(mutating: cItemGuid),
                        UnsafeMutablePointer(mutating: cAgentId)
                    )
                }
            }
        }

        let response = try JSONDecoder.fray.decode(
            FFIResponse<EmptyData>.self,
            from: result.data(using: .utf8)!
        )
        if !response.ok {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
    }

    // MARK: - Config

    /// Get a config value
    func getConfig(key: String) throws -> String? {
        try ensureConnected()

        let result = key.withCString { cKey in
            callFFI {
                FrayGetConfig(handle, UnsafeMutablePointer(mutating: cKey))
            }
        }

        guard let data = result.data(using: .utf8) else {
            throw FrayError.decodingFailed("Invalid UTF-8 response")
        }

        let response = try JSONDecoder.fray.decode(FFIResponse<String?>.self, from: data)
        if !response.ok {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
        return response.data ?? nil
    }

    /// Set a config value
    func setConfig(key: String, value: String) throws {
        try ensureConnected()

        let result = key.withCString { cKey in
            value.withCString { cValue in
                callFFI {
                    FraySetConfig(
                        handle,
                        UnsafeMutablePointer(mutating: cKey),
                        UnsafeMutablePointer(mutating: cValue)
                    )
                }
            }
        }

        let response = try JSONDecoder.fray.decode(
            FFIResponse<EmptyData>.self,
            from: result.data(using: .utf8)!
        )
        if !response.ok {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
    }

    // MARK: - Private Helpers

    private func ensureConnected() throws {
        guard isConnected else {
            throw FrayError.notConnected
        }
    }

    /// Safely call FFI with optional string, passing NULL if nil
    private func withOptionalCString<R>(
        _ string: String?,
        _ body: (UnsafeMutablePointer<CChar>?) -> R
    ) -> R {
        if let s = string {
            return s.withCString { body(UnsafeMutablePointer(mutating: $0)) }
        } else {
            return body(nil)
        }
    }

    /// Call FFI function and convert result to String
    private func callFFI(_ call: () -> UnsafeMutablePointer<CChar>?) -> String {
        Self.callFFI(call)
    }

    /// Static version of callFFI for use with handle-less FFI functions
    private static func callFFI(_ call: () -> UnsafeMutablePointer<CChar>?) -> String {
        guard let ptr = call() else {
            return #"{"ok":false,"error":"null response"}"#
        }
        defer { FrayFreeString(ptr) }
        return String(cString: ptr)
    }

    /// Decode FFI response JSON to expected type
    private func decode<T: Decodable>(_ type: T.Type, from json: String) throws -> T {
        guard let data = json.data(using: .utf8) else {
            throw FrayError.decodingFailed("Invalid UTF-8 response")
        }

        let response = try JSONDecoder.fray.decode(FFIResponse<T>.self, from: data)
        guard response.ok, let result = response.data else {
            throw FrayError.ffi(response.error ?? "unknown error")
        }
        return result
    }
}

/// Empty data placeholder for FFI calls that return no data
private struct EmptyData: Decodable {}
