package db

import "github.com/adamavenir/fray/internal/types"

const (
	messagesFile      = "messages.jsonl"
	agentsFile        = "agents.jsonl"
	questionsFile     = "questions.jsonl"
	threadsFile       = "threads.jsonl"
	projectConfigFile = "fray-config.json"
)

// MessageJSONLRecord represents a message entry in JSONL.
type MessageJSONLRecord struct {
	Type             string              `json:"type"`
	ID               string              `json:"id"`
	ChannelID        *string             `json:"channel_id"`
	Home             string              `json:"home,omitempty"`
	FromAgent        string              `json:"from_agent"`
	Body             string              `json:"body"`
	Mentions         []string            `json:"mentions"`
	Reactions        map[string][]string `json:"reactions,omitempty"`
	MsgType          types.MessageType   `json:"message_type"`
	References       *string             `json:"references,omitempty"`
	SurfaceMessage   *string             `json:"surface_message,omitempty"`
	ReplyTo          *string             `json:"reply_to"`
	QuoteMessageGUID *string             `json:"quote_message_guid,omitempty"`
	TS               int64               `json:"ts"`
	EditedAt         *int64              `json:"edited_at"`
	ArchivedAt       *int64              `json:"archived_at"`
}

// MessageUpdateJSONLRecord represents a message update entry in JSONL.
type MessageUpdateJSONLRecord struct {
	Type       string               `json:"type"`
	ID         string               `json:"id"`
	Body       *string              `json:"body,omitempty"`
	EditedAt   *int64               `json:"edited_at,omitempty"`
	ArchivedAt *int64               `json:"archived_at,omitempty"`
	Reactions  *map[string][]string `json:"reactions,omitempty"`
	Reason     *string              `json:"reason,omitempty"`
}

// QuestionJSONLRecord represents a question entry in JSONL.
type QuestionJSONLRecord struct {
	Type       string                 `json:"type"`
	GUID       string                 `json:"guid"`
	Re         string                 `json:"re"`
	FromAgent  string                 `json:"from_agent"`
	ToAgent    *string                `json:"to,omitempty"`
	Status     string                 `json:"status"`
	ThreadGUID *string                `json:"thread_guid,omitempty"`
	AskedIn    *string                `json:"asked_in,omitempty"`
	AnsweredIn *string                `json:"answered_in,omitempty"`
	Options    []types.QuestionOption `json:"options,omitempty"`
	CreatedAt  int64                  `json:"created_at"`
}

// QuestionUpdateJSONLRecord represents a question update entry in JSONL.
type QuestionUpdateJSONLRecord struct {
	Type       string  `json:"type"`
	GUID       string  `json:"guid"`
	Status     *string `json:"status,omitempty"`
	ToAgent    *string `json:"to,omitempty"`
	ThreadGUID *string `json:"thread_guid,omitempty"`
	AskedIn    *string `json:"asked_in,omitempty"`
	AnsweredIn *string `json:"answered_in,omitempty"`
}

// ThreadJSONLRecord represents a thread entry in JSONL.
type ThreadJSONLRecord struct {
	Type              string   `json:"type"`
	GUID              string   `json:"guid"`
	Name              string   `json:"name"`
	ParentThread      *string  `json:"parent_thread,omitempty"`
	Subscribed        []string `json:"subscribed,omitempty"`
	Status            string   `json:"status"`
	ThreadType        string   `json:"thread_type,omitempty"`
	CreatedAt         int64    `json:"created_at"`
	AnchorMessageGUID *string  `json:"anchor_message_guid,omitempty"`
	AnchorHidden      bool     `json:"anchor_hidden,omitempty"`
	LastActivityAt    *int64   `json:"last_activity_at,omitempty"`
}

// ThreadUpdateJSONLRecord represents a thread update entry in JSONL.
type ThreadUpdateJSONLRecord struct {
	Type              string  `json:"type"`
	GUID              string  `json:"guid"`
	Name              *string `json:"name,omitempty"`
	Status            *string `json:"status,omitempty"`
	ThreadType        *string `json:"thread_type,omitempty"`
	ParentThread      *string `json:"parent_thread,omitempty"`
	AnchorMessageGUID *string `json:"anchor_message_guid,omitempty"`
	AnchorHidden      *bool   `json:"anchor_hidden,omitempty"`
	LastActivityAt    *int64  `json:"last_activity_at,omitempty"`
}

