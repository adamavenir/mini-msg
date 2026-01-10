package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/router"
	"github.com/adamavenir/fray/internal/types"
)

// Daemon watches for @mentions and spawns managed agents.
type Daemon struct {
	mu           sync.RWMutex
	project      core.Project
	database     *sql.DB
	debouncer    *MentionDebouncer
	detector     ActivityDetector
	router       *router.Router      // mlld-based routing for mention interpretation
	processes    map[string]*Process // agent_id -> process
	spawning     map[string]bool     // agent_id -> true if spawn in progress (prevents races)
	handled      map[string]bool     // agent_id -> true if exit already handled
	drivers      map[string]Driver   // driver name -> driver
	stopCh       chan struct{}
	cancelFunc   context.CancelFunc // cancels spawned process contexts
	wg           sync.WaitGroup
	lockPath     string
	pollInterval time.Duration
	debug        bool
}

// LockInfo represents the daemon lock file contents.
type LockInfo struct {
	PID       int   `json:"pid"`
	StartedAt int64 `json:"started_at"`
}

// Config holds daemon configuration options.
type Config struct {
	PollInterval time.Duration
	Debug        bool
}

// DefaultConfig returns default daemon configuration.
func DefaultConfig() Config {
	return Config{
		PollInterval: 1 * time.Second,
	}
}

// New creates a new daemon for the given project.
func New(project core.Project, database *sql.DB, cfg Config) *Daemon {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultConfig().PollInterval
	}

	frayDir := filepath.Dir(project.DBPath)
	d := &Daemon{
		project:      project,
		database:     database,
		debouncer:    NewMentionDebouncer(database, project.DBPath),
		detector:     NewActivityDetector(),
		router:       router.New(frayDir),
		processes:    make(map[string]*Process),
		spawning:     make(map[string]bool),
		handled:      make(map[string]bool),
		drivers:      make(map[string]Driver),
		stopCh:       make(chan struct{}),
		lockPath:     filepath.Join(frayDir, "daemon.lock"),
		pollInterval: cfg.PollInterval,
		debug:        cfg.Debug,
	}

	// Register drivers
	d.drivers["claude"] = &ClaudeDriver{}
	d.drivers["codex"] = &CodexDriver{}
	d.drivers["opencode"] = &OpencodeDriver{}

	return d
}

// Start begins the daemon watch loop.
func (d *Daemon) Start(ctx context.Context) error {
	// Acquire lock
	if err := d.acquireLock(); err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}

	// Clean up stale presence states from previous daemon runs
	d.cleanupStalePresence()

	// Create cancellable context for spawned processes
	procCtx, cancel := context.WithCancel(ctx)
	d.cancelFunc = cancel

	d.wg.Add(1)
	go d.watchLoop(procCtx)

	return nil
}

// Stop gracefully shuts down the daemon.
func (d *Daemon) Stop() error {
	// Signal watch loop to stop
	close(d.stopCh)

	// Cancel process contexts - this kills spawned processes via CommandContext,
	// allowing monitorProcess goroutines to exit
	if d.cancelFunc != nil {
		d.cancelFunc()
	}

	// Wait for all goroutines (watchLoop and monitorProcess) to finish
	d.wg.Wait()

	// Cleanup any remaining resources
	d.mu.Lock()
	for agentID, proc := range d.processes {
		driver := d.getDriver(agentID)
		if driver != nil {
			driver.Cleanup(proc)
		}
		if proc.Cmd.Process != nil {
			d.detector.Cleanup(proc.Cmd.Process.Pid)
		}
	}
	d.processes = make(map[string]*Process)
	d.spawning = make(map[string]bool)
	d.handled = make(map[string]bool)
	d.mu.Unlock()

	// Release lock
	return d.releaseLock()
}

// acquireLock creates the lock file, detecting stale locks.
func (d *Daemon) acquireLock() error {
	// Check for existing lock
	if data, err := os.ReadFile(d.lockPath); err == nil {
		var info LockInfo
		if json.Unmarshal(data, &info) == nil {
			// Check if process is still running using syscall.Kill with signal 0
			if syscall.Kill(info.PID, 0) == nil {
				return fmt.Errorf("daemon already running (pid %d)", info.PID)
			}
			// Stale lock, remove it
		}
	}

	// Write new lock
	info := LockInfo{
		PID:       os.Getpid(),
		StartedAt: time.Now().Unix(),
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(d.lockPath, data, 0600)
}

// releaseLock removes the lock file.
func (d *Daemon) releaseLock() error {
	return os.Remove(d.lockPath)
}

// debugf logs a debug message if debug mode is enabled.
func (d *Daemon) debugf(format string, args ...any) {
	if d.debug {
		fmt.Fprintf(os.Stderr, "[daemon] "+format+"\n", args...)
	}
}

// truncate shortens a string for debug output.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// isSchemaError checks if an error is a SQLite schema mismatch.
func isSchemaError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such column") ||
		strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "has no column")
}

