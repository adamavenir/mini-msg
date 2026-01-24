package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

const (
	messagesFile      = "messages.jsonl"
	agentsFile        = "agents.jsonl"
	agentStateFile    = "agent-state.jsonl"
	questionsFile     = "questions.jsonl"
	threadsFile       = "threads.jsonl"
	permissionsFile   = "permissions.jsonl"
	projectConfigFile = "fray-config.json"
	runtimeFile       = "runtime.jsonl"
)

// MessageJSONLRecord represents a message entry in JSONL.
type MessageJSONLRecord struct {
	Type             string              `json:"type"`
	ID               string              `json:"id"`
	ChannelID        *string             `json:"channel_id"`
	Home             string              `json:"home,omitempty"`
	FromAgent        string              `json:"from_agent"`
	Origin           string              `json:"origin,omitempty"`
	Seq              int64               `json:"seq,omitempty"`
	SessionID        *string             `json:"session_id,omitempty"`
	Body             string              `json:"body"`
	Mentions         []string            `json:"mentions"`
	ForkSessions     map[string]string   `json:"fork_sessions,omitempty"`
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

// MessageDeleteJSONLRecord represents a message deletion tombstone.
type MessageDeleteJSONLRecord struct {
	Type      string  `json:"type"` // "message_delete"
	ID        string  `json:"id"`
	DeletedBy *string `json:"deleted_by,omitempty"`
	Seq       int64   `json:"seq,omitempty"`
	TS        int64   `json:"ts"`
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

// ThreadDeleteJSONLRecord represents a thread deletion tombstone.
type ThreadDeleteJSONLRecord struct {
	Type     string `json:"type"` // "thread_delete"
	ThreadID string `json:"thread_id"`
	Seq      int64  `json:"seq,omitempty"`
	TS       int64  `json:"ts"`
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
	Type        string `json:"type"`
	MessageGUID string `json:"message_guid"`
	ThreadGUID  string `json:"thread_guid"`
	UnpinnedBy  string `json:"unpinned_by"`
	UnpinnedAt  int64  `json:"unpinned_at"`
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
	Type       string `json:"type"`
	ThreadGUID string `json:"thread_guid"`
	UnpinnedBy string `json:"unpinned_by"`
	UnpinnedAt int64  `json:"unpinned_at"`
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

// FaveRemoveJSONLRecord represents a fave removal tombstone.
type FaveRemoveJSONLRecord struct {
	Type     string `json:"type"` // "fave_remove"
	AgentID  string `json:"agent_id"`
	ItemType string `json:"item_type"`
	ItemGUID string `json:"item_guid"`
	Seq      int64  `json:"seq,omitempty"`
	TS       int64  `json:"ts"`
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

// RoleReleaseJSONLRecord represents releasing a held role.
type RoleReleaseJSONLRecord struct {
	Type     string `json:"type"` // "role_release"
	AgentID  string `json:"agent_id"`
	RoleName string `json:"role_name"`
	Seq      int64  `json:"seq,omitempty"`
	TS       int64  `json:"ts"`
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

// WakeConditionJSONLRecord represents a wake condition in JSONL.
type WakeConditionJSONLRecord struct {
	Type           string   `json:"type"` // "wake_condition"
	GUID           string   `json:"guid"`
	AgentID        string   `json:"agent_id"`
	SetBy          string   `json:"set_by"`
	CondType       string   `json:"cond_type"` // on_mention, after, pattern, prompt
	Pattern        *string  `json:"pattern,omitempty"`
	OnAgents       []string `json:"on_agents,omitempty"`
	InThread       *string  `json:"in_thread,omitempty"`
	AfterMs        *int64   `json:"after_ms,omitempty"`
	UseRouter      bool     `json:"use_router,omitempty"`
	Prompt         *string  `json:"prompt,omitempty"`
	PromptText     *string  `json:"prompt_text,omitempty"`
	PollIntervalMs *int64   `json:"poll_interval_ms,omitempty"`
	PersistMode    string   `json:"persist_mode,omitempty"` // "", "persist", "persist_until_bye", "persist_restore_back"
	Paused         bool     `json:"paused,omitempty"`
	CreatedAt      int64    `json:"created_at"`
	ExpiresAt      *int64   `json:"expires_at,omitempty"`
}

// WakeConditionClearJSONLRecord represents clearing wake conditions.
type WakeConditionClearJSONLRecord struct {
	Type      string `json:"type"` // "wake_condition_clear"
	AgentID   string `json:"agent_id"`
	ClearedAt int64  `json:"cleared_at"`
}

// WakeConditionDeleteJSONLRecord represents deleting a specific wake condition.
type WakeConditionDeleteJSONLRecord struct {
	Type      string `json:"type"` // "wake_condition_delete"
	GUID      string `json:"guid"`
	DeletedAt int64  `json:"deleted_at"`
}

// WakeConditionPauseJSONLRecord represents pausing wake conditions (for restore-on-back).
type WakeConditionPauseJSONLRecord struct {
	Type     string `json:"type"` // "wake_condition_pause"
	AgentID  string `json:"agent_id"`
	PausedAt int64  `json:"paused_at"`
}

// WakeConditionResumeJSONLRecord represents resuming wake conditions.
type WakeConditionResumeJSONLRecord struct {
	Type      string `json:"type"` // "wake_condition_resume"
	AgentID   string `json:"agent_id"`
	ResumedAt int64  `json:"resumed_at"`
}

// WakeConditionClearByByeJSONLRecord represents clearing persist-until-bye conditions.
type WakeConditionClearByByeJSONLRecord struct {
	Type      string `json:"type"` // "wake_condition_clear_by_bye"
	AgentID   string `json:"agent_id"`
	ClearedAt int64  `json:"cleared_at"`
}

// WakeConditionResetJSONLRecord represents resetting a timer condition.
type WakeConditionResetJSONLRecord struct {
	Type      string `json:"type"` // "wake_condition_reset"
	GUID      string `json:"guid"`
	ExpiresAt int64  `json:"expires_at"`
	ResetAt   int64  `json:"reset_at"`
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
	Version        int                          `json:"version"`
	StorageVersion int                          `json:"storage_version,omitempty"`
	ChannelID      string                       `json:"channel_id,omitempty"`
	ChannelName    string                       `json:"channel_name,omitempty"`
	CreatedAt      string                       `json:"created_at,omitempty"`
	KnownAgents    map[string]ProjectKnownAgent `json:"known_agents,omitempty"`
	MachineAliases map[string]string            `json:"machine_aliases,omitempty"`
}

// JobCreateJSONLRecord represents a job creation event in JSONL.
type JobCreateJSONLRecord struct {
	Type       string            `json:"type"`
	GUID       string            `json:"guid"`
	Name       string            `json:"name"`
	Context    *types.JobContext `json:"context,omitempty"`
	OwnerAgent string            `json:"owner_agent"`
	Status     string            `json:"status"`
	ThreadGUID string            `json:"thread_guid"`
	CreatedAt  int64             `json:"created_at"`
}

// JobUpdateJSONLRecord represents a job status update event in JSONL.
type JobUpdateJSONLRecord struct {
	Type        string `json:"type"`
	GUID        string `json:"guid"`
	Status      string `json:"status"`
	CompletedAt *int64 `json:"completed_at,omitempty"`
}

// JobWorkerJoinJSONLRecord represents a worker joining a job.
type JobWorkerJoinJSONLRecord struct {
	Type     string `json:"type"`
	JobGUID  string `json:"job_guid"`
	AgentID  string `json:"agent_id"`
	WorkerID string `json:"worker_id"`
	JobIdx   int    `json:"job_idx"`
	JoinedAt int64  `json:"joined_at"`
}

// JobWorkerLeaveJSONLRecord represents a worker leaving a job.
type JobWorkerLeaveJSONLRecord struct {
	Type     string `json:"type"`
	JobGUID  string `json:"job_guid"`
	AgentID  string `json:"agent_id"`
	WorkerID string `json:"worker_id"`
	LeftAt   int64  `json:"left_at"`
}

// PermissionJSONLRecord represents a permission request entry in JSONL.
type PermissionJSONLRecord struct {
	Type        string                   `json:"type"`
	GUID        string                   `json:"guid"`
	FromAgent   string                   `json:"from_agent"`
	SessionID   string                   `json:"session_id,omitempty"`
	Tool        string                   `json:"tool"`
	Action      string                   `json:"action"`
	Rationale   string                   `json:"rationale"`
	Options     []types.PermissionOption `json:"options"`
	Status      string                   `json:"status"`
	ChosenIndex *int                     `json:"chosen_index,omitempty"`
	RespondedBy *string                  `json:"responded_by,omitempty"`
	CreatedAt   int64                    `json:"created_at"`
	RespondedAt *int64                   `json:"responded_at,omitempty"`
}

// PermissionUpdateJSONLRecord represents a permission response entry in JSONL.
type PermissionUpdateJSONLRecord struct {
	Type        string `json:"type"`
	GUID        string `json:"guid"`
	Status      string `json:"status"`
	ChosenIndex *int   `json:"chosen_index,omitempty"`
	RespondedBy string `json:"responded_by"`
	RespondedAt int64  `json:"responded_at"`
}

type machineIDFile struct {
	ID        string `json:"id"`
	Seq       int64  `json:"seq"`
	CreatedAt int64  `json:"created_at"`
}

// GetStorageVersion returns the storage_version from fray-config.json.
// Defaults to 1 when config is missing or storage_version is unset.
func GetStorageVersion(projectPath string) int {
	config, err := ReadProjectConfig(projectPath)
	if err != nil || config == nil || config.StorageVersion == 0 {
		if hasV2Sentinel(projectPath) {
			return 2
		}
		return 1
	}
	if config.StorageVersion < 2 && hasV2Sentinel(projectPath) {
		return 2
	}
	return config.StorageVersion
}

func hasV2Sentinel(projectPath string) bool {
	frayDir := resolveFrayDir(projectPath)
	_, err := os.Stat(filepath.Join(frayDir, "shared", ".v2"))
	return err == nil
}

func legacyWriteBlocked(projectPath string) (bool, []string, error) {
	if !hasV2Sentinel(projectPath) {
		return false, nil, nil
	}
	frayDir := resolveFrayDir(projectPath)
	legacyFiles := []string{messagesFile, threadsFile, questionsFile, agentsFile}
	var present []string
	for _, name := range legacyFiles {
		path := filepath.Join(frayDir, name)
		if _, err := os.Stat(path); err == nil {
			present = append(present, name)
		} else if !os.IsNotExist(err) {
			return false, nil, err
		}
	}
	if len(present) == 0 {
		return false, nil, nil
	}
	return true, present, nil
}

func ensureLegacyWriteAllowed(projectPath string) error {
	blocked, files, err := legacyWriteBlocked(projectPath)
	if err != nil {
		return err
	}
	if !blocked {
		return nil
	}
	return fmt.Errorf("legacy JSONL files detected in multi-machine project (%s). Remove legacy files or re-run `fray migrate --multi-machine`", strings.Join(files, ", "))
}

// IsMultiMachineMode reports whether storage_version >= 2.
func IsMultiMachineMode(projectPath string) bool {
	return GetStorageVersion(projectPath) >= 2
}

// GetLocalMachineID returns the ID from local/machine-id or empty string.
func GetLocalMachineID(projectPath string) string {
	frayDir := resolveFrayDir(projectPath)
	path := filepath.Join(frayDir, "local", "machine-id")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var record machineIDFile
	if err := json.Unmarshal(data, &record); err != nil {
		return ""
	}
	return record.ID
}

// GetSharedMachinesDirs returns paths to all shared machine directories.
func GetSharedMachinesDirs(projectPath string) []string {
	frayDir := resolveFrayDir(projectPath)
	machinesRoot := filepath.Join(frayDir, "shared", "machines")
	entries, err := os.ReadDir(machinesRoot)
	if err != nil {
		return nil
	}
	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(machinesRoot, entry.Name()))
		}
	}
	sort.Strings(dirs)
	return dirs
}

// MachineIDExists reports whether a shared machine directory exists for the ID.
func MachineIDExists(projectPath, machineID string) bool {
	if machineID == "" {
		return false
	}
	frayDir := resolveFrayDir(projectPath)
	path := filepath.Join(frayDir, "shared", "machines", machineID)
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// GetLocalMachineDir returns the shared directory for the local machine.
func GetLocalMachineDir(projectPath string) string {
	localID := GetLocalMachineID(projectPath)
	if localID == "" {
		return ""
	}
	frayDir := resolveFrayDir(projectPath)
	return filepath.Join(frayDir, "shared", "machines", localID)
}

// GetLocalRuntimePath returns the local runtime.jsonl path.
func GetLocalRuntimePath(projectPath string) string {
	frayDir := resolveFrayDir(projectPath)
	return filepath.Join(frayDir, "local", "runtime.jsonl")
}

// GetNextSequence returns the next per-machine sequence value.
func GetNextSequence(projectPath string) (int64, error) {
	frayDir := resolveFrayDir(projectPath)
	localDir := filepath.Join(frayDir, "local")
	if err := ensureDir(localDir); err != nil {
		return 0, err
	}

	path := filepath.Join(localDir, "machine-id")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return 0, err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	if _, err := file.Seek(0, 0); err != nil {
		return 0, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return 0, err
	}

	record, seqPresent, parsed := parseMachineIDFile(data)
	if !parsed {
		record.ID = extractMachineIDFromRaw(data)
	}
	if record.ID == "" {
		return 0, fmt.Errorf("machine-id missing id")
	}

	seq := record.Seq
	if !seqPresent {
		recovered, err := recoverSequenceFromJSONL(projectPath, record.ID)
		if err != nil {
			return 0, err
		}
		seq = recovered
	}

	next := seq + 1
	record.Seq = next
	if record.CreatedAt == 0 {
		record.CreatedAt = time.Now().Unix()
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		return 0, err
	}
	encoded = append(encoded, '\n')

	if _, err := file.Seek(0, 0); err != nil {
		return 0, err
	}
	if err := file.Truncate(0); err != nil {
		return 0, err
	}
	if _, err := file.Write(encoded); err != nil {
		return 0, err
	}
	if err := file.Sync(); err != nil {
		return 0, err
	}

	return next, nil
}

func parseMachineIDFile(data []byte) (machineIDFile, bool, bool) {
	var record machineIDFile
	if len(bytes.TrimSpace(data)) == 0 {
		return record, false, false
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return record, false, false
	}

	if rawID, ok := raw["id"]; ok {
		_ = json.Unmarshal(rawID, &record.ID)
	}
	if rawCreated, ok := raw["created_at"]; ok {
		_ = json.Unmarshal(rawCreated, &record.CreatedAt)
	}

	seqPresent := false
	if rawSeq, ok := raw["seq"]; ok {
		var seq int64
		if err := json.Unmarshal(rawSeq, &seq); err == nil {
			seqPresent = true
			record.Seq = seq
		}
	}

	return record, seqPresent, true
}

func extractMachineIDFromRaw(data []byte) string {
	raw := string(data)
	idx := strings.Index(raw, `"id"`)
	if idx == -1 {
		return ""
	}
	raw = raw[idx+len(`"id"`):]
	colon := strings.Index(raw, ":")
	if colon == -1 {
		return ""
	}
	raw = strings.TrimLeft(raw[colon+1:], " \t\r\n")
	if !strings.HasPrefix(raw, `"`) {
		return ""
	}
	raw = raw[1:]
	end := strings.Index(raw, `"`)
	if end == -1 {
		return ""
	}
	return raw[:end]
}

func recoverSequenceFromJSONL(projectPath, machineID string) (int64, error) {
	frayDir := resolveFrayDir(projectPath)
	var dirs []string
	if machineID != "" {
		dirs = []string{filepath.Join(frayDir, "shared", "machines", machineID)}
	} else {
		dirs = GetSharedMachinesDirs(projectPath)
	}

	var maxSeq int64
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			lines, err := readJSONLLines(filepath.Join(dir, entry.Name()))
			if err != nil {
				return 0, err
			}
			for _, line := range lines {
				var envelope struct {
					Seq *int64 `json:"seq"`
				}
				if err := json.Unmarshal([]byte(line), &envelope); err != nil {
					continue
				}
				if envelope.Seq != nil && *envelope.Seq > maxSeq {
					maxSeq = *envelope.Seq
				}
			}
		}
	}

	return maxSeq, nil
}
