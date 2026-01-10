package daemon

import (
	"database/sql"
	"strings"
	"sync"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// MentionDebouncer tracks mention watermarks and pending mentions per agent.
type MentionDebouncer struct {
	mu          sync.RWMutex
	pending     map[string][]string // agent_id -> []msg_id
	database    *sql.DB
	projectPath string
}

// NewMentionDebouncer creates a debouncer backed by the given database.
func NewMentionDebouncer(database *sql.DB, projectPath string) *MentionDebouncer {
	return &MentionDebouncer{
		pending:     make(map[string][]string),
		database:    database,
		projectPath: projectPath,
	}
}

// GetWatermark returns the last processed msg_id for an agent.
// Returns empty string if no watermark exists.
func (d *MentionDebouncer) GetWatermark(agentID string) string {
	agent, err := db.GetAgent(d.database, agentID)
	if err != nil || agent == nil {
		return ""
	}

	if agent.MentionWatermark != nil {
		return *agent.MentionWatermark
	}
	return ""
}

// GetReactionWatermark returns the last processed reaction timestamp (ms) for an agent.
// Returns 0 if no watermark exists.
func (d *MentionDebouncer) GetReactionWatermark(agentID string) int64 {
	agent, err := db.GetAgent(d.database, agentID)
	if err != nil || agent == nil {
		return 0
	}
	if agent.ReactionWatermark != nil {
		return *agent.ReactionWatermark
	}
	return 0
}

// UpdateReactionWatermark persists a new reaction watermark for an agent.
func (d *MentionDebouncer) UpdateReactionWatermark(agentID string, timestampMs int64) error {
	return db.UpdateAgentReactionWatermark(d.database, agentID, timestampMs)
}

// UpdateWatermark persists a new watermark for an agent.
// Updates both SQLite and JSONL.
func (d *MentionDebouncer) UpdateWatermark(agentID, msgID string) error {
	// Update SQLite
	if err := db.UpdateAgentWatermark(d.database, agentID, msgID); err != nil {
		return err
	}

	// Persist to JSONL
	return db.AppendAgentUpdate(d.projectPath, db.AgentUpdateJSONLRecord{
		AgentID:          agentID,
		MentionWatermark: &msgID,
	})
}

// QueueMention adds a pending mention for an agent.
// Used when agent is spawning/active and shouldn't be interrupted.
// Deduplicates to prevent same mention being queued multiple times.
func (d *MentionDebouncer) QueueMention(agentID, msgID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check for duplicate
	for _, id := range d.pending[agentID] {
		if id == msgID {
			return
		}
	}

	d.pending[agentID] = append(d.pending[agentID], msgID)
}

// FlushPending returns and clears all pending mentions for an agent.
// Call this when spawning a new session to include all queued mentions.
func (d *MentionDebouncer) FlushPending(agentID string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	pending := d.pending[agentID]
	delete(d.pending, agentID)
	return pending
}

// HasPending returns true if the agent has pending mentions.
func (d *MentionDebouncer) HasPending(agentID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return len(d.pending[agentID]) > 0
}

// PendingCount returns the number of pending mentions for an agent.
func (d *MentionDebouncer) PendingCount(agentID string) int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return len(d.pending[agentID])
}

// IsSelfMention returns true if the message is from the given agent.
func IsSelfMention(msg types.Message, agentID string) bool {
	return msg.FromAgent == agentID
}

// IsDirectAddress returns true if the agent is mentioned at the start of the message.
// Direct address means the agent is being spoken TO, not just mentioned.
// Examples:
//   - "@alice @bob hey" → alice and bob are direct (mentioned before content)
//   - "hey @alice" → alice is NOT direct (mentioned mid-sentence)
//   - "cc @alice" → alice is NOT direct (CC pattern = FYI)
//   - "FYI @alice" → alice is NOT direct (FYI pattern)
func IsDirectAddress(msg types.Message, agentID string) bool {
	body := strings.TrimSpace(msg.Body)
	bodyLower := strings.ToLower(body)

	// Check for FYI patterns at start - these are never direct
	fyiPrefixes := []string{"fyi ", "fyi:", "cc ", "cc:", "heads up ", "just so you know "}
	for _, prefix := range fyiPrefixes {
		if strings.HasPrefix(bodyLower, prefix) {
			return false
		}
	}

	// Message must start with @ to be direct address
	if !strings.HasPrefix(body, "@") {
		return false
	}

	// Find where the @-block ends (first non-@ word)
	// The @-block is contiguous mentions at the start: "@a @b @c hey" → block is "@a @b @c"
	words := strings.Fields(body)
	for _, word := range words {
		if !strings.HasPrefix(word, "@") {
			// Hit first non-mention word, stop checking
			break
		}
		// Check if this mention matches the agent (with prefix matching via ".")
		mention := strings.TrimPrefix(word, "@")
		mention = strings.TrimRight(mention, ".,;:!?") // Strip trailing punctuation
		if matchesMention(mention, agentID) {
			return true
		}
	}

	return false
}

// matchesMention returns true if the mention matches the agent.
// Uses prefix matching with "." as separator: "@alice" matches "alice", "alice.1", "alice.frontend"
func matchesMention(mention, agentID string) bool {
	if mention == agentID {
		return true
	}
	// Prefix match: mention "alice" matches agentID "alice.1"
	if strings.HasPrefix(agentID, mention+".") {
		return true
	}
	// Exact match on prefix: agentID "alice" matches mention "alice.1" (agent gets mentions for sub-agents)
	if strings.HasPrefix(mention, agentID+".") {
		return true
	}
	return false
}

