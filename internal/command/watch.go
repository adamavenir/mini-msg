package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/daemon"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewWatchCmd creates the watch command.
func NewWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Stream messages in real-time",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			last, _ := cmd.Flags().GetInt("last")
			includeArchived, _ := cmd.Flags().GetBool("archived")
			asAgent, _ := cmd.Flags().GetString("as")

			// Resolve agent filter - use --as flag or fall back to FRAY_AGENT_ID env var
			var filterAgent string
			if asAgent != "" {
				resolved, err := resolveAgentRef(ctx, asAgent)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				filterAgent = resolved
			} else if envAgent := os.Getenv("FRAY_AGENT_ID"); envAgent != "" {
				filterAgent = envAgent
			}

			projectName := GetProjectName(ctx.Project.Root)
			out := cmd.OutOrStdout()
			var agentBases map[string]struct{}
			if !ctx.JSONMode {
				agentBases, err = db.GetAgentBases(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			var cursor *types.MessageCursor
			if last == 0 {
				cursor, err = db.GetLastMessageCursor(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if !ctx.JSONMode {
					watchLabel := "watching"
					if filterAgent != "" {
						watchLabel = fmt.Sprintf("watching @%s", filterAgent)
					}
					fmt.Fprintf(out, "--- %s (Ctrl+C to stop) ---\n", watchLabel)
				}
			} else {
				recent, err := db.GetMessages(ctx.DB, &types.MessageQueryOptions{Limit: last, IncludeArchived: includeArchived})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				recent, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, recent)
				if err != nil {
					return writeCommandError(cmd, err)
				}

				// Filter to agent-relevant messages if --as is set
				if filterAgent != "" {
					filtered := make([]types.Message, 0, len(recent))
					for _, msg := range recent {
						if isMessageRelevantToAgent(ctx.DB, msg, filterAgent) {
							filtered = append(filtered, msg)
						}
					}
					recent = filtered
				}

				if len(recent) > 0 {
					if ctx.JSONMode {
						for _, msg := range recent {
							_ = json.NewEncoder(out).Encode(msg)
						}
					} else {
						for _, msg := range recent {
							fmt.Fprintln(out, FormatMessage(msg, projectName, agentBases))
						}
						watchLabel := "watching"
						if filterAgent != "" {
							watchLabel = fmt.Sprintf("watching @%s", filterAgent)
						}
						fmt.Fprintf(out, "--- %s (Ctrl+C to stop) ---\n", watchLabel)
					}
					lastMsg := recent[len(recent)-1]
					cursor = &types.MessageCursor{GUID: lastMsg.ID, TS: lastMsg.TS}
				} else if !ctx.JSONMode {
					watchLabel := "watching"
					if filterAgent != "" {
						watchLabel = fmt.Sprintf("watching @%s", filterAgent)
					}
					fmt.Fprintf(out, "--- %s (Ctrl+C to stop) ---\n", watchLabel)
				}
			}

			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			// Heartbeat tracking for daemon-managed agents
			agentID := os.Getenv("FRAY_AGENT_ID")
			var minCheckinMs int64
			var lastActivityTime time.Time
			var lastWarningLevel int // 0=none, 1=5min, 2=2min, 3=1min

			if agentID != "" {
				agent, err := db.GetAgent(ctx.DB, agentID)
				if err == nil && agent != nil && agent.Invoke != nil {
					_, _, minCheckinMs, _ = daemon.GetTimeouts(agent.Invoke)
				}
				if minCheckinMs == 0 {
					minCheckinMs = 600000 // default 10m
				}

				// Get actual last activity time (matches daemon's done-detection logic)
				// Use max of: last post, last heartbeat, or now (for new sessions)
				lastPostTs, _ := db.GetAgentLastPostTime(ctx.DB, agentID)
				lastHeartbeatTs := int64(0)
				if agent != nil && agent.LastHeartbeat != nil {
					lastHeartbeatTs = *agent.LastHeartbeat
				}

				// Pick the most recent activity
				lastActivityMs := lastPostTs
				if lastHeartbeatTs > lastActivityMs {
					lastActivityMs = lastHeartbeatTs
				}

				if lastActivityMs > 0 {
					lastActivityTime = time.UnixMilli(lastActivityMs)
				} else {
					// No prior activity - treat as just started
					lastActivityTime = time.Now()
				}

				if !ctx.JSONMode {
					elapsed := time.Since(lastActivityTime).Round(time.Second)
					remaining := time.Duration(minCheckinMs)*time.Millisecond - elapsed
					if remaining < 0 {
						remaining = 0
					}
					fmt.Fprintf(out, "[heartbeat] @%s: last activity %s ago, recycle in %s\n",
						agentID, elapsed, remaining.Round(time.Second))
				}
			}

			// Heartbeat status ticker (every 30s)
			var heartbeatTicker *time.Ticker
			if agentID != "" && !ctx.JSONMode {
				heartbeatTicker = time.NewTicker(30 * time.Second)
				defer heartbeatTicker.Stop()
			}

			for {
				select {
				case <-stop:
					return nil
				case <-ticker.C:
					newMessages, err := db.GetMessages(ctx.DB, &types.MessageQueryOptions{Since: cursor, IncludeArchived: includeArchived})
					if err != nil {
						return writeCommandError(cmd, err)
					}
					newMessages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, newMessages)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					if len(newMessages) == 0 {
						continue
					}

					// Update cursor first (before filtering) so we don't re-fetch filtered messages
					lastMsg := newMessages[len(newMessages)-1]
					cursor = &types.MessageCursor{GUID: lastMsg.ID, TS: lastMsg.TS}

					// Check if any message is from our agent (resets timer)
					if agentID != "" {
						for _, msg := range newMessages {
							if msg.FromAgent == agentID {
								lastActivityTime = time.Now()
								lastWarningLevel = 0
							}
						}
					}

					// Filter to agent-relevant messages if --as is set
					if filterAgent != "" {
						filtered := make([]types.Message, 0, len(newMessages))
						for _, msg := range newMessages {
							if isMessageRelevantToAgent(ctx.DB, msg, filterAgent) {
								filtered = append(filtered, msg)
							}
						}
						newMessages = filtered
					}

					if len(newMessages) == 0 {
						continue
					}

					if ctx.JSONMode {
						encoder := json.NewEncoder(out)
						for _, msg := range newMessages {
							_ = encoder.Encode(msg)
						}
					} else {
						for _, msg := range newMessages {
							fmt.Fprintln(out, FormatMessage(msg, projectName, agentBases))
						}
					}

				case <-func() <-chan time.Time {
					if heartbeatTicker != nil {
						return heartbeatTicker.C
					}
					return nil
				}():
					// Show heartbeat status
					elapsed := time.Since(lastActivityTime)
					remaining := time.Duration(minCheckinMs)*time.Millisecond - elapsed
					if remaining < 0 {
						remaining = 0
					}

					// Warn at thresholds: 5min, 2min, 1min
					warningLevel := 0
					if remaining <= 1*time.Minute {
						warningLevel = 3
					} else if remaining <= 2*time.Minute {
						warningLevel = 2
					} else if remaining <= 5*time.Minute {
						warningLevel = 1
					}

					if warningLevel > lastWarningLevel {
						lastWarningLevel = warningLevel
						switch warningLevel {
						case 1:
							fmt.Fprintf(out, "[heartbeat] ⚠️  %s until checkin timeout. Post something or run: fray heartbeat --as %s\n",
								remaining.Round(time.Second), agentID)
						case 2:
							fmt.Fprintf(out, "[heartbeat] ⚠️  %s remaining! Post or: fray heartbeat --as %s\n",
								remaining.Round(time.Second), agentID)
						case 3:
							fmt.Fprintf(out, "[heartbeat] 🚨 %s remaining! POST NOW or: fray heartbeat --as %s\n",
								remaining.Round(time.Second), agentID)
						}
					}
				}
			}
		},
	}

	cmd.Flags().Int("last", 10, "show last N messages before streaming")
	cmd.Flags().Bool("archived", false, "include archived messages")
	cmd.Flags().String("as", "", "filter to agent-relevant events (mentions, reactions, replies)")
	return cmd
}

