package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/llm"
	"github.com/adamavenir/fray/internal/router"
	"github.com/adamavenir/fray/internal/types"
)

// checkWakeConditions checks pattern, timer, and on-mention wake conditions.
func (d *Daemon) checkWakeConditions(ctx context.Context, agents []types.Agent) {
	// Build agent map for quick lookup
	agentMap := make(map[string]types.Agent)
	for _, agent := range agents {
		agentMap[agent.AgentID] = agent
	}

	// Get all active wake conditions
	conditions, err := db.GetWakeConditions(d.database, "")
	if err != nil {
		d.debugf("wake: error getting wake conditions: %v", err)
		return
	}

	if len(conditions) == 0 {
		return
	}

	d.debugf("wake: checking %d wake conditions", len(conditions))

	// Check each condition
	for _, cond := range conditions {
		agent, ok := agentMap[cond.AgentID]
		if !ok {
			d.debugf("wake: condition for non-managed agent @%s, skipping", cond.AgentID)
			continue
		}

		// Only wake idle/offline agents
		if agent.Presence != types.PresenceIdle && agent.Presence != types.PresenceOffline {
			d.debugf("wake: @%s not idle/offline (presence: %s), skipping", agent.AgentID, agent.Presence)
			continue
		}

		triggered, triggerMsg := d.checkWakeCondition(ctx, cond, agent)
		if !triggered {
			continue
		}

		d.debugf("wake: condition %s triggered for @%s", cond.GUID, agent.AgentID)

		// Handle condition based on persist mode
		switch cond.PersistMode {
		case types.WakePersist, types.WakePersistUntilBye, types.WakePersistRestoreOnBack:
			// Persistent condition: don't delete, just update created_at for timer conditions
			if cond.Type == types.WakeConditionAfter && cond.AfterMs != nil {
				// Reset the timer for next trigger
				newExpiresAt := time.Now().Unix() + (*cond.AfterMs / 1000)
				if err := db.ResetTimerCondition(d.database, d.project.DBPath, cond.GUID, newExpiresAt); err != nil {
					d.debugf("wake: error resetting timer condition %s: %v", cond.GUID, err)
				}
			}
			d.debugf("wake: condition %s is persistent (%s), keeping active", cond.GUID, cond.PersistMode)
		default:
			// One-shot: delete the wake condition
			if err := db.DeleteWakeCondition(d.database, d.project.DBPath, cond.GUID); err != nil {
				d.debugf("wake: error deleting condition %s: %v", cond.GUID, err)
			}
		}

		// Spawn the agent
		d.mu.Lock()
		d.spawning[agent.AgentID] = true
		d.mu.Unlock()

		_, err := d.spawnAgent(ctx, agent, triggerMsg)

		d.mu.Lock()
		delete(d.spawning, agent.AgentID)
		d.mu.Unlock()

		if err != nil {
			d.debugf("wake: @%s spawn error: %v", agent.AgentID, err)
		}
	}
}

// checkWakeCondition checks if a specific wake condition is triggered.
// Returns true and the trigger message ID if triggered.
func (d *Daemon) checkWakeCondition(ctx context.Context, cond types.WakeCondition, agent types.Agent) (bool, string) {
	switch cond.Type {
	case types.WakeConditionAfter:
		// Timer-based: check if expired
		if cond.ExpiresAt != nil && time.Now().Unix() >= *cond.ExpiresAt {
			d.debugf("wake: timer condition expired for @%s", agent.AgentID)
			return true, ""
		}
		return false, ""

	case types.WakeConditionOnMention:
		// Check if any watched agent posted since condition was created
		return d.checkOnMentionWake(cond, agent)

	case types.WakeConditionPattern:
		// Check for pattern matches in recent messages
		return d.checkPatternWake(ctx, cond, agent)

	case types.WakeConditionPrompt:
		// LLM-evaluated condition with polling
		return d.checkPromptWake(ctx, cond, agent)

	default:
		d.debugf("wake: unknown condition type %s", cond.Type)
		return false, ""
	}
}