// IsLocked returns true if a daemon is currently running.
func IsLocked(frayDir string) bool {
	lockPath := filepath.Join(frayDir, "daemon.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}

	var info LockInfo
	if json.Unmarshal(data, &info) != nil {
		return false
	}

	// Use syscall.Kill with signal 0 to check if process exists
	// This is more reliable than proc.Signal(nil) on macOS
	return syscall.Kill(info.PID, 0) == nil
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
		types.PresenceActive,
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
		// Reset to idle so mentions can spawn fresh, and set LeftAt to indicate session ended
		d.debugf("startup cleanup: @%s was %s, resetting to idle", agent.AgentID, agent.Presence)
		if err := db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agent.AgentID, agent.Presence, types.PresenceIdle, "startup_cleanup", "startup", agent.Status); err != nil {
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

// watchLoop is the main daemon loop.
func (d *Daemon) watchLoop(ctx context.Context) {
	defer d.wg.Done()

	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.poll(ctx)
		}
	}
}

// poll checks for new mentions and updates process states.
func (d *Daemon) poll(ctx context.Context) {
	// Get managed agents
	agents, err := d.getManagedAgents()
	if err != nil {
		d.debugf("poll: error getting managed agents: %v", err)
		// Check for schema errors
		if isSchemaError(err) {
			fmt.Fprintf(os.Stderr, "Error: database schema mismatch. Run 'fray rebuild' to fix.\n")
			fmt.Fprintf(os.Stderr, "Details: %v\n", err)
			// Signal stop - can't continue with schema errors
			close(d.stopCh)
		}
		return
	}

	if len(agents) == 0 {
		d.debugf("poll: no managed agents found")
		return
	}

	d.debugf("poll: checking %d managed agents", len(agents))

	// Check for new mentions and reactions for each managed agent
	for _, agent := range agents {
		d.checkMentions(ctx, agent)
		d.checkReactions(ctx, agent)
	}

	// Update presence for running processes
	d.updatePresence()
}

// getManagedAgents returns all agents with managed=true.
func (d *Daemon) getManagedAgents() ([]types.Agent, error) {
	allAgents, err := db.GetAllAgents(d.database)
	if err != nil {
		return nil, err
	}

	var managed []types.Agent
	for _, agent := range allAgents {
		if agent.Managed {
			managed = append(managed, agent)
		}
	}
	return managed, nil
}

