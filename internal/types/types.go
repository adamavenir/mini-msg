package types

// MessageType represents the source of a message.
type MessageType string

const (
	MessageTypeAgent   MessageType = "agent"
	MessageTypeUser    MessageType = "user"
	MessageTypeEvent   MessageType = "event"
	MessageTypeSurface MessageType = "surface"
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
	ID             string              `json:"id"`
	TS             int64               `json:"ts"`
	ChannelID      *string             `json:"channel_id,omitempty"`
	Home           string              `json:"home,omitempty"`
	FromAgent      string              `json:"from_agent"`
	Body           string              `json:"body"`
	Mentions       []string            `json:"mentions"`
	Reactions      map[string][]string `json:"reactions"`
	Type           MessageType         `json:"type"`
	References     *string             `json:"references,omitempty"`
	SurfaceMessage *string             `json:"surface_message,omitempty"`
	ReplyTo        *string             `json:"reply_to,omitempty"`
	EditedAt       *int64              `json:"edited_at,omitempty"`
	Edited         bool                `json:"edited,omitempty"`
	EditCount      int                 `json:"edit_count,omitempty"`
	ArchivedAt     *int64              `json:"archived_at,omitempty"`
}

// MessageVersion represents a version of a message body.
type MessageVersion struct {
	Version    int    `json:"version"`
	Body       string `json:"body"`
	Timestamp  int64  `json:"timestamp"`
	Reason     string `json:"reason,omitempty"`
	IsOriginal bool   `json:"is_original,omitempty"`
	IsCurrent  bool   `json:"is_current,omitempty"`
}

// MessageVersionHistory summarizes all versions of a message.
type MessageVersionHistory struct {
	MessageID    string           `json:"message_id"`
	VersionCount int              `json:"version_count"`
	IsArchived   bool             `json:"is_archived,omitempty"`
	Versions     []MessageVersion `json:"versions"`
}

// MessageRow is a raw database row representation of a message.
type MessageRow struct {
	GUID           string
	TS             int64
	ChannelID      *string
	Home           string
	FromAgent      string
	Body           string
	Mentions       string
	Reactions      string
	Type           MessageType
	References     *string
	SurfaceMessage *string
	ReplyTo        *string
	EditedAt       *int64
	ArchivedAt     *int64
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
	Home            *string
	Filter          *Filter
	Unfiltered      bool
	UnreadOnly      bool
	AgentPrefix     string
	IncludeArchived bool
}

// QuestionQueryOptions controls question queries.
type QuestionQueryOptions struct {
	Statuses   []QuestionStatus
	ThreadGUID *string
	RoomOnly   bool
	ToAgent    *string
}

// ThreadQueryOptions controls thread queries.
type ThreadQueryOptions struct {
	SubscribedAgent *string
	ParentThread    *string
	Status          *ThreadStatus
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

// QuestionStatus represents question lifecycle state.
type QuestionStatus string

const (
	QuestionStatusUnasked  QuestionStatus = "unasked"
	QuestionStatusOpen     QuestionStatus = "open"
	QuestionStatusAnswered QuestionStatus = "answered"
	QuestionStatusClosed   QuestionStatus = "closed"
)

// Question represents a tracked question.
type Question struct {
	GUID       string         `json:"guid"`
	Re         string         `json:"re"`
	FromAgent  string         `json:"from_agent"`
	ToAgent    *string        `json:"to_agent,omitempty"`
	Status     QuestionStatus `json:"status"`
	ThreadGUID *string        `json:"thread_guid,omitempty"`
	AskedIn    *string        `json:"asked_in,omitempty"`
	AnsweredIn *string        `json:"answered_in,omitempty"`
	CreatedAt  int64          `json:"created_at"`
}

// ThreadStatus represents thread lifecycle state.
type ThreadStatus string

const (
	ThreadStatusOpen     ThreadStatus = "open"
	ThreadStatusArchived ThreadStatus = "archived"
)

// Thread represents a container thread.
type Thread struct {
	GUID         string       `json:"guid"`
	Name         string       `json:"name"`
	ParentThread *string      `json:"parent_thread,omitempty"`
	Status       ThreadStatus `json:"status"`
	CreatedAt    int64        `json:"created_at"`
}

// ThreadSubscription records a thread subscription.
type ThreadSubscription struct {
	ThreadGUID   string `json:"thread_guid"`
	AgentID      string `json:"agent_id"`
	SubscribedAt int64  `json:"subscribed_at"`
}

// ThreadMessage records membership of a message in a thread.
type ThreadMessage struct {
	ThreadGUID  string `json:"thread_guid"`
	MessageGUID string `json:"message_guid"`
	AddedBy     string `json:"added_by"`
	AddedAt     int64  `json:"added_at"`
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
