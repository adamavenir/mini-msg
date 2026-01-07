package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewHereCmd creates the here command.
func NewHereCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "here",
		Short: "List active agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			includeAll, _ := cmd.Flags().GetBool("all")

			var agents []types.Agent
			if includeAll {
				all, err := db.GetAllAgents(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				for _, agent := range all {
					if agent.LeftAt == nil {
						agents = append(agents, agent)
					}
				}
			} else {
				staleHours := 4
				if value, err := db.GetConfig(ctx.DB, "stale_hours"); err == nil && value != "" {
					staleHours = parseInt(value, staleHours)
				}
				agents, err = db.GetActiveAgents(ctx.DB, staleHours)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			claimCounts, err := db.GetClaimCountsByAgent(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			messageCounts, err := getMessageCounts(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			allRoles, err := db.GetAllAgentRoles(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"agents": buildHerePayload(agents, claimCounts, messageCounts, allRoles),
					"total":  len(agents),
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			if len(agents) == 0 {
				fmt.Fprintln(out, "No active agents")
				return nil
			}

			fmt.Fprintf(out, "ACTIVE AGENTS (%d):\n", len(agents))
			for _, agent := range agents {
				claimCount := claimCounts[agent.AgentID]
				claimInfo := ""
				if claimCount > 0 {
					plural := "s"
					if claimCount == 1 {
						plural = ""
					}
					claimInfo = fmt.Sprintf(" (%d claim%s)", claimCount, plural)
				}
				status := ""
				if agent.Status != nil && *agent.Status != "" {
					status = " - " + *agent.Status
				}
				roleInfo := ""
				if roles := allRoles[agent.AgentID]; roles != nil {
					roleInfo = formatRoleInfo(roles)
				}
				fmt.Fprintf(out, "  @%s%s%s%s\n", agent.AgentID, roleInfo, claimInfo, status)
				fmt.Fprintf(out, "    last seen: %s\n", formatRelative(agent.LastSeen))
			}

			return nil
		},
	}

	cmd.Flags().Bool("all", false, "include stale agents")
	return cmd
}

func buildHerePayload(agents []types.Agent, claimCounts map[string]int64, messageCounts map[string]int64, allRoles map[string]*types.AgentRoles) []map[string]any {
	payload := make([]map[string]any, 0, len(agents))
	for _, agent := range agents {
		entry := map[string]any{
			"agent_id":      agent.GUID,
			"display_name":  agent.AgentID,
			"status":        agent.Status,
			"last_active":   timeISO(agent.LastSeen),
			"message_count": messageCounts[agent.AgentID],
			"claim_count":   claimCounts[agent.AgentID],
		}
		if roles := allRoles[agent.AgentID]; roles != nil {
			entry["roles_held"] = roles.Held
			entry["roles_playing"] = roles.Playing
		}
		payload = append(payload, entry)
	}
	return payload
}

func formatRoleInfo(roles *types.AgentRoles) string {
	if len(roles.Held) == 0 && len(roles.Playing) == 0 {
		return ""
	}
	allRoles := append([]string{}, roles.Held...)
	for _, r := range roles.Playing {
		allRoles = append(allRoles, r+"*")
	}
	return " [" + strings.Join(allRoles, ", ") + "]"
}

func timeISO(ts int64) string {
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}