// ThreadSubscribeJSONLRecord represents a subscription event.
type ThreadSubscribeJSONLRecord struct {
	Type         string `json:"type"`
	ThreadGUID   string `json:"thread_guid"`
	AgentID      string `json:"agent_id"`
	SubscribedAt int64  `json:"subscribed_at"`
}

// ThreadUnsubscribeJSONLRecord represents an unsubscribe event.
type ThreadUnsubscribeJSONLRecord struct {
	Type           string `json:"type"`
	ThreadGUID     string `json:"thread_guid"`
	AgentID        string `json:"agent_id"`
	UnsubscribedAt int64  `json:"unsubscribed_at"`
}

// ThreadMessageJSONLRecord represents a thread membership event.
type ThreadMessageJSONLRecord struct {
	Type        string `json:"type"`
	ThreadGUID  string `json:"thread_guid"`
	MessageGUID string `json:"message_guid"`
	AddedBy     string `json:"added_by"`
	AddedAt     int64  `json:"added_at"`
}

// ThreadMessageRemoveJSONLRecord represents a removal event.
type ThreadMessageRemoveJSONLRecord struct {
	Type        string `json:"type"`
	ThreadGUID  string `json:"thread_guid"`
	MessageGUID string `json:"message_guid"`
	RemovedBy   string `json:"removed_by"`
	RemovedAt   int64  `json:"removed_at"`
}

// MessagePinJSONLRecord represents a message pin event.
type MessagePinJSONLRecord struct {
	Type        string `json:"type"`
	MessageGUID string `json:"message_guid"`
	ThreadGUID  string `json:"thread_guid"`
	PinnedBy    string `json:"pinned_by"`
	PinnedAt    int64  `json:"pinned_at"`
}

// MessageUnpinJSONLRecord represents a message unpin event.
type MessageUnpinJSONLRecord struct {
	Type         string `json:"type"`
	MessageGUID  string `json:"message_guid"`
	ThreadGUID   string `json:"thread_guid"`
	UnpinnedBy   string `json:"unpinned_by"`
	UnpinnedAt   int64  `json:"unpinned_at"`
}

// MessageMoveJSONLRecord represents a message move event.
type MessageMoveJSONLRecord struct {
	Type        string `json:"type"`
	MessageGUID string `json:"message_guid"`
	OldHome     string `json:"old_home"`
	NewHome     string `json:"new_home"`
	MovedBy     string `json:"moved_by"`
	MovedAt     int64  `json:"moved_at"`
}

// ThreadPinJSONLRecord represents a thread pin event.
type ThreadPinJSONLRecord struct {
	Type       string `json:"type"`
	ThreadGUID string `json:"thread_guid"`
	PinnedBy   string `json:"pinned_by"`
	PinnedAt   int64  `json:"pinned_at"`
}

// ThreadUnpinJSONLRecord represents a thread unpin event.
type ThreadUnpinJSONLRecord struct {
	Type        string `json:"type"`
	ThreadGUID  string `json:"thread_guid"`
	UnpinnedBy  string `json:"unpinned_by"`
	UnpinnedAt  int64  `json:"unpinned_at"`
}

// ThreadMuteJSONLRecord represents a thread mute event.
type ThreadMuteJSONLRecord struct {
	Type       string `json:"type"`
	ThreadGUID string `json:"thread_guid"`
	AgentID    string `json:"agent_id"`
	MutedAt    int64  `json:"muted_at"`
	ExpiresAt  *int64 `json:"expires_at,omitempty"`
}

// ThreadUnmuteJSONLRecord represents a thread unmute event.
type ThreadUnmuteJSONLRecord struct {
	Type       string `json:"type"`
	ThreadGUID string `json:"thread_guid"`
	AgentID    string `json:"agent_id"`
	UnmutedAt  int64  `json:"unmuted_at"`
}

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
}

