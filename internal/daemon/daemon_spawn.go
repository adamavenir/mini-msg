package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/adamavenir/fray/internal/aap"
	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// runStdoutRepair evaluates captured stdout to determine if it should be posted.
// Returns nil if stdout-repair.mld doesn't exist (graceful degradation).
func (d *Daemon) runStdoutRepair(stdout string, lastPost *string, agentID string) *types.StdoutRepairResult {
	// Check if stdout-repair.mld exists (try new location, fall back to legacy)
	stdoutRepairPath := filepath.Join(d.project.Root, ".fray", "llm", "routers", "stdout-repair.mld")
	if _, err := os.Stat(stdoutRepairPath); os.IsNotExist(err) {
		// Try legacy location
		stdoutRepairPath = filepath.Join(d.project.Root, ".fray", "llm", "stdout-repair.mld")
		if _, err := os.Stat(stdoutRepairPath); os.IsNotExist(err) {
			d.debugf("stdout-repair: template not found, skipping")
			return nil
		}
	}

	// Build payload
	payload := types.StdoutRepairPayload{
		Stdout:   stdout,
		LastPost: lastPost,
		AgentID:  agentID,
	}

	// Encode payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		d.debugf("stdout-repair: marshal error: %v", err)
		return nil
	}

	// Run mlld with --payload flag
	cmd := exec.Command("mlld", "--payload", fmt.Sprintf("@payload=%s", string(payloadJSON)), stdoutRepairPath)
	cmd.Dir = d.project.Root

	output, err := cmd.Output()
	if err != nil {
		d.debugf("stdout-repair: mlld error: %v", err)
		return nil
	}

	// Parse result
	var result types.StdoutRepairResult
	if err := json.Unmarshal(output, &result); err != nil {
		d.debugf("stdout-repair: parse error: %v (output: %s)", err, output)
		return nil
	}

	return &result
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
	// Exception: if a fork session ID was specified via @agent#sessid, use that instead.
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