// isMessageRelevantToAgent checks if a message is relevant to the specified agent.
// Relevant = mentions agent, is a reply to agent's message, or is a reaction to agent's message.
func isMessageRelevantToAgent(database *sql.DB, msg types.Message, agentPrefix string) bool {
	// Check if message is FROM the agent (always show own messages)
	if msg.FromAgent == agentPrefix {
		return true
	}
	parsed, err := core.ParseAgentID(msg.FromAgent)
	if err == nil && parsed.Base == agentPrefix {
		return true
	}

	// Check if message mentions the agent
	for _, mention := range msg.Mentions {
		if mention == "all" || mention == agentPrefix {
			return true
		}
		if len(mention) > len(agentPrefix) && mention[:len(agentPrefix)+1] == agentPrefix+"." {
			return true
		}
	}

	// Check if message is a reply to one of the agent's messages
	if msg.ReplyTo != nil {
		parentMsg, err := db.GetMessage(database, *msg.ReplyTo)
		if err == nil && parentMsg != nil {
			if parentMsg.FromAgent == agentPrefix {
				return true
			}
			parsed, err := core.ParseAgentID(parentMsg.FromAgent)
			if err == nil && parsed.Base == agentPrefix {
				return true
			}
		}
	}

	// Check if this is an event message about a reaction to agent's message
	if msg.Type == types.MessageTypeEvent && msg.References != nil {
		refMsg, err := db.GetMessage(database, *msg.References)
		if err == nil && refMsg != nil {
			if refMsg.FromAgent == agentPrefix {
				return true
			}
			parsed, err := core.ParseAgentID(refMsg.FromAgent)
			if err == nil && parsed.Base == agentPrefix {
				return true
			}
		}
	}

	return false
}
