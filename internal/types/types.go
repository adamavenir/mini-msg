package types

// MessageType represents the source of a message.
type MessageType string

const (
	MessageTypeAgent MessageType = "agent"
	MessageTypeUser  MessageType = "user"
	MessageTypeEvent MessageType = "event"
)

// Agent represents agent identity and presence.
type Agent struct {
	GUID         string  `json:"guid"`
	AgentID      string  `json:"agent_id"`
	Status       *string `json:"status,omitempty"`
	Purpose      *string `json:"purpose,omitempty"`
	RegisteredAt int64   `json:"registered_at"`
	LastSeen     int64   `json:"last_seen"`
	LeftAt       *int64  `json:"left_at,omitempty"`
}

// Message represents a room message.
type Message struct {
	ID         string      `json:"id"`
	TS         int64       `json:"ts"`
	ChannelID  *string     `json:"channel_id,omitempty"`
	FromAgent  string      `json:"from_agent"`
	Body       string      `json:"body"`
	Mentions   []string    `json:"mentions"`
	Type       MessageType `json:"type"`
	ReplyTo    *string     `json:"reply_to,omitempty"`
	EditedAt   *int64      `json:"edited_at,omitempty"`
	ArchivedAt *int64      `json:"archived_at,omitempty"`
}

// MessageRow is a raw database row representation of a message.
type MessageRow struct {
	GUID       string
	TS         int64
	ChannelID  *string
	FromAgent  string
	Body       string
	Mentions   string
	Type       MessageType
	ReplyTo    *string
	EditedAt   *int64
	ArchivedAt *int64
}

// LinkedProject represents a cross-project link.
type LinkedProject struct {
	Alias string `json:"alias"`
	Path  string `json:"path"`
}

// ConfigEntry represents a key/value config row.
type ConfigEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ParsedAgentID represents parsed agent ID components.
type ParsedAgentID struct {
	Base    string
	Version *int
	Full    string
}

// OptionalString represents a nullable string update.
type OptionalString struct {
	Set   bool
	Value *string
}

// OptionalInt64 represents a nullable int64 update.
type OptionalInt64 struct {
	Set   bool
	Value *int64
}

// MessageQueryOptions controls message queries.
type MessageQueryOptions struct {
	Limit           int
	Since           *MessageCursor
	SinceID         string
	Before          *MessageCursor
	BeforeID        string
	Filter          *Filter
	Unfiltered      bool
	UnreadOnly      bool
	AgentPrefix     string
	IncludeArchived bool
}

// MessageCursor represents a stable paging cursor.
type MessageCursor struct {
	GUID string `json:"guid"`
	TS   int64  `json:"ts"`
}

// Filter stores per-agent filtering preferences.
type Filter struct {
	AgentID         string
	MentionsPattern *string
}

// FilterRow is a raw filter row.
type FilterRow struct {
	AgentID         string
	MentionsPattern *string
}

// ReadReceipt tracks read state.
type ReadReceipt struct {
	MessageGUID string `json:"message_guid"`
	AgentPrefix string `json:"agent_prefix"`
	ReadAt      int64  `json:"read_at"`
}

// ClaimType represents a claim category.
type ClaimType string

const (
	ClaimTypeFile  ClaimType = "file"
	ClaimTypeBD    ClaimType = "bd"
	ClaimTypeIssue ClaimType = "issue"
)

// Claim represents a resource claim.
type Claim struct {
	ID        int64     `json:"id"`
	AgentID   string    `json:"agent_id"`
	ClaimType ClaimType `json:"claim_type"`
	Pattern   string    `json:"pattern"`
	Reason    *string   `json:"reason,omitempty"`
	CreatedAt int64     `json:"created_at"`
	ExpiresAt *int64    `json:"expires_at,omitempty"`
}

// ClaimInput represents new-claim data.
type ClaimInput struct {
	AgentID   string    `json:"agent_id"`
	ClaimType ClaimType `json:"claim_type"`
	Pattern   string    `json:"pattern"`
	Reason    *string   `json:"reason,omitempty"`
	ExpiresAt *int64    `json:"expires_at,omitempty"`
}
