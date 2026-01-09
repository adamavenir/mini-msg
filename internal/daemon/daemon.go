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
	router       *router.Router     // mlld-based routing for mention interpretation
	processes    map[string]*Process // agent_id -> process
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
		// Reset to idle so mentions can spawn fresh
		d.debugf("startup cleanup: @%s was %s, resetting to idle", agent.AgentID, agent.Presence)
		if err := db.UpdateAgentPresence(d.database, agent.AgentID, types.PresenceIdle); err != nil {
			d.debugf("startup cleanup: error updating @%s: %v", agent.AgentID, err)
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

	// Check for new mentions for each managed agent
	for _, agent := range agents {
		d.checkMentions(ctx, agent)
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

		// Check if agent has an active process (busy states)
		// Detect orphaned state: presence=active but no tracked process
		d.mu.RLock()
		_, hasProcess := d.processes[agent.AgentID]
		d.mu.RUnlock()

		isBusyState := agent.Presence == types.PresenceSpawning ||
			agent.Presence == types.PresencePrompting ||
			agent.Presence == types.PresencePrompted ||
			agent.Presence == types.PresenceActive

		if isBusyState && hasProcess {
			// Agent has running process, queue the mention
			d.debugf("    %s: queued (agent busy)", msg.ID)
			d.debouncer.QueueMention(agent.AgentID, msg.ID)
			hasQueued = true
			continue
		}

		if isBusyState && !hasProcess {
			// Orphaned state: presence says busy but no tracked process
			// Reset to idle and proceed to spawn
			d.debugf("    %s: orphaned presence detected (was %s), resetting to idle", msg.ID, agent.Presence)
			db.UpdateAgentPresence(d.database, agent.AgentID, types.PresenceIdle)
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

		// Try to spawn - spawnAgent returns the last msgID included in wake prompt
		lastIncluded, err := d.spawnAgent(ctx, agent, msg.ID)
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
	if err := db.UpdateAgentPresence(d.database, agent.AgentID, types.PresenceSpawning); err != nil {
		return "", err
	}

	// Build wake prompt and get all included mentions
	prompt, allMentions := d.buildWakePrompt(agent, triggerMsgID)
	d.debugf("  wake prompt includes %d mentions", len(allMentions))

	// Spawn process
	proc, err := driver.Spawn(ctx, agent, prompt)
	if err != nil {
		d.debugf("  spawn error: %v", err)
		db.UpdateAgentPresence(d.database, agent.AgentID, types.PresenceError)
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

	// Wake prompt 
	var prompt string
	if minCheckin > 0 {
		minCheckinMins := minCheckin / 60000
		prompt = fmt.Sprintf(`You've been @mentioned. Check fray for context.

Trigger messages:
%s

Run: /fly $ARGUMENTS if this is the start of a new session 
Run: fray get %s

---
As soon as you 'fray back', post a reply in fray (in the same thread where the message was received, using the flag "--reply-to <msg-id>") as quickly as you can to acknowledge the user, then continue.  Don't use the literal word 'ack' as it can sound like a panicked reply. Be casual and mix it up.`,
			triggerInfo, agent.AgentID)
		_ = minCheckinMins // Reserved for future use
	} else {
		prompt = fmt.Sprintf(`You've been @mentioned. Check fray for context.

Trigger messages:
%s

Run: /fly $ARGUMENTS if this is the start of a new session
Run: fray get %s 

As soon as you 'fray back', post a reply in fray (in the same thread where the message was received, using the flag "--reply-to <msg-id>") as quickly as you can to acknowledge the user, then continue. Don't use the literal word 'ack' as it can sound like a panicked reply. Be casual and mix it up.`,
			triggerInfo, agent.AgentID)
	}

	return prompt, allMentions
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
						db.UpdateAgentPresence(d.database, agentID, types.PresencePrompted)
					}
				} else if newInput > 0 {
					// Context being sent to API (new input tokens)
					if agent.Presence == types.PresenceSpawning {
						d.debugf("  @%s: spawning→prompting (new input tokens: %d)", agentID, newInput)
						db.UpdateAgentPresence(d.database, agentID, types.PresencePrompting)
					}
				}
			}

			// Check spawn timeout (applies to spawning state only)
			if agent.Presence == types.PresenceSpawning && elapsed > spawnTimeout {
				db.UpdateAgentPresence(d.database, agentID, types.PresenceError)
			}

		case types.PresenceActive:
			// Check for idle transition based on fray activity
			pid := proc.Cmd.Process.Pid
			lastActivity := d.detector.LastActivityTime(pid)
			if time.Since(lastActivity).Milliseconds() > idleAfter {
				db.UpdateAgentPresence(d.database, agentID, types.PresenceIdle)
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

	// Record session end for audit trail
	sessionEnd := types.SessionEnd{
		AgentID:    agentID,
		SessionID:  proc.SessionID,
		ExitCode:   exitCode,
		DurationMs: time.Since(proc.StartedAt).Milliseconds(),
		EndedAt:    time.Now().Unix(),
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
		if exitCode == 0 {
			db.UpdateAgentPresence(d.database, agentID, types.PresenceIdle)
		} else if exitCode == -1 {
			// Signal kill (SIGTERM/SIGINT) - treat as idle, not error
			// This handles: user Ctrl-C, daemon restart, network interruption
			d.debugf("@%s: exit_code=-1 (signal kill) → idle (not error)", agentID)
			db.UpdateAgentPresence(d.database, agentID, types.PresenceIdle)
		} else {
			db.UpdateAgentPresence(d.database, agentID, types.PresenceError)
		}
		// NOTE: Do NOT clear session ID here. Session remains resumable until agent runs `fray bye`.
		// Daemon-initiated exits (done-detection) are soft ends; session context persists on disk.

		// Set left_at so fray back knows this was a proper session end (not orphaned)
		now := time.Now().Unix()
		db.UpdateAgent(d.database, agentID, db.AgentUpdates{
			LeftAt: types.OptionalInt64{Set: true, Value: &now},
		})

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
