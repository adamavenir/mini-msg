package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
	mlld "github.com/mlld-lang/mlld/sdk/go"
)

// runCloseQuestionsCommand closes all questions attached to a message.
// Syntax: /close #msg-xyz or /close (uses last selected in open-qs)
func (m *Model) runCloseQuestionsCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("/close requires a message ID (#msg-xyz)")
	}

	// Parse message ID from args
	ref := args[0]
	if strings.HasPrefix(ref, "#") {
		ref = ref[1:]
	}

	// Resolve message GUID from prefix (use ResolveReplyReference logic)
	resolution, err := ResolveReplyReference(m.db, "#"+ref)
	if err != nil {
		return nil, fmt.Errorf("could not resolve message: %v", err)
	}
	if resolution.Kind == ReplyNone {
		return nil, fmt.Errorf("no message found matching: #%s", ref)
	}
	if resolution.Kind == ReplyAmbiguous {
		return nil, fmt.Errorf("ambiguous message reference: #%s", ref)
	}

	messageID := resolution.ReplyTo

	// Find all questions attached to this message
	questions, err := db.GetQuestions(m.db, &types.QuestionQueryOptions{
		AskedIn: &messageID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find questions: %v", err)
	}

	if len(questions) == 0 {
		return nil, fmt.Errorf("no questions found for message #%s", ref)
	}

	// Close all questions
	closed := 0
	for _, q := range questions {
		if q.Status == types.QuestionStatusAnswered {
			continue // Already closed
		}
		status := string(types.QuestionStatusAnswered)
		_, err := db.UpdateQuestion(m.db, q.GUID, db.QuestionUpdates{
			Status: types.OptionalString{Set: true, Value: &status},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to close question: %v", err)
		}

		// Write update to JSONL
		_ = db.AppendQuestionUpdate(m.projectDBPath, db.QuestionUpdateJSONLRecord{
			Type:   "question_update",
			GUID:   q.GUID,
			Status: &status,
		})
		closed++
	}

	if closed == 0 {
		m.status = fmt.Sprintf("All %d questions already closed", len(questions))
	} else {
		m.status = fmt.Sprintf("Closed %d question(s) for #%s", closed, ref)
	}

	// Refresh question counts
	m.refreshQuestionCounts()

	return nil, nil
}

// runMlldScriptCommand runs mlld scripts from .fray/llm/run/ or llm/run/.
// With no args, lists available scripts. With a script name, runs it.
// Scripts in .fray/llm/run/ run from .fray/, scripts in llm/run/ run from project root.
func (m *Model) runMlldScriptCommand(args []string) error {
	frayRunDir := filepath.Join(m.projectRoot, ".fray", "llm", "run")
	projRunDir := filepath.Join(m.projectRoot, "llm", "run")

	// Collect scripts from both locations
	var allScripts []string
	seen := make(map[string]bool)

	if scripts, err := listMlldScripts(frayRunDir); err == nil {
		for _, s := range scripts {
			if !seen[s] {
				seen[s] = true
				allScripts = append(allScripts, s)
			}
		}
	}
	if scripts, err := listMlldScripts(projRunDir); err == nil {
		for _, s := range scripts {
			if !seen[s] {
				seen[s] = true
				allScripts = append(allScripts, s)
			}
		}
	}

	if len(args) == 0 {
		// List scripts
		if len(allScripts) == 0 {
			m.status = "No scripts found (create .fray/llm/run/*.mld or llm/run/*.mld)"
			return nil
		}
		lines := []string{"Available scripts:"}
		for _, name := range allScripts {
			lines = append(lines, "  /run "+name)
		}
		msg := newEventMessage(strings.Join(lines, "\n"))
		m.messages = append(m.messages, msg)
		m.refreshViewport(false)
		return nil
	}

	// Find and run the specified script
	scriptName := args[0]

	// Check fray location first, then project location
	var scriptPath string
	var workingDir string

	frayPath := filepath.Join(frayRunDir, scriptName+".mld")
	projPath := filepath.Join(projRunDir, scriptName+".mld")

	if _, err := os.Stat(frayPath); err == nil {
		scriptPath = frayPath
		workingDir = filepath.Join(m.projectRoot, ".fray")
	} else if _, err := os.Stat(projPath); err == nil {
		scriptPath = projPath
		workingDir = m.projectRoot
	} else {
		return fmt.Errorf("script not found: %s", scriptName)
	}

	// Execute with mlld using the SDK
	client := mlld.New()
	client.Timeout = 5 * time.Minute
	client.WorkingDir = workingDir

	m.status = fmt.Sprintf("Running %s...", scriptName)

	result, err := client.Execute(scriptPath, nil, nil)
	if err != nil {
		return fmt.Errorf("script error: %v", err)
	}

	// Display output - parse structured output like the CLI does
	output := extractMlldCleanOutput(result)
	if output != "" {
		msg := newEventMessage(fmt.Sprintf("[%s]\n%s", scriptName, output))
		m.messages = append(m.messages, msg)
	}

	m.status = fmt.Sprintf("Ran %s", scriptName)
	m.refreshViewport(false)
	m.input.SetValue("")
	return nil
}

// listMlldScripts returns names of .mld files in the given directory.
func listMlldScripts(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var scripts []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".mld") {
			scripts = append(scripts, strings.TrimSuffix(name, ".mld"))
		}
	}
	return scripts, nil
}

