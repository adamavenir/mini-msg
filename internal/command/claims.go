package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewClaimsCmd creates the claims command.
func NewClaimsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claims [agent]",
		Short: "List active claims",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			claimType, _ := cmd.Flags().GetString("type")

			if _, err := db.PruneExpiredClaims(ctx.DB); err != nil {
				return writeCommandError(cmd, err)
			}

			var claims []types.Claim
			if len(args) > 0 {
				agentID := ResolveAgentRef(args[0], ctx.ProjectConfig)
				claims, err = db.GetClaimsByAgent(ctx.DB, agentID)
			} else {
				claims, err = db.GetAllClaims(ctx.DB)
			}
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if claimType != "" {
				filtered := make([]types.Claim, 0, len(claims))
				for _, claim := range claims {
					if string(claim.ClaimType) == claimType {
						filtered = append(filtered, claim)
					}
				}
				claims = filtered
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(claims)
			}

			out := cmd.OutOrStdout()
			if len(claims) == 0 {
				if len(args) > 0 {
					agentID := ResolveAgentRef(args[0], ctx.ProjectConfig)
					fmt.Fprintf(out, "No claims for @%s\n", agentID)
				} else {
					fmt.Fprintln(out, "No active claims")
				}
				return nil
			}

			fmt.Fprintf(out, "CLAIMS (%d):\n", len(claims))

			byAgent := map[string][]types.Claim{}
			for _, claim := range claims {
				byAgent[claim.AgentID] = append(byAgent[claim.AgentID], claim)
			}

			for agentID, agentClaims := range byAgent {
				fmt.Fprintf(out, "\n  @%s:\n", agentID)
				for _, claim := range agentClaims {
					typePrefix := ""
					if claim.ClaimType != types.ClaimTypeFile {
						typePrefix = fmt.Sprintf("%s:", claim.ClaimType)
					}
					age := formatRelative(claim.CreatedAt)
					expiry := ""
					if claim.ExpiresAt != nil {
						remaining := *claim.ExpiresAt - time.Now().Unix()
						if remaining > 0 {
							expiry = fmt.Sprintf(" (%dm left)", remaining/60)
						} else {
							expiry = " (expired)"
						}
					}
					reason := ""
					if claim.Reason != nil && *claim.Reason != "" {
						reason = " - " + *claim.Reason
					}
					fmt.Fprintf(out, "    %s%s (%s ago)%s%s\n", typePrefix, claim.Pattern, age, expiry, reason)
				}
			}

			return nil
		},
	}

	cmd.Flags().String("type", "", "filter by claim type (file, bd, issue)")
	return cmd
}
