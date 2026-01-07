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
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// Daemon watches for @mentions and spawns managed agents.
type Daemon struct {
	mu           sync.RWMutex
	project      core.Project
	database     *sql.DB
	debouncer    *MentionDebouncer
	detector     ActivityDetector
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

	d := &Daemon{
		project:      project,
		database:     database,
		debouncer:    NewMentionDebouncer(database, project.DBPath),
		detector:     NewActivityDetector(),
		processes:    make(map[string]*Process),
		handled:      make(map[string]bool),
		drivers:      make(map[string]Driver),
		stopCh:       make(chan struct{}),
		lockPath:     filepath.Join(filepath.Dir(project.DBPath), "daemon.lock"),
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
			if base, ok := d.detector.(*FallbackDetector); ok {
				base.Cleanup(proc.Cmd.Process.Pid)
			} else if darwin, ok := d.detector.(*DarwinDetector); ok {
				darwin.Cleanup(proc.Cmd.Process.Pid)
			}
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
			// Check if process is still running
			proc, err := os.FindProcess(info.PID)
			if err == nil && proc.Signal(nil) == nil {
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

	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return false
	}
	return proc.Signal(nil) == nil
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

		// Skip non-direct mentions (mid-sentence, FYI, CC patterns)
		// These will show up in agent's mentions but don't trigger spawn
		if !IsDirectAddress(msg, agent.AgentID) {
			d.debugf("    %s: skip (not direct address) - body: %q", msg.ID, truncate(msg.Body, 50))
			if !hasQueued && !spawned {
				lastProcessedID = msg.ID
			}
			continue
		}

		// Check thread ownership - only human or thread owner can trigger spawn
		var thread *types.Thread
		if msg.Home != "" && msg.Home != "room" {
			thread, _ = db.GetThread(d.database, msg.Home)
		}
		if !CanTriggerSpawn(msg, thread) {
			isHuman := msg.Type == types.MessageTypeUser
			d.debugf("    %s: skip (ownership check failed) - from: %s, type: %s, isHuman: %v", msg.ID, msg.FromAgent, msg.Type, isHuman)
			if !hasQueued && !spawned {
				lastProcessedID = msg.ID
			}
			continue
		}

		// If we already spawned this poll, or agent is busy, queue the mention
		// Note: Don't advance watermark for queued messages - pending is in-memory,
		// so on restart we need to re-query and re-queue them
		if spawned || agent.Presence == types.PresenceSpawning || agent.Presence == types.PresenceActive {
			d.debugf("    %s: queued (agent busy or already spawned)", msg.ID)
			d.debouncer.QueueMention(agent.AgentID, msg.ID)
			hasQueued = true
			continue
		}

		d.debugf("    %s: triggering spawn", msg.ID)

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
		d.debouncer.UpdateWatermark(agent.AgentID, lastProcessedID)
	}
}

// getMessagesAfter returns messages mentioning agent after the given watermark.
func (d *Daemon) getMessagesAfter(watermark, agentID string) ([]types.Message, error) {
	opts := &types.MessageQueryOptions{
		Limit: 100,
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

	d.debugf("  spawned pid %d, session %s", proc.Cmd.Process.Pid, proc.SessionID)

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
	minCheckinMins := minCheckin / 60000

	// Wake prompt with checkin explanation
	prompt := fmt.Sprintf(`You've been @mentioned. Check fray for context.

Trigger messages: %v

Run: fray get %s

---
Checkin: Posting to fray resets a %dm timer. Silence = session recycled (resumable on @mention).`,
		allMentions, agent.AgentID, minCheckinMins)

	return prompt, allMentions
}

// updatePresence checks running processes and updates their presence.
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

		pid := proc.Cmd.Process.Pid
		if d.detector.IsActive(pid) {
			db.UpdateAgentPresence(d.database, agentID, types.PresenceActive)
		} else {
			// Check timeouts
			agent, _ := db.GetAgent(d.database, agentID)
			if agent != nil && agent.Invoke != nil {
				spawnTimeout, idleAfter, minCheckin, maxRuntime := GetTimeouts(agent.Invoke)
				elapsed := time.Since(proc.StartedAt).Milliseconds()

				// Zombie safety net: kill after max_runtime regardless of state (0 = unlimited)
				if maxRuntime > 0 && elapsed > maxRuntime {
					d.killProcess(agentID, proc, "max_runtime exceeded")
					continue
				}

				if agent.Presence == types.PresenceSpawning && elapsed > spawnTimeout {
					// Spawning timeout - mark as error
					db.UpdateAgentPresence(d.database, agentID, types.PresenceError)
				} else if agent.Presence == types.PresenceActive {
					lastActivity := d.detector.LastActivityTime(pid)
					if time.Since(lastActivity).Milliseconds() > idleAfter {
						db.UpdateAgentPresence(d.database, agentID, types.PresenceIdle)
					}
				} else if agent.Presence == types.PresenceIdle {
					// Done-detection: if idle AND no fray activity (posts or heartbeat) for min_checkin, kill session
					lastPostTs, _ := db.GetAgentLastPostTime(d.database, agentID)
					lastHeartbeatTs := int64(0)
					if agent.LastHeartbeat != nil {
						lastHeartbeatTs = *agent.LastHeartbeat
					}

					// Use the most recent of: last post, last heartbeat, or spawn time
					lastActivity := proc.StartedAt.UnixMilli()
					if lastPostTs > lastActivity {
						lastActivity = lastPostTs
					}
					if lastHeartbeatTs > lastActivity {
						lastActivity = lastHeartbeatTs
					}

					msSinceActivity := time.Now().UnixMilli() - lastActivity
					if msSinceActivity > minCheckin {
						d.killProcess(agentID, proc, "done-detection: idle + no fray activity")
					}
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

	// Detect and store Claude Code session ID for next --resume
	if claudeSessionID := FindClaudeSessionID(d.project.Root); claudeSessionID != "" {
		db.UpdateAgentSessionID(d.database, agentID, claudeSessionID)
	}

	// Cleanup process resources
	driver := d.getDriver(agentID)
	if driver != nil {
		driver.Cleanup(proc)
	}

	// Only update presence and remove from map if this is the current process
	if isCurrentProc {
		if exitCode == 0 {
			db.UpdateAgentPresence(d.database, agentID, types.PresenceIdle)
		} else {
			db.UpdateAgentPresence(d.database, agentID, types.PresenceError)
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