// monitorProcess drains stdout/stderr and waits for process exit.
func (d *Daemon) monitorProcess(agentID string, proc *Process) {
	defer d.wg.Done()

	// Create ring buffers for stdout/stderr capture (4KB each)
	proc.StdoutBuffer = NewStdoutBuffer(4096)
	proc.StderrBuffer = NewStdoutBuffer(4096)

	// Drain stdout/stderr to prevent blocking
	var wg sync.WaitGroup

	if proc.Stdout != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 4096)
			for {
				n, err := proc.Stdout.Read(buf)
				if n > 0 {
					// Capture to ring buffer for later processing
					proc.StdoutBuffer.Write(buf[:n])
					if proc.Cmd.Process != nil {
						d.detector.RecordActivity(proc.Cmd.Process.Pid)
					}
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
				if n > 0 {
					// Capture to ring buffer for error debugging
					proc.StderrBuffer.Write(buf[:n])
					if proc.Cmd.Process != nil {
						d.detector.RecordActivity(proc.Cmd.Process.Pid)
					}
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
	shouldRespawnBRB := d.handleProcessExit(agentID, proc)
	d.mu.Unlock()

	// If agent requested BRB, spawn fresh session immediately
	if shouldRespawnBRB {
		d.debugf("@%s: spawning fresh session (BRB respawn)", agentID)
		// Re-fetch agent to get latest state
		agent, err := db.GetAgent(d.database, agentID)
		if err != nil || agent == nil {
			d.debugf("@%s: BRB spawn failed - agent not found", agentID)
			return
		}
		// Spawn with continuation prompt
		d.spawnBRBAgent(context.Background(), *agent)
	}
}

// detectSpawnMode checks the trigger message for /fly, /hop, /land, /hand patterns.
// Returns the spawn mode and optionally a user message that follows the command.
func detectSpawnMode(body string, agentID string) (SpawnMode, string) {
	// Look for patterns like "@agent /fly", "@agent /hop", "@agent /land", "@agent /hand"
	patterns := []struct {
		suffix string
		mode   SpawnMode
	}{
		{" /fly", SpawnModeFly},
		{" /hop", SpawnModeHop},
		{" /land", SpawnModeLand},
		{" /hand", SpawnModeHand},
	}

	agentMention := "@" + agentID
	bodyLower := strings.ToLower(body)
	for _, p := range patterns {
		pattern := strings.ToLower(agentMention + p.suffix)
		if strings.HasPrefix(bodyLower, pattern) {
			// Extract any user message after the command
			remainder := strings.TrimSpace(body[len(agentMention)+len(p.suffix):])
			return p.mode, remainder
		}
	}
	return SpawnModeNormal, ""
}

// PromptPayload is the input to prompt template execution.
type PromptPayload struct {
	Agent         string   `json:"agent"`
	TriggerMsgIDs []string `json:"triggerMsgIDs"`
	UserTask      string   `json:"userTask,omitempty"`
}

// executePromptTemplate runs an mlld template and returns the prompt string.
// Falls back to empty string if template doesn't exist or execution fails.
// Template location:
//   - prompts/: mention-fresh, mention-resume (daemon spawn context)
//   - slash/: fly, hop, land (skill templates)
func (d *Daemon) executePromptTemplate(templateName string, payload PromptPayload) (string, error) {
	// Determine template directory based on template type
	var templateDir string
	switch templateName {
	case "mention-fresh", "mention-resume":
		templateDir = "prompts"
	default:
		templateDir = "slash"
	}
	templatePath := filepath.Join(d.project.Root, ".fray", "llm", templateDir, templateName+".mld")

	// Check if template exists
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		d.debugf("  template %s not found at %s", templateName, templatePath)
		return "", fmt.Errorf("template not found: %s", templateName)
	}

	// Execute the template
	result, err := d.mlldClient.Execute(templatePath, payload, nil)
	if err != nil {
		d.debugf("  mlld execute error for %s: %v", templateName, err)
		return "", fmt.Errorf("mlld execute failed: %w", err)
	}

	return result.Output, nil
}

// buildWakePrompt creates the prompt for waking an agent.
// Uses mlld templates from .fray/llm/prompts/ and .fray/llm/slash/ with fallback to inline prompts.
// Returns the prompt, spawn mode, and the list of all msgIDs included.
func (d *Daemon) buildWakePrompt(agent types.Agent, triggerMsgID string) (string, SpawnMode, []string) {
	// Include any pending mentions
	pending := d.debouncer.FlushPending(agent.AgentID)
	allMentions := append([]string{triggerMsgID}, pending...)

	// Check for /fly, /hop, /land patterns in trigger message
	triggerMsg, _ := db.GetMessage(d.database, triggerMsgID)
	spawnMode := SpawnModeNormal
	var userMessage string
	if triggerMsg != nil {
		spawnMode, userMessage = detectSpawnMode(triggerMsg.Body, agent.AgentID)
	}

	// Determine which template to use based on spawn mode
	var templateName string
	switch spawnMode {
	case SpawnModeFly:
		templateName = "mention-fresh"
	case SpawnModeHop:
		templateName = "hop"
	case SpawnModeLand:
		templateName = "land"
	default:
		templateName = "mention-resume"
	}

	// Try to execute mlld template
	payload := PromptPayload{
		Agent:         agent.AgentID,
		TriggerMsgIDs: allMentions,
		UserTask:      userMessage,
	}

	if prompt, err := d.executePromptTemplate(templateName, payload); err == nil && prompt != "" {
		d.debugf("  using mlld template %s", templateName)
		return prompt, spawnMode, allMentions
	}

	// Fallback to inline prompts if template execution fails
	d.debugf("  falling back to inline prompt (template %s unavailable)", templateName)
	return d.buildInlinePrompt(agent, triggerMsgID, spawnMode, userMessage, allMentions)
}

// buildInlinePrompt generates a prompt using hardcoded templates (fallback).
func (d *Daemon) buildInlinePrompt(agent types.Agent, triggerMsgID string, spawnMode SpawnMode, userMessage string, allMentions []string) (string, SpawnMode, []string) {
	// Build trigger info with thread context
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

	var triggerLines []string
	for home, msgIDs := range homeGroups {
		if home == "room" {
			triggerLines = append(triggerLines, fmt.Sprintf("Room: %v", msgIDs))
		} else {
			triggerLines = append(triggerLines, fmt.Sprintf("Thread %s: %v", home, msgIDs))
		}
	}
	triggerInfo := strings.Join(triggerLines, "\n")

	// Build prompt based on spawn mode
	var prompt string
	switch spawnMode {
	case SpawnModeFly:
		prompt = d.buildFlyPromptInline(agent, userMessage, triggerInfo)
	case SpawnModeHop:
		prompt = d.buildHopPromptInline(agent, userMessage, triggerInfo)
	case SpawnModeLand:
		prompt = d.buildLandPromptInline(agent, triggerInfo)
	case SpawnModeHand:
		prompt = d.buildHandPromptInline(agent, triggerInfo)
	default:
		prompt = d.buildResumePromptInline(agent, triggerInfo)
	}

	return prompt, spawnMode, allMentions
}

// buildFlyPromptInline creates the fallback prompt for /fly command spawns.
func (d *Daemon) buildFlyPromptInline(agent types.Agent, userMessage, triggerInfo string) string {
	taskContext := ""
	if userMessage != "" {
		taskContext = fmt.Sprintf("\n\nUser's task:\n%s", userMessage)
	}

	return fmt.Sprintf(`**You are @%s.** Starting a new session.

Trigger messages:
%s
%s
---
This is a fresh session. Check your notes (fray get meta/%s/notes) for prior context, then proceed with the task.

IMPORTANT: Users only see messages posted to fray, so:
1. Send a quick casual acknowledgement of the request ASAP so the user knows you received their message.
2. Your stdout is not visible. Post progress updates and summaries to fray as you go so users can follow your work.`,
		agent.AgentID, triggerInfo, taskContext, agent.AgentID)
}

// buildHopPromptInline creates the fallback prompt for /hop command spawns.
func (d *Daemon) buildHopPromptInline(agent types.Agent, userMessage, triggerInfo string) string {
	taskContext := ""
	if userMessage != "" {
		taskContext = fmt.Sprintf("\n\nUser's task:\n%s", userMessage)
	}

	return fmt.Sprintf(`**You are @%s.** Quick hop-in task.

Trigger messages:
%s
%s
---
Complete this task efficiently, then run 'fray bye %s' when done. If you go idle without completing, you'll be auto-terminated.

IMPORTANT: Users only see messages posted to fray, so:
1. Send a quick casual acknowledgement of the request ASAP so the user knows you received their message.
2. Your stdout is not visible. Post progress updates and summaries to fray as you go so users can follow your work.`,
		agent.AgentID, triggerInfo, taskContext, agent.AgentID)
}

// buildLandPromptInline creates the fallback prompt for /land command messages.
func (d *Daemon) buildLandPromptInline(agent types.Agent, triggerInfo string) string {
	return fmt.Sprintf(`**@%s** - User is asking you to close out your session (longterm).

Trigger: %s

Generate a standup report and clean up your session:
1. Post a brief standup to the room
2. Update your notes thread in fray in meta/%s/notes with handoff context
3. Clear claims: fray clear @%s
4. Leave: fray bye %s "standup message"`,
		agent.AgentID, triggerInfo, agent.AgentID, agent.AgentID, agent.AgentID)
}

func (d *Daemon) buildHandPromptInline(agent types.Agent, triggerInfo string) string {
	return fmt.Sprintf(`**@%s** - Hot handoff requested. Work continues immediately in fresh context.

Trigger: %s

Hand off to fresh context:
1. Post brief "handing off" message to room
2. Create beads for discovered work: bd create "..." --type task
3. Update your notes thread in fray in meta/%s/notes with current state (preserve details, don't condense)
4. Clear claims: fray clear @%s
5. Hand off: fray brb %s "handing off to fresh context"`,
		agent.AgentID, triggerInfo, agent.AgentID, agent.AgentID, agent.AgentID)
}

// buildResumePromptInline creates the fallback prompt for regular @mention wakes.
func (d *Daemon) buildResumePromptInline(agent types.Agent, triggerInfo string) string {
	return fmt.Sprintf(`**You are @%s.** Check fray for context.

Trigger messages:
%s

---
Reply in the thread where you were mentioned (use --reply-to <msg-id>). If you can answer immediately, just answer. Otherwise, acknowledge briefly then continue.

IMPORTANT: Users only see messages posted to fray, so:
1. Send a quick casual acknowledgement of the request ASAP so the user knows you received their message.
2. Your stdout is not visible. Post progress updates and summaries to fray as you go so users can follow your work.`,
		agent.AgentID, triggerInfo)
}

// getSessionMessages retrieves recent messages from a specific session for fork context.
// sessionID can be a prefix (e.g., "25912084") which will match full UUIDs like "25912084-8d46-497b-...".
func (d *Daemon) getSessionMessages(sessionID, agentID string, limit int) []types.Message {
	// Query messages where session_id matches prefix (supports @agent#sessid with short IDs)
	rows, err := d.database.Query(`
		SELECT guid, ts, channel_id, home, from_agent, session_id, body, mentions, fork_sessions, type, "references", surface_message, reply_to, quote_message_guid, edited_at, archived_at, reactions
		FROM fray_messages
		WHERE session_id LIKE ? || '%' AND from_agent = ?
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
// Uses token polling for spawning→prompting→prompted transitions.
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
		case types.PresenceSpawning, types.PresencePrompting, types.PresencePrompted, types.PresenceCompacting:
			// Poll for token-based state transitions
			// Compare against baseline to detect NEW tokens (important for resumed sessions)
			tokenState := GetTokenStateForDriver(agent.Invoke.Driver, proc.SessionID)
			if tokenState != nil {
				newInput := tokenState.TotalInput - proc.BaselineInput
				newOutput := tokenState.TotalOutput - proc.BaselineOutput

				// Update token watermarks for spawn decision detection
				// This persists across daemon restarts
				db.UpdateAgentTokenWatermarks(d.database, agentID, tokenState.TotalInput, tokenState.TotalOutput)

				if newOutput > 0 {
					// Agent is generating response (new output tokens)
					if agent.Presence != types.PresencePrompted {
						d.debugf("  @%s: %s→prompted (new output tokens: %d)", agentID, agent.Presence, newOutput)
						db.UpdateAgentPresence(d.database, agentID, types.PresencePrompted)
					}
					// Initialize token tracking for active state idle detection
					proc.LastInputTokens = tokenState.TotalInput
					proc.LastOutputTokens = tokenState.TotalOutput
					proc.LastTokenCheck = time.Now()
				} else if newInput > 0 {
					// Context being sent to API (new input tokens)
					if agent.Presence == types.PresenceSpawning {
						d.debugf("  @%s: spawning→prompting (new input tokens: %d)", agentID, newInput)
						db.UpdateAgentPresence(d.database, agentID, types.PresencePrompting)
					}
				}
			}

			// Fallback: if agent has posted to fray since spawn, transition to active.
			// This handles cases where transcript parsing is unavailable.
			lastPostTs, _ := db.GetAgentLastPostTime(d.database, agentID)
			if lastPostTs > proc.StartedAt.UnixMilli() {
				d.debugf("  @%s: %s→active (fray post detected)", agentID, agent.Presence)
				db.UpdateAgentPresence(d.database, agentID, types.PresenceActive)
				// Sync token tracking to current values so we don't immediately trigger idle
				if tokenState != nil {
					proc.LastInputTokens = tokenState.TotalInput
					proc.LastOutputTokens = tokenState.TotalOutput
				}
				proc.LastTokenCheck = time.Now() // Reset idle timer
				continue                         // Skip spawn timeout check since agent is clearly working
			}

			// Check spawn timeout (applies to spawning state only)
			if agent.Presence == types.PresenceSpawning && elapsed > spawnTimeout {
				db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agentID, agent.Presence, types.PresenceError, "spawn_timeout", "daemon", agent.Status)
			}

		case types.PresenceActive:
			// Check fray post FIRST - if agent posted, reset idle timer before checking idle
			lastPostTs, _ := db.GetAgentLastPostTime(d.database, agentID)
			if lastPostTs > proc.LastTokenCheck.UnixMilli() {
				proc.LastTokenCheck = time.Now()
			}

			// Poll for idle detection - if tokens stop changing, go idle
			// Track both input AND output tokens: output = Claude responding, input = tool results
			tokenState := GetTokenStateForDriver(agent.Invoke.Driver, proc.SessionID)
			if tokenState != nil {
				currentInput := tokenState.TotalInput
				currentOutput := tokenState.TotalOutput
				// Update token watermarks for spawn decision detection
				db.UpdateAgentTokenWatermarks(d.database, agentID, currentInput, currentOutput)

				// Activity detected if EITHER input or output tokens increased
				// Input increases during tool use (tool results), output increases during response
				if currentOutput > proc.LastOutputTokens || currentInput > proc.LastInputTokens {
					proc.LastInputTokens = currentInput
					proc.LastOutputTokens = currentOutput
					proc.LastTokenCheck = time.Now()
				} else if time.Since(proc.LastTokenCheck).Milliseconds() > idleAfter {
					// No token activity for idle_after_ms → idle
					d.debugf("  @%s: active→idle (no token activity for %dms)", agentID, idleAfter)
					db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agentID, agent.Presence, types.PresenceIdle, "idle_timeout", "daemon", agent.Status)
				}
			} else {
				// Token parsing unavailable - fall back to stdout activity detection
				pid := proc.Cmd.Process.Pid
				lastActivity := d.detector.LastActivityTime(pid)
				if time.Since(lastActivity).Milliseconds() > idleAfter {
					db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agentID, agent.Presence, types.PresenceIdle, "idle_timeout", "daemon", agent.Status)
				}
			}

		case types.PresenceIdle:
			// /hop auto-bye: if this is a hop session and agent goes idle, terminate
			if proc.SpawnMode == SpawnModeHop {
				d.debugf("  @%s: hop session went idle, auto-terminating", agentID)
				d.killProcess(agentID, proc, "hop auto-bye: idle")
				continue
			}

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
// Returns true if agent requested BRB respawn (caller should spawn fresh session).
func (d *Daemon) handleProcessExit(agentID string, proc *Process) bool {
	// Check if this proc is still the current one for this agent.
	// A new process may have been spawned, in which case we shouldn't
	// update presence (new process owns that), but we still record session_end
	// for audit trail.
	currentProc := d.processes[agentID]
	isCurrentProc := currentProc == proc

	// For current proc, check handled flag to prevent duplicate session_end.
	// For old proc (isCurrentProc=false), always record - no duplication possible.
	if isCurrentProc && d.handled[agentID] {
		return false
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

	// Capture stderr on error exits for debugging
	if exitCode != 0 && proc.StderrBuffer != nil {
		stderr := strings.TrimSpace(proc.StderrBuffer.String())
		if len(stderr) > 0 {
			sessionEnd.Stderr = &stderr
		}
	}

	db.AppendSessionEnd(d.project.DBPath, sessionEnd)

	// Capture and persist usage snapshot before unwatching
	// This provides durability across transcript rotation and daemon restarts
	if proc.SessionID != "" {
		d.captureUsageSnapshot(agentID, proc.SessionID)
		d.unwatchSessionUsage(proc.SessionID)
		// Clear token cache so fresh sessions don't show stale data
		driver := d.getDriver(agentID)
		if driver != nil {
			ClearTokenCacheForDriver(driver.Name(), proc.SessionID)
		}
	}

	// Session ID is now stored at spawn time (we generate it ourselves with --session-id)
	// No need to detect it from Claude's files anymore - see fix for fray-8ld6

	// Evaluate stdout buffer for valuable content to post
	if proc.StdoutBuffer != nil && isCurrentProc {
		stdout := proc.StdoutBuffer.String()
		if len(strings.TrimSpace(stdout)) > 0 {
			// Get last post from this session for duplicate detection
			var lastPost *string
			if proc.SessionID != "" {
				msgs := d.getSessionMessages(proc.SessionID, agentID, 1)
				if len(msgs) > 0 {
					lastPost = &msgs[len(msgs)-1].Body
				}
			}

			// Run stdout repair
			result := d.runStdoutRepair(stdout, lastPost, agentID)
			if result != nil && result.Post && len(strings.TrimSpace(result.Content)) > 0 {
				d.debugf("stdout-repair: posting %d chars for @%s", len(result.Content), agentID)
				// Create message from stdout
				msgID, _ := core.GenerateGUID("msg-")
				msg := types.Message{
					ID:        msgID,
					TS:        time.Now().UnixMilli(),
					FromAgent: agentID,
					Body:      result.Content,
					Type:      types.MessageTypeAgent,
					Home:      "room", // Post to room
				}
				if proc.SessionID != "" {
					msg.SessionID = &proc.SessionID
				}
				if err := db.AppendMessage(d.project.DBPath, msg); err != nil {
					d.debugf("stdout-repair: post error: %v", err)
				}
			} else if result != nil {
				d.debugf("stdout-repair: skipping - %s", result.Reason)
			}
		}
	}

	// Cleanup process resources
	driver := d.getDriver(agentID)
	if driver != nil {
		driver.Cleanup(proc)
	}

	// Only update presence and remove from map if this is the current process
	shouldRespawnBRB := false
	if isCurrentProc {
		// Check current presence - if agent already ran `fray bye`, presence is offline
		// and we should respect that rather than overwriting with idle
		// If agent ran `fray brb`, presence is brb and we should respawn immediately
		agent, _ := db.GetAgent(d.database, agentID)
		alreadyOffline := agent != nil && agent.Presence == types.PresenceOffline
		wantsBRB := agent != nil && agent.Presence == types.PresenceBRB
		prevPresence := types.PresenceState("")
		if agent != nil {
			prevPresence = agent.Presence
		}

		if wantsBRB {
			// Agent requested immediate respawn via `fray brb`
			d.debugf("@%s: BRB detected - will respawn immediately", agentID)
			shouldRespawnBRB = true
			// Set to idle (normal state) - spawn will set to spawning
			var status *string
			if agent != nil {
				status = agent.Status
			}
			db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agentID, prevPresence, types.PresenceIdle, "brb_exit", "daemon", status)
			// Clear session ID so fresh session starts
			db.UpdateAgentSessionID(d.database, agentID, "")
			// No cooldown for BRB - that's the point
		} else if !alreadyOffline {
			var reason string
			var newPresence types.PresenceState
			if exitCode == 0 {
				reason = "exit_ok"
				// Clean exit without fray bye → idle (resumable via @mention)
				// Only fray bye sets presence to offline (explicit logout)
				newPresence = types.PresenceIdle
				// Set 30s cooldown after clean exit - prevents immediate re-spawn
				// Cooldown is cleared by: fray bye, interrupt syntax, or expiration
				d.cooldownUntil[agentID] = time.Now().Add(30 * time.Second)
				d.debugf("@%s: setting 30s cooldown (expires %s)", agentID, d.cooldownUntil[agentID].Format(time.RFC3339))
			} else if exitCode == -1 {
				// Signal kill (SIGTERM/SIGINT) - treat as idle (resumable)
				// This handles: user Ctrl-C, daemon restart, network interruption
				// Session context on disk is typically still valid after signal
				d.debugf("@%s: exit_code=-1 (signal kill) → idle (resumable)", agentID)
				reason = "signal_kill"
				newPresence = types.PresenceIdle
			} else {
				// Check for likely session resume failure:
				// - Quick failure (< 30s)
				// - Non-zero exit code
				// - Had a session ID (was trying to resume)
				durationSec := time.Since(proc.StartedAt).Seconds()
				if durationSec < 30 && proc.SessionID != "" {
					// Likely resume failure - set to offline so next spawn starts fresh
					d.debugf("@%s: quick failure (%ds, exit=%d) with session %s - likely resume failure, clearing session",
						agentID, int(durationSec), exitCode, proc.SessionID)
					reason = "resume_failure"
					newPresence = types.PresenceOffline
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
		// (only if not already set by fray bye/brb)
		if agent == nil || agent.LeftAt == nil {
			now := time.Now().Unix()
			db.UpdateAgent(d.database, agentID, db.AgentUpdates{
				LeftAt: types.OptionalInt64{Set: true, Value: &now},
			})
		}

		delete(d.processes, agentID)
	}
	return shouldRespawnBRB
}

// getDriver returns the driver for an agent.
func (d *Daemon) getDriver(agentID string) Driver {
	agent, err := db.GetAgent(d.database, agentID)
	if err != nil || agent == nil || agent.Invoke == nil {
		return nil
	}
	return d.drivers[agent.Invoke.Driver]
}

// getAgentResolution resolves an agent address using AAP.
// Returns nil if resolution fails (e.g., agent has no AAP identity).
func (d *Daemon) getAgentResolution(agentID string) *aap.Resolution {
	aapDir, err := core.AAPConfigDir()
	if err != nil {
		return nil
	}

	frayDir := filepath.Dir(d.project.DBPath)
	projectAAPDir := filepath.Join(d.project.Root, ".aap")

	resolver, err := aap.NewResolver(aap.ResolverOpts{
		GlobalRegistry:  aapDir,
		ProjectRegistry: projectAAPDir,
		FrayCompat:      true,
		FrayPath:        frayDir,
	})
	if err != nil {
		return nil
	}

	res, err := resolver.Resolve("@" + agentID)
	if err != nil {
		return nil
	}

	return res
}

// getInterruptInfo extracts interrupt info for an agent from a message body.
// Returns nil if no interrupt syntax found for this agent.
func (d *Daemon) getInterruptInfo(msg types.Message, agentID string) *types.InterruptInfo {
	// Get agent bases for mention validation
	bases, _ := db.GetAgentBases(d.database)
	result := core.ExtractMentionsWithSession(msg.Body, bases)
	if info, ok := result.Interrupts[agentID]; ok {
		return &info
	}
	return nil
}

// handleInterrupt processes an interrupt request for an agent.
// Returns (skipSpawn, clearSession):
// - skipSpawn: true if caller should skip normal spawn logic (noSpawn was set)
// - clearSession: true if !! was used and caller should clear agent.LastSessionID
func (d *Daemon) handleInterrupt(ctx context.Context, agent types.Agent, msg types.Message, info types.InterruptInfo) (bool, bool) {
	d.debugf("    %s: interrupt detected (double=%v, noSpawn=%v)", msg.ID, info.Double, info.NoSpawn)

	// Clear cooldown (interrupts always bypass)
	if _, hasCooldown := d.cooldownUntil[agent.AgentID]; hasCooldown {
		delete(d.cooldownUntil, agent.AgentID)
		d.debugf("    %s: cleared cooldown (interrupt bypass)", msg.ID)
	}

	// Kill running process if any
	d.mu.Lock()
	proc, hasProcess := d.processes[agent.AgentID]
	d.mu.Unlock()

	if hasProcess && proc.Cmd.Process != nil {
		d.debugf("    %s: killing running process (pid %d)", msg.ID, proc.Cmd.Process.Pid)
		proc.Cmd.Process.Kill()
		// Wait for monitorProcess to clean up - give it a moment
		time.Sleep(100 * time.Millisecond)
	}

	// If !! prefix: clear session ID for fresh start
	clearSession := false
	if info.Double {
		d.debugf("    %s: clearing session ID (fresh start)", msg.ID)
		db.UpdateAgentSessionID(d.database, agent.AgentID, "")
		clearSession = true
	}

	// If ! suffix: don't spawn after interrupt
	if info.NoSpawn {
		d.debugf("    %s: noSpawn set, skipping spawn", msg.ID)
		return true, clearSession // Skip normal spawn
	}

	// Normal interrupt: proceed to spawn (caller will handle)
	return false, clearSession
}

// ClearCooldown clears the cooldown for an agent (called from bye command).
func (d *Daemon) ClearCooldown(agentID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.cooldownUntil, agentID)
}