// IsFYIPattern returns true if the message starts with an FYI/CC pattern.
// These are informational mentions that don't require immediate attention.
func IsFYIPattern(msg types.Message) bool {
	bodyLower := strings.ToLower(strings.TrimSpace(msg.Body))
	fyiPrefixes := []string{"fyi ", "fyi:", "cc ", "cc:", "heads up ", "just so you know "}
	for _, prefix := range fyiPrefixes {
		if strings.HasPrefix(bodyLower, prefix) {
			return true
		}
	}
	return false
}

// CanTriggerSpawn returns true if the message author can trigger a spawn for the agent.
// Rules:
// - Human (non-agent) can always trigger
// - Agent with "wake" trust can trigger
// - Thread owner can trigger in their own thread
func CanTriggerSpawn(database *sql.DB, msg types.Message, thread *types.Thread) bool {
	// Check if author is human (message type "user" vs "agent")
	isHuman := msg.Type == types.MessageTypeUser

	if isHuman {
		return true
	}

	// Author is an agent - check if they have "wake" trust
	if HasWakeTrust(database, msg.FromAgent) {
		return true
	}

	// Author is an agent - allowed if they own the thread
	if thread != nil && thread.OwnerAgent != nil && *thread.OwnerAgent == msg.FromAgent {
		return true
	}

	return false
}

// HasWakeTrust returns true if the agent has "wake" trust permission.
// Agents with wake trust can trigger spawns for other agents.
func HasWakeTrust(database *sql.DB, agentID string) bool {
	if database == nil {
		return false
	}

	agent, err := db.GetAgent(database, agentID)
	if err != nil || agent == nil || agent.Invoke == nil {
		return false
	}

	for _, t := range agent.Invoke.Trust {
		if t == "wake" {
			return true
		}
	}
	return false
}

// IsAllMentionOnly returns true if the agent is mentioned only via @all expansion.
// This means the message body contains @all but not a direct @agentid mention.
// Used to suppress daemon spawns for @all (ambient notification, not action request).
func IsAllMentionOnly(msg types.Message, agentID string) bool {
	// Check if @all is in mentions
	hasAll := false
	for _, m := range msg.Mentions {
		if m == "all" {
			hasAll = true
			break
		}
	}
	if !hasAll {
		return false
	}

	// Check if this agent is directly mentioned (not via @all)
	// Look for @agentid in the message body
	body := msg.Body
	// Direct mention would be @agentid or @agentid.suffix
	directPattern := "@" + agentID
	if strings.Contains(body, directPattern) {
		// Could be @agentid or @agentid.something - check for exact match or prefix
		// e.g., "@opus" should match "@opus" or "@opus.1" but not "@opusX"
		idx := strings.Index(body, directPattern)
		if idx >= 0 {
			end := idx + len(directPattern)
			if end >= len(body) {
				// @agentid at end of body - direct mention
				return false
			}
			nextChar := body[end]
			// Valid continuation chars for mentions: ., space, punctuation
			if nextChar == '.' || nextChar == ' ' || nextChar == ',' ||
				nextChar == ':' || nextChar == '!' || nextChar == '?' ||
				nextChar == '\n' || nextChar == '\t' {
				return false
			}
		}
	}

	// Agent is only in mentions because of @all expansion
	return true
}

// IsReplyToAgent returns true if the message is a reply to a message from the given agent.
// This requires database access to look up the parent message.
func IsReplyToAgent(database *sql.DB, msg types.Message, agentID string) bool {
	if msg.ReplyTo == nil || *msg.ReplyTo == "" {
		return false
	}

	// Look up the parent message
	parent, err := db.GetMessage(database, *msg.ReplyTo)
	if err != nil || parent == nil {
		return false
	}

	// Check if parent was from this agent (with prefix matching)
	if parent.FromAgent == agentID {
		return true
	}
	// Prefix match: agentID "alice" gets replies to "alice.1"
	if strings.HasPrefix(parent.FromAgent, agentID+".") {
		return true
	}
	return false
}

// AgentAlreadyReplied returns true if the agent has already posted a reply to the given message.
// This is used to avoid re-spawning for messages the agent already responded to.
func AgentAlreadyReplied(database *sql.DB, msgID, agentID string) bool {
	replies, err := db.GetReplies(database, msgID)
	if err != nil {
		return false
	}

	for _, reply := range replies {
		if reply.FromAgent == agentID {
			return true
		}
		// Prefix match: "alice" matches "alice.1"
		if strings.HasPrefix(reply.FromAgent, agentID+".") {
			return true
		}
	}
	return false
}

// ShouldSpawn determines if a mention should trigger a spawn.
// Returns false if:
// - Message is a self-mention
// - Agent is currently spawning/active (mention should be queued instead)
// Note: Watermark filtering is done by the caller via GetMessagesWithMention.
func (d *MentionDebouncer) ShouldSpawn(agent types.Agent, msg types.Message) bool {
	// Never spawn on self-mention
	if IsSelfMention(msg, agent.AgentID) {
		return false
	}

	// Check presence state - only spawn if offline or idle
	switch agent.Presence {
	case types.PresenceOffline, types.PresenceIdle, "":
		return true
	case types.PresenceSpawning, types.PresencePrompting, types.PresencePrompted, types.PresenceActive:
		// Queue instead of spawning - agent is already running
		return false
	case types.PresenceError:
		// Could retry on error, but be conservative
		return false
	}

	return false
}
