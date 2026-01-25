package daemon

import (
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// isActiveByTokens checks if an agent is actively working based on token activity.
// This is used for spawn decisions: if tokens changed recently, the agent is active
// and @mentions should be queued rather than spawning a new session.
//
// Returns true if:
// - Agent has a session AND tokens changed within the idle threshold (5s default)
// - Agent is generating tokens right now (comparing current vs stored watermarks)
//
// This replaces the process-based check (hasProcess) which doesn't survive daemon restarts.
func (d *Daemon) isActiveByTokens(agent types.Agent) bool {
	// No session = not active
	if agent.LastSessionID == nil || *agent.LastSessionID == "" {
		return false
	}

	// Get idle threshold (default 5s)
	idleThresholdMs := int64(5000)
	if agent.Invoke != nil && agent.Invoke.IdleAfterMs > 0 {
		idleThresholdMs = agent.Invoke.IdleAfterMs
	}

	// Try to get current tokens via usage watcher or direct fetch
	var currentInput, currentOutput int64
	var hasCurrentTokens bool

	if d.usageWatcher != nil {
		if usageState := d.usageWatcher.GetSessionUsageSnapshot(*agent.LastSessionID); usageState != nil {
			currentInput = usageState.InputTokens
			currentOutput = usageState.OutputTokens
			hasCurrentTokens = true
		}
	}

	// If we have current tokens, compare against stored watermarks
	if hasCurrentTokens {
		// If tokens have changed (increased or mini-compaction happened), agent is active
		tokensChanged := currentInput != agent.LastKnownInput || currentOutput != agent.LastKnownOutput
		if tokensChanged {
			// Update watermarks for next check
			db.UpdateAgentTokenWatermarks(d.database, agent.AgentID, currentInput, currentOutput)
			return true
		}
	}

	// Fall back to time-based check using stored TokensUpdatedAt
	// If tokens were updated recently, consider agent active
	if agent.TokensUpdatedAt > 0 {
		timeSinceUpdate := time.Now().UnixMilli() - agent.TokensUpdatedAt
		if timeSinceUpdate < idleThresholdMs {
			return true
		}
	}

	return false
}

// cleanupStalePresence resets stale presence states on daemon startup.
// Agents may be stuck in active/spawning/prompting states from a previous
// daemon run that crashed or was killed. Since we have no tracked processes
// at startup, these states are orphaned and should be reset to idle.
func (d *Daemon) cleanupStalePresence() {
	agents, err := d.getManagedAgents()
	if err != nil {
		d.debugf("startup cleanup: error getting agents: %v", err)
		return
	}

	staleStates := []types.PresenceState{
		types.PresenceSpawning,
		types.PresencePrompting,
		types.PresencePrompted,
		types.PresenceCompacting,
		types.PresenceActive,
		// Note: PresenceIdle is NOT stale - it means session ended naturally but agent is resumable
	}

	for _, agent := range agents {
		isStale := false
		for _, stale := range staleStates {
			if agent.Presence == stale {
				isStale = true
				break
			}
		}

		if !isStale {
			continue
		}

		// Agent has a "busy" presence but daemon just started (no tracked processes)
		// Reset to offline so mentions can spawn fresh, and set LeftAt to indicate session ended
		d.debugf("startup cleanup: @%s was %s, resetting to offline", agent.AgentID, agent.Presence)
		if err := db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agent.AgentID, agent.Presence, types.PresenceOffline, "startup_cleanup", "startup", agent.Status); err != nil {
			d.debugf("startup cleanup: error updating @%s presence: %v", agent.AgentID, err)
		}
		// Also set LeftAt if not already set, to indicate the orphaned session ended
		if agent.LeftAt == nil {
			now := time.Now().Unix()
			if err := db.UpdateAgent(d.database, agent.AgentID, db.AgentUpdates{
				LeftAt: types.OptionalInt64{Set: true, Value: &now},
			}); err != nil {
				d.debugf("startup cleanup: error updating @%s left_at: %v", agent.AgentID, err)
			}
		}
	}
}
