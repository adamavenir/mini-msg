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
	GUID         string
	AgentID      string
	Status       *string
	Purpose      *string
	RegisteredAt int64
	LastSeen     int64
	LeftAt       *int64
}

// Message represents a room message.
type Message struct {
	ID         string
	TS         int64
	ChannelID  *string
	FromAgent  string
	Body       string
	Mentions   []string
	Type       MessageType
	ReplyTo    *string
	EditedAt   *int64
	ArchivedAt *int64
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
	Alias string
	Path  string
}

// ConfigEntry represents a key/value config row.
type ConfigEntry struct {
	Key   string
	Value string
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
	GUID string
	TS   int64
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
	MessageGUID string
	AgentPrefix string
	ReadAt      int64
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
	ID        int64
	AgentID   string
	ClaimType ClaimType
	Pattern   string
	Reason    *string
	CreatedAt int64
	ExpiresAt *int64
}

// ClaimInput represents new-claim data.
type ClaimInput struct {
	AgentID   string
	ClaimType ClaimType
	Pattern   string
	Reason    *string
	ExpiresAt *int64
}
