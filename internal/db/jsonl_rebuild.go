package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/adamavenir/fray/internal/types"
)

// normalizeTimestamp converts millisecond timestamps to seconds.
// Timestamps > 10 trillion are assumed to be in milliseconds and are divided by 1000.
func normalizeTimestamp(ts int64) int64 {
	if ts > 10000000000 {
		return ts / 1000
	}
	return ts
}

// TxStarter is implemented by *sql.DB to start transactions.
type TxStarter interface {
	Begin() (*sql.Tx, error)
}

// RebuildDatabaseFromJSONL resets the SQLite cache using JSONL sources.
// If db supports transactions (*sql.DB), the entire rebuild is wrapped in a
// transaction to prevent other queries from seeing partial state during rebuild.
func RebuildDatabaseFromJSONL(db DBTX, projectPath string) error {
	// If db supports transactions, wrap the entire rebuild in a transaction.
	// This prevents other queries (e.g., daemon watermark checks) from seeing
	// partial state between DROP TABLE and INSERT operations.
	if txStarter, ok := db.(TxStarter); ok {
		tx, err := txStarter.Begin()
		if err != nil {
			return fmt.Errorf("begin rebuild transaction: %w", err)
		}
		if err := rebuildDatabaseFromJSONLWith(tx, projectPath); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	// Already in a transaction (db is *sql.Tx), just run directly
	return rebuildDatabaseFromJSONLWith(db, projectPath)
}

// rebuildDatabaseFromJSONLWith does the actual rebuild work.
func rebuildDatabaseFromJSONLWith(db DBTX, projectPath string) error {
	if IsMultiMachineMode(projectPath) {
		if err := validateChecksums(projectPath); err != nil {
			return err
		}
		if err := updateCollisionLog(projectPath); err != nil {
			log.Printf("warning: collision log update failed: %v", err)
		}
	}

	messages, err := ReadMessages(projectPath)
	if err != nil {
		return err
	}
	pinEvents, err := ReadMessagePins(projectPath)
	if err != nil {
		return err
	}
	threadPinEvents, err := ReadThreadPins(projectPath)
	if err != nil {
		return err
	}
	threadMuteEvents, err := ReadThreadMutes(projectPath)
	if err != nil {
		return err
	}
	questions, err := ReadQuestions(projectPath)
	if err != nil {
		return err
	}
	threads, subEvents, msgEvents, err := ReadThreads(projectPath)
	if err != nil {
		return err
	}
	agents, err := ReadAgents(projectPath)
	if err != nil {
		return err
	}
	var descriptors []AgentDescriptor
	if IsMultiMachineMode(projectPath) {
		descriptors, err = ReadAgentDescriptors(projectPath)
		if err != nil {
			return err
		}
		agents, err = mergeAgentsFromMessages(agents, messages)
		if err != nil {
			return err
		}
		agents, err = mergeAgentsFromDescriptors(agents, descriptors)
		if err != nil {
			return err
		}
	}
	ghostCursors, err := ReadGhostCursors(projectPath)
	if err != nil {
		return err
	}
	reactions, err := ReadReactions(projectPath)
	if err != nil {
		return err
	}
	faveEvents, err := ReadFaves(projectPath)
	if err != nil {
		return err
	}
	roleEvents, err := ReadRoles(projectPath)
	if err != nil {
		return err
	}
	config, err := ReadProjectConfig(projectPath)
	if err != nil {
		return err
	}

	if _, err := db.Exec("DROP TABLE IF EXISTS fray_reactions"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_messages"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_agent_sessions"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_agents"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_agent_descriptors"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_read_receipts"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_questions"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_thread_messages"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_thread_subscriptions"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_threads"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_thread_pins"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_thread_mutes"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_ghost_cursors"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_faves"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_role_assignments"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_session_roles"); err != nil {
		return err
	}
	if err := initSchemaWith(db); err != nil {
		return fmt.Errorf("initSchemaWith: %w", err)
	}

	if config != nil && config.ChannelID != "" {
		if _, err := db.Exec("INSERT OR REPLACE INTO fray_config (key, value) VALUES (?, ?)", "channel_id", config.ChannelID); err != nil {
			return err
		}
		if config.ChannelName != "" {
			if _, err := db.Exec("INSERT OR REPLACE INTO fray_config (key, value) VALUES (?, ?)", "channel_name", config.ChannelName); err != nil {
				return err
			}
		}
	}

	if len(descriptors) > 0 {
		insertDescriptor := `
			INSERT OR REPLACE INTO fray_agent_descriptors (
				agent_id, display_name, capabilities, updated_at
			) VALUES (?, ?, ?, ?)
		`
		for _, descriptor := range descriptors {
			if descriptor.AgentID == "" {
				continue
			}
			var capabilitiesJSON *string
			if len(descriptor.Capabilities) > 0 {
				data, err := json.Marshal(descriptor.Capabilities)
				if err != nil {
					return err
				}
				s := string(data)
				capabilitiesJSON = &s
			}
			if _, err := db.Exec(insertDescriptor,
				descriptor.AgentID,
				descriptor.DisplayName,
				capabilitiesJSON,
				descriptor.TS,
			); err != nil {
				return err
			}
		}
	}

	insertAgent := `
		INSERT OR REPLACE INTO fray_agents (
			guid, agent_id, status, purpose, avatar, registered_at, last_seen, left_at, managed, invoke, presence, presence_changed_at, mention_watermark, last_heartbeat, session_mode, last_session_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	for _, agent := range agents {
		status := agent.Status
		if status == nil {
			status = agent.Goal
		}
		purpose := agent.Purpose
		if purpose == nil {
			purpose = agent.Bio
		}

		var invokeJSON *string
		if agent.Invoke != nil {
			data, err := json.Marshal(agent.Invoke)
			if err != nil {
				return err
			}
			s := string(data)
			invokeJSON = &s
		}

		managed := 0
		if agent.Managed {
			managed = 1
		}

		presence := agent.Presence
		if presence == "" {
			presence = "offline"
		}

		// presence_changed_at will be NULL on rebuild, populated on next presence change
		if _, err := db.Exec(insertAgent,
			agent.ID,
			agent.AgentID,
			status,
			purpose,
			agent.Avatar,
			agent.RegisteredAt,
			agent.LastSeen,
			agent.LeftAt,
			managed,
			invokeJSON,
			presence,
			nil, // presence_changed_at - will be set on next presence update
			agent.MentionWatermark,
			agent.LastHeartbeat,
			agent.SessionMode,
			agent.LastSessionID,
		); err != nil {
			return err
		}
	}

	insertMessage := `
		INSERT OR REPLACE INTO fray_messages (
			guid, ts, channel_id, home, from_agent, origin, session_id, body, mentions, fork_sessions, type, "references", surface_message, reply_to, quote_message_guid, edited_at, archived_at, reactions
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	for _, message := range messages {
		mentionsJSON, err := json.Marshal(message.Mentions)
		if err != nil {
			return err
		}
		var forkSessionsJSON *string
		if len(message.ForkSessions) > 0 {
			data, err := json.Marshal(message.ForkSessions)
			if err != nil {
				return err
			}
			s := string(data)
			forkSessionsJSON = &s
		}
		reactionsJSON, err := json.Marshal(normalizeReactionsLegacy(message.Reactions))
		if err != nil {
			return err
		}
		msgType := message.MsgType
		if msgType == "" {
			msgType = types.MessageTypeAgent
		}

		home := message.Home
		if home == "" {
			home = "room"
		}

		if _, err := db.Exec(insertMessage,
			message.ID,
			normalizeTimestamp(message.TS),
			message.ChannelID,
			home,
			message.FromAgent,
			message.Origin,
			message.SessionID,
			message.Body,
			string(mentionsJSON),
			forkSessionsJSON,
			msgType,
			message.References,
			message.SurfaceMessage,
			message.ReplyTo,
			message.QuoteMessageGUID,
			message.EditedAt,
			message.ArchivedAt,
			string(reactionsJSON),
		); err != nil {
			return err
		}
	}

	if len(questions) > 0 {
		insertQuestion := `
			INSERT OR REPLACE INTO fray_questions (
				guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, options, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		for _, question := range questions {
			status := question.Status
			if status == "" {
				status = string(types.QuestionStatusUnasked)
			}
			optionsJSON := "[]"
			if len(question.Options) > 0 {
				optBytes, err := json.Marshal(question.Options)
				if err != nil {
					return err
				}
				optionsJSON = string(optBytes)
			}
			if _, err := db.Exec(insertQuestion,
				question.GUID,
				question.Re,
				question.FromAgent,
				question.ToAgent,
				status,
				question.ThreadGUID,
				question.AskedIn,
				question.AnsweredIn,
				optionsJSON,
				question.CreatedAt,
			); err != nil {
				return err
			}
		}
	}

	if len(threads) > 0 {
		// Topologically sort threads so parents are inserted before children
		// (required for FK constraint on parent_thread)
		threads = topoSortThreads(threads)

		insertThread := `
			INSERT OR REPLACE INTO fray_threads (
				guid, name, parent_thread, status, type, created_at, anchor_message_guid, anchor_hidden, last_activity_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		for i, thread := range threads {
			status := thread.Status
			if status == "" {
				status = string(types.ThreadStatusOpen)
			}
			threadType := thread.ThreadType
			if threadType == "" {
				threadType = string(types.ThreadTypeStandard)
			}
			anchorHidden := 0
			if thread.AnchorHidden {
				anchorHidden = 1
			}
			if _, err := db.Exec(insertThread,
				thread.GUID,
				thread.Name,
				thread.ParentThread,
				status,
				threadType,
				thread.CreatedAt,
				thread.AnchorMessageGUID,
				anchorHidden,
				thread.LastActivityAt,
			); err != nil {
				parent := ""
				if thread.ParentThread != nil {
					parent = *thread.ParentThread
				}
				return fmt.Errorf("thread[%d] %s name=%q parent=%q: %w", i, thread.GUID, thread.Name, parent, err)
			}
		}

		subscriptions := make(map[string]map[string]int64)
		for _, thread := range threads {
			if len(thread.Subscribed) == 0 {
				continue
			}
			set := make(map[string]int64, len(thread.Subscribed))
			for _, agentID := range thread.Subscribed {
				if agentID == "" {
					continue
				}
				set[agentID] = thread.CreatedAt
			}
			if len(set) > 0 {
				subscriptions[thread.GUID] = set
			}
		}

		// Build set of valid thread GUIDs for FK validation
		threadGUIDs := make(map[string]bool, len(threads))
		for _, t := range threads {
			threadGUIDs[t.GUID] = true
		}

		for _, event := range subEvents {
			// Skip events for non-existent threads (archived/deleted)
			if !threadGUIDs[event.ThreadGUID] {
				continue
			}
			set, ok := subscriptions[event.ThreadGUID]
			if !ok {
				set = make(map[string]int64)
				subscriptions[event.ThreadGUID] = set
			}
			switch event.Type {
			case "thread_subscribe":
				set[event.AgentID] = event.At
			case "thread_unsubscribe":
				delete(set, event.AgentID)
			}
		}

		for threadGUID, set := range subscriptions {
			for agentID, subscribedAt := range set {
				if _, err := db.Exec(`
					INSERT OR REPLACE INTO fray_thread_subscriptions (thread_guid, agent_id, subscribed_at)
					VALUES (?, ?, ?)
				`, threadGUID, agentID, subscribedAt); err != nil {
					return fmt.Errorf("subscription thread=%s agent=%s: %w", threadGUID, agentID, err)
				}
			}
		}

		threadMessages := make(map[string]map[string]ThreadMessageJSONLRecord)
		for _, event := range msgEvents {
			// Skip events for non-existent threads
			if !threadGUIDs[event.ThreadGUID] {
				continue
			}
			switch event.Type {
			case "thread_message":
				if _, ok := threadMessages[event.ThreadGUID]; !ok {
					threadMessages[event.ThreadGUID] = make(map[string]ThreadMessageJSONLRecord)
				}
				threadMessages[event.ThreadGUID][event.MessageGUID] = ThreadMessageJSONLRecord{
					ThreadGUID:  event.ThreadGUID,
					MessageGUID: event.MessageGUID,
					AddedBy:     event.AddedBy,
					AddedAt:     event.AddedAt,
				}
			case "thread_message_remove":
				if set, ok := threadMessages[event.ThreadGUID]; ok {
					delete(set, event.MessageGUID)
				}
			}
		}

		for _, messages := range threadMessages {
			for _, entry := range messages {
				if _, err := db.Exec(`
					INSERT OR REPLACE INTO fray_thread_messages (thread_guid, message_guid, added_by, added_at)
					VALUES (?, ?, ?, ?)
				`, entry.ThreadGUID, entry.MessageGUID, entry.AddedBy, entry.AddedAt); err != nil {
					return fmt.Errorf("thread_messages thread=%s msg=%s: %w", entry.ThreadGUID, entry.MessageGUID, err)
				}
			}
		}
	}

	// Rebuild message pins
	if len(pinEvents) > 0 {
		// Track current pin state per (message, thread) pair
		type pinKey struct {
			messageGUID string
			threadGUID  string
		}
		pins := make(map[pinKey]MessagePinEvent)

		for _, event := range pinEvents {
			key := pinKey{messageGUID: event.MessageGUID, threadGUID: event.ThreadGUID}
			switch event.Type {
			case "message_pin":
				pins[key] = event
			case "message_unpin":
				delete(pins, key)
			}
		}

		for _, pin := range pins {
			if _, err := db.Exec(`
				INSERT OR REPLACE INTO fray_message_pins (message_guid, thread_guid, pinned_by, pinned_at)
				VALUES (?, ?, ?, ?)
			`, pin.MessageGUID, pin.ThreadGUID, pin.PinnedBy, pin.PinnedAt); err != nil {
				return err
			}
		}
	}

	// Rebuild thread pins
	if len(threadPinEvents) > 0 {
		threadPins := make(map[string]threadPinEvent)

		for _, event := range threadPinEvents {
			switch event.Type {
			case "thread_pin":
				threadPins[event.ThreadGUID] = event
			case "thread_unpin":
				delete(threadPins, event.ThreadGUID)
			}
		}

		for _, pin := range threadPins {
			if _, err := db.Exec(`
				INSERT OR REPLACE INTO fray_thread_pins (thread_guid, pinned_by, pinned_at)
				VALUES (?, ?, ?)
			`, pin.ThreadGUID, pin.PinnedBy, pin.PinnedAt); err != nil {
				return err
			}
		}
	}

	// Rebuild thread mutes
	if len(threadMuteEvents) > 0 {
		type muteKey struct {
			threadGUID string
			agentID    string
		}
		mutes := make(map[muteKey]threadMuteEvent)

		for _, event := range threadMuteEvents {
			key := muteKey{threadGUID: event.ThreadGUID, agentID: event.AgentID}
			switch event.Type {
			case "thread_mute":
				mutes[key] = event
			case "thread_unmute":
				delete(mutes, key)
			}
		}

		for _, mute := range mutes {
			if _, err := db.Exec(`
				INSERT OR REPLACE INTO fray_thread_mutes (thread_guid, agent_id, muted_at, expires_at)
				VALUES (?, ?, ?, ?)
			`, mute.ThreadGUID, mute.AgentID, mute.MutedAt, mute.ExpiresAt); err != nil {
				return err
			}
		}
	}

	// Rebuild ghost cursors
	if len(ghostCursors) > 0 {
		for _, cursor := range ghostCursors {
			mustRead := 0
			if cursor.MustRead {
				mustRead = 1
			}
			if _, err := db.Exec(`
				INSERT OR REPLACE INTO fray_ghost_cursors (agent_id, home, message_guid, must_read, set_at)
				VALUES (?, ?, ?, ?, ?)
			`, cursor.AgentID, cursor.Home, cursor.MessageGUID, mustRead, cursor.SetAt); err != nil {
				return err
			}
		}
	}

	// Rebuild reactions from reaction records
	if len(reactions) > 0 {
		for _, r := range reactions {
			if _, err := db.Exec(`
				INSERT INTO fray_reactions (message_guid, agent_id, emoji, reacted_at)
				VALUES (?, ?, ?, ?)
			`, r.MessageGUID, r.AgentID, r.Emoji, r.ReactedAt); err != nil {
				return err
			}
		}
	}

	// Rebuild faves from fave events
	if len(faveEvents) > 0 {
		type faveKey struct {
			agentID  string
			itemType string
			itemGUID string
		}
		faves := make(map[faveKey]FaveEvent)

		for _, event := range faveEvents {
			key := faveKey{agentID: event.AgentID, itemType: event.ItemType, itemGUID: event.ItemGUID}
			switch event.Type {
			case "agent_fave":
				faves[key] = event
			case "agent_unfave", "fave_remove":
				delete(faves, key)
			}
		}

		for _, fave := range faves {
			if _, err := db.Exec(`
				INSERT OR REPLACE INTO fray_faves (agent_id, item_type, item_guid, faved_at)
				VALUES (?, ?, ?, ?)
			`, fave.AgentID, fave.ItemType, fave.ItemGUID, fave.FavedAt); err != nil {
				return err
			}
		}
	}

	// Rebuild roles from role events
	if len(roleEvents) > 0 {
		// Track held roles (persistent assignments)
		type heldKey struct {
			agentID  string
			roleName string
		}
		heldRoles := make(map[heldKey]int64) // assignedAt

		// Track session roles
		type sessionKey struct {
			agentID  string
			roleName string
		}
		sessionRoles := make(map[sessionKey]roleEvent)

		for _, event := range roleEvents {
			switch event.Type {
			case "role_hold":
				key := heldKey{agentID: event.AgentID, roleName: event.RoleName}
				heldRoles[key] = event.AssignedAt
			case "role_drop", "role_release":
				key := heldKey{agentID: event.AgentID, roleName: event.RoleName}
				delete(heldRoles, key)
			case "role_play":
				key := sessionKey{agentID: event.AgentID, roleName: event.RoleName}
				sessionRoles[key] = event
			case "role_stop":
				key := sessionKey{agentID: event.AgentID, roleName: event.RoleName}
				delete(sessionRoles, key)
			}
		}

		// Insert held roles
		for key, assignedAt := range heldRoles {
			if _, err := db.Exec(`
				INSERT OR REPLACE INTO fray_role_assignments (agent_id, role_name, assigned_at)
				VALUES (?, ?, ?)
			`, key.agentID, key.roleName, assignedAt); err != nil {
				return err
			}
		}

		// Insert session roles
		for _, role := range sessionRoles {
			if _, err := db.Exec(`
				INSERT OR REPLACE INTO fray_session_roles (agent_id, role_name, session_id, started_at)
				VALUES (?, ?, ?, ?)
			`, role.AgentID, role.RoleName, role.SessionID, role.StartedAt); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetLatestUsageSnapshot reads the most recent usage snapshot for a session from JSONL.
// Returns nil if no snapshot exists for the session.
