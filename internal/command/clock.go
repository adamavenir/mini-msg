package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adamavenir/fray/internal/daemon"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewClockCmd creates the clock command for ambient agent status.
func NewClockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clock",
		Short: "Show agent status: heartbeat timer + notification counts",
		Long: `Display a status line for agents running in the background.

Shows:
- Heartbeat timer (time until session may be recycled)
- Count of unread @mentions
- Count of replies to your messages

Updates in place every few seconds. Press Ctrl+C to stop.

Requires FRAY_AGENT_ID env var or --as flag.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID, _ := cmd.Flags().GetString("as")
			if agentID == "" {
				agentID = os.Getenv("FRAY_AGENT_ID")
			}
			if agentID == "" {
				return writeCommandError(cmd, fmt.Errorf("--as flag or FRAY_AGENT_ID env var required"))
			}

			agent, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: %s", agentID))
			}

			// Get timeout config
			var minCheckinMs int64
			if agent.Invoke != nil {
				_, _, minCheckinMs, _ = daemon.GetTimeouts(agent.Invoke)
			}
			if minCheckinMs == 0 {
				minCheckinMs = 600000 // default 10m
			}

			out := cmd.OutOrStdout()
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()

			// Show initial status
			showClockStatus(ctx.DB, out, agentID, minCheckinMs, ctx.JSONMode)

			for {
				select {
				case <-stop:
					if !ctx.JSONMode {
						fmt.Fprintln(out) // newline after status line
					}
					return nil
				case <-ticker.C:
					showClockStatus(ctx.DB, out, agentID, minCheckinMs, ctx.JSONMode)
				}
			}
		},
	}

	cmd.Flags().String("as", "", "agent to show status for (uses FRAY_AGENT_ID if not set)")

	return cmd
}

func showClockStatus(database *sql.DB, out io.Writer, agentID string, minCheckinMs int64, jsonMode bool) {
	// Get last activity time
	lastPostTs, _ := db.GetAgentLastPostTime(database, agentID)
	agent, _ := db.GetAgent(database, agentID)

	lastHeartbeatTs := int64(0)
	if agent != nil && agent.LastHeartbeat != nil {
		lastHeartbeatTs = *agent.LastHeartbeat
	}

	// Pick most recent activity
	lastActivityMs := lastPostTs
	if lastHeartbeatTs > lastActivityMs {
		lastActivityMs = lastHeartbeatTs
	}

	var lastActivityTime time.Time
	if lastActivityMs > 0 {
		lastActivityTime = time.UnixMilli(lastActivityMs)
	} else {
		lastActivityTime = time.Now()
	}

	elapsed := time.Since(lastActivityTime)
	remaining := time.Duration(minCheckinMs)*time.Millisecond - elapsed
	if remaining < 0 {
		remaining = 0
	}

	// Count unread mentions (since last post)
	mentionOpts := &types.MessageQueryOptions{
		UnreadOnly:  true,
		AgentPrefix: agentID,
	}
	// Include all homes, not just room
	emptyHome := ""
	mentionOpts.Home = &emptyHome

	mentions, _ := db.GetMessagesWithMention(database, agentID, mentionOpts)
	mentionCount := len(mentions)

	// Count replies to agent's messages (since last post)
	replyOpts := &types.MessageQueryOptions{
		IncludeRepliesToAgent: agentID,
		AgentPrefix:           agentID,
	}
	replyOpts.Home = &emptyHome

	// Get messages since last activity that are replies to this agent
	if lastActivityMs > 0 {
		replyOpts.Since = &types.MessageCursor{TS: lastActivityMs}
	}

	replies, _ := db.GetMessagesWithMention(database, agentID, replyOpts)
	// Filter to only actual replies (not mentions)
	replyCount := 0
	for _, msg := range replies {
		if msg.ReplyTo != nil {
			replyCount++
		}
	}

	if jsonMode {
		json.NewEncoder(out).Encode(map[string]any{
			"agent_id":     agentID,
			"remaining_ms": remaining.Milliseconds(),
			"remaining":    remaining.Round(time.Second).String(),
			"mentions":     mentionCount,
			"replies":      replyCount,
		})
	} else {
		// Clear line and write status
		status := fmt.Sprintf("\r@%s: %s remaining", agentID, remaining.Round(time.Second))

		if mentionCount > 0 || replyCount > 0 {
			status += " |"
			if mentionCount > 0 {
				status += fmt.Sprintf(" %d mentions", mentionCount)
			}
			if replyCount > 0 {
				status += fmt.Sprintf(" %d replies", replyCount)
			}
		}

		// Pad to clear previous content
		status += "          "
		fmt.Fprint(out, status)
	}
}