// checkOnMentionWake checks if any watched agent has posted.
func (d *Daemon) checkOnMentionWake(cond types.WakeCondition, agent types.Agent) (bool, string) {
	if len(cond.OnAgents) == 0 {
		return false, ""
	}

	// Get recent messages since condition was created
	opts := &types.MessageQueryOptions{
		Limit: 50,
	}

	// Scope to thread if specified
	if cond.InThread != nil {
		opts.Home = cond.InThread
	}

	messages, err := db.GetMessages(d.database, opts)
	if err != nil {
		d.debugf("wake: error getting messages for on-mention check: %v", err)
		return false, ""
	}

	// Check if any message is from a watched agent and after condition creation
	for _, msg := range messages {
		if msg.TS <= cond.CreatedAt {
			continue
		}

		// Skip meta/ unless explicitly scoped
		if cond.InThread == nil && len(msg.Home) >= 5 && msg.Home[:5] == "meta/" {
			continue
		}

		// Check if from a watched agent
		for _, watchedAgent := range cond.OnAgents {
			if msg.FromAgent == watchedAgent || strings.HasPrefix(msg.FromAgent, watchedAgent+".") {
				d.debugf("wake: @%s posted (watched by @%s)", msg.FromAgent, agent.AgentID)
				return true, msg.ID
			}
		}
	}

	return false, ""
}

// checkPatternWake checks for pattern matches in recent messages.
func (d *Daemon) checkPatternWake(ctx context.Context, cond types.WakeCondition, agent types.Agent) (bool, string) {
	if cond.Pattern == nil {
		return false, ""
	}

	// Compile the pattern
	compiled := cond.CompilePattern()
	if compiled == nil {
		d.debugf("wake: failed to compile pattern for @%s", agent.AgentID)
		return false, ""
	}

	// Get recent messages since condition was created
	opts := &types.MessageQueryOptions{
		Limit: 50,
	}

	// Scope to thread if specified
	if cond.InThread != nil {
		opts.Home = cond.InThread
	}

	messages, err := db.GetMessages(d.database, opts)
	if err != nil {
		d.debugf("wake: error getting messages for pattern check: %v", err)
		return false, ""
	}

	// Check each message against the pattern
	for _, msg := range messages {
		if msg.TS <= cond.CreatedAt {
			continue
		}

		// Skip meta/ unless explicitly scoped
		if cond.InThread == nil && !cond.MatchesThread(msg.Home) {
			continue
		}

		// Check pattern match
		if !compiled.MatchesMessage(msg.Body) {
			continue
		}

		d.debugf("wake: pattern matched for @%s in msg %s", agent.AgentID, msg.ID)

		// If router enabled, assess with haiku
		if cond.UseRouter {
			shouldWake := d.assessWakeWithRouter(ctx, cond, msg, agent)
			if !shouldWake {
				d.debugf("wake: router rejected wake for @%s", agent.AgentID)
				continue
			}
		}

		return true, msg.ID
	}

	return false, ""
}

// assessWakeWithRouter uses the wake-router.mld template to assess if agent should wake.
func (d *Daemon) assessWakeWithRouter(ctx context.Context, cond types.WakeCondition, msg types.Message, agent types.Agent) bool {
	// Check if wake-router.mld exists
	wakeRouterPath := filepath.Join(d.project.Root, ".fray", "llm", "wake-router.mld")
	if _, err := os.Stat(wakeRouterPath); os.IsNotExist(err) {
		d.debugf("wake: wake-router.mld not found, defaulting to wake")
		return true
	}

	// Build payload for wake router
	payload := types.WakeRouterPayload{
		Message: msg.Body,
		From:    msg.FromAgent,
		Agent:   agent.AgentID,
		Pattern: *cond.Pattern,
	}
	if msg.Home != "room" {
		payload.Thread = &msg.Home
	}

	// Use existing router infrastructure
	result := d.router.Route(router.RouterPayload{
		Message: msg.Body,
		From:    msg.FromAgent,
		Agent:   agent.AgentID,
		Thread:  payload.Thread,
	})

	// Use router's shouldSpawn decision
	return result.ShouldSpawn
}