// extractMlldCleanOutput parses the SDK result and returns clean output.
// The mlld CLI outputs progress/errors to stdout before the JSON, so we may
// need to extract the JSON portion and parse it.
func extractMlldCleanOutput(result *mlld.ExecuteResult) string {
	// If we have effects, extract content from them (structured output worked)
	if len(result.Effects) > 0 {
		var outputLines []string
		for _, effect := range result.Effects {
			content := strings.TrimSpace(effect.Content)
			if content != "" {
				outputLines = append(outputLines, content)
			}
		}

		// Also include result.Output if meaningful and not duplicated
		mainOutput := strings.TrimSpace(result.Output)
		if mainOutput != "" {
			joined := strings.Join(outputLines, "\n")
			if !strings.Contains(joined, mainOutput) {
				outputLines = append([]string{mainOutput}, outputLines...)
			}
		}

		return strings.Join(outputLines, "\n")
	}

	// If result.Output contains mixed content (raw output with JSON at the end),
	// try to extract just the meaningful parts
	rawOutput := result.Output

	// Try to find JSON at the end and extract the pre-JSON content
	if jsonStart := strings.LastIndex(rawOutput, "\n{"); jsonStart != -1 {
		preJSON := strings.TrimSpace(rawOutput[:jsonStart])
		if preJSON != "" {
			return preJSON
		}
	}

	// Fall back to returning trimmed raw output
	return strings.TrimSpace(rawOutput)
}

