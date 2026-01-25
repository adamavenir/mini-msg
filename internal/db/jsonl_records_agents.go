package db

import "github.com/adamavenir/fray/internal/types"

// AgentJSONLRecord represents an agent entry in JSONL.
type AgentJSONLRecord struct {
	Type             string              `json:"type"`
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	GlobalName       *string             `json:"global_name,omitempty"`
	HomeChannel      *string             `json:"home_channel,omitempty"`
	CreatedAt        *string             `json:"created_at,omitempty"`
	ActiveStatus     *string             `json:"active_status,omitempty"`
	AgentID          string              `json:"agent_id"`
	Status           *string             `json:"status,omitempty"`
	Purpose          *string             `json:"purpose,omitempty"`
	Avatar           *string             `json:"avatar,omitempty"`
	Goal             *string             `json:"goal,omitempty"`
	Bio              *string             `json:"bio,omitempty"`
	RegisteredAt     int64               `json:"registered_at"`
	LastSeen         int64               `json:"last_seen"`
	LeftAt           *int64              `json:"left_at"`
	Managed          bool                `json:"managed,omitempty"`
	Invoke           *types.InvokeConfig `json:"invoke,omitempty"`
	Presence         string              `json:"presence,omitempty"`
	MentionWatermark *string             `json:"mention_watermark,omitempty"`
	LastHeartbeat    *int64              `json:"last_heartbeat,omitempty"`
	SessionMode      string              `json:"session_mode,omitempty"`
	LastSessionID    *string             `json:"last_session_id,omitempty"`
}

// AgentUpdateJSONLRecord represents an agent update entry in JSONL.
type AgentUpdateJSONLRecord struct {
	Type             string              `json:"type"`
	AgentID          string              `json:"agent_id"`
	Status           *string             `json:"status,omitempty"`
	Purpose          *string             `json:"purpose,omitempty"`
	Avatar           *string             `json:"avatar,omitempty"`
	AAPGUID          *string             `json:"aap_guid,omitempty"`
	LastSeen         *int64              `json:"last_seen,omitempty"`
	LeftAt           *int64              `json:"left_at,omitempty"`
	Managed          *bool               `json:"managed,omitempty"`
	Invoke           *types.InvokeConfig `json:"invoke,omitempty"`
	Presence         *string             `json:"presence,omitempty"`
	MentionWatermark *string             `json:"mention_watermark,omitempty"`
	LastHeartbeat    *int64              `json:"last_heartbeat,omitempty"`
	SessionMode      *string             `json:"session_mode,omitempty"`
	LastSessionID    *string             `json:"last_session_id,omitempty"`
}

// SessionStartJSONLRecord represents a session start event in JSONL.
type SessionStartJSONLRecord struct {
	Type        string  `json:"type"`
	AgentID     string  `json:"agent_id"`
	SessionID   string  `json:"session_id"`
	TriggeredBy *string `json:"triggered_by,omitempty"`
	ThreadGUID  *string `json:"thread_guid,omitempty"`
	StartedAt   int64   `json:"started_at"`
}

// SessionEndJSONLRecord represents a session end event in JSONL.
type SessionEndJSONLRecord struct {
	Type       string `json:"type"`
	AgentID    string `json:"agent_id"`
	SessionID  string `json:"session_id"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	EndedAt    int64  `json:"ended_at"`
}

// SessionShutdownJSONLRecord represents a graceful shutdown event in JSONL.
type SessionShutdownJSONLRecord struct {
	Type            string   `json:"type"` // "session_shutdown"
	AgentID         string   `json:"agent_id"`
	SessionID       string   `json:"session_id"`
	UnprocessedMsgs []string `json:"unprocessed_msgs,omitempty"`
	NewWatermark    *string  `json:"new_watermark,omitempty"`
	ShutdownAt      int64    `json:"shutdown_at"`
	ShutdownReason  string   `json:"shutdown_reason"`
}

// UsageSnapshotJSONLRecord represents a usage snapshot in JSONL.
// Persisted on session end for durability across transcript rotation.
type UsageSnapshotJSONLRecord struct {
	Type           string `json:"type"` // "usage_snapshot"
	AgentID        string `json:"agent_id"`
	SessionID      string `json:"session_id"`
	Driver         string `json:"driver"`
	Model          string `json:"model,omitempty"`
	InputTokens    int64  `json:"input_tokens"`
	OutputTokens   int64  `json:"output_tokens"`
	CachedTokens   int64  `json:"cached_tokens"`
	ContextLimit   int64  `json:"context_limit"`
	ContextPercent int    `json:"context_percent"`
	CapturedAt     int64  `json:"captured_at"`
}

// SessionHeartbeatJSONLRecord represents a session heartbeat event in JSONL.
type SessionHeartbeatJSONLRecord struct {
	Type      string `json:"type"`
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	At        int64  `json:"at"`
}

// PresenceEventJSONLRecord represents a presence state transition in JSONL.
// This provides an audit trail of all presence changes for debugging.
type PresenceEventJSONLRecord struct {
	Type    string  `json:"type"`             // "presence_event"
	AgentID string  `json:"agent_id"`         // Agent whose presence changed
	From    string  `json:"from"`             // Previous presence state (or empty if first)
	To      string  `json:"to"`               // New presence state
	Status  *string `json:"status,omitempty"` // Agent status at time of change (for status_update events)
	Reason  string  `json:"reason"`           // Why: spawn, bye, back, exit_ok, exit_error, signal_kill, startup_cleanup, reset, status_update
	Source  string  `json:"source"`           // Who: daemon, command, startup, status
	TS      int64   `json:"ts"`               // Unix timestamp of the change
}

// AgentDescriptorJSONLRecord represents a shared agent descriptor event.
type AgentDescriptorJSONLRecord struct {
	Type         string   `json:"type"` // "agent_descriptor"
	AgentID      string   `json:"agent_id"`
	DisplayName  *string  `json:"display_name,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Seq          int64    `json:"seq,omitempty"`
	TS           int64    `json:"ts"`
}

// GhostCursorJSONLRecord represents a ghost cursor event in JSONL.
type GhostCursorJSONLRecord struct {
	Type        string `json:"type"` // "ghost_cursor"
	AgentID     string `json:"agent_id"`
	Home        string `json:"home"`
	MessageGUID string `json:"message_guid"`
	MustRead    bool   `json:"must_read"`
	SetAt       int64  `json:"set_at"`
}

// CursorClearJSONLRecord represents clearing a ghost cursor.
type CursorClearJSONLRecord struct {
	Type    string `json:"type"` // "cursor_clear"
	AgentID string `json:"agent_id"`
	Home    string `json:"home"`
	Seq     int64  `json:"seq,omitempty"`
	TS      int64  `json:"ts"`
}
