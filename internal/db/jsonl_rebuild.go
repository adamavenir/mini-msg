package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

func resolveFrayDir(projectPath string) string {
	if strings.HasSuffix(projectPath, ".db") {
		return filepath.Dir(projectPath)
	}
	if filepath.Base(projectPath) == ".fray" {
		return projectPath
	}
	return filepath.Join(projectPath, ".fray")
}

func ensureDir(dirPath string) error {
	return os.MkdirAll(dirPath, 0o755)
}

func touchDatabaseFile(projectPath string) {
	frayDir := resolveFrayDir(projectPath)
	if strings.HasSuffix(projectPath, ".db") {
		frayDir = filepath.Dir(projectPath)
	}
	path := filepath.Join(frayDir, "fray.db")
	_, err := os.Stat(path)
	if err != nil {
		return
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

// UpdateProjectConfig merges updates into the project config.
func UpdateProjectConfig(projectPath string, updates ProjectConfig) (*ProjectConfig, error) {
	frayDir := resolveFrayDir(projectPath)
	if err := ensureDir(frayDir); err != nil {
		return nil, err
	}

	existing, err := ReadProjectConfig(projectPath)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		existing = &ProjectConfig{
			Version:     1,
			KnownAgents: map[string]ProjectKnownAgent{},
		}
	}

	if existing.KnownAgents == nil {
		existing.KnownAgents = map[string]ProjectKnownAgent{}
	}

	for id, agent := range updates.KnownAgents {
		prior := existing.KnownAgents[id]
		existing.KnownAgents[id] = mergeKnownAgent(prior, agent)
	}

	if updates.Version != 0 {
		existing.Version = updates.Version
	}
	if updates.ChannelID != "" {
		existing.ChannelID = updates.ChannelID
	}
	if updates.ChannelName != "" {
		existing.ChannelName = updates.ChannelName
	}
	if updates.CreatedAt != "" {
		existing.CreatedAt = updates.CreatedAt
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	configPath := filepath.Join(frayDir, projectConfigFile)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return nil, err
	}

	return existing, nil
}

func mergeKnownAgent(existing, updates ProjectKnownAgent) ProjectKnownAgent {
	merged := existing
	if updates.Name != nil {
		merged.Name = updates.Name
	}
	if updates.GlobalName != nil {
		merged.GlobalName = updates.GlobalName
	}
	if updates.HomeChannel != nil {
		merged.HomeChannel = updates.HomeChannel
	}
	if updates.CreatedAt != nil {
		merged.CreatedAt = updates.CreatedAt
	}
	if updates.FirstSeen != nil {
		merged.FirstSeen = updates.FirstSeen
	}
	if updates.Status != nil {
		merged.Status = updates.Status
	}
	if updates.Nicks != nil {
		merged.Nicks = updates.Nicks
	}
	return merged
}

// ReadProjectConfig reads the project config file.
func ReadProjectConfig(projectPath string) (*ProjectConfig, error) {
	frayDir := resolveFrayDir(projectPath)
	path := filepath.Join(frayDir, projectConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var config ProjectConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// RebuildDatabaseFromJSONL resets the SQLite cache using JSONL sources.
func RebuildDatabaseFromJSONL(db DBTX, projectPath string) error {
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
	ghostCursors, err := ReadGhostCursors(projectPath)
	if err != nil {
		return err
	}
	config, err := ReadProjectConfig(projectPath)
	if err != nil {
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
	if err := initSchemaWith(db); err != nil {
		return err
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

	insertAgent := `
		INSERT OR REPLACE INTO fray_agents (
			guid, agent_id, status, purpose, registered_at, last_seen, left_at, managed, invoke, presence, mention_watermark, last_heartbeat
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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

		if _, err := db.Exec(insertAgent,
			agent.ID,
			agent.AgentID,
			status,
			purpose,
			agent.RegisteredAt,
			agent.LastSeen,
			agent.LeftAt,
			managed,
			invokeJSON,
			presence,
			agent.MentionWatermark,
			agent.LastHeartbeat,
		); err != nil {
			return err
		}
	}

	insertMessage := `
		INSERT OR REPLACE INTO fray_messages (
			guid, ts, channel_id, home, from_agent, body, mentions, type, "references", surface_message, reply_to, edited_at, archived_at, reactions
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	for _, message := range messages {
		mentionsJSON, err := json.Marshal(message.Mentions)
		if err != nil {
			return err
		}
		reactionsJSON, err := json.Marshal(normalizeReactions(message.Reactions))
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
			message.TS,
			message.ChannelID,
			home,
			message.FromAgent,
			message.Body,
			string(mentionsJSON),
			msgType,
			message.References,
			message.SurfaceMessage,
			message.ReplyTo,
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
		insertThread := `
			INSERT OR REPLACE INTO fray_threads (
				guid, name, parent_thread, status, created_at, anchor_message_guid, anchor_hidden, last_activity_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`
		for _, thread := range threads {
			status := thread.Status
			if status == "" {
				status = string(types.ThreadStatusOpen)
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
				thread.CreatedAt,
				thread.AnchorMessageGUID,
				anchorHidden,
				thread.LastActivityAt,
			); err != nil {
				return err
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

		for _, event := range subEvents {
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
					return err
				}
			}
		}

		threadMessages := make(map[string]map[string]ThreadMessageJSONLRecord)
		for _, event := range msgEvents {
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
					return err
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

	return nil
}
