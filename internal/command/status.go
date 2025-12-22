package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewStatusCmd creates the status command.
func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <agent> [message]",
		Short: "Update status with optional claims",
		Args:  cobra.RangeArgs(1, 2),
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
			message := ""
			if len(args) > 1 {
				message = args[1]
			}

			agent, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", agentID))
			}

			clear, _ := cmd.Flags().GetBool("clear")
			if clear {
				clearedItems, clearedCount, err := clearClaims(ctx.DB, agentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}

				now := time.Now().Unix()
				updates := db.AgentUpdates{
					Status:   types.OptionalString{Set: true, Value: nil},
					LastSeen: types.OptionalInt64{Set: true, Value: &now},
				}
				if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
					return writeCommandError(cmd, err)
				}
				if updated, err := db.GetAgent(ctx.DB, agentID); err == nil && updated != nil {
					if err := db.AppendAgent(ctx.Project.DBPath, *updated); err != nil {
						return writeCommandError(cmd, err)
					}
				}

				body := "status cleared"
				if clearedCount > 0 {
					plural := "s"
					if clearedCount == 1 {
						plural = ""
					}
					body = fmt.Sprintf("status cleared (released %d claim%s)", clearedCount, plural)
				}
				msg, err := db.CreateMessage(ctx.DB, types.Message{
					TS:        now,
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

				if ctx.JSONMode {
					payload := map[string]any{
						"agent_id":        agentID,
						"action":          "cleared",
						"claims_released": clearedCount,
						"items":           clearedItems,
					}
					return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
				}

				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "@%s status cleared\n", agentID)
				if clearedCount > 0 {
					plural := "s"
					if clearedCount == 1 {
						plural = ""
					}
					fmt.Fprintf(out, "  Released %d claim%s\n", clearedCount, plural)
				}
				return nil
			}

			if _, err := db.PruneExpiredClaims(ctx.DB); err != nil {
				return writeCommandError(cmd, err)
			}

			ttl, _ := cmd.Flags().GetString("ttl")
			var expiresAt *int64
			if ttl != "" {
				seconds, err := parseDuration(ttl)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				value := time.Now().Unix() + seconds
				expiresAt = &value
			}

			claims, err := collectClaims(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			created := make([]types.Claim, 0, len(claims))
			for _, claim := range claims {
				createdClaim, err := db.CreateClaim(ctx.DB, types.ClaimInput{
					AgentID:   agentID,
					ClaimType: claim.ClaimType,
					Pattern:   claim.Pattern,
					Reason:    optionalString(message),
					ExpiresAt: expiresAt,
				})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				created = append(created, *createdClaim)
			}

			now := time.Now().Unix()
			updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
			if message != "" {
				updates.Status = types.OptionalString{Set: true, Value: &message}
			}
			if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}
			if updated, err := db.GetAgent(ctx.DB, agentID); err == nil && updated != nil {
				if err := db.AppendAgent(ctx.Project.DBPath, *updated); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			body := message
			if body == "" {
				body = "status update"
			}
			if len(created) > 0 {
				claimList := buildClaimList(created)
				if message != "" {
					body = fmt.Sprintf("%s [claimed: %s]", message, claimList)
				} else {
					body = fmt.Sprintf("claimed: %s", claimList)
				}
			}

			createdMsg, err := db.CreateMessage(ctx.DB, types.Message{
				TS:        now,
				FromAgent: agentID,
				Body:      body,
				Mentions:  []string{},
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if err := db.AppendMessage(ctx.Project.DBPath, createdMsg); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"agent_id":   agentID,
					"status":     optionalString(message),
					"claims":     claimsToPayload(created),
					"expires_at": expiresAt,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			if message != "" {
				fmt.Fprintf(out, "@%s: %s\n", agentID, message)
			} else {
				fmt.Fprintf(out, "@%s status updated\n", agentID)
			}
			if len(created) > 0 {
				fmt.Fprintln(out, "  Claimed:")
				for _, claim := range created {
					typePrefix := ""
					if claim.ClaimType != types.ClaimTypeFile {
						typePrefix = fmt.Sprintf("%s:", claim.ClaimType)
					}
					fmt.Fprintf(out, "    %s%s\n", typePrefix, claim.Pattern)
				}
				if expiresAt != nil {
					ttlMinutes := int((*expiresAt - time.Now().Unix()) / 60)
					fmt.Fprintf(out, "  Expires in %d minutes\n", ttlMinutes)
				}
			}

			return nil
		},
	}

	cmd.Flags().String("file", "", "claim a single file")
	cmd.Flags().String("files", "", "claim multiple files (comma-separated globs)")
	cmd.Flags().String("bd", "", "claim a beads issue")
	cmd.Flags().String("issue", "", "claim a GitHub issue")
	cmd.Flags().String("ttl", "", "expiration time for claims (e.g., 2h, 30m, 1d)")
	cmd.Flags().Bool("clear", false, "clear all claims and reset status")

	return cmd
}

func clearClaims(dbConn *sql.DB, agentID string) ([]string, int64, error) {
	existing, err := db.GetClaimsByAgent(dbConn, agentID)
	if err != nil {
		return nil, 0, err
	}

	items := make([]string, 0, len(existing))
	for _, claim := range existing {
		if claim.ClaimType == types.ClaimTypeFile {
			items = append(items, claim.Pattern)
		} else {
			items = append(items, fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern))
		}
	}

	cleared, err := db.DeleteClaimsByAgent(dbConn, agentID)
	if err != nil {
		return nil, 0, err
	}
	return items, cleared, nil
}