// checkMentions looks for new @mentions of an agent.
func (d *Daemon) checkMentions(ctx context.Context, agent types.Agent) {
	// Get messages mentioning this agent since watermark
	watermark := d.debouncer.GetWatermark(agent.AgentID)
	messages, err := d.getMessagesAfter(watermark, agent.AgentID)
	if err != nil {
		d.debugf("  @%s: error getting messages: %v", agent.AgentID, err)
		return
	}
	if len(messages) == 0 {
		return
	}

	d.debugf("  @%s: found %d messages since watermark %s (presence: %s)", agent.AgentID, len(messages), watermark, agent.Presence)

	spawned := false
	hasQueued := false
	var lastProcessedID string

	for _, msg := range messages {
		// Skip self-mentions - only advance watermark if we haven't queued anything.
		// Once we queue, we can't advance past queued messages (they'd be lost on restart).
		if IsSelfMention(msg, agent.AgentID) {
			d.debugf("    %s: skip (self-mention)", msg.ID)
			if !hasQueued && !spawned {
				lastProcessedID = msg.ID
			}
			continue
		}

		// Check if this is a direct address OR a reply to the agent's message
		// Direct address: @agent at start of message
		// Reply to agent: threaded reply to something the agent wrote
		isDirectAddress := IsDirectAddress(msg, agent.AgentID)
		isReplyToAgent := IsReplyToAgent(d.database, msg, agent.AgentID)

		// FYI patterns (fyi @agent, cc @agent, etc) are informational - skip entirely
		if IsFYIPattern(msg) {
			d.debugf("    %s: skip (FYI pattern)", msg.ID)
			if !hasQueued && !spawned {
				lastProcessedID = msg.ID
			}
			continue
		}

		// Ambiguous mention: not direct address and not reply to agent
		// Route through Haiku to decide if agent should be woken
		isAmbiguousMention := !isDirectAddress && !isReplyToAgent
		if isAmbiguousMention {
			var threadHome *string
			if msg.Home != "" && msg.Home != "room" {
				threadHome = &msg.Home
			}
			routerResult := d.router.Route(router.RouterPayload{
				Message: msg.Body,
				From:    msg.FromAgent,
				Agent:   agent.AgentID,
				Thread:  threadHome,
			})
			d.debugf("    %s: ambiguous mention - router says shouldSpawn=%v (confidence=%.2f)",
				msg.ID, routerResult.ShouldSpawn, routerResult.Confidence)

			if !routerResult.ShouldSpawn {
				d.debugf("    %s: skip (router: wait)", msg.ID)
				if !hasQueued && !spawned {
					lastProcessedID = msg.ID
				}
				continue
			}
		}

		// Check who can trigger spawn: human, agent with wake trust, or thread owner
		var thread *types.Thread
		if msg.Home != "" && msg.Home != "room" {
			thread, _ = db.GetThread(d.database, msg.Home)
		}
		if !CanTriggerSpawn(d.database, msg, thread) {
			isHuman := msg.Type == types.MessageTypeUser
			d.debugf("    %s: skip (ownership check failed) - from: %s, type: %s, isHuman: %v", msg.ID, msg.FromAgent, msg.Type, isHuman)
			if !hasQueued && !spawned {
				lastProcessedID = msg.ID
			}
			continue
		}

		// Re-fetch agent's current presence from DB to get fresh state.
		// This ensures we see presence updates from external commands (e.g., fray back)
		// that may have run since we fetched the agent at the start of poll().
		currentAgent, err := db.GetAgent(d.database, agent.AgentID)
		if err != nil {
			d.debugf("    %s: error re-fetching agent: %v", msg.ID, err)
			continue
		}
		if currentAgent != nil {
			agent.Presence = currentAgent.Presence
		}

		// If we already spawned this poll, queue the mention
		// Note: Don't advance watermark for queued messages - pending is in-memory,
		// so on restart we need to re-query and re-queue them
		if spawned {
			d.debugf("    %s: queued (already spawned this poll)", msg.ID)
			d.debouncer.QueueMention(agent.AgentID, msg.ID)
			hasQueued = true
			continue
		}

		// Check if agent has a tracked process OR a spawn in progress
		// Key invariant: if we have a process or spawn lock, queue - never spawn duplicates
		d.mu.RLock()
		_, hasProcess := d.processes[agent.AgentID]
		isSpawning := d.spawning[agent.AgentID]
		d.mu.RUnlock()

		if isSpawning {
			// Spawn already in progress - queue to avoid race condition
			d.debugf("    %s: queued (spawn in progress)", msg.ID)
			d.debouncer.QueueMention(agent.AgentID, msg.ID)
			hasQueued = true
			continue
		}

		if hasProcess {
			// Agent has running process - queue regardless of presence state
			// (presence may be 'idle' if stdout went quiet, but process is still running)
			d.debugf("    %s: queued (process running)", msg.ID)
			d.debouncer.QueueMention(agent.AgentID, msg.ID)
			hasQueued = true
			continue
		}

		// No tracked process - check if presence indicates orphaned state
		isBusyState := agent.Presence == types.PresenceSpawning ||
			agent.Presence == types.PresencePrompting ||
			agent.Presence == types.PresencePrompted ||
			agent.Presence == types.PresenceActive

		if isBusyState {
			// Orphaned state: presence says busy but no tracked process
			// Reset to idle and proceed to spawn
			d.debugf("    %s: orphaned presence detected (was %s), resetting to idle", msg.ID, agent.Presence)
			db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agent.AgentID, agent.Presence, types.PresenceIdle, "orphaned_reset", "daemon", agent.Status)
			agent.Presence = types.PresenceIdle
		}

		// Error state blocks auto-spawn - requires manual recovery via "fray back"
		if agent.Presence == types.PresenceError {
			d.debugf("    %s: skip (agent in error state, needs manual recovery)", msg.ID)
			// Advance watermark to avoid retrying the same message
			if !hasQueued {
				lastProcessedID = msg.ID
			}
			continue
		}

		// Direct addresses, chained replies, and router-approved ambiguous mentions spawn
		if isAmbiguousMention {
			d.debugf("    %s: ambiguous mention approved by router - triggering spawn", msg.ID)
		} else {
			d.debugf("    %s: direct=%v reply=%v - triggering spawn", msg.ID, isDirectAddress, isReplyToAgent)
		}

		// Set spawn lock BEFORE calling spawnAgent to prevent race with next poll
		d.mu.Lock()
		d.spawning[agent.AgentID] = true
		d.mu.Unlock()

		// Try to spawn - spawnAgent returns the last msgID included in wake prompt
		lastIncluded, err := d.spawnAgent(ctx, agent, msg.ID)

		// Clear spawn lock (process is now tracked in d.processes if successful)
		d.mu.Lock()
		delete(d.spawning, agent.AgentID)
		d.mu.Unlock()

		if err != nil {
			d.debugf("    %s: spawn failed: %v", msg.ID, err)
			continue
		}

		// Spawn succeeded - advance watermark past all messages in wake prompt
		lastProcessedID = lastIncluded

		// Mark as spawned to queue remaining mentions
		spawned = true
		agent.Presence = types.PresenceSpawning // Update local copy
	}

	// Update watermark to last fully processed message
	if lastProcessedID != "" {
		d.debugf("  @%s: updating watermark to %s", agent.AgentID, lastProcessedID)
		if err := d.debouncer.UpdateWatermark(agent.AgentID, lastProcessedID); err != nil {
			d.debugf("  @%s: watermark update failed: %v", agent.AgentID, err)
		}
	}
}

