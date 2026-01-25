package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/adamavenir/fray/internal/usage"
)

// acquireLock creates the lock file, detecting stale locks.
// If force is set, kills any existing daemon first.
func (d *Daemon) acquireLock() error {
	// Check for existing lock
	if data, err := os.ReadFile(d.lockPath); err == nil {
		var info LockInfo
		if json.Unmarshal(data, &info) == nil {
			// Check if process is still running using syscall.Kill with signal 0
			if syscall.Kill(info.PID, 0) == nil {
				if d.force {
					// Kill existing daemon
					d.debugf("killing existing daemon (pid %d)", info.PID)
					if err := syscall.Kill(info.PID, syscall.SIGTERM); err != nil {
						return fmt.Errorf("failed to kill existing daemon (pid %d): %w", info.PID, err)
					}
					// Wait briefly for it to exit
					for i := 0; i < 10; i++ {
						time.Sleep(100 * time.Millisecond)
						if syscall.Kill(info.PID, 0) != nil {
							break // Process exited
						}
					}
					// If still running, force kill
					if syscall.Kill(info.PID, 0) == nil {
						d.debugf("daemon didn't respond to SIGTERM, sending SIGKILL")
						syscall.Kill(info.PID, syscall.SIGKILL)
						time.Sleep(100 * time.Millisecond)
					}
				} else {
					return fmt.Errorf("daemon already running (pid %d)", info.PID)
				}
			}
			// Stale lock (or we just killed it), remove it
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
		msg := fmt.Sprintf(format, args...)
		if msg == d.lastDebugMsg {
			d.lastDebugCount++
			// Overwrite the line with updated count using carriage return
			fmt.Fprintf(os.Stderr, "\r[daemon] %s (%d)", msg, d.lastDebugCount)
		} else {
			// Flush previous repeated message with newline if needed
			if d.lastDebugCount > 1 {
				fmt.Fprintf(os.Stderr, "\n")
			}
			d.lastDebugMsg = msg
			d.lastDebugCount = 1
			fmt.Fprintf(os.Stderr, "[daemon] %s\n", msg)
		}
	}
}

// usageWatchLoop handles usage watcher events.
func (d *Daemon) usageWatchLoop() {
	defer d.wg.Done()

	for {
		select {
		case <-d.stopCh:
			return

		case event, ok := <-d.usageWatcher.Events():
			if !ok {
				return
			}
			d.handleUsageEvent(event)

		case err, ok := <-d.usageWatcher.Errors():
			if !ok {
				return
			}
			d.debugf("usage watcher error: %v", err)
		}
	}
}

// handleUsageEvent processes a usage change event.
func (d *Daemon) handleUsageEvent(event usage.UsageEvent) {
	d.debugf("usage event: session=%s tokens=%d delta=%d",
		event.SessionID, event.Usage.InputTokens, event.TokensDelta)

	// Find the agent with this session ID
	d.mu.RLock()
	var agentID string
	for id, proc := range d.processes {
		if proc.SessionID == event.SessionID {
			agentID = id
			break
		}
	}
	d.mu.RUnlock()

	if agentID == "" {
		return
	}

	// Update agent's usage in the database (could be used for status reporting)
	// For now, just log it. Future: emit to a channel or update DB.
	d.debugf("agent %s: context at %d%% (%d/%d tokens)",
		agentID,
		event.Usage.ContextPercent,
		event.Usage.InputTokens,
		event.Usage.ContextLimit)
}

// watchSessionUsage starts watching a session's transcript for usage updates.
func (d *Daemon) watchSessionUsage(sessionID, driver string) {
	if d.usageWatcher == nil {
		return
	}

	if err := d.usageWatcher.WatchSession(sessionID, driver); err != nil {
		d.debugf("failed to watch session %s: %v", sessionID, err)
	}
}

// unwatchSessionUsage stops watching a session's transcript.
func (d *Daemon) unwatchSessionUsage(sessionID string) {
	if d.usageWatcher == nil {
		return
	}

	d.usageWatcher.UnwatchSession(sessionID)
}

// captureUsageSnapshot persists the final usage state to agents.jsonl.
// This provides durability: if transcripts are rotated or the daemon restarts,
// the last known token counts are still available for display.
func (d *Daemon) captureUsageSnapshot(agentID, sessionID string) {
	// Get the last known usage from the watcher (before unwatching)
	var sessionUsage *usage.SessionUsage
	if d.usageWatcher != nil {
		sessionUsage = d.usageWatcher.GetSessionUsageSnapshot(sessionID)
	}

	// If watcher doesn't have it, try direct fetch
	if sessionUsage == nil {
		agent, err := db.GetAgent(d.database, agentID)
		if err != nil || agent == nil || agent.Invoke == nil {
			return
		}
		sessionUsage, _ = usage.GetSessionUsageByDriver(sessionID, agent.Invoke.Driver)
	}

	// Skip if no meaningful usage data
	if sessionUsage == nil || (sessionUsage.InputTokens == 0 && sessionUsage.OutputTokens == 0) {
		return
	}

	snapshot := types.UsageSnapshot{
		AgentID:        agentID,
		SessionID:      sessionID,
		Driver:         sessionUsage.Driver,
		Model:          sessionUsage.Model,
		InputTokens:    sessionUsage.InputTokens,
		OutputTokens:   sessionUsage.OutputTokens,
		CachedTokens:   sessionUsage.CachedTokens,
		ContextLimit:   sessionUsage.ContextLimit,
		ContextPercent: sessionUsage.ContextPercent,
		CapturedAt:     time.Now().Unix(),
	}

	if err := db.AppendUsageSnapshot(d.project.DBPath, snapshot); err != nil {
		d.debugf("failed to persist usage snapshot for @%s: %v", agentID, err)
	} else {
		d.debugf("persisted usage snapshot for @%s: %d input, %d output, %d%% context",
			agentID, snapshot.InputTokens, snapshot.OutputTokens, snapshot.ContextPercent)
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
