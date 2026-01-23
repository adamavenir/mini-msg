package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// PermissionRequestInput is the JSON structure received from Claude Code PermissionRequest hooks.
type PermissionRequestInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// PermissionHookResponse is what we return to Claude Code.
type PermissionHookResponse struct {
	Decision    string   `json:"decision"`              // approve, deny
	Reason      string   `json:"reason,omitempty"`      // Why approved/denied
	Permissions []string `json:"permissions,omitempty"` // Patterns to add to allow list
}

// NewHookPermissionCmd handles Claude PermissionRequest hooks.
func NewHookPermissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook-permission",
		Short: "PermissionRequest hook handler (internal)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var input PermissionRequestInput
			if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
				return fmt.Errorf("parse hook input: %w", err)
			}

			agentID := os.Getenv("FRAY_AGENT_ID")
			sessionID := os.Getenv("CLAUDE_SESSION_ID")

			// Create permission request
			project, err := core.DiscoverProject("")
			if err != nil {
				return fmt.Errorf("discover project: %w", err)
			}

			// If FRAY_AGENT_ID not set, try to look up by session ID
			if agentID == "" && sessionID != "" {
				dbConn, dbErr := db.OpenDatabase(project)
				if dbErr == nil {
					defer dbConn.Close()
					if agent, lookupErr := db.GetAgentBySessionID(dbConn, sessionID); lookupErr == nil && agent != nil {
						agentID = agent.AgentID
					}
				}
			}

			// Fallback to unknown if still not resolved
			if agentID == "" {
				agentID = "unknown"
			}

			guid, err := core.GenerateGUID("perm")
			if err != nil {
				return fmt.Errorf("generate guid: %w", err)
			}

			// Extract action from tool input
			action := extractAction(input.ToolName, input.ToolInput)

			// Create permission request with options
			req := types.PermissionRequest{
				GUID:      guid,
				FromAgent: agentID,
				SessionID: sessionID,
				Tool:      input.ToolName,
				Action:    action,
				Rationale: "Agent needs this permission to complete the current task",
				Options: []types.PermissionOption{
					{
						Label:    "Allow once",
						Patterns: []string{fmt.Sprintf("%s(%s)", input.ToolName, action)},
						Scope:    types.PermissionScopeOnce,
					},
					{
						Label:    "Allow this pattern (session)",
						Patterns: []string{fmt.Sprintf("%s:*", input.ToolName)},
						Scope:    types.PermissionScopeSession,
					},
					{
						Label:    "Allow for all agents (project)",
						Patterns: []string{fmt.Sprintf("%s:*", input.ToolName)},
						Scope:    types.PermissionScopeProject,
						Warning:  "Persists in settings for all agents",
					},
				},
				Status:    types.PermissionStatusPending,
				CreatedAt: time.Now().Unix(),
			}

			// Store the permission request
			if err := db.AppendPermissionRequest(project.Root, req); err != nil {
				return fmt.Errorf("store permission request: %w", err)
			}

			// Post a message to the room about this permission request
			msgGUID, err := core.GenerateGUID("msg")
			if err != nil {
				return fmt.Errorf("generate msg guid: %w", err)
			}

			config, err := db.ReadProjectConfig(project.Root)
			if err != nil {
				return fmt.Errorf("read config: %w", err)
			}

			channelID := ""
			if config != nil {
				channelID = config.ChannelID
			}

			// Create simple text message
			messageBody := fmt.Sprintf("@%s requested approval for %s: %s\n`fray approve` to review requests",
				agentID, input.ToolName, action)

			// Create an event message for the permission request
			msg := types.Message{
				ID:        msgGUID,
				ChannelID: &channelID,
				Home:      "room",
				FromAgent: "system",
				Body:      messageBody,
				Mentions:  []string{agentID},
				Type:      types.MessageTypeEvent,
				TS:        time.Now().Unix(),
			}

			if err := db.AppendMessage(project.Root, msg); err != nil {
				return fmt.Errorf("append message: %w", err)
			}

			// For now, we block and wait for a response
			// In the future, this could poll for approval
			// For MVP, we'll return a message asking user to approve
			response := PermissionHookResponse{
				Decision: "deny",
				Reason:   fmt.Sprintf("Permission request %s pending user approval. Use 'fray approve %s <option>' to approve.", guid, guid),
			}

			return json.NewEncoder(os.Stdout).Encode(response)
		},
	}

	return cmd
}

func extractAction(toolName string, toolInput json.RawMessage) string {
	if toolName == "Bash" {
		var bashInput struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(toolInput, &bashInput); err == nil && bashInput.Command != "" {
			// Truncate long commands
			if len(bashInput.Command) > 100 {
				return bashInput.Command[:100] + "..."
			}
			return bashInput.Command
		}
	}

	// For other tools, try to extract a meaningful action
	var generic map[string]any
	if err := json.Unmarshal(toolInput, &generic); err == nil {
		// Look for common action fields
		for _, key := range []string{"action", "command", "operation", "path", "file"} {
			if v, ok := generic[key]; ok {
				return fmt.Sprintf("%v", v)
			}
		}
	}

	return "unknown action"
}
