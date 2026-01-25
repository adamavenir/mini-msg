package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/adamavenir/fray/internal/aap"
	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// spawnAgent starts a new session for an agent.
// Returns the last msgID included in the wake prompt (for watermark tracking).
func (d *Daemon) spawnAgent(ctx context.Context, agent types.Agent, triggerMsgID string) (string, error) {
	// Rate-limit spawns to prevent resource exhaustion when multiple agents
	// need to spawn simultaneously. This adds a small delay between spawns
	// to allow the OS to clean up file descriptors from previous spawns.
	const minSpawnInterval = 500 * time.Millisecond
	d.mu.Lock()
	elapsed := time.Since(d.lastSpawnTime)
	if elapsed < minSpawnInterval {
		delay := minSpawnInterval - elapsed
		d.mu.Unlock()
		d.debugf("  rate-limiting spawn for @%s, waiting %v", agent.AgentID, delay)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		d.mu.Lock()
	}
	d.lastSpawnTime = time.Now()
	d.mu.Unlock()

	if agent.Invoke == nil || agent.Invoke.Driver == "" {
		return "", fmt.Errorf("agent %s has no driver configured", agent.AgentID)
	}

	driver := d.drivers[agent.Invoke.Driver]
	if driver == nil {
		return "", fmt.Errorf("unknown driver: %s", agent.Invoke.Driver)
	}

	d.debugf("  spawning @%s with driver %s", agent.AgentID, agent.Invoke.Driver)

	// Check AAP identity for security logging
	if agent.AAPGUID == nil {
		d.debugf("  warning: agent @%s has no AAP identity", agent.AgentID)
	} else {
		// Optionally verify identity via resolver
		aapDir, err := core.AAPConfigDir()
		if err == nil {
			frayDir := filepath.Dir(d.project.DBPath)
			resolver, err := aap.NewResolver(aap.ResolverOpts{
				GlobalRegistry: aapDir,
				FrayCompat:     true,
				FrayPath:       frayDir,
			})
			if err == nil {
				res, err := resolver.Resolve("@" + agent.AgentID)
				if err != nil {
					d.debugf("  warning: failed to resolve AAP identity for @%s: %v", agent.AgentID, err)
				} else {
					d.debugf("  AAP identity verified: %s (trust: %s)", res.Identity.Record.GUID, res.TrustLevel)
				}
			}
		}
	}

	// Determine session mode: fork (#XXX), new (#n), or resumed (empty)
	// Check for fork spawn first: was this agent mentioned with @agent#sessid syntax?
	var sessionMode string
	var forkSessionID string
	triggerMsg, _ := db.GetMessage(d.database, triggerMsgID)
	if triggerMsg != nil && triggerMsg.ForkSessions != nil {
		if forkSessID, ok := triggerMsg.ForkSessions[agent.AgentID]; ok && forkSessID != "" {
			forkSessionID = forkSessID
			// Fork spawn: use first 3 chars of fork session ID
			if len(forkSessID) >= 3 {
				sessionMode = forkSessID[:3]
			} else {
				sessionMode = forkSessID
			}
			d.debugf("  fork spawn from session %s, mode=%s", forkSessID, sessionMode)
		}
	}

	// If agent was offline (from bye), clear session ID so driver starts fresh.
	// We keep LastSessionID in DB for token usage display, but clear it for spawning.
	// Exception: if a fork session has been specified via @agent#sessid, use that instead.
	isNewSession := false
	if forkSessionID != "" {
		// Fork spawn: use the explicitly specified session ID
		agent.LastSessionID = &forkSessionID
		d.debugf("  fork spawn: using session %s from mention", forkSessionID)
	} else if agent.Presence == types.PresenceOffline {
		d.debugf("  agent was offline, starting fresh session")
		agent.LastSessionID = nil // Clear locally for driver, DB retains for display
		isNewSession = true
	}

	// If not a fork and starting fresh, mark as new session
	if sessionMode == "" && isNewSession {
		sessionMode = "n"
		d.debugf("  new session, mode=n")
	} else if sessionMode == "" {
		d.debugf("  resuming existing session")
	}

	// Update presence to spawning
	prevPresence := agent.Presence
	if err := db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agent.AgentID, prevPresence, types.PresenceSpawning, "spawn", "daemon", agent.Status); err != nil {
		return "", err
	}

	// Clear LeftAt - spawn implies back (agent is rejoining)
	// This prevents "agent has left" errors if agent tries to post before running fray back
	if err := db.UpdateAgent(d.database, agent.AgentID, db.AgentUpdates{
		LeftAt: types.OptionalInt64{Set: true, Value: nil},
	}); err != nil {
		d.debugf("  error clearing left_at: %v", err)
	}

	// Build wake prompt and get all included mentions
	prompt, spawnMode, allMentions := d.buildWakePrompt(agent, triggerMsgID)
	d.debugf("  wake prompt includes %d mentions, mode=%s", len(allMentions), spawnMode)

	// Add trigger info to context for driver to use
	triggerInfo := TriggerInfo{MsgID: triggerMsgID}
	if triggerMsg != nil {
		triggerInfo.Home = triggerMsg.Home
	}
	spawnCtx := ContextWithTrigger(ctx, triggerInfo)

	// Spawn process
	proc, err := driver.Spawn(spawnCtx, agent, prompt)
	if err != nil {
		d.debugf("  spawn error: %v", err)
		db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agent.AgentID, types.PresenceSpawning, types.PresenceError, "spawn_error", "daemon", agent.Status)
		return "", err
	}

	// Track spawn mode for special handling (e.g., /hop auto-bye)
	proc.SpawnMode = spawnMode

	d.debugf("  spawned pid %d", proc.Cmd.Process.Pid)

	// Wait for session ID if driver captures it asynchronously (e.g., codex)
	// Give up to 5 seconds for the session ID to be captured from stdout
	if proc.SessionIDReady != nil {
		select {
		case <-proc.SessionIDReady:
			d.debugf("  session ID captured: %s", proc.SessionID)
		case <-time.After(5 * time.Second):
			d.debugf("  warning: session ID capture timed out")
		}
	}

	if proc.SessionID != "" {
		d.debugf("  session %s", proc.SessionID)
	}

	// Capture baseline token counts for resumed sessions
	// This lets us detect prompting/prompted by checking for token INCREASE from baseline
	if proc.SessionID != "" {
		if tokenState := GetTokenStateForDriver(agent.Invoke.Driver, proc.SessionID); tokenState != nil {
			proc.BaselineInput = tokenState.TotalInput
			proc.BaselineOutput = tokenState.TotalOutput
			d.debugf("  baseline tokens: input=%d, output=%d", proc.BaselineInput, proc.BaselineOutput)
		}
	}

	// Store session ID for future resume - this ensures each agent keeps their own session
	if proc.SessionID != "" {
		d.debugf("  storing session ID %s for @%s", proc.SessionID, agent.AgentID)
		if err := db.UpdateAgentSessionID(d.database, agent.AgentID, proc.SessionID); err != nil {
			d.debugf("  ERROR storing session ID: %v", err)
		}
		// Also persist to JSONL for durability across DB rebuilds
		db.AppendAgentUpdate(d.project.DBPath, db.AgentUpdateJSONLRecord{
			AgentID:       agent.AgentID,
			LastSessionID: &proc.SessionID,
		})
	}

	// Store session mode for display in activity panel (#n, #XXX, or empty for resume)
	db.UpdateAgentSessionMode(d.database, agent.AgentID, sessionMode)

	// Append session mode update to JSONL for persistence
	if sessionMode != "" {
		db.AppendAgentUpdate(d.project.DBPath, db.AgentUpdateJSONLRecord{
			Type:        "agent_update",
			AgentID:     agent.AgentID,
			SessionMode: &sessionMode,
		})
	}

	// Track process
	d.mu.Lock()
	d.processes[agent.AgentID] = proc
	delete(d.handled, agent.AgentID) // Clear handled flag for new session
	d.mu.Unlock()

	// Record session start
	sessionStart := types.SessionStart{
		AgentID:     agent.AgentID,
		SessionID:   proc.SessionID,
		TriggeredBy: &triggerMsgID,
		StartedAt:   time.Now().Unix(),
	}
	db.AppendSessionStart(d.project.DBPath, sessionStart)

	// Start watching session's transcript for usage updates
	if proc.SessionID != "" {
		d.watchSessionUsage(proc.SessionID, agent.Invoke.Driver)
	}

	// Initialize activity record
	if proc.Cmd.Process != nil {
		d.detector.RecordActivity(proc.Cmd.Process.Pid)
	}

	// Start goroutine to monitor process lifecycle
	d.wg.Add(1)
	go d.monitorProcess(agent.AgentID, proc)

	// Return the last mention included in the prompt
	lastMention := triggerMsgID
	if len(allMentions) > 0 {
		lastMention = allMentions[len(allMentions)-1]
	}
	return lastMention, nil
}

