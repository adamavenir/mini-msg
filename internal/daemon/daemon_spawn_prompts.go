package daemon

import (
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

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