// checkReactions looks for new reactions to an agent's messages.
// Reactions are treated like mentions - they can trigger spawns.
func (d *Daemon) checkReactions(ctx context.Context, agent types.Agent) {
	watermark := d.debouncer.GetReactionWatermark(agent.AgentID)
	reactions, err := db.GetReactionsToAgentSince(d.database, agent.AgentID, watermark)
	if err != nil {
		d.debugf("  @%s: error getting reactions: %v", agent.AgentID, err)
		return
	}
	if len(reactions) == 0 {
		return
	}

	d.debugf("  @%s: found %d reactions since watermark %d", agent.AgentID, len(reactions), watermark)

	// Check if agent has a tracked process OR spawn in progress - same invariant as checkMentions
	d.mu.RLock()
	_, hasProcess := d.processes[agent.AgentID]
	isSpawning := d.spawning[agent.AgentID]
	d.mu.RUnlock()

	var lastProcessedAt int64

	for _, reaction := range reactions {
		d.debugf("    reaction from %s: %s on %s", reaction.ReactedBy, reaction.Emoji, reaction.MessageGUID)

		// If spawn is in progress, skip - prevents race condition
		if isSpawning {
			d.debugf("    @%s: spawn in progress, skipping", agent.AgentID)
			lastProcessedAt = reaction.ReactedAt
			continue
		}

		// If we have a running process, don't spawn - just update watermark
		if hasProcess {
			d.debugf("    @%s: process running, skipping spawn", agent.AgentID)
			lastProcessedAt = reaction.ReactedAt
			continue
		}

		// No tracked process - check presence to decide whether to spawn
		switch agent.Presence {
		case types.PresenceOffline, types.PresenceIdle, "":
			// Can spawn - trigger on reaction
			d.debugf("    @%s: spawning on reaction from %s", agent.AgentID, reaction.ReactedBy)

			// Update watermark BEFORE spawning to prevent race with next poll cycle
			if err := d.debouncer.UpdateReactionWatermark(agent.AgentID, reaction.ReactedAt); err != nil {
				d.debugf("    @%s: reaction watermark update failed: %v", agent.AgentID, err)
			}

			// Set spawn lock BEFORE calling spawnAgent to prevent race
			d.mu.Lock()
			d.spawning[agent.AgentID] = true
			d.mu.Unlock()

			// Spawn the agent with the reacted message as trigger
			_, err := d.spawnAgent(ctx, agent, reaction.MessageGUID)

			// Clear spawn lock
			d.mu.Lock()
			delete(d.spawning, agent.AgentID)
			d.mu.Unlock()

			if err != nil {
				d.debugf("    @%s: spawn error: %v", agent.AgentID, err)
			}
			// Only spawn once per poll cycle
			return

		case types.PresenceSpawning, types.PresencePrompting, types.PresencePrompted, types.PresenceActive:
			// Orphaned state: presence busy but no process (we checked above)
			// Reset and spawn
			d.debugf("    @%s: orphaned presence (%s), resetting to idle", agent.AgentID, agent.Presence)
			db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agent.AgentID, agent.Presence, types.PresenceIdle, "orphaned_reset", "daemon", agent.Status)
			agent.Presence = types.PresenceIdle

			// Update watermark BEFORE spawning
			if err := d.debouncer.UpdateReactionWatermark(agent.AgentID, reaction.ReactedAt); err != nil {
				d.debugf("    @%s: reaction watermark update failed: %v", agent.AgentID, err)
			}

			// Set spawn lock BEFORE calling spawnAgent to prevent race
			d.mu.Lock()
			d.spawning[agent.AgentID] = true
			d.mu.Unlock()

			// Now spawn
			_, err := d.spawnAgent(ctx, agent, reaction.MessageGUID)

			// Clear spawn lock
			d.mu.Lock()
			delete(d.spawning, agent.AgentID)
			d.mu.Unlock()

			if err != nil {
				d.debugf("    @%s: spawn error: %v", agent.AgentID, err)
			}
			return

		default:
			d.debugf("    @%s: skipping (presence: %s)", agent.AgentID, agent.Presence)
			lastProcessedAt = reaction.ReactedAt
		}
	}

	// Update watermark for non-spawn cases (skipped reactions)
	if lastProcessedAt > 0 {
		d.debugf("  @%s: updating reaction watermark to %d", agent.AgentID, lastProcessedAt)
		if err := d.debouncer.UpdateReactionWatermark(agent.AgentID, lastProcessedAt); err != nil {
			d.debugf("  @%s: reaction watermark update failed: %v", agent.AgentID, err)
		}
	}
}

