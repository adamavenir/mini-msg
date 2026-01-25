package daemon

import (
	"context"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/router"
	"github.com/adamavenir/fray/internal/types"
)

// checkMentions looks for new @mentions of an agent.
func (d *Daemon) checkMentions(ctx context.Context, agent types.Agent) {
	// Get messages mentioning this agent since watermark
	watermark := d.debouncer.GetWatermark(agent.AgentID)
	messages, err := d.getMessagesAfter(watermark, agent.AgentID)
	if err != nil {
		// Check if watermark message was pruned/archived
		if watermark != "" && strings.Contains(err.Error(), "message not found") {
			d.debugf("  @%s: watermark %s was pruned, advancing to latest", agent.AgentID, watermark)
			// Get the latest message ID to use as new watermark
			latestMsgs, latestErr := db.GetMessages(d.database, &types.MessageQueryOptions{Limit: 1})
			if latestErr == nil && len(latestMsgs) > 0 {
				newWatermark := latestMsgs[0].ID
				if err := d.debouncer.UpdateWatermark(agent.AgentID, newWatermark); err != nil {
					d.debugf("  @%s: failed to update watermark: %v", agent.AgentID, err)
				} else {
					d.debugf("  @%s: watermark advanced to %s", agent.AgentID, newWatermark)
				}
			}
			return
		}
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

		// Skip @all mentions - they're ambient notifications, not action requests.
		// The mention is tracked (watermark advances) but no spawn is triggered.
		if IsAllMentionOnly(msg, agent.AgentID) {
			d.debugf("    %s: skip (@all only - ambient notification)", msg.ID)
			if !hasQueued && !spawned {
				lastProcessedID = msg.ID
			}
			continue
		}

		// Staleness gate: skip messages older than 20 minutes at daemon startup.
		// This prevents stale mentions from triggering spawns after daemon restarts.
		// Only applies to messages that predate daemon start (not new messages during runtime).
		const stalenessThreshold = 20 * time.Minute
		msgTime := time.Unix(msg.TS, 0)
		if msgTime.Before(d.startedAt.Add(-stalenessThreshold)) {
			d.debugf("    %s: skip (stale - %v before daemon start)", msg.ID, d.startedAt.Sub(msgTime).Round(time.Minute))
			if !hasQueued && !spawned {
				lastProcessedID = msg.ID
			}
			continue
		}

		// Check for interrupt syntax (!@agent, !!@agent, !@agent!, !!@agent!)
		// Interrupts bypass cooldown and can kill running processes
		isInterrupt := false
		if interruptInfo := d.getInterruptInfo(msg, agent.AgentID); interruptInfo != nil {
			skipSpawn, clearSession := d.handleInterrupt(ctx, agent, msg, *interruptInfo)
			// If !! was used, clear local agent's session ID so spawnAgent starts fresh
			if clearSession {
				agent.LastSessionID = nil
			}
			if skipSpawn {
				// noSpawn was set, just advance watermark
				if !hasQueued {
					lastProcessedID = msg.ID
				}
				continue
			}
			// Interrupt handled, proceed to spawn (cooldown cleared, process killed if any)
			isInterrupt = true
		}

		// Check if this is a direct address OR a reply to the agent's message
		// Direct address: @agent at start of message
		// Reply to agent: threaded reply to something the agent wrote
		isDirectAddress := IsDirectAddress(msg, agent.AgentID)
		isReplyToAgent := IsReplyToAgent(d.database, msg, agent.AgentID)

		// Skip reply-to-agent messages if agent already replied to them.
		// This prevents double-spawns when watermark wasn't updated during session.
		if isReplyToAgent && AgentAlreadyReplied(d.database, msg.ID, agent.AgentID) {
			d.debugf("    %s: skip (agent already replied)", msg.ID)
			if !hasQueued && !spawned {
				lastProcessedID = msg.ID
			}
			continue
		}

		// Skip direct addresses if agent already replied to them.
		// Safety net: prevents infinite spawn loops when watermark regresses during DB rebuild.
		if isDirectAddress && AgentAlreadyReplied(d.database, msg.ID, agent.AgentID) {
			d.debugf("    %s: skip (agent already replied to direct address)", msg.ID)
			if !hasQueued && !spawned {
				lastProcessedID = msg.ID
			}
			continue
		}

		// FYI patterns (fyi @agent, cc @agent, etc) are informational - skip entirely
		if IsFYIPattern(msg) {
			d.debugf("    %s: skip (FYI pattern)", msg.ID)
			if !hasQueued && !spawned {
				lastProcessedID = msg.ID
			}
			continue
		}

		// Check if base agent has active job workers - skip spawn to avoid ambiguity
		// Job workers use bracket notation: dev[abc-1], not dot notation
		if isJobAmbiguous, err := db.IsAmbiguousMention(d.database, agent.AgentID); err == nil && isJobAmbiguous {
			d.debugf("    %s: skip (base agent has active job workers - ambiguous)", msg.ID)
			if !hasQueued && !spawned {
				lastProcessedID = msg.ID
			}
			continue
		}

		// Ambiguous mention: not direct address and not reply to agent
		// Route through Haiku to decide if agent should be woken
		// Interrupts (!@agent) are explicit spawn requests, skip router check
		isAmbiguousMention := !isDirectAddress && !isReplyToAgent && !isInterrupt
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
			// Clear cooldown if agent ran `fray bye` (presence is offline)
			// Note: bye command advances watermark past unprocessed mentions,
			// so GetMessagesWithMention won't return them even with cooldown cleared
			if agent.Presence == types.PresenceOffline {
				if _, hasCooldown := d.cooldownUntil[agent.AgentID]; hasCooldown {
					delete(d.cooldownUntil, agent.AgentID)
					d.debugf("    %s: cooldown cleared (agent ran fray bye)", msg.ID)
				}
			}
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
		// Exception: interrupts bypass this (they already killed the process)
		d.mu.RLock()
		_, hasProcess := d.processes[agent.AgentID]
		isSpawning := d.spawning[agent.AgentID]
		d.mu.RUnlock()

		if isSpawning && !isInterrupt {
			// Spawn already in progress - queue to avoid race condition
			d.debugf("    %s: queued (spawn in progress)", msg.ID)
			d.debouncer.QueueMention(agent.AgentID, msg.ID)
			hasQueued = true
			continue
		}

		if hasProcess && !isInterrupt {
			// Agent has running process - queue regardless of presence state
			// (presence may be 'idle' if stdout went quiet, but process is still running)
			d.debugf("    %s: queued (process running)", msg.ID)
			d.debouncer.QueueMention(agent.AgentID, msg.ID)
			hasQueued = true
			continue
		}

		// Token-based activity check: if agent has recent token activity, they're working
		// This survives daemon restarts (unlike process map) and uses 5s idle threshold
		if !isInterrupt && d.isActiveByTokens(agent) {
			d.debugf("    %s: queued (agent active by tokens)", msg.ID)
			d.debouncer.QueueMention(agent.AgentID, msg.ID)
			hasQueued = true
			continue
		}

		// No tracked process - check if presence indicates orphaned state
		isBusyState := agent.Presence == types.PresenceSpawning ||
			agent.Presence == types.PresencePrompting ||
			agent.Presence == types.PresencePrompted ||
			agent.Presence == types.PresenceCompacting ||
			agent.Presence == types.PresenceActive

		if isBusyState {
			// Orphaned state: presence says busy but no tracked process
			// Reset to idle and proceed to spawn
			d.debugf("    %s: orphaned presence detected (was %s), resetting to idle", msg.ID, agent.Presence)
			db.UpdateAgentPresenceWithAudit(d.database, d.project.DBPath, agent.AgentID, agent.Presence, types.PresenceIdle, "orphaned_reset", "daemon", agent.Status)
			agent.Presence = types.PresenceIdle
		}

		// Error state blocks auto-spawn - requires manual recovery via "fray back"
		// Exception: interrupts (!@agent, !!@agent) bypass error state
		if agent.Presence == types.PresenceError && !isInterrupt {
			d.debugf("    %s: skip (agent in error state, needs manual recovery)", msg.ID)
			// Advance watermark to avoid retrying the same message
			if !hasQueued {
				lastProcessedID = msg.ID
			}
			continue
		}

		// Check cooldown - queue mention if in cooldown period
		// Skip for interrupts (they already cleared cooldown in handleInterrupt)
		if !isInterrupt {
			if cooldownExpires, inCooldown := d.cooldownUntil[agent.AgentID]; inCooldown {
				if time.Now().Before(cooldownExpires) {
					remaining := time.Until(cooldownExpires).Round(time.Second)
					d.debugf("    %s: queued (cooldown: %v remaining)", msg.ID, remaining)
					d.debouncer.QueueMention(agent.AgentID, msg.ID)
					// Advance watermark even for queued messages during cooldown.
					// This prevents the same messages from being re-queued on every poll cycle.
					// Trade-off: daemon restart during cooldown may cause some messages to be missed,
					// but this is acceptable since cooldown is short (30s) and restart is rare.
					lastProcessedID = msg.ID
					continue
				}
				// Cooldown expired - clear it and proceed to spawn
				delete(d.cooldownUntil, agent.AgentID)
				d.debugf("    %s: cooldown expired, proceeding to spawn", msg.ID)
			}
		}

		// Direct addresses, chained replies, router-approved mentions, and interrupts spawn
		if isInterrupt {
			d.debugf("    %s: interrupt - triggering spawn", msg.ID)
		} else if isAmbiguousMention {
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

		// Route reaction through haiku to check if it warrants waking
		if d.router.ReactionRouterAvailable() {
			routerResult := d.router.RouteReaction(router.ReactionPayload{
				Emoji:   reaction.Emoji,
				Message: reaction.MessageBody,
				Agent:   agent.AgentID,
			})
			d.debugf("    @%s: reaction router says shouldSpawn=%v (confidence=%.2f)",
				agent.AgentID, routerResult.ShouldSpawn, routerResult.Confidence)

			if !routerResult.ShouldSpawn {
				d.debugf("    @%s: reaction router rejected wake, skipping", agent.AgentID)
				lastProcessedAt = reaction.ReactedAt
				continue
			}
		}

		// No tracked process - check presence to decide whether to spawn
		switch agent.Presence {
		case types.PresenceOffline, types.PresenceIdle, "":
			// Can spawn - reaction approved by router (or no router available)
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

		case types.PresenceSpawning, types.PresencePrompting, types.PresencePrompted, types.PresenceCompacting, types.PresenceActive:
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
