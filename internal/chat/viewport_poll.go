package chat

import (
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
)

const pollInterval = time.Second
const activityPollInterval = 250 * time.Millisecond

type pollMsg struct {
	roomMessages    []types.Message
	threadMessages  []types.Message
	threadID        string
	questions       []types.Question
	threads         []types.Thread
	mentionMessages []types.Message
	managedAgents   []types.Agent
	agentTokenUsage map[string]*TokenUsage
}

type activityPollMsg struct {
	managedAgents   []types.Agent
	agentTokenUsage map[string]*TokenUsage
	daemonStartedAt int64
}

func (m *Model) pollCmd() tea.Cmd {
	cursor := m.lastCursor
	mentionCursor := m.lastMentionCursor
	username := m.username
	includeArchived := m.includeArchived
	showUpdates := m.showUpdates
	currentThread := m.currentThread
	currentPseudo := m.currentPseudo
	projectDBPath := m.projectDBPath

	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		options := types.MessageQueryOptions{Since: cursor, IncludeArchived: includeArchived}
		roomMessages, err := db.GetMessages(m.db, &options)
		if err != nil {
			return errMsg{err: err}
		}
		roomMessages, err = db.ApplyMessageEditCounts(m.projectDBPath, roomMessages)
		if err != nil {
			return errMsg{err: err}
		}
		roomMessages = filterUpdates(roomMessages, showUpdates)

		threadID := ""
		threadMessages := []types.Message(nil)
		if currentThread != nil {
			threadID = currentThread.GUID
			threadMessages, err = db.GetThreadMessages(m.db, currentThread.GUID)
			if err != nil {
				return errMsg{err: err}
			}
			threadMessages, err = db.ApplyMessageEditCounts(m.projectDBPath, threadMessages)
			if err != nil {
				return errMsg{err: err}
			}
			threadMessages = filterUpdates(threadMessages, showUpdates)
		}

		var questions []types.Question
		if currentPseudo != "" {
			roomOnly := true
			var threadGUID *string
			if currentThread != nil {
				roomOnly = false
				threadGUID = &currentThread.GUID
			}
			query := types.QuestionQueryOptions{
				ThreadGUID: threadGUID,
				RoomOnly:   roomOnly,
			}
			switch currentPseudo {
			case pseudoThreadOpen:
				query.Statuses = []types.QuestionStatus{types.QuestionStatusOpen}
			case pseudoThreadClosed:
				query.Statuses = []types.QuestionStatus{types.QuestionStatusAnswered}
			case pseudoThreadWonder:
				query.Statuses = []types.QuestionStatus{types.QuestionStatusUnasked}
			case pseudoThreadStale:
				query.Statuses = []types.QuestionStatus{types.QuestionStatusOpen}
			}
			questions, err = db.GetQuestions(m.db, &query)
			if err != nil {
				return errMsg{err: err}
			}
			if currentPseudo == pseudoThreadStale {
				cutoff := time.Now().Unix() - questionStaleSeconds
				filtered := make([]types.Question, 0, len(questions))
				for _, question := range questions {
					if question.CreatedAt > 0 && question.CreatedAt < cutoff {
						filtered = append(filtered, question)
					}
				}
				questions = filtered
			}
		}

		// Fetch thread list for live updates
		threads, err := db.GetThreads(m.db, &types.ThreadQueryOptions{})
		if err != nil {
			threads = nil // Don't fail the poll, just return empty
		}

		// Fetch mentions from all threads for notifications
		var mentionMessages []types.Message
		if username != "" {
			allHomes := "" // empty string = all homes (room + threads)
			mentionOpts := &types.MessageQueryOptions{
				Since:                 mentionCursor,
				IncludeArchived:       false,
				Home:                  &allHomes,
				IncludeRepliesToAgent: username,
			}
			mentionMessages, _ = db.GetMessagesWithMention(m.db, username, mentionOpts)
		}

		// Fetch managed agents for activity panel
		managedAgents, _ := db.GetManagedAgents(m.db)

		// Fetch token usage for active agents with JSONL fallback
		agentTokenUsage := make(map[string]*TokenUsage)
		for _, agent := range managedAgents {
			if agent.LastSessionID != nil && *agent.LastSessionID != "" {
				// Use fallback version to try persisted snapshots if transcript unavailable
				if usage := getTokenUsageWithFallback(*agent.LastSessionID, projectDBPath); usage != nil {
					agentTokenUsage[agent.AgentID] = usage
				}
			}
		}

		return pollMsg{
			roomMessages:    roomMessages,
			threadMessages:  threadMessages,
			threadID:        threadID,
			questions:       questions,
			threads:         threads,
			mentionMessages: mentionMessages,
			managedAgents:   managedAgents,
			agentTokenUsage: agentTokenUsage,
		}
	})
}

// activityPollMsg is a faster-polling message for activity panel updates only.
// This runs at 250ms to catch fast state transitions (spawning→prompting→prompted).
func (m *Model) activityPollCmd() tea.Cmd {
	projectDBPath := m.projectDBPath
	return tea.Tick(activityPollInterval, func(time.Time) tea.Msg {
		// Fetch managed agents for activity panel
		managedAgents, _ := db.GetManagedAgents(m.db)

		// Fetch token usage for active agents with JSONL fallback
		agentTokenUsage := make(map[string]*TokenUsage)
		for _, agent := range managedAgents {
			if agent.LastSessionID != nil && *agent.LastSessionID != "" {
				// Use fallback version to try persisted snapshots if transcript unavailable
				if usage := getTokenUsageWithFallback(*agent.LastSessionID, projectDBPath); usage != nil {
					agentTokenUsage[agent.AgentID] = usage
				}
			}
		}

		// Check daemon started_at for restart detection
		daemonStartedAt := readDaemonStartedAt(projectDBPath)

		return activityPollMsg{
			managedAgents:   managedAgents,
			agentTokenUsage: agentTokenUsage,
			daemonStartedAt: daemonStartedAt,
		}
	})
}
