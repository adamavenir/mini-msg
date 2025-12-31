package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

type hookOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
	Continue          *bool  `json:"continue,omitempty"`
}

// NewHookSessionCmd handles Claude SessionStart hooks.
func NewHookSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook-session <event>",
		Short: "SessionStart hook handler (internal)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			output := hookOutput{}
			event := args[0]

			projectPath := os.Getenv("CLAUDE_PROJECT_DIR")
			project, err := core.DiscoverProject(projectPath)
			if err != nil {
				return writeHookOutput(cmd, output)
			}

			dbConn, err := db.OpenDatabase(project)
			if err != nil {
				return writeHookOutput(cmd, output)
			}
			defer dbConn.Close()
			if err := db.InitSchema(dbConn); err != nil {
				return writeHookOutput(cmd, output)
			}

			agentID := os.Getenv("FRAY_AGENT_ID")
			if agentID == "" {
				output.AdditionalContext = buildHookRegistrationContext(dbConn)
				return writeHookOutput(cmd, output)
			}

			roomMessages, mentionMessages, agentBase := fetchHookMessages(dbConn, agentID, 10, 5)
			output.AdditionalContext = buildHookSessionContext(event, agentID, agentBase, roomMessages, mentionMessages)
			return writeHookOutput(cmd, output)
		},
	}

	return cmd
}

func buildHookRegistrationContext(dbConn *sql.DB) string {
	staleHours := 4
	if raw, err := db.GetConfig(dbConn, "stale_hours"); err == nil {
		staleHours = parseInt(raw, 4)
	}

	activeAgents, err := db.GetActiveAgents(dbConn, staleHours)
	if err != nil {
		activeAgents = nil
	}

	var builder strings.Builder
	builder.WriteString("[fray] This project uses fray for agent coordination.\n")
	builder.WriteString("You are not registered. To join the room:\n\n")
	builder.WriteString("  fray new <name> --goal \"your current task\"\n\n")

	if len(activeAgents) > 0 {
		builder.WriteString("Active agents: ")
		for i, agent := range activeAgents {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(agent.AgentID)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("Use /skill fray-chat for conversation guidance.")
	return builder.String()
}

func fetchHookMessages(dbConn *sql.DB, agentID string, roomLimit, mentionLimit int) ([]types.Message, []types.Message, string) {
	roomMessages, err := db.GetMessages(dbConn, &types.MessageQueryOptions{Limit: roomLimit})
	if err != nil {
		roomMessages = nil
	}

	agentBase := agentID
	if parsed, err := core.ParseAgentID(agentID); err == nil {
		agentBase = parsed.Base
	} else if idx := strings.LastIndex(agentID, "."); idx != -1 {
		agentBase = agentID[:idx]
	}

	mentionMessages, err := db.GetMessagesWithMention(dbConn, agentBase, &types.MessageQueryOptions{Limit: roomLimit + mentionLimit})
	if err != nil {
		mentionMessages = nil
	}

	roomIDs := make(map[string]struct{}, len(roomMessages))
	for _, msg := range roomMessages {
		roomIDs[msg.ID] = struct{}{}
	}

	filteredMentions := make([]types.Message, 0, mentionLimit)
	for _, msg := range mentionMessages {
		if _, ok := roomIDs[msg.ID]; ok {
			continue
		}
		filteredMentions = append(filteredMentions, msg)
		if len(filteredMentions) == mentionLimit {
			break
		}
	}

	return roomMessages, filteredMentions, agentBase
}

func buildHookSessionContext(event, agentID, agentBase string, roomMessages, mentionMessages []types.Message) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("[fray] You are %s. Session %s.\n\n", agentID, event))

	if len(roomMessages) > 0 {
		builder.WriteString("ROOM:\n")
		for _, msg := range roomMessages {
			builder.WriteString(fmt.Sprintf("[%s] %s: %s\n", msg.ID, msg.FromAgent, msg.Body))
		}
	} else {
		builder.WriteString("ROOM: (no messages yet)\n")
	}

	if len(mentionMessages) > 0 {
		builder.WriteString("\n")
		builder.WriteString(fmt.Sprintf("@%s:\n", agentBase))
		for _, msg := range mentionMessages {
			builder.WriteString(fmt.Sprintf("[%s] %s: %s\n", msg.ID, msg.FromAgent, msg.Body))
		}
	}

	builder.WriteString(fmt.Sprintf("\nPost: fray post --as %s \"message\"", agentID))
	return builder.String()
}

func writeHookOutput(cmd *cobra.Command, output hookOutput) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	return encoder.Encode(output)
}
