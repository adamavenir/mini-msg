package daemon

import (
	"database/sql"
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
