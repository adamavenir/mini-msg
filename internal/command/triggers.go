package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

// NewTriggersCmd creates the triggers command for auditing agent triggers.
func NewTriggersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "triggers",
		Short: "Audit agent trigger events",
		Long: `Show trigger events (session starts) with their trigger messages.

Useful for debugging why agents were triggered or investigating duplicate triggers.

Examples:
  fray triggers                    # Show last 10 triggers
  fray triggers --agent opus       # Filter to specific agent
  fray triggers --last 20          # Show last 20 triggers
  fray triggers --all              # Include manual starts (no trigger message)
  fray triggers --presence         # Show presence state changes
  fray triggers --presence --agent opus  # Presence for specific agent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentFilter, _ := cmd.Flags().GetString("agent")
			limit, _ := cmd.Flags().GetInt("last")
			includeAll, _ := cmd.Flags().GetBool("all")
			showPresence, _ := cmd.Flags().GetBool("presence")

			if showPresence {
				return showPresenceEvents(cmd, ctx, agentFilter, limit)
			}
			return showTriggerEvents(cmd, ctx, agentFilter, limit, includeAll)
		},
	}

	cmd.Flags().StringP("agent", "a", "", "filter to specific agent")
	cmd.Flags().Int("last", 10, "number of events to show")
	cmd.Flags().Bool("all", false, "include manual starts (no trigger message)")
	cmd.Flags().Bool("presence", false, "show presence state changes instead of triggers")

	return cmd
}

func showTriggerEvents(cmd *cobra.Command, ctx *CommandContext, agentFilter string, limit int, includeAll bool) error {
	events, err := db.ReadTriggerEvents(ctx.Project.Root)
	if err != nil {
		return writeCommandError(cmd, err)
	}

	// Filter
	var filtered []db.TriggerEvent
	for _, e := range events {
		if agentFilter != "" && e.AgentID != agentFilter {
			continue
		}
		if !includeAll && e.TriggeredBy == nil {
			continue
		}
		filtered = append(filtered, e)
		if len(filtered) >= limit {
			break
		}
	}

	if ctx.JSONMode {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"triggers": filtered,
			"count":    len(filtered),
		})
	}

	out := cmd.OutOrStdout()
	if len(filtered) == 0 {
		fmt.Fprintln(out, "No trigger events found")
		return nil
	}

	fmt.Fprintf(out, "TRIGGER EVENTS (%d):\n\n", len(filtered))
	for _, e := range filtered {
		// Format timestamp
		ts := time.Unix(e.StartedAt, 0)
		timeStr := ts.Format("Jan 02 15:04")

		// Trigger source
		triggerStr := "(manual)"
		if e.TriggeredBy != nil {
			triggerStr = *e.TriggeredBy
		}

		// Status indicator
		statusStr := "running"
		if e.EndedAt != nil {
			if e.ExitCode != nil && *e.ExitCode == 0 {
				statusStr = "completed"
			} else if e.ExitCode != nil {
				statusStr = fmt.Sprintf("exit(%d)", *e.ExitCode)
			} else {
				statusStr = "ended"
			}
			if e.DurationMs != nil {
				statusStr += fmt.Sprintf(" %s", formatDurationMs(*e.DurationMs))
			}
		}

		// Thread context
		threadStr := ""
		if e.ThreadGUID != nil {
			threadStr = fmt.Sprintf(" in %s", *e.ThreadGUID)
		}

		fmt.Fprintf(out, "  @%s  %s  [%s]\n", e.AgentID, timeStr, statusStr)
		fmt.Fprintf(out, "    trigger: %s%s\n", triggerStr, threadStr)
		fmt.Fprintf(out, "    session: %s\n\n", truncateSession(e.SessionID))
	}

	return nil
}

func showPresenceEvents(cmd *cobra.Command, ctx *CommandContext, agentFilter string, limit int) error {
	events, err := db.ReadPresenceEvents(ctx.Project.Root)
	if err != nil {
		return writeCommandError(cmd, err)
	}

	// Filter
	var filtered []db.PresenceEvent
	for _, e := range events {
		if agentFilter != "" && e.AgentID != agentFilter {
			continue
		}
		filtered = append(filtered, e)
		if len(filtered) >= limit {
			break
		}
	}

	if ctx.JSONMode {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"events": filtered,
			"count":  len(filtered),
		})
	}

	out := cmd.OutOrStdout()
	if len(filtered) == 0 {
		fmt.Fprintln(out, "No presence events found")
		return nil
	}

	fmt.Fprintf(out, "PRESENCE EVENTS (%d):\n\n", len(filtered))
	for _, e := range filtered {
		ts := time.Unix(e.TS, 0)
		timeStr := ts.Format("Jan 02 15:04:05")

		stateChange := e.To
		if e.From != "" {
			stateChange = e.From + " → " + e.To
		}

		statusStr := ""
		if e.Status != nil && *e.Status != "" {
			statusStr = fmt.Sprintf(" \"%s\"", *e.Status)
		}

		fmt.Fprintf(out, "  @%s  %s  %s\n", e.AgentID, timeStr, stateChange)
		fmt.Fprintf(out, "    reason: %s  source: %s%s\n\n", e.Reason, e.Source, statusStr)
	}

	return nil
}

func formatDurationMs(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func truncateSession(sessionID string) string {
	if len(sessionID) <= 12 {
		return sessionID
	}
	// Show first 8 and last 4 characters with ellipsis
	if strings.Contains(sessionID, "-") {
		// UUID format: show first segment
		parts := strings.Split(sessionID, "-")
		if len(parts) > 0 {
			return parts[0] + "..."
		}
	}
	return sessionID[:8] + "..."
}
