package daemon

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

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
