package daemon

import (
	"context"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

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