// spawnBRBAgent spawns a fresh session for an agent that requested BRB.
// Uses a continuation prompt instead of wake prompt since there are no trigger messages.
func (d *Daemon) spawnBRBAgent(ctx context.Context, agent types.Agent) error {
	// Rate-limit spawns (same as spawnAgent)
	const minSpawnInterval = 500 * time.Millisecond
	d.mu.Lock()
	elapsed := time.Since(d.lastSpawnTime)
	if elapsed < minSpawnInterval {
		delay := minSpawnInterval - elapsed
		d.mu.Unlock()
		d.debugf("  rate-limiting BRB spawn for @%s, waiting %v", agent.AgentID, delay)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
		d.mu.Lock()
	}
	d.lastSpawnTime = time.Now()
	d.mu.Unlock()

	if agent.Invoke == nil || agent.Invoke.Driver == "" {
		return fmt.Errorf("agent %s has no driver configured", agent.AgentID)
	}

	driver := d.drivers[agent.Invoke.Driver]
	if driver == nil {
		return fmt.Errorf("unknown driver: %s", agent.Invoke.Driver)
	}

	d.debugf("  spawning @%s with driver %s (BRB continuation)", agent.AgentID, agent.Invoke.Driver)

	// Update presence to spawning
	prevPresence := agent.Presence
	if err := db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agent.AgentID, prevPresence, types.PresenceSpawning, "brb_spawn", "daemon", agent.Status); err != nil {
		return err
	}

	// Build continuation prompt (no trigger messages)
	prompt := d.buildContinuationPrompt(agent)

	// Spawn process
	proc, err := driver.Spawn(ctx, agent, prompt)
	if err != nil {
		d.debugf("  BRB spawn error: %v", err)
		db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agent.AgentID, types.PresenceSpawning, types.PresenceError, "brb_spawn_error", "daemon", agent.Status)
		return err
	}

	d.debugf("  spawned pid %d (BRB)", proc.Cmd.Process.Pid)

	// Wait for session ID if driver captures it asynchronously
	if proc.SessionIDReady != nil {
		select {
		case <-proc.SessionIDReady:
			d.debugf("  session ID captured: %s", proc.SessionID)
		case <-time.After(5 * time.Second):
			d.debugf("  warning: session ID capture timed out")
		}
	}

	if proc.SessionID != "" {
		d.debugf("  session %s", proc.SessionID)
	}

	// Store session ID for future resume
	if proc.SessionID != "" {
		db.UpdateAgentSessionID(d.database, agent.AgentID, proc.SessionID)
		// Also persist to JSONL for durability across DB rebuilds
		db.AppendAgentUpdate(d.project.DBPath, db.AgentUpdateJSONLRecord{
			AgentID:       agent.AgentID,
			LastSessionID: &proc.SessionID,
		})
	}

	// Track process
	d.mu.Lock()
	d.processes[agent.AgentID] = proc
	delete(d.handled, agent.AgentID)
	d.mu.Unlock()

	// Record session start (no trigger message for BRB)
	sessionStart := types.SessionStart{
		AgentID:   agent.AgentID,
		SessionID: proc.SessionID,
		StartedAt: time.Now().Unix(),
		// TriggeredBy is nil for BRB spawns
	}
	db.AppendSessionStart(d.project.DBPath, sessionStart)

	// Start watching session's transcript for usage updates
	if proc.SessionID != "" {
		d.watchSessionUsage(proc.SessionID, agent.Invoke.Driver)
	}

	// Initialize activity record
	if proc.Cmd.Process != nil {
		d.detector.RecordActivity(proc.Cmd.Process.Pid)
	}

	// Start goroutine to monitor process lifecycle
	d.wg.Add(1)
	go d.monitorProcess(agent.AgentID, proc)

	return nil
}

// buildContinuationPrompt creates the prompt for BRB respawns.
// Unlike wake prompt, there are no trigger messages - agent is continuing from prior session.
func (d *Daemon) buildContinuationPrompt(agent types.Agent) string {
	return fmt.Sprintf(`**You are @%s.** Continuing from previous session.

Your last session ended with 'fray brb'. Pick up where you left off.

Run: /fly %s
Run: fray get %s/notes

Check your notes for context from the previous session.`,
		agent.AgentID, agent.AgentID, agent.AgentID)
}