// getMessagesAfter returns messages mentioning agent after the given watermark.
// Includes mentions in all threads (not just room) and replies to agent's messages.
func (d *Daemon) getMessagesAfter(watermark, agentID string) ([]types.Message, error) {
	// Empty string means all threads (room + threads)
	allHomes := ""
	opts := &types.MessageQueryOptions{
		Limit:                 100,
		Home:                  &allHomes,
		IncludeRepliesToAgent: agentID,
	}
	if watermark != "" {
		opts.SinceID = watermark
	}

	return db.GetMessagesWithMention(d.database, agentID, opts)
}

// spawnAgent starts a new session for an agent.
// Returns the last msgID included in the wake prompt (for watermark tracking).
func (d *Daemon) spawnAgent(ctx context.Context, agent types.Agent, triggerMsgID string) (string, error) {
	if agent.Invoke == nil || agent.Invoke.Driver == "" {
		return "", fmt.Errorf("agent %s has no driver configured", agent.AgentID)
	}

	driver := d.drivers[agent.Invoke.Driver]
	if driver == nil {
		return "", fmt.Errorf("unknown driver: %s", agent.Invoke.Driver)
	}

	d.debugf("  spawning @%s with driver %s", agent.AgentID, agent.Invoke.Driver)

	// Update presence to spawning
	prevPresence := agent.Presence
	if err := db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agent.AgentID, prevPresence, types.PresenceSpawning, "spawn", "daemon", agent.Status); err != nil {
		return "", err
	}

	// Build wake prompt and get all included mentions
	prompt, allMentions := d.buildWakePrompt(agent, triggerMsgID)
	d.debugf("  wake prompt includes %d mentions", len(allMentions))

	// Spawn process
	proc, err := driver.Spawn(ctx, agent, prompt)
	if err != nil {
		d.debugf("  spawn error: %v", err)
		db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agent.AgentID, types.PresenceSpawning, types.PresenceError, "spawn_error", "daemon", agent.Status)
		return "", err
	}

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
		if ccState := GetCCUsageStateForDriver(agent.Invoke.Driver, proc.SessionID); ccState != nil {
			proc.BaselineInput = ccState.TotalInput
			proc.BaselineOutput = ccState.TotalOutput
			d.debugf("  baseline tokens: input=%d, output=%d", proc.BaselineInput, proc.BaselineOutput)
		}
	}

	// Store session ID for future resume - this ensures each agent keeps their own session
	if proc.SessionID != "" {
		db.UpdateAgentSessionID(d.database, agent.AgentID, proc.SessionID)
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

// monitorProcess drains stdout/stderr and waits for process exit.
func (d *Daemon) monitorProcess(agentID string, proc *Process) {
	defer d.wg.Done()

	// Drain stdout/stderr to prevent blocking
	var wg sync.WaitGroup

	if proc.Stdout != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 4096)
			for {
				n, err := proc.Stdout.Read(buf)
				if n > 0 && proc.Cmd.Process != nil {
					d.detector.RecordActivity(proc.Cmd.Process.Pid)
				}
				if err != nil {
					break
				}
			}
		}()
	}

	if proc.Stderr != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 4096)
			for {
				n, err := proc.Stderr.Read(buf)
				if n > 0 && proc.Cmd.Process != nil {
					d.detector.RecordActivity(proc.Cmd.Process.Pid)
				}
				if err != nil {
					break
				}
			}
		}()
	}

	// Wait for I/O to complete
	wg.Wait()

	// Wait for process to exit
	proc.Cmd.Wait()

	// Handle exit
	d.mu.Lock()
	d.handleProcessExit(agentID, proc)
	d.mu.Unlock()
}