// runByeCommand sends bye for a specific agent.
// Syntax: /bye @agent [message]
func (m *Model) runByeCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /bye @agent [message]")
	}

	// Parse agent ID (strip @ prefix if present)
	agentRef := args[0]
	agentID := strings.TrimPrefix(agentRef, "@")
	if agentID == "" {
		return nil, fmt.Errorf("usage: /bye @agent [message]")
	}

	// Optional message
	message := ""
	if len(args) > 1 {
		message = strings.Join(args[1:], " ")
	}

	// Get agent from database
	agent, err := db.GetAgent(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: @%s", agentID)
	}

	now := time.Now().Unix()
	nowMs := time.Now().UnixMilli()

	// Clear claims
	clearedClaims, err := db.DeleteClaimsByAgent(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to clear claims: %w", err)
	}

	// Clear session roles
	sessionRoles, err := db.GetSessionRoles(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session roles: %w", err)
	}
	clearedRoles, err := db.ClearSessionRoles(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to clear roles: %w", err)
	}
	for _, role := range sessionRoles {
		if err := db.AppendRoleStop(m.projectDBPath, agentID, role.RoleName, nowMs); err != nil {
			return nil, fmt.Errorf("failed to persist role stop: %w", err)
		}
	}

	// Handle wake condition lifecycle on bye
	clearedWake, err := db.ClearPersistUntilByeConditions(m.db, m.projectDBPath, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to clear wake conditions: %w", err)
	}
	pausedWake, err := db.PauseWakeConditions(m.db, m.projectDBPath, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to pause wake conditions: %w", err)
	}

	// Post optional message
	var posted *types.Message
	if message != "" {
		bases, err := db.GetAgentBases(m.db)
		if err != nil {
			return nil, err
		}
		mentions := core.ExtractMentions(message, bases)
		mentions = core.ExpandAllMention(mentions, bases)
		created, err := db.CreateMessage(m.db, types.Message{
			TS:        now,
			FromAgent: agentID,
			Body:      message,
			Mentions:  mentions,
		})
		if err != nil {
			return nil, err
		}
		if err := db.AppendMessage(m.projectDBPath, created); err != nil {
			return nil, err
		}
		posted = &created
	}

	// Post leave event
	eventMsg, err := db.CreateMessage(m.db, types.Message{
		TS:        now,
		FromAgent: agentID,
		Body:      fmt.Sprintf("@%s left", agentID),
		Type:      types.MessageTypeEvent,
	})
	if err != nil {
		return nil, err
	}
	if err := db.AppendMessage(m.projectDBPath, eventMsg); err != nil {
		return nil, err
	}

	// Update agent
	updates := db.AgentUpdates{
		LeftAt:   types.OptionalInt64{Set: true, Value: &now},
		LastSeen: types.OptionalInt64{Set: true, Value: &now},
		Status:   types.OptionalString{Set: true, Value: nil},
	}
	if err := db.UpdateAgent(m.db, agentID, updates); err != nil {
		return nil, err
	}

	// For managed agents, set presence to offline
	if agent.Managed {
		if err := db.UpdateAgentPresenceWithAudit(m.db, m.projectDBPath, agentID, agent.Presence, types.PresenceOffline, "bye", "chat", agent.Status); err != nil {
			return nil, err
		}
	}

	// Persist agent update
	updated, err := db.GetAgent(m.db, agentID)
	if err != nil {
		return nil, err
	}
	if updated != nil {
		if err := db.AppendAgent(m.projectDBPath, *updated); err != nil {
			return nil, err
		}
	}

	// Build status message
	var parts []string
	parts = append(parts, fmt.Sprintf("@%s left", agentID))
	if posted != nil {
		parts = append(parts, fmt.Sprintf("posted [%s]", posted.ID))
	}
	if clearedClaims > 0 {
		parts = append(parts, fmt.Sprintf("%d claims cleared", clearedClaims))
	}
	if clearedRoles > 0 {
		parts = append(parts, fmt.Sprintf("%d roles cleared", clearedRoles))
	}
	if clearedWake > 0 {
		parts = append(parts, fmt.Sprintf("%d wake cleared", clearedWake))
	}
	if pausedWake > 0 {
		parts = append(parts, fmt.Sprintf("%d wake paused", pausedWake))
	}

	m.status = strings.Join(parts, ", ")
	m.input.SetValue("")
	return nil, nil
}

// runFlyCommand spawns an offline agent with /fly skill context.
// Syntax: /fly @agent [message]
func (m *Model) runFlyCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /fly @agent [message]")
	}

	agentRef := args[0]
	agentID := strings.TrimPrefix(agentRef, "@")
	if agentID == "" {
		return nil, fmt.Errorf("usage: /fly @agent [message]")
	}

	// Get agent from database
	agent, err := db.GetAgent(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: @%s", agentID)
	}

	// State guard: agent must be offline
	if agent.Presence != types.PresenceOffline && agent.Presence != "" {
		return nil, fmt.Errorf("@%s is %s - run /bye @%s first", agentID, agent.Presence, agentID)
	}

	// Build trigger message
	now := time.Now().Unix()
	userMessage := ""
	if len(args) > 1 {
		userMessage = strings.Join(args[1:], " ")
	}

	// Post the trigger message: "@agent /fly"
	bases, err := db.GetAgentBases(m.db)
	if err != nil {
		return nil, err
	}
	triggerBody := fmt.Sprintf("@%s /fly", agentID)
	mentions := core.ExtractMentions(triggerBody, bases)
	mentions = core.ExpandAllMention(mentions, bases)

	triggerMsg, err := db.CreateMessage(m.db, types.Message{
		TS:        now,
		FromAgent: m.username,
		Body:      triggerBody,
		Mentions:  mentions,
		Type:      types.MessageTypeUser,
	})
	if err != nil {
		return nil, err
	}
	if err := db.AppendMessage(m.projectDBPath, triggerMsg); err != nil {
		return nil, err
	}

	// Post optional user message as a separate message that can be replied to
	if userMessage != "" {
		userMsgMentions := core.ExtractMentions(userMessage, bases)
		userMsgMentions = core.ExpandAllMention(userMsgMentions, bases)
		userMsg, err := db.CreateMessage(m.db, types.Message{
			TS:        now,
			FromAgent: m.username,
			Body:      userMessage,
			Mentions:  userMsgMentions,
			Type:      types.MessageTypeUser,
		})
		if err != nil {
			return nil, err
		}
		if err := db.AppendMessage(m.projectDBPath, userMsg); err != nil {
			return nil, err
		}
	}

	m.status = fmt.Sprintf("/fly @%s - daemon will spawn", agentID)
	m.input.SetValue("")

	// Reload messages to show the trigger
	if err := m.reloadMessages(); err != nil {
		return nil, err
	}

	return nil, nil
}

