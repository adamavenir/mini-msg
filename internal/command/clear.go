package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewClearCmd creates the clear command.
func NewClearCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clear <agent>",
		Short: "Clear claims for an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID, err := resolveAgentRef(ctx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}
			agent, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", agentID))
			}

			file, _ := cmd.Flags().GetString("file")
			bd, _ := cmd.Flags().GetString("bd")
			issue, _ := cmd.Flags().GetString("issue")

			cleared := int64(0)
			clearedItems := []string{}

			if file != "" {
				deleted, err := db.DeleteClaim(ctx.DB, types.ClaimTypeFile, file)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if deleted {
					cleared++
					clearedItems = append(clearedItems, file)
				}
			}
			if bd != "" {
				pattern := stripHash(bd)
				deleted, err := db.DeleteClaim(ctx.DB, types.ClaimTypeBD, pattern)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if deleted {
					cleared++
					clearedItems = append(clearedItems, fmt.Sprintf("bd:%s", pattern))
				}
			}
			if issue != "" {
				pattern := stripHash(issue)
				deleted, err := db.DeleteClaim(ctx.DB, types.ClaimTypeIssue, pattern)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if deleted {
					cleared++
					clearedItems = append(clearedItems, fmt.Sprintf("issue:%s", pattern))
				}
			}

			if file == "" && bd == "" && issue == "" {
				existing, err := db.GetClaimsByAgent(ctx.DB, agentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				for _, claim := range existing {
					if claim.ClaimType == types.ClaimTypeFile {
						clearedItems = append(clearedItems, claim.Pattern)
					} else {
						clearedItems = append(clearedItems, fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern))
					}
				}
				cleared, err = db.DeleteClaimsByAgent(ctx.DB, agentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if cleared > 0 {
				body := fmt.Sprintf("cleared claims: %s", joinList(clearedItems))
				msg, err := db.CreateMessage(ctx.DB, types.Message{
					TS:        time.Now().Unix(),
					FromAgent: agentID,
					Body:      body,
					Mentions:  []string{},
				})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendMessage(ctx.Project.DBPath, msg); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"agent_id": agentID,
					"cleared":  cleared,
					"items":    clearedItems,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			if cleared == 0 {
				fmt.Fprintf(out, "No claims to clear for @%s\n", agentID)
				return nil
			}
			plural := "s"
			if cleared == 1 {
				plural = ""
			}
			fmt.Fprintf(out, "@%s cleared %d claim%s:\n", agentID, cleared, plural)
			for _, item := range clearedItems {
				fmt.Fprintf(out, "  %s\n", item)
			}
			return nil
		},
	}

	cmd.Flags().String("file", "", "clear a specific file claim")
	cmd.Flags().String("bd", "", "clear a specific beads issue claim")
	cmd.Flags().String("issue", "", "clear a specific GitHub issue claim")
	return cmd
}
