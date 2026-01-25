package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

// NewPruneCmd creates the prune command.
func NewPruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune <target>",
		Short: "Archive old messages with cold storage guardrails",
		Long: `Archive old messages from a specific thread or the main room.

Target can be:
  main, room     - prune the main room
  <thread-name>  - prune a specific thread by name
  <thread-id>    - prune a specific thread by ID (thrd-*)

By default, messages with replies, faves, reactions, or pins are protected.
Use --with to remove protections (allow pruning those items).
Use --without to only prune items lacking those attributes.

Protection types: replies, faves, reacts

Examples:
  fray prune main              # Prune main room (respects all protections)
  fray prune main --keep 50    # Keep last 50 messages in room
  fray prune design-thread     # Prune specific thread
  fray prune main --before msg-abc123      # Prune everything before msg-abc123
  fray prune main --before abc             # Same, with prefix matching
  fray prune main --with faves             # Also prune faved messages
  fray prune main --without reacts         # Only prune messages without reactions
  fray prune main --without faves,reacts   # Only prune messages without faves or reactions

Note: Pinned messages cannot be pruned; they must be unpinned first.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			keep, _ := cmd.Flags().GetInt("keep")
			pruneAll, _ := cmd.Flags().GetBool("all")
			withReact, _ := cmd.Flags().GetString("with-react")
			withFlags, _ := cmd.Flags().GetStringSlice("with")
			withoutFlags, _ := cmd.Flags().GetStringSlice("without")
			beforeMsgID, _ := cmd.Flags().GetString("before")

			// Parse protection options
			opts := parsePruneProtectionOpts(withFlags, withoutFlags)

			if keep < 0 {
				return writeCommandError(cmd, fmt.Errorf("invalid --keep value: %d", keep))
			}

			// Resolve target to home value
			target := "main"
			if len(args) > 0 {
				target = args[0]
			}

			home, threadName, err := resolvePruneTarget(ctx.DB, target)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := checkPruneGuardrails(ctx.Project.Root); err != nil {
				return writeCommandError(cmd, err)
			}

			// Resolve --before message ID prefix if provided
			var beforeGUID string
			if beforeMsgID != "" {
				beforeMsgID = strings.TrimPrefix(beforeMsgID, "#")
				beforeMsgID = strings.TrimPrefix(beforeMsgID, "msg-")
				msg, err := db.GetMessageByPrefix(ctx.DB, beforeMsgID)
				if err != nil {
					return writeCommandError(cmd, fmt.Errorf("could not resolve message: %s", beforeMsgID))
				}
				beforeGUID = msg.ID
			}

			var result pruneResult
			if withReact != "" {
				// Reaction-based pruning: prune messages with specific reaction
				result, err = pruneMessagesWithReaction(ctx.Project.DBPath, home, withReact)
			} else {
				// Standard pruning: keep N most recent messages or prune before a message
				result, err = pruneMessages(ctx.Project.DBPath, keep, pruneAll, home, opts, beforeGUID)
			}
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.RebuildDatabaseFromJSONL(ctx.DB, ctx.Project.DBPath); err != nil {
				return writeCommandError(cmd, err)
			}

			// Fix stale watermarks pointing to pruned messages
			if err := fixStaleWatermarks(ctx.DB, ctx.Project.DBPath); err != nil {
				// Log but don't fail - watermarks will self-heal on next daemon poll
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not fix stale watermarks: %v\n", err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"kept":     result.Kept,
					"archived": result.Archived,
					"target":   home,
				}
				if result.ClearedHistory {
					payload["history"] = nil
				} else {
					payload["history"] = result.HistoryPath
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			targetDesc := "room"
			if threadName != "" {
				targetDesc = threadName
			}
			if result.ClearedHistory {
				fmt.Fprintf(out, "Pruned %s to last %d messages. history.jsonl cleared.\n", targetDesc, result.Kept)
				return nil
			}
			fmt.Fprintf(out, "Pruned %s to last %d messages. Archived to history.jsonl\n", targetDesc, result.Kept)
			return nil
		},
	}

	cmd.Flags().Int("keep", 20, "number of recent messages to keep")
	cmd.Flags().Bool("all", false, "delete history.jsonl before pruning")
	cmd.Flags().String("before", "", "prune messages before this message ID (keeps msg and everything after)")
	cmd.Flags().String("with-react", "", "prune messages with this reaction (e.g., :filed: or 📁)")
	cmd.Flags().StringSlice("with", nil, "remove protections: replies,faves,reacts (allow pruning those)")
	cmd.Flags().StringSlice("without", nil, "only prune items lacking these: replies,faves,reacts")
	return cmd
}
