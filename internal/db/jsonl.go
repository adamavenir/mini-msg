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
	Type           string              `json:"type"`
	ID             string              `json:"id"`
	ChannelID      *string             `json:"channel_id"`
	Home           string              `json:"home,omitempty"`
	FromAgent      string              `json:"from_agent"`
	Body           string              `json:"body"`
	Mentions       []string            `json:"mentions"`
	Reactions      map[string][]string `json:"reactions,omitempty"`
	MsgType        types.MessageType   `json:"message_type"`
	References     *string             `json:"references,omitempty"`
	SurfaceMessage *string             `json:"surface_message,omitempty"`
	ReplyTo        *string             `json:"reply_to"`
	TS             int64               `json:"ts"`
	EditedAt       *int64              `json:"edited_at"`
	ArchivedAt     *int64              `json:"archived_at"`
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
	Type         string   `json:"type"`
	GUID         string   `json:"guid"`
	Name         string   `json:"name"`
	ParentThread *string  `json:"parent_thread,omitempty"`
	Subscribed   []string `json:"subscribed,omitempty"`
	Status       string   `json:"status"`
	CreatedAt    int64    `json:"created_at"`
}

// ThreadUpdateJSONLRecord represents a thread update entry in JSONL.
type ThreadUpdateJSONLRecord struct {
	Type         string  `json:"type"`
	GUID         string  `json:"guid"`
	Name         *string `json:"name,omitempty"`
	Status       *string `json:"status,omitempty"`
	ParentThread *string `json:"parent_thread,omitempty"`
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
	Goal             *string             `json:"goal,omitempty"`
	Bio              *string             `json:"bio,omitempty"`
	RegisteredAt     int64               `json:"registered_at"`
	LastSeen         int64               `json:"last_seen"`
	LeftAt           *int64              `json:"left_at"`
	Managed          bool                `json:"managed,omitempty"`
	Invoke           *types.InvokeConfig `json:"invoke,omitempty"`
	Presence         string              `json:"presence,omitempty"`
	MentionWatermark *string             `json:"mention_watermark,omitempty"`
}

// AgentUpdateJSONLRecord represents an agent update entry in JSONL.
type AgentUpdateJSONLRecord struct {
	Type             string              `json:"type"`
	AgentID          string              `json:"agent_id"`
	Status           *string             `json:"status,omitempty"`
	Purpose          *string             `json:"purpose,omitempty"`
	LastSeen         *int64              `json:"last_seen,omitempty"`
	LeftAt           *int64              `json:"left_at,omitempty"`
	Managed          *bool               `json:"managed,omitempty"`
	Invoke           *types.InvokeConfig `json:"invoke,omitempty"`
	Presence         *string             `json:"presence,omitempty"`
	MentionWatermark *string             `json:"mention_watermark,omitempty"`
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
