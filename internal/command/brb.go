package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewBRBCmd creates the brb command for seamless session handoff.
func NewBRBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "brb <agent> [message]",
		Short: "Hand off to fresh session (daemon spawns immediately)",
		Long: `End current session and signal daemon to immediately spawn a fresh session.

Unlike 'bye' which waits for @mention to respawn, 'brb' tells the daemon
to spawn a new session right away with continuation context.

Use this when you need a fresh context window but want to continue working.`,
		Args: cobra.RangeArgs(1, 2),
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

			now := time.Now().Unix()
			nowMs := time.Now().UnixMilli()

			// Clear claims (like bye)
			clearedClaims, err := db.DeleteClaimsByAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			// Clear session roles (like bye)
			sessionRoles, err := db.GetSessionRoles(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			clearedRoles, err := db.ClearSessionRoles(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			for _, role := range sessionRoles {
				if err := db.AppendRoleStop(ctx.Project.DBPath, agentID, role.RoleName, nowMs); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			// Handle wake condition lifecycle on brb
			// 1. Clear persist-until-bye conditions (same as bye)
			clearedWake, err := db.ClearPersistUntilByeConditions(ctx.DB, ctx.Project.DBPath, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			// 2. Resume paused conditions (opposite of bye - this is persist-restore-on-back)
			resumedWake, err := db.ResumeWakeConditions(ctx.DB, ctx.Project.DBPath, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			// Post optional message
			var posted *types.Message
			if message != "" {
				bases, err := db.GetAgentBases(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				mentions := core.ExtractMentions(message, bases)
				mentions = core.ExpandAllMention(mentions, bases)
				created, err := db.CreateMessage(ctx.DB, types.Message{
					TS:        now,
					FromAgent: agentID,
					Body:      message,
					Mentions:  mentions,
				})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendMessage(ctx.Project.DBPath, created); err != nil {
					return writeCommandError(cmd, err)
				}
				posted = &created
			}

			// Post brb event (different from "left")
			eventMsg, err := db.CreateMessage(ctx.DB, types.Message{
				TS:        now,
				FromAgent: agentID,
				Body:      fmt.Sprintf("@%s brb", agentID),
				Type:      types.MessageTypeEvent,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if err := db.AppendMessage(ctx.Project.DBPath, eventMsg); err != nil {
				return writeCommandError(cmd, err)
			}

			// Update agent timestamps
			updates := db.AgentUpdates{
				LeftAt:   types.OptionalInt64{Set: true, Value: &now},
				LastSeen: types.OptionalInt64{Set: true, Value: &now},
				Status:   types.OptionalString{Set: true, Value: nil},
			}
			if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}

			// For managed agents, set presence to BRB (signals daemon to respawn immediately)
			// Unlike bye, we DON'T clear session ID - daemon will start fresh anyway
			if agent.Managed {
				if err := db.UpdateAgentPresenceWithAudit(ctx.DB, ctx.Project.DBPath, agentID, agent.Presence, types.PresenceBRB, "brb", "command", agent.Status); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			updated, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if updated != nil {
				if err := db.AppendAgent(ctx.Project.DBPath, *updated); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"agent_id":               agentID,
					"status":                 "brb",
					"message_id":             nil,
					"claims_cleared":         clearedClaims,
					"roles_cleared":          clearedRoles,
					"wake_conditions_cleared": clearedWake,
					"wake_conditions_resumed": resumedWake,
				}
				if posted != nil {
					payload["message_id"] = posted.ID
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Be right back, @%s! Daemon will spawn fresh session.\n", agentID)
			if posted != nil {
				fmt.Fprintf(out, "  Posted: [%s] %s\n", posted.ID, message)
			}
			if clearedClaims > 0 {
				plural := "s"
				if clearedClaims == 1 {
					plural = ""
				}
				fmt.Fprintf(out, "  Released %d claim%s\n", clearedClaims, plural)
			}
			if clearedRoles > 0 {
				plural := "s"
				if clearedRoles == 1 {
					plural = ""
				}
				fmt.Fprintf(out, "  Released %d session role%s\n", clearedRoles, plural)
			}
			if clearedWake > 0 {
				plural := "s"
				if clearedWake == 1 {
					plural = ""
				}
				fmt.Fprintf(out, "  Cleared %d wake condition%s\n", clearedWake, plural)
			}
			if resumedWake > 0 {
				plural := "s"
				if resumedWake == 1 {
					plural = ""
				}
				fmt.Fprintf(out, "  Resumed %d wake condition%s\n", resumedWake, plural)
			}
			return nil
		},
	}

	return cmd
}