// buildWakePrompt creates the prompt for waking an agent.
// Returns the prompt and the list of all msgIDs included.
func (d *Daemon) buildWakePrompt(agent types.Agent, triggerMsgID string) (string, []string) {
	// Include any pending mentions
	pending := d.debouncer.FlushPending(agent.AgentID)
	allMentions := append([]string{triggerMsgID}, pending...)

	// Get min_checkin for the prompt
	_, _, minCheckin, _ := GetTimeouts(agent.Invoke)

	// Check for fork spawn: was this agent mentioned with @agent#sessid syntax?
	var forkSessionID string
	triggerMsg, _ := db.GetMessage(d.database, triggerMsgID)
	if triggerMsg != nil && triggerMsg.ForkSessions != nil {
		if sessID, ok := triggerMsg.ForkSessions[agent.AgentID]; ok {
			forkSessionID = sessID
		}
	}

	// Group messages by home (thread) for better context
	homeGroups := make(map[string][]string)
	for _, msgID := range allMentions {
		msg, err := db.GetMessage(d.database, msgID)
		if err != nil || msg == nil {
			homeGroups["room"] = append(homeGroups["room"], msgID)
			continue
		}
		home := msg.Home
		if home == "" {
			home = "room"
		}
		homeGroups[home] = append(homeGroups[home], msgID)
	}

	// Build trigger info with thread context
	var triggerLines []string
	for home, msgIDs := range homeGroups {
		if home == "room" {
			triggerLines = append(triggerLines, fmt.Sprintf("Room: %v", msgIDs))
		} else {
			triggerLines = append(triggerLines, fmt.Sprintf("Thread %s: %v", home, msgIDs))
		}
	}
	triggerInfo := strings.Join(triggerLines, "\n")

	// Build fork context if this is a fork spawn
	forkContext := ""
	if forkSessionID != "" {
		// Get messages from the fork session for context
		forkMsgs := d.getSessionMessages(forkSessionID, agent.AgentID, 5)
		if len(forkMsgs) > 0 {
			var msgSummaries []string
			for _, msg := range forkMsgs {
				preview := msg.Body
				if len(preview) > 100 {
					preview = preview[:100] + "..."
				}
				msgSummaries = append(msgSummaries, fmt.Sprintf("  - [%s] %s", msg.ID, preview))
			}
			forkContext = fmt.Sprintf(`
**Fork Context (session %s):**
This spawn was triggered with @%s#%s syntax, meaning you should have context from a prior session.
Recent messages from that session:
%s

Use this context to continue the work without re-reading everything.
`,
				forkSessionID, agent.AgentID, forkSessionID, strings.Join(msgSummaries, "\n"))
		} else {
			forkContext = fmt.Sprintf(`
**Fork Context (session %s):**
This spawn was triggered with @%s#%s syntax, but no messages from that session were found.
The session ID may be invalid or the messages may have been pruned.
`,
				forkSessionID, agent.AgentID, forkSessionID)
		}
	}

	// Wake prompt - explicitly state agent identity to override any cached context
	var prompt string
	if minCheckin > 0 {
		minCheckinMins := minCheckin / 60000
		prompt = fmt.Sprintf(`**You are @%s.** Check fray for context.

Trigger messages:
%s
%s
Run: /fly %s if this is the start of a new session
Run: fray get %s

---
As soon as you 'fray back', post a reply in fray (in the same thread where the message was received, using the flag "--reply-to <msg-id>") as quickly as you can to acknowledge the user, then continue.  Don't use the literal word 'ack' as it can sound like a panicked reply. Be casual and mix it up.`,
			agent.AgentID, triggerInfo, forkContext, agent.AgentID, agent.AgentID)
		_ = minCheckinMins // Reserved for future use
	} else {
		prompt = fmt.Sprintf(`**You are @%s.** Check fray for context.

Trigger messages:
%s
%s
Run: /fly %s if this is the start of a new session
Run: fray get %s

As soon as you 'fray back', post a reply in fray (in the same thread where the message was received, using the flag "--reply-to <msg-id>") as quickly as you can to acknowledge the user, then continue. Don't use the literal word 'ack' as it can sound like a panicked reply. Be casual and mix it up.`,
			agent.AgentID, triggerInfo, forkContext, agent.AgentID, agent.AgentID)
	}

	return prompt, allMentions
}

