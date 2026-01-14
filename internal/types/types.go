package types

// MessageType represents the source of a message.
type MessageType string

const (
	MessageTypeAgent     MessageType = "agent"
	MessageTypeUser      MessageType = "user"
	MessageTypeEvent     MessageType = "event"
	MessageTypeSurface   MessageType = "surface"
	MessageTypeTombstone MessageType = "tombstone"
)

// PresenceState represents the agent's daemon-managed presence.
type PresenceState string

const (
	PresenceActive    PresenceState = "active"
	PresenceSpawning  PresenceState = "spawning"
	PresencePrompting PresenceState = "prompting" // input_tokens > 0, sending context to API
	PresencePrompted  PresenceState = "prompted"  // output_tokens > 0, agent is generating
	PresenceIdle      PresenceState = "idle"
	PresenceError     PresenceState = "error"
	PresenceOffline   PresenceState = "offline"
	PresenceBRB       PresenceState = "brb"       // agent wants immediate respawn (fray brb)
)

// PromptDelivery specifies how prompts are passed to CLI.
type PromptDelivery string

const (
	PromptDeliveryArgs     PromptDelivery = "args"
	PromptDeliveryStdin    PromptDelivery = "stdin"
	PromptDeliveryTempfile PromptDelivery = "tempfile"
)

// InvokeConfig holds driver-specific configuration for spawning agents.
type InvokeConfig struct {
	Driver         string         `json:"driver,omitempty"`           // claude, codex, opencode
	Model          string         `json:"model,omitempty"`            // model to use (e.g., sonnet-1m for 1M context Sonnet)
	Trust          []string       `json:"trust,omitempty"`            // trust capabilities: "wake" allows this agent to wake others
	Config         map[string]any `json:"config,omitempty"`           // driver-specific config
	PromptDelivery PromptDelivery `json:"prompt_delivery,omitempty"`  // args, stdin, tempfile
	SpawnTimeoutMs int64          `json:"spawn_timeout_ms,omitempty"` // max time in 'spawning' before 'error' (default: 30000)
	IdleAfterMs    int64          `json:"idle_after_ms,omitempty"`    // time since activity before 'idle' (default: 5000)
	MinCheckinMs   int64          `json:"min_checkin_ms,omitempty"`   // done-detection: idle + no fray posts for this duration = kill (default: 0 = disabled)
	MaxRuntimeMs   int64          `json:"max_runtime_ms,omitempty"`   // zombie safety net: forced termination (default: 0 = unlimited)
}

// Agent represents agent identity and presence.
type Agent struct {
	GUID             string         `json:"guid"`
	AgentID          string         `json:"agent_id"`
	AAPGUID          *string        `json:"aap_guid,omitempty"` // Link to AAP identity
	Status           *string        `json:"status,omitempty"`
	Purpose          *string        `json:"purpose,omitempty"`
	Avatar           *string        `json:"avatar,omitempty"`            // single-char avatar for display
	RegisteredAt     int64          `json:"registered_at"`
	LastSeen         int64          `json:"last_seen"`
	LeftAt           *int64         `json:"left_at,omitempty"`
	Managed          bool           `json:"managed,omitempty"`           // whether daemon controls this agent
	Invoke           *InvokeConfig  `json:"invoke,omitempty"`            // daemon invocation config
	Presence         PresenceState  `json:"presence,omitempty"`          // daemon-tracked presence state
	MentionWatermark  *string        `json:"mention_watermark,omitempty"`  // last processed mention msg_id
	ReactionWatermark *int64         `json:"reaction_watermark,omitempty"` // last processed reaction timestamp (ms)
	LastHeartbeat     *int64         `json:"last_heartbeat,omitempty"`     // last silent checkin timestamp (ms)
	LastSessionID     *string        `json:"last_session_id,omitempty"`    // Claude Code session ID for --resume
	SessionMode       string         `json:"session_mode,omitempty"`       // "" (resumed), "n" (new), or 3-char fork prefix
	JobID             *string        `json:"job_id,omitempty"`             // FK to fray_jobs.guid (nil for regular agents)
	JobIdx            *int           `json:"job_idx,omitempty"`            // worker index within job (0-based)
	IsEphemeral       bool           `json:"is_ephemeral,omitempty"`       // true for job workers
}

