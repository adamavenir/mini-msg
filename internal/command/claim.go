package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewClaimCmd creates the claim command.
func NewClaimCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claim <agent>",
		Short: "Claim resources to prevent collision",
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

			if _, err := db.PruneExpiredClaims(ctx.DB); err != nil {
				return writeCommandError(cmd, err)
			}

			ttl, _ := cmd.Flags().GetString("ttl")
			reason, _ := cmd.Flags().GetString("reason")
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
			if len(claims) == 0 {
				return writeCommandError(cmd, fmt.Errorf("no claims specified. Use --file, --files, --bd, or --issue"))
			}

			created := make([]types.Claim, 0, len(claims))
			for _, claim := range claims {
				createdClaim, err := db.CreateClaim(ctx.DB, types.ClaimInput{
					AgentID:   agentID,
					ClaimType: claim.ClaimType,
					Pattern:   claim.Pattern,
					Reason:    optionalString(reason),
					ExpiresAt: expiresAt,
				})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				created = append(created, *createdClaim)
			}

			claimList := buildClaimList(created)
			messageBody := fmt.Sprintf("claimed: %s", claimList)
			createdMsg, err := db.CreateMessage(ctx.DB, types.Message{
				TS:        time.Now().Unix(),
				FromAgent: agentID,
				Body:      messageBody,
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
					"claims":     claimsToPayload(created),
					"expires_at": expiresAt,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "@%s claimed:\n", agentID)
			for _, claim := range claims {
				typePrefix := ""
				if claim.ClaimType != types.ClaimTypeFile {
					typePrefix = fmt.Sprintf("%s:", claim.ClaimType)
				}
				fmt.Fprintf(out, "  %s%s\n", typePrefix, claim.Pattern)
			}
			if expiresAt != nil {
				ttlMinutes := int((*expiresAt - time.Now().Unix()) / 60)
				fmt.Fprintf(out, "  Expires in %d minutes\n", ttlMinutes)
			}

			return nil
		},
	}

	cmd.Flags().String("file", "", "claim a single file")
	cmd.Flags().String("files", "", "claim multiple files (comma-separated globs)")
	cmd.Flags().String("bd", "", "claim a beads issue")
	cmd.Flags().String("issue", "", "claim a GitHub issue")
	cmd.Flags().String("ttl", "", "expiration time (e.g., 2h, 30m, 1d)")
	cmd.Flags().String("reason", "", "reason for claim")

	return cmd
}

func collectClaims(cmd *cobra.Command) ([]types.ClaimInput, error) {
	file, _ := cmd.Flags().GetString("file")
	files, _ := cmd.Flags().GetString("files")
	bd, _ := cmd.Flags().GetString("bd")
	issue, _ := cmd.Flags().GetString("issue")

	claims := []types.ClaimInput{}
	if file != "" {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeFile, Pattern: file})
	}
	if files != "" {
		for _, pattern := range splitCommaList(files) {
			claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeFile, Pattern: pattern})
		}
	}
	if bd != "" {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeBD, Pattern: stripHash(bd)})
	}
	if issue != "" {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeIssue, Pattern: stripHash(issue)})
	}

	return claims, nil
}

func buildClaimList(claims []types.Claim) string {
	parts := make([]string, 0, len(claims))
	for _, claim := range claims {
		if claim.ClaimType == types.ClaimTypeFile {
			parts = append(parts, claim.Pattern)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern))
	}
	return joinList(parts)
}

func claimsToPayload(claims []types.Claim) []map[string]any {
	payload := make([]map[string]any, 0, len(claims))
	for _, claim := range claims {
		payload = append(payload, map[string]any{
			"type":    claim.ClaimType,
			"pattern": claim.Pattern,
		})
	}
	return payload
}