// getSessionMessages retrieves recent messages from a specific session for fork context.
func (d *Daemon) getSessionMessages(sessionID, agentID string, limit int) []types.Message {
	// Query messages where session_id matches
	rows, err := d.database.Query(`
		SELECT guid, ts, channel_id, home, from_agent, session_id, body, mentions, fork_sessions, type, "references", surface_message, reply_to, quote_message_guid, edited_at, archived_at, reactions
		FROM fray_messages
		WHERE session_id = ? AND from_agent = ?
		ORDER BY ts DESC
		LIMIT ?
	`, sessionID, agentID, limit)
	if err != nil {
		d.debugf("getSessionMessages error: %v", err)
		return nil
	}
	defer rows.Close()

	var messages []types.Message
	for rows.Next() {
		var msg types.Message
		var channelID, home, sessionIDVal, forkSessions, references, surfaceMessage, replyTo, quoteMsgGUID sql.NullString
		var editedAt, archivedAt sql.NullInt64
		var mentionsJSON, reactionsJSON string

		err := rows.Scan(
			&msg.ID, &msg.TS, &channelID, &home, &msg.FromAgent, &sessionIDVal,
			&msg.Body, &mentionsJSON, &forkSessions, &msg.Type,
			&references, &surfaceMessage, &replyTo, &quoteMsgGUID,
			&editedAt, &archivedAt, &reactionsJSON,
		)
		if err != nil {
			d.debugf("getSessionMessages scan error: %v", err)
			continue
		}

		if channelID.Valid {
			msg.ChannelID = &channelID.String
		}
		if home.Valid {
			msg.Home = home.String
		}
		if sessionIDVal.Valid {
			msg.SessionID = &sessionIDVal.String
		}
		if references.Valid {
			msg.References = &references.String
		}
		if surfaceMessage.Valid {
			msg.SurfaceMessage = &surfaceMessage.String
		}
		if replyTo.Valid {
			msg.ReplyTo = &replyTo.String
		}
		if quoteMsgGUID.Valid {
			msg.QuoteMessageGUID = &quoteMsgGUID.String
		}
		if editedAt.Valid {
			msg.EditedAt = &editedAt.Int64
		}
		if archivedAt.Valid {
			msg.ArchivedAt = &archivedAt.Int64
		}

		// Parse mentions JSON
		if mentionsJSON != "" {
			json.Unmarshal([]byte(mentionsJSON), &msg.Mentions)
		}
		// Parse fork_sessions JSON
		if forkSessions.Valid && forkSessions.String != "" {
			json.Unmarshal([]byte(forkSessions.String), &msg.ForkSessions)
		}

		messages = append(messages, msg)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages
}

// updatePresence checks running processes and updates their presence.
// Uses ccusage polling for spawning→prompting→prompted transitions.
// Implements done-detection: idle presence + no fray posts for min_checkin = kill session.
func (d *Daemon) updatePresence() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for agentID, proc := range d.processes {
		if proc.Cmd.ProcessState != nil {
			// Process has exited
			d.handleProcessExit(agentID, proc)
			continue
		}

		agent, _ := db.GetAgent(d.database, agentID)
		if agent == nil || agent.Invoke == nil {
			continue
		}

		spawnTimeout, idleAfter, minCheckin, maxRuntime := GetTimeouts(agent.Invoke)
		elapsed := time.Since(proc.StartedAt).Milliseconds()

		// Zombie safety net: kill after max_runtime regardless of state (0 = unlimited)
		if maxRuntime > 0 && elapsed > maxRuntime {
			d.killProcess(agentID, proc, "max_runtime exceeded")
			continue
		}

		// Handle presence state transitions based on current state
		switch agent.Presence {
		case types.PresenceSpawning, types.PresencePrompting, types.PresencePrompted:
			// Poll ccusage for token-based state transitions
			// Compare against baseline to detect NEW tokens (important for resumed sessions)
			ccState := GetCCUsageStateForDriver(agent.Invoke.Driver, proc.SessionID)
			if ccState != nil {
				newInput := ccState.TotalInput - proc.BaselineInput
				newOutput := ccState.TotalOutput - proc.BaselineOutput

				if newOutput > 0 {
					// Agent is generating response (new output tokens)
					if agent.Presence != types.PresencePrompted {
						d.debugf("  @%s: prompting→prompted (new output tokens: %d)", agentID, newOutput)
						// Note: Not auditing prompting→prompted as it's a high-frequency internal transition
						db.UpdateAgentPresence(d.database, agentID, types.PresencePrompted)
					}
				} else if newInput > 0 {
					// Context being sent to API (new input tokens)
					if agent.Presence == types.PresenceSpawning {
						d.debugf("  @%s: spawning→prompting (new input tokens: %d)", agentID, newInput)
						// Note: Not auditing spawning→prompting as it's a high-frequency internal transition
						db.UpdateAgentPresence(d.database, agentID, types.PresencePrompting)
					}
				}
			}

			// Check spawn timeout (applies to spawning state only)
			if agent.Presence == types.PresenceSpawning && elapsed > spawnTimeout {
				db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agentID, agent.Presence, types.PresenceError, "spawn_timeout", "daemon", agent.Status)
			}

		case types.PresenceActive:
			// Check for idle transition based on fray activity
			pid := proc.Cmd.Process.Pid
			lastActivity := d.detector.LastActivityTime(pid)
			if time.Since(lastActivity).Milliseconds() > idleAfter {
				db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agentID, agent.Presence, types.PresenceIdle, "idle_timeout", "daemon", agent.Status)
			}

		case types.PresenceIdle:
			// Done-detection: if idle AND no fray activity for min_checkin, kill session
			if minCheckin > 0 {
				lastPostTs, _ := db.GetAgentLastPostTime(d.database, agentID)
				lastHeartbeatTs := int64(0)
				if agent.LastHeartbeat != nil {
					lastHeartbeatTs = *agent.LastHeartbeat
				}

				// Use the most recent of: last post, last heartbeat, or spawn time
				lastFrayActivity := proc.StartedAt.UnixMilli()
				if lastPostTs > lastFrayActivity {
					lastFrayActivity = lastPostTs
				}
				if lastHeartbeatTs > lastFrayActivity {
					lastFrayActivity = lastHeartbeatTs
				}

				msSinceActivity := time.Now().UnixMilli() - lastFrayActivity
				if msSinceActivity > minCheckin {
					d.killProcess(agentID, proc, "done-detection: idle + no fray activity")
				}
			}
		}
	}
}

// killProcess terminates a process and records the reason.
func (d *Daemon) killProcess(agentID string, proc *Process, reason string) {
	if proc.Cmd.Process != nil {
		proc.Cmd.Process.Kill()
	}
	// handleProcessExit will be called by monitorProcess when it detects the exit
}