// ReactionEntry represents a single reaction from an agent.
type ReactionEntry struct {
	AgentID   string `json:"agent_id"`
	ReactedAt int64  `json:"reacted_at"`
}

// InterruptInfo describes how to interrupt an agent.
type InterruptInfo struct {
	Double  bool `json:"double"`   // !! prefix: clear session (fresh start)
	NoSpawn bool `json:"no_spawn"` // ! suffix: don't spawn after interrupt
}

// Message represents a room message.
type Message struct {
	ID               string                     `json:"id"`
	TS               int64                      `json:"ts"`
	ChannelID        *string                    `json:"channel_id,omitempty"`
	Home             string                     `json:"home,omitempty"`
	FromAgent        string                     `json:"from_agent"`
	SessionID        *string                    `json:"session_id,omitempty"`
	Body             string                     `json:"body"`
	Mentions         []string                   `json:"mentions"`
	ForkSessions     map[string]string          `json:"fork_sessions,omitempty"`
	Reactions        map[string][]ReactionEntry `json:"reactions"`
	Type             MessageType                `json:"type"`
	References       *string                    `json:"references,omitempty"`
	SurfaceMessage   *string                    `json:"surface_message,omitempty"`
	ReplyTo          *string                    `json:"reply_to,omitempty"`
	QuoteMessageGUID *string                    `json:"quote_message_guid,omitempty"`
	EditedAt         *int64                     `json:"edited_at,omitempty"`
	Edited           bool                       `json:"edited,omitempty"`
	EditCount        int                        `json:"edit_count,omitempty"`
	ArchivedAt       *int64                     `json:"archived_at,omitempty"`
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
	GUID             string
	TS               int64
	ChannelID        *string
	Home             string
	FromAgent        string
	Body             string
	Mentions         string
	Reactions        string
	Type             MessageType
	References       *string
	SurfaceMessage   *string
	ReplyTo          *string
	QuoteMessageGUID *string
	EditedAt         *int64
	ArchivedAt       *int64
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

// OptionalBool represents a nullable bool update.
type OptionalBool struct {
	Set   bool
	Value bool
}

// MessageQueryOptions controls message queries.
type MessageQueryOptions struct {
	Limit                 int
	Since                 *MessageCursor
	SinceID               string
	Before                *MessageCursor
	BeforeID              string
	Home                  *string
	Filter                *Filter
	Unfiltered            bool
	UnreadOnly            bool
	AgentPrefix           string
	IncludeArchived       bool
	IncludeRepliesToAgent string // Include replies to messages from this agent prefix
}

// QuestionQueryOptions controls question queries.
type QuestionQueryOptions struct {
	Statuses     []QuestionStatus
	ThreadGUID   *string
	RoomOnly     bool
	ToAgent      *string
	AskedIn      *string // Filter by source message GUID
	NoTargetOnly bool    // Filter to questions with no to_agent (anyone can answer)
}

// ThreadQueryOptions controls thread queries.
type ThreadQueryOptions struct {
	SubscribedAgent *string
	ParentThread    *string
	Status          *ThreadStatus
	IncludeArchived bool
	SortByActivity  bool // Sort by last_activity_at DESC instead of created_at ASC
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

// QuestionOption represents a proposed answer with pros/cons.
type QuestionOption struct {
	Label string   `json:"label"`
	Pros  []string `json:"pros,omitempty"`
	Cons  []string `json:"cons,omitempty"`
}

// Question represents a tracked question.
type Question struct {
	GUID       string           `json:"guid"`
	Re         string           `json:"re"`
	FromAgent  string           `json:"from_agent"`
	ToAgent    *string          `json:"to_agent,omitempty"`
	Status     QuestionStatus   `json:"status"`
	ThreadGUID *string          `json:"thread_guid,omitempty"`
	AskedIn    *string          `json:"asked_in,omitempty"`
	AnsweredIn *string          `json:"answered_in,omitempty"`
	Options    []QuestionOption `json:"options,omitempty"`
	CreatedAt  int64            `json:"created_at"`
}

// PermissionStatus represents permission request lifecycle state.
type PermissionStatus string

const (
	PermissionStatusPending  PermissionStatus = "pending"
	PermissionStatusApproved PermissionStatus = "approved"
	PermissionStatusDenied   PermissionStatus = "denied"
	PermissionStatusExpired  PermissionStatus = "expired"
)

// PermissionScope determines how long an approval lasts.
type PermissionScope string

const (
	PermissionScopeOnce    PermissionScope = "once"    // Just this invocation
	PermissionScopeSession PermissionScope = "session" // This agent's session
	PermissionScopeProject PermissionScope = "project" // Persists in settings, all agents
)

// PermissionOption represents a selectable approval option.
type PermissionOption struct {
	Label    string          `json:"label"`
	Patterns []string        `json:"patterns"`          // Permission patterns to grant
	Scope    PermissionScope `json:"scope"`             // How long approval lasts
	Note     string          `json:"note,omitempty"`    // Why this is safe
	Warning  string          `json:"warning,omitempty"` // If this includes risky operations
}

// PermissionRequest represents a pending permission approval request.
type PermissionRequest struct {
	GUID        string             `json:"guid"`
	FromAgent   string             `json:"from_agent"`
	SessionID   string             `json:"session_id,omitempty"`
	Tool        string             `json:"tool"`                   // e.g., "Bash", "mcp__github"
	Action      string             `json:"action"`                 // e.g., "rm -rf node_modules"
	Rationale   string             `json:"rationale"`              // Why the agent needs this
	Options     []PermissionOption `json:"options"`                // Available approval choices
	Status      PermissionStatus   `json:"status"`                 // pending, approved, denied, expired
	ChosenIndex *int               `json:"chosen_index,omitempty"` // Which option was chosen
	RespondedBy *string            `json:"responded_by,omitempty"` // Who approved/denied
	CreatedAt   int64              `json:"created_at"`
	RespondedAt *int64             `json:"responded_at,omitempty"`
}

// InteractiveAction represents a clickable action in an event message.
// This enables reusable interactive UI patterns for permissions, questions, etc.
type InteractiveAction struct {
	ID      string `json:"id"`                // Unique action ID (e.g., "approve-1", "deny")
	Label   string `json:"label"`             // Display text (e.g., "[1] Allow once")
	Command string `json:"command"`           // CLI command to execute (e.g., "fray approve perm-abc 1")
	Style   string `json:"style,omitempty"`   // Style hint: "primary", "danger", "muted"
	Confirm bool   `json:"confirm,omitempty"` // Require confirmation before executing
}

// InteractiveEvent wraps event metadata with clickable actions.
// Messages with Type=event can embed this for interactive rendering.
type InteractiveEvent struct {
	Kind       string              `json:"kind"`                  // Event type: "permission", "question", "approval"
	TargetGUID string              `json:"target_guid"`           // GUID of related entity (permission, question, etc.)
	Title      string              `json:"title"`                 // Event title
	Body       string              `json:"body"`                  // Event body/description
	Actions    []InteractiveAction `json:"actions"`               // Clickable action buttons
	Status     string              `json:"status,omitempty"`      // Current status: "pending", "resolved"
	ResolvedBy *string             `json:"resolved_by,omitempty"` // Who resolved it
}

// ThreadStatus represents thread lifecycle state.
type ThreadStatus string

const (
	ThreadStatusOpen     ThreadStatus = "open"
	ThreadStatusArchived ThreadStatus = "archived"
)

// ThreadType represents thread category.
type ThreadType string

const (
	ThreadTypeStandard  ThreadType = "standard"  // normal user-created thread
	ThreadTypeKnowledge ThreadType = "knowledge" // knowledge hierarchy (meta, role, agent)
	ThreadTypeSystem    ThreadType = "system"    // system-managed (notes, keys, jrnl)
)

// Thread represents a container thread.
type Thread struct {
	GUID              string       `json:"guid"`
	Name              string       `json:"name"`
	ParentThread      *string      `json:"parent_thread,omitempty"`
	Status            ThreadStatus `json:"status"`
	Type              ThreadType   `json:"type,omitempty"`
	CreatedAt         int64        `json:"created_at"`
	CreatedBy         *string      `json:"created_by,omitempty"`  // agent_id or "human" who created thread
	OwnerAgent        *string      `json:"owner_agent,omitempty"` // agent who owns thread for daemon purposes (nil = human-only)
	AnchorMessageGUID *string      `json:"anchor_message_guid,omitempty"`
	AnchorHidden      bool         `json:"anchor_hidden,omitempty"`
	LastActivityAt    *int64       `json:"last_activity_at,omitempty"`
}

// ThreadWithStats extends Thread with computed statistics.
type ThreadWithStats struct {
	Thread
	MessageCount int `json:"message_count"`
	ChildCount   int `json:"child_count"`
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

// SessionStart records when a daemon spawns an agent session.
type SessionStart struct {
	AgentID     string  `json:"agent_id"`
	SessionID   string  `json:"session_id"`
	TriggeredBy *string `json:"triggered_by,omitempty"` // msg_id that triggered spawn
	ThreadGUID  *string `json:"thread_guid,omitempty"`  // thread context if applicable
	StartedAt   int64   `json:"started_at"`
}

// SessionEnd records when an agent session completes.
type SessionEnd struct {
	AgentID    string  `json:"agent_id"`
	SessionID  string  `json:"session_id"`
	ExitCode   int     `json:"exit_code"`
	DurationMs int64   `json:"duration_ms"`
	EndedAt    int64   `json:"ended_at"`
	LastMsgID  *string `json:"last_msg_id,omitempty"`
	Stderr     *string `json:"stderr,omitempty"`
}

// SessionHeartbeat records periodic session health updates.
type SessionHeartbeat struct {
	AgentID   string        `json:"agent_id"`
	SessionID string        `json:"session_id"`
	Status    PresenceState `json:"status"`
	At        int64         `json:"at"`
}

// GhostCursor represents a recommended read position for session handoff.
// Unlike read_to (actual read position), ghost cursor is where an outgoing
// agent says the next agent should START reading from.
//
// Session-aware unread logic:
//
//  1. Ghost cursor persists across views (not cleared on first view) so the same
//     context can be useful for multiple sessions working on related things.
//
//  2. SessionAckAt tracks whether the cursor has been "acknowledged" this session:
//     - NULL: cursor not yet viewed this session → use as unread boundary
//     - Set: cursor already viewed → use read receipts instead
//
//  3. On session start (fray back/new): SessionAckAt is cleared, so the cursor
//     becomes the unread boundary again for the new session.
//
// Flow: new session → ghost cursor boundary → ack → read receipts for rest of session
type GhostCursor struct {
	AgentID      string `json:"agent_id"`
	Home         string `json:"home"`                     // "room" or thread GUID
	MessageGUID  string `json:"message_guid"`             // start reading from here
	MustRead     bool   `json:"must_read"`                // inject full content vs hint only
	SetAt        int64  `json:"set_at"`
	SessionAckAt *int64 `json:"session_ack_at,omitempty"` // when first viewed this session
}

// RoleAssignment represents a persistent role held by an agent.
type RoleAssignment struct {
	AgentID    string `json:"agent_id"`
	RoleName   string `json:"role_name"`
	AssignedAt int64  `json:"assigned_at"`
}

// SessionRole represents a session-scoped role being played by an agent.
type SessionRole struct {
	AgentID   string  `json:"agent_id"`
	RoleName  string  `json:"role_name"`
	SessionID *string `json:"session_id,omitempty"`
	StartedAt int64   `json:"started_at"`
}

// AgentRoles summarizes an agent's held and playing roles.
type AgentRoles struct {
	AgentID  string   `json:"agent_id"`
	Held     []string `json:"held"`
	Playing  []string `json:"playing"`
}

// JobStatus represents job lifecycle state.
type JobStatus string

const (
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// JobContext holds optional context for a job (issues, threads, messages).
type JobContext struct {
	Issues   []string `json:"issues,omitempty"`
	Threads  []string `json:"threads,omitempty"`
	Messages []string `json:"messages,omitempty"`
}

// Job represents a parallel work session.
type Job struct {
	GUID        string      `json:"guid"`
	Name        string      `json:"name"`
	Context     *JobContext `json:"context,omitempty"`
	OwnerAgent  string      `json:"owner_agent"`
	Status      JobStatus   `json:"status"`
	ThreadGUID  string      `json:"thread_guid"`
	CreatedAt   int64       `json:"created_at"`
	CompletedAt *int64      `json:"completed_at,omitempty"`
}