// AgentUpdateJSONLRecord represents an agent update entry in JSONL.
type AgentUpdateJSONLRecord struct {
	Type             string              `json:"type"`
	AgentID          string              `json:"agent_id"`
	Status           *string             `json:"status,omitempty"`
	Purpose          *string             `json:"purpose,omitempty"`
	Avatar           *string             `json:"avatar,omitempty"`
	LastSeen         *int64              `json:"last_seen,omitempty"`
	LeftAt           *int64              `json:"left_at,omitempty"`
	Managed          *bool               `json:"managed,omitempty"`
	Invoke           *types.InvokeConfig `json:"invoke,omitempty"`
	Presence         *string             `json:"presence,omitempty"`
	MentionWatermark *string             `json:"mention_watermark,omitempty"`
	LastHeartbeat    *int64              `json:"last_heartbeat,omitempty"`
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

// SessionHeartbeatJSONLRecord represents a session heartbeat event in JSONL.
type SessionHeartbeatJSONLRecord struct {
	Type      string `json:"type"`
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	At        int64  `json:"at"`
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

// ReactionJSONLRecord represents a reaction event in JSONL.
type ReactionJSONLRecord struct {
	Type        string `json:"type"` // "reaction"
	MessageGUID string `json:"message_guid"`
	AgentID     string `json:"agent_id"`
	Emoji       string `json:"emoji"`
	ReactedAt   int64  `json:"reacted_at"`
}

// AgentFaveJSONLRecord represents a fave event in JSONL.
type AgentFaveJSONLRecord struct {
	Type     string `json:"type"` // "agent_fave"
	AgentID  string `json:"agent_id"`
	ItemType string `json:"item_type"` // "thread" | "message"
	ItemGUID string `json:"item_guid"`
	FavedAt  int64  `json:"faved_at"`
}

// AgentUnfaveJSONLRecord represents an unfave event in JSONL.
type AgentUnfaveJSONLRecord struct {
	Type      string `json:"type"` // "agent_unfave"
	AgentID   string `json:"agent_id"`
	ItemType  string `json:"item_type"` // "thread" | "message"
	ItemGUID  string `json:"item_guid"`
	UnfavedAt int64  `json:"unfaved_at"`
}

// RoleHoldJSONLRecord represents a role hold (persistent assignment) event.
type RoleHoldJSONLRecord struct {
	Type       string `json:"type"` // "role_hold"
	AgentID    string `json:"agent_id"`
	RoleName   string `json:"role_name"`
	AssignedAt int64  `json:"assigned_at"`
}

// RoleDropJSONLRecord represents a role drop (removal) event.
type RoleDropJSONLRecord struct {
	Type      string `json:"type"` // "role_drop"
	AgentID   string `json:"agent_id"`
	RoleName  string `json:"role_name"`
	DroppedAt int64  `json:"dropped_at"`
}

// RolePlayJSONLRecord represents a session-scoped role play event.
type RolePlayJSONLRecord struct {
	Type      string  `json:"type"` // "role_play"
	AgentID   string  `json:"agent_id"`
	RoleName  string  `json:"role_name"`
	SessionID *string `json:"session_id,omitempty"`
	StartedAt int64   `json:"started_at"`
}

// RoleStopJSONLRecord represents stopping a session role.
type RoleStopJSONLRecord struct {
	Type      string `json:"type"` // "role_stop"
	AgentID   string `json:"agent_id"`
	RoleName  string `json:"role_name"`
	StoppedAt int64  `json:"stopped_at"`
}

// ProjectKnownAgent stores per-project known-agent data.
type ProjectKnownAgent struct {
	Name        *string  `json:"name,omitempty"`
	GlobalName  *string  `json:"global_name,omitempty"`
	HomeChannel *string  `json:"home_channel,omitempty"`
	CreatedAt   *string  `json:"created_at,omitempty"`
	FirstSeen   *string  `json:"first_seen,omitempty"`
	Status      *string  `json:"status,omitempty"`
	Nicks       []string `json:"nicks,omitempty"`
}

// ProjectConfig represents the per-project config file.
type ProjectConfig struct {
	Version     int                          `json:"version"`
	ChannelID   string                       `json:"channel_id,omitempty"`
	ChannelName string                       `json:"channel_name,omitempty"`
	CreatedAt   string                       `json:"created_at,omitempty"`
	KnownAgents map[string]ProjectKnownAgent `json:"known_agents,omitempty"`
}