// handleProcessExit cleans up after a process exits.
// Must be called with d.mu held. Safe to call multiple times.
func (d *Daemon) handleProcessExit(agentID string, proc *Process) {
	// Check if this proc is still the current one for this agent.
	// A new process may have been spawned, in which case we shouldn't
	// update presence (new process owns that), but we still record session_end
	// for audit trail.
	currentProc := d.processes[agentID]
	isCurrentProc := currentProc == proc

	// For current proc, check handled flag to prevent duplicate session_end.
	// For old proc (isCurrentProc=false), always record - no duplication possible.
	if isCurrentProc && d.handled[agentID] {
		return
	}
	if isCurrentProc {
		d.handled[agentID] = true
	}

	exitCode := 0
	if proc.Cmd.ProcessState != nil {
		exitCode = proc.Cmd.ProcessState.ExitCode()
	}

	// Get the last message from this session for boundary tracking
	var lastMsgID *string
	if proc.SessionID != "" {
		msgs := d.getSessionMessages(proc.SessionID, agentID, 1)
		if len(msgs) > 0 {
			lastMsgID = &msgs[len(msgs)-1].ID
		}
	}

	// Record session end for audit trail
	sessionEnd := types.SessionEnd{
		AgentID:    agentID,
		SessionID:  proc.SessionID,
		ExitCode:   exitCode,
		DurationMs: time.Since(proc.StartedAt).Milliseconds(),
		EndedAt:    time.Now().Unix(),
		LastMsgID:  lastMsgID,
	}
	db.AppendSessionEnd(d.project.DBPath, sessionEnd)

	// Session ID is now stored at spawn time (we generate it ourselves with --session-id)
	// No need to detect it from Claude's files anymore - see fix for fray-8ld6

	// Cleanup process resources
	driver := d.getDriver(agentID)
	if driver != nil {
		driver.Cleanup(proc)
	}

	// Only update presence and remove from map if this is the current process
	if isCurrentProc {
		// Check current presence - if agent already ran `fray bye`, presence is offline
		// and we should respect that rather than overwriting with idle
		agent, _ := db.GetAgent(d.database, agentID)
		alreadyOffline := agent != nil && agent.Presence == types.PresenceOffline
		prevPresence := types.PresenceState("")
		if agent != nil {
			prevPresence = agent.Presence
		}

		if !alreadyOffline {
			var reason string
			var newPresence types.PresenceState
			if exitCode == 0 {
				reason = "exit_ok"
				newPresence = types.PresenceIdle
			} else if exitCode == -1 {
				// Signal kill (SIGTERM/SIGINT) - treat as idle, not error
				// This handles: user Ctrl-C, daemon restart, network interruption
				d.debugf("@%s: exit_code=-1 (signal kill) → idle (not error)", agentID)
				reason = "signal_kill"
				newPresence = types.PresenceIdle
			} else {
				// Check for likely session resume failure:
				// - Quick failure (< 30s)
				// - Non-zero exit code
				// - Had a session ID (was trying to resume)
				durationSec := time.Since(proc.StartedAt).Seconds()
				if durationSec < 30 && proc.SessionID != "" {
					// Likely resume failure - set to idle so next spawn starts fresh
					d.debugf("@%s: quick failure (%ds, exit=%d) with session %s - likely resume failure, clearing session",
						agentID, int(durationSec), exitCode, proc.SessionID)
					reason = "resume_failure"
					newPresence = types.PresenceIdle
					// Clear session ID to prevent retry loop
					db.UpdateAgentSessionID(d.database, agentID, "")
				} else {
					reason = "exit_error"
					newPresence = types.PresenceError
				}
			}
			var status *string
			if agent != nil {
				status = agent.Status
			}
			db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agentID, prevPresence, newPresence, reason, "daemon", status)
		}
		// NOTE: Do NOT clear session ID here for normal exits. Session remains resumable until agent runs `fray bye`.
		// Daemon-initiated exits (done-detection) are soft ends; session context persists on disk.
		// Exception: resume_failure clears session ID above to prevent retry loop.

		// Set left_at so fray back knows this was a proper session end (not orphaned)
		// (only if not already set by fray bye)
		if agent == nil || agent.LeftAt == nil {
			now := time.Now().Unix()
			db.UpdateAgent(d.database, agentID, db.AgentUpdates{
				LeftAt: types.OptionalInt64{Set: true, Value: &now},
			})
		}

		delete(d.processes, agentID)
	}
}

// getDriver returns the driver for an agent.
func (d *Daemon) getDriver(agentID string) Driver {
	agent, err := db.GetAgent(d.database, agentID)
	if err != nil || agent == nil || agent.Invoke == nil {
		return nil
	}
	return d.drivers[agent.Invoke.Driver]
}
