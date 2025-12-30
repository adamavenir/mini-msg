package command

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewWhoCmd creates the who command.
func NewWhoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "who <agent|here>",
		Short: "Show agent details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			if args[0] == "here" {
				staleHours := 4
				if value, err := db.GetConfig(ctx.DB, "stale_hours"); err == nil && value != "" {
					staleHours = parseInt(value, staleHours)
				}
				agents, err := db.GetActiveAgents(ctx.DB, staleHours)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if ctx.JSONMode {
					payload := make([]agentDetails, 0, len(agents))
					for _, agent := range agents {
						payload = append(payload, toAgentDetails(agent))
					}
					return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
				}
				out := cmd.OutOrStdout()
				if len(agents) == 0 {
					fmt.Fprintln(out, "No active agents")
					return nil
				}
				for _, agent := range agents {
					displayAgent(out, agent, staleHours)
					fmt.Fprintln(out)
				}
				return nil
			}

			agent, err := resolveAgentByRef(ctx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(toAgentDetails(*agent))
			}
			staleHours := 4
			if value, err := db.GetConfig(ctx.DB, "stale_hours"); err == nil && value != "" {
				staleHours = parseInt(value, staleHours)
			}
			displayAgent(cmd.OutOrStdout(), *agent, staleHours)
			return nil
		},
	}

	return cmd
}

type agentDetails struct {
	GUID         string `json:"guid"`
	AgentID      string `json:"agent_id"`
	Status       string `json:"status"`
	Purpose      string `json:"purpose"`
	RegisteredAt int64  `json:"registered_at"`
	LastSeen     int64  `json:"last_seen"`
	LeftAt       *int64 `json:"left_at,omitempty"`
}

func toAgentDetails(agent types.Agent) agentDetails {
	return agentDetails{
		GUID:         agent.GUID,
		AgentID:      agent.AgentID,
		Status:       normalizeOptionalValue(agent.Status),
		Purpose:      normalizeOptionalValue(agent.Purpose),
		RegisteredAt: agent.RegisteredAt,
		LastSeen:     agent.LastSeen,
		LeftAt:       agent.LeftAt,
	}
}

func displayAgent(out io.Writer, agent types.Agent, staleHours int) {
	registeredAt := formatRelative(agent.RegisteredAt)
	lastSeen := formatRelative(agent.LastSeen)
	activeStatus := "active"
	if agent.LeftAt != nil {
		activeStatus = "left"
	} else if isStale(agent.LastSeen, staleHours) {
		activeStatus = "stale"
	}

	fmt.Fprintln(out, agent.AgentID)
	fmt.Fprintf(out, "  Status: %s\n", formatOptionalValue(agent.Status))
	fmt.Fprintf(out, "  Purpose: %s\n", formatOptionalValue(agent.Purpose))
	fmt.Fprintf(out, "  Registered: %s\n", registeredAt)
	fmt.Fprintf(out, "  Last seen: %s\n", lastSeen)
	fmt.Fprintf(out, "  Active: %s\n", activeStatus)
}