// runHopCommand spawns an offline agent with /hop skill context (auto-bye on idle).
// Syntax: /hop @agent [message]
func (m *Model) runHopCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /hop @agent [message]")
	}

	agentRef := args[0]
	agentID := strings.TrimPrefix(agentRef, "@")
	if agentID == "" {
		return nil, fmt.Errorf("usage: /hop @agent [message]")
	}

	// Get agent from database
	agent, err := db.GetAgent(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: @%s", agentID)
	}

	// State guard: agent must be offline or idle
	if agent.Presence != types.PresenceOffline && agent.Presence != types.PresenceIdle && agent.Presence != "" {
		return nil, fmt.Errorf("@%s is %s - run /bye @%s first", agentID, agent.Presence, agentID)
	}

	// Build trigger message
	now := time.Now().Unix()
	userMessage := ""
	if len(args) > 1 {
		userMessage = strings.Join(args[1:], " ")
	}

	// Post the trigger message: "@agent /hop"
	bases, err := db.GetAgentBases(m.db)
	if err != nil {
		return nil, err
	}
	triggerBody := fmt.Sprintf("@%s /hop", agentID)
	mentions := core.ExtractMentions(triggerBody, bases)
	mentions = core.ExpandAllMention(mentions, bases)

	triggerMsg, err := db.CreateMessage(m.db, types.Message{
		TS:        now,
		FromAgent: m.username,
		Body:      triggerBody,
		Mentions:  mentions,
		Type:      types.MessageTypeUser,
	})
	if err != nil {
		return nil, err
	}
	if err := db.AppendMessage(m.projectDBPath, triggerMsg); err != nil {
		return nil, err
	}

	// Post optional user message as a separate message that can be replied to
	if userMessage != "" {
		userMsgMentions := core.ExtractMentions(userMessage, bases)
		userMsgMentions = core.ExpandAllMention(userMsgMentions, bases)
		userMsg, err := db.CreateMessage(m.db, types.Message{
			TS:        now,
			FromAgent: m.username,
			Body:      userMessage,
			Mentions:  userMsgMentions,
			Type:      types.MessageTypeUser,
		})
		if err != nil {
			return nil, err
		}
		if err := db.AppendMessage(m.projectDBPath, userMsg); err != nil {
			return nil, err
		}
	}

	m.status = fmt.Sprintf("/hop @%s - daemon will spawn (auto-bye on idle)", agentID)
	m.input.SetValue("")

	// Reload messages to show the trigger
	if err := m.reloadMessages(); err != nil {
		return nil, err
	}

	return nil, nil
}

// runLandCommand asks an active/idle agent to run /land closeout.
// Syntax: /land @agent
func (m *Model) runLandCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /land @agent")
	}

	agentRef := args[0]
	agentID := strings.TrimPrefix(agentRef, "@")
	if agentID == "" {
		return nil, fmt.Errorf("usage: /land @agent")
	}

	// Get agent from database
	agent, err := db.GetAgent(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: @%s", agentID)
	}

	// State guard: agent must be active or idle (has a running session)
	if agent.Presence != types.PresenceActive && agent.Presence != types.PresenceIdle &&
		agent.Presence != types.PresencePrompting && agent.Presence != types.PresencePrompted {
		return nil, fmt.Errorf("@%s is %s - nothing to land", agentID, agent.Presence)
	}

	// Post the trigger message: "@agent /land"
	now := time.Now().Unix()
	bases, err := db.GetAgentBases(m.db)
	if err != nil {
		return nil, err
	}
	triggerBody := fmt.Sprintf("@%s /land", agentID)
	mentions := core.ExtractMentions(triggerBody, bases)
	mentions = core.ExpandAllMention(mentions, bases)

	triggerMsg, err := db.CreateMessage(m.db, types.Message{
		TS:        now,
		FromAgent: m.username,
		Body:      triggerBody,
		Mentions:  mentions,
		Type:      types.MessageTypeUser,
	})
	if err != nil {
		return nil, err
	}
	if err := db.AppendMessage(m.projectDBPath, triggerMsg); err != nil {
		return nil, err
	}

	m.status = fmt.Sprintf("/land @%s - asked to run /land", agentID)
	m.input.SetValue("")

	// Reload messages to show the trigger
	if err := m.reloadMessages(); err != nil {
		return nil, err
	}

	return nil, nil
}