// checkPromptWake evaluates LLM-based prompt conditions with polling.
func (d *Daemon) checkPromptWake(ctx context.Context, cond types.WakeCondition, agent types.Agent) (bool, string) {
	if cond.PromptText == nil || cond.PollIntervalMs == nil {
		return false, ""
	}

	// Check if poll interval has elapsed since last check
	now := time.Now().UnixMilli()
	if cond.LastPolledAt != nil {
		elapsed := now - *cond.LastPolledAt
		if elapsed < *cond.PollIntervalMs {
			// Not time to poll yet
			return false, ""
		}
	}

	// Update last_polled_at timestamp
	if err := db.UpdateLastPolledAt(d.database, cond.GUID, now); err != nil {
		d.debugf("wake: error updating last_polled_at for %s: %v", cond.GUID, err)
	}

	d.debugf("wake: evaluating prompt condition for @%s", agent.AgentID)

	// Gather agent statuses for context
	agents, err := d.getManagedAgents()
	if err != nil {
		d.debugf("wake: error getting agents for prompt eval: %v", err)
		return false, ""
	}

	var agentStatuses []types.AgentStatusForPrompt
	for _, a := range agents {
		idleSeconds := int64(0)
		if a.LastSeen > 0 {
			idleSeconds = (time.Now().Unix() - a.LastSeen)
		}
		agentStatuses = append(agentStatuses, types.AgentStatusForPrompt{
			Name:        a.AgentID,
			Presence:    string(a.Presence),
			Status:      a.Status,
			IdleSeconds: idleSeconds,
		})
	}

	// Use wake-prompt.mld to evaluate
	shouldWake := d.evaluatePromptCondition(ctx, cond, agent, agentStatuses)
	if shouldWake {
		d.debugf("wake: prompt condition triggered for @%s", agent.AgentID)
		return true, ""
	}

	return false, ""
}

// evaluatePromptCondition runs the wake-prompt.mld template.
func (d *Daemon) evaluatePromptCondition(ctx context.Context, cond types.WakeCondition, agent types.Agent, agentStatuses []types.AgentStatusForPrompt) bool {
	// Check if wake-prompt.mld exists
	wakePromptPath := filepath.Join(d.project.Root, ".fray", "llm", "wake-prompt.mld")
	if _, err := os.Stat(wakePromptPath); os.IsNotExist(err) {
		// Try to copy the default template
		if err := d.ensureWakePromptTemplate(); err != nil {
			d.debugf("wake: wake-prompt.mld not found and couldn't create default: %v", err)
			return false
		}
	}

	// Build payload
	payload := types.WakePromptPayload{
		Agent:    agent.AgentID,
		Prompt:   *cond.PromptText,
		Agents:   agentStatuses,
		InThread: cond.InThread,
	}

	// Run the mlld script
	result, err := d.runWakePrompt(payload)
	if err != nil {
		d.debugf("wake: error running wake-prompt.mld: %v", err)
		return false
	}

	d.debugf("wake: prompt eval result for @%s: shouldWake=%v, reason=%s, confidence=%.2f",
		agent.AgentID, result.ShouldWake, result.Reason, result.Confidence)

	return result.ShouldWake
}

// ensureWakePromptTemplate creates the wake-prompt.mld template if it doesn't exist.
func (d *Daemon) ensureWakePromptTemplate() error {
	llmDir := filepath.Join(d.project.Root, ".fray", "llm")
	if err := os.MkdirAll(llmDir, 0755); err != nil {
		return err
	}

	wakePromptPath := filepath.Join(llmDir, "wake-prompt.mld")
	if _, err := os.Stat(wakePromptPath); os.IsNotExist(err) {
		content, err := llm.ReadTemplate(llm.WakePromptTemplate)
		if err != nil {
			return err
		}
		return os.WriteFile(wakePromptPath, content, 0644)
	}
	return nil
}

// runWakePrompt executes the wake-prompt.mld template with the given payload.
func (d *Daemon) runWakePrompt(payload types.WakePromptPayload) (*types.WakePromptResult, error) {
	// Encode payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	// Run mlld with --payload flag
	wakePromptPath := filepath.Join(d.project.Root, ".fray", "llm", "wake-prompt.mld")
	cmd := exec.Command("mlld", "--payload", fmt.Sprintf("@payload=%s", string(payloadJSON)), wakePromptPath)
	cmd.Dir = d.project.Root

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("mlld failed: %w", err)
	}

	// Parse result
	var result types.WakePromptResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse mlld output: %w", err)
	}

	return &result, nil
}
