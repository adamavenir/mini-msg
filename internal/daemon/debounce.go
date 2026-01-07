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

// CanTriggerSpawn returns true if the message author can trigger a spawn for the agent.
// Rules:
// - In room: only human (non-agent) can trigger
// - In thread with owner: human OR owner can trigger
// - In thread without owner (user-started): only human can trigger
func CanTriggerSpawn(msg types.Message, thread *types.Thread) bool {
	// Check if author is human (message type "user" vs "agent")
	isHuman := msg.Type == types.MessageTypeUser

	if isHuman {
		return true
	}

	// Author is an agent - only allowed if they own the thread
	if thread != nil && thread.OwnerAgent != nil && *thread.OwnerAgent == msg.FromAgent {
		return true
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
	case types.PresenceSpawning, types.PresenceActive:
		// Queue instead of spawning
		return false
	case types.PresenceError:
		// Could retry on error, but be conservative
		return false
	}

	return false
}
