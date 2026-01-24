package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/adamavenir/fray/internal/daemon"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/adamavenir/fray/internal/usage"
	"github.com/spf13/cobra"
)

// NewDaemonCmd creates the daemon command.
func NewDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "daemon",
		Aliases: []string{"d"},
		Short:   "Run the agent orchestration daemon",
		Long: `Start the daemon that watches for @mentions and spawns managed agents.

The daemon:
- Polls for new @mentions of managed agents
- Spawns agent sessions via configured drivers (claude, codex, opencode)
- Tracks agent presence (spawning, active, idle, error, offline)
- Records session lifecycle events to agents.jsonl
- Enforces 30s cooldown after clean exits (prevents rapid restart loops)

Interrupt Syntax (bypasses cooldown):
  !@agent     - interrupt + resume same session
  !!@agent    - interrupt + start fresh session (clears context)
  !@agent!    - interrupt, don't spawn after
  !!@agent!   - force end, don't restart

Only one daemon can run per project (enforced via lock file).
Use Ctrl+C or SIGTERM to gracefully shut down.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			// Don't defer close - daemon needs the connection

			pollInterval, _ := cmd.Flags().GetDuration("poll-interval")
			if pollInterval == 0 {
				pollInterval = 1 * time.Second
			}
			debug, _ := cmd.Flags().GetBool("debug")
			force, _ := cmd.Flags().GetBool("force")
			watchSync, _ := cmd.Flags().GetBool("watch")

			cfg := daemon.Config{
				PollInterval: pollInterval,
				Debug:        debug,
				Force:        force,
				WatchSync:    watchSync,
			}

			d := daemon.New(cmdCtx.Project, cmdCtx.DB, cfg)

			// Set up signal handling for graceful shutdown
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			// Start daemon
			if err := d.Start(ctx); err != nil {
				cmdCtx.DB.Close()
				return writeCommandError(cmd, err)
			}

			if cmdCtx.JSONMode {
				json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"status":        "started",
					"poll_interval": pollInterval.String(),
				})
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Daemon started (poll interval: %s)\n", pollInterval)
				fmt.Fprintln(cmd.OutOrStdout(), "Watching for @mentions of managed agents...")
				fmt.Fprintln(cmd.OutOrStdout(), "Press Ctrl+C to stop")
			}

			// Wait for shutdown signal
			<-sigCh

			if !cmdCtx.JSONMode {
				fmt.Fprintln(cmd.OutOrStdout(), "\nShutting down...")
			}

			// Graceful shutdown
			if err := d.Stop(); err != nil {
				cmdCtx.DB.Close()
				return writeCommandError(cmd, err)
			}

			cmdCtx.DB.Close()

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"status": "stopped",
				})
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Daemon stopped")
			return nil
		},
	}

	cmd.Flags().Duration("poll-interval", 1*time.Second, "how often to poll for mentions")
	cmd.Flags().Bool("debug", false, "enable debug logging")
	cmd.Flags().Bool("force", false, "kill existing daemon if running")
	cmd.Flags().Bool("watch", false, "watch shared files and rebuild on changes")

	cmd.AddCommand(NewDaemonStatusCmd())
	cmd.AddCommand(NewDaemonResetCmd())

	return cmd
}

// NewDaemonResetCmd creates the daemon reset command.
func NewDaemonResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset [agents...]",
		Short: "Reset managed agent presence states to offline",
		Long: `Reset presence state for managed agents.

Use this when:
- Daemon crashed and left agents in stale states
- Database has inconsistent presence data
- Starting fresh after manual intervention

Specified agents (or all if none given) will be set to "offline" presence.
Running agents will not be affected - this only updates the database.

Use --clear-sessions to also clear session IDs, forcing fresh spawns
instead of resumes on next @mention.

Examples:
  fray daemon reset                    # Reset all managed agents
  fray daemon reset opus clank         # Reset specific agents
  fray daemon reset @pm --clear-sessions  # Reset @pm and clear its session`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			clearSessions, _ := cmd.Flags().GetBool("clear-sessions")

			// Get all managed agents
			allAgents, err := db.GetManagedAgents(cmdCtx.DB)
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("failed to get managed agents: %w", err))
			}

			// Build agent map for lookup
			agentMap := make(map[string]types.Agent)
			for _, agent := range allAgents {
				agentMap[agent.AgentID] = agent
			}

			// Determine which agents to reset
			var managedAgents []types.Agent
			if len(args) > 0 {
				// Reset only specified agents
				for _, name := range args {
					// Strip @ prefix if present
					name = strings.TrimPrefix(name, "@")
					if agent, ok := agentMap[name]; ok {
						managedAgents = append(managedAgents, agent)
					} else {
						if !cmdCtx.JSONMode {
							fmt.Fprintf(cmd.OutOrStdout(), "Warning: @%s is not a managed agent, skipping\n", name)
						}
					}
				}
			} else {
				// Reset all managed agents
				managedAgents = allAgents
			}

			if len(managedAgents) == 0 {
				if cmdCtx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"status":           "no_agents",
						"count":            0,
						"sessions_cleared": false,
					})
				}
				fmt.Fprintln(cmd.OutOrStdout(), "No managed agents to reset")
				return nil
			}

			// Reset each agent's presence to offline
			resetCount := 0
			sessionsCleared := 0
			for _, agent := range managedAgents {
				if err := db.UpdateAgentPresenceWithAudit(cmdCtx.DB, cmdCtx.Project.DBPath, agent.AgentID, agent.Presence, types.PresenceOffline, "reset", "command", agent.Status); err != nil {
					if !cmdCtx.JSONMode {
						fmt.Fprintf(cmd.OutOrStdout(), "Warning: failed to reset @%s: %v\n", agent.AgentID, err)
					}
					continue
				}
				resetCount++

				// Clear session ID if requested
				if clearSessions {
					if err := db.UpdateAgentSessionID(cmdCtx.DB, agent.AgentID, ""); err != nil {
						if !cmdCtx.JSONMode {
							fmt.Fprintf(cmd.OutOrStdout(), "Warning: failed to clear session for @%s: %v\n", agent.AgentID, err)
						}
					} else {
						sessionsCleared++
					}
				}

				if !cmdCtx.JSONMode {
					msg := fmt.Sprintf("Reset @%s presence to offline (was: %s)", agent.AgentID, agent.Presence)
					if clearSessions {
						msg += " + cleared session"
					}
					fmt.Fprintln(cmd.OutOrStdout(), msg)
				}
			}

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"status":           "reset",
					"count":            resetCount,
					"sessions_cleared": sessionsCleared,
				})
			}

			summary := fmt.Sprintf("\nReset %d agent(s) to offline", resetCount)
			if clearSessions {
				summary += fmt.Sprintf(", cleared %d session(s)", sessionsCleared)
			}
			fmt.Fprintln(cmd.OutOrStdout(), summary)
			return nil
		},
	}

	cmd.Flags().Bool("clear-sessions", false, "also clear session IDs (forces fresh spawns)")

	return cmd
}

// NewDaemonStatusCmd creates the daemon status command.
func NewDaemonStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check daemon status and show agent info",
		Long: `Check if the daemon is running and show managed agent status.

Shows detailed info for each managed agent including:
- Presence state and when it changed
- Session ID and mode
- Token usage (input/output/context %)
- Time since last activity`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			frayDir := cmdCtx.Project.Root + "/.fray"
			isLocked := daemon.IsLocked(frayDir)

			// Get all managed agents
			managedAgents, _ := db.GetManagedAgents(cmdCtx.DB)

			if cmdCtx.JSONMode {
				// Build detailed agent info for JSON output
				agentInfo := make([]map[string]any, 0, len(managedAgents))
				for _, agent := range managedAgents {
					info := map[string]any{
						"agent_id": agent.AgentID,
						"presence": string(agent.Presence),
					}
					if agent.PresenceChangedAt != nil {
						info["presence_changed_at"] = *agent.PresenceChangedAt
						info["presence_age_sec"] = (time.Now().UnixMilli() - *agent.PresenceChangedAt) / 1000
					}
					if agent.LastSessionID != nil && *agent.LastSessionID != "" {
						info["session_id"] = *agent.LastSessionID
						// Get token usage for session
						if sessionUsage, err := usage.GetSessionUsage(*agent.LastSessionID); err == nil && sessionUsage != nil {
							info["input_tokens"] = sessionUsage.InputTokens
							info["output_tokens"] = sessionUsage.OutputTokens
							info["cached_tokens"] = sessionUsage.CachedTokens
							info["context_percent"] = sessionUsage.ContextPercent
							info["context_limit"] = sessionUsage.ContextLimit
						}
					}
					if agent.SessionMode != "" {
						info["session_mode"] = agent.SessionMode
					}
					if agent.LastKnownInput > 0 {
						info["last_known_input"] = agent.LastKnownInput
					}
					if agent.LastKnownOutput > 0 {
						info["last_known_output"] = agent.LastKnownOutput
					}
					if agent.TokensUpdatedAt > 0 {
						info["tokens_updated_at"] = agent.TokensUpdatedAt
						info["tokens_age_sec"] = (time.Now().UnixMilli() - agent.TokensUpdatedAt) / 1000
					}
					if agent.Invoke != nil {
						info["driver"] = agent.Invoke.Driver
					}
					agentInfo = append(agentInfo, info)
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"running": isLocked,
					"agents":  agentInfo,
				})
			}

			if isLocked {
				fmt.Fprintln(cmd.OutOrStdout(), "Daemon is running")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Daemon is not running")
			}

			// Show detailed info for all managed agents
			if len(managedAgents) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintln(cmd.OutOrStdout(), "Managed agents:")
				now := time.Now()
				for _, agent := range managedAgents {
					// Skip offline agents without sessions unless they're in error
					if agent.Presence == types.PresenceOffline && agent.LastSessionID == nil && agent.PresenceChangedAt == nil {
						continue
					}

					driver := "-"
					if agent.Invoke != nil {
						driver = agent.Invoke.Driver
					}

					// Presence age
					presenceAge := ""
					if agent.PresenceChangedAt != nil {
						age := now.Sub(time.UnixMilli(*agent.PresenceChangedAt))
						if age < time.Minute {
							presenceAge = fmt.Sprintf("%ds", int(age.Seconds()))
						} else if age < time.Hour {
							presenceAge = fmt.Sprintf("%dm", int(age.Minutes()))
						} else {
							presenceAge = fmt.Sprintf("%dh", int(age.Hours()))
						}
					}

					// Token info
					tokenInfo := ""
					if agent.LastSessionID != nil && *agent.LastSessionID != "" {
						if sessionUsage, err := usage.GetSessionUsage(*agent.LastSessionID); err == nil && sessionUsage != nil {
							tokenInfo = fmt.Sprintf("in:%dk out:%dk ctx:%d%%",
								sessionUsage.InputTokens/1000,
								sessionUsage.OutputTokens/1000,
								sessionUsage.ContextPercent)
						}
					}

					// Token watermark age
					watermarkAge := ""
					if agent.TokensUpdatedAt > 0 {
						age := now.Sub(time.UnixMilli(agent.TokensUpdatedAt))
						if age < time.Minute {
							watermarkAge = fmt.Sprintf("tok:%ds", int(age.Seconds()))
						} else if age < time.Hour {
							watermarkAge = fmt.Sprintf("tok:%dm", int(age.Minutes()))
						} else {
							watermarkAge = fmt.Sprintf("tok:%dh", int(age.Hours()))
						}
					}

					// Session mode
					sessionMode := ""
					if agent.SessionMode != "" {
						sessionMode = fmt.Sprintf("#%s", agent.SessionMode)
					}

					// Session ID (truncated)
					sessionID := ""
					if agent.LastSessionID != nil && *agent.LastSessionID != "" {
						sid := *agent.LastSessionID
						if len(sid) > 8 {
							sessionID = sid[:8]
						} else {
							sessionID = sid
						}
					}

					// Build output line
					line := fmt.Sprintf("  @%-12s %-10s %-6s %-6s",
						agent.AgentID,
						agent.Presence,
						driver,
						presenceAge)

					if sessionID != "" {
						line += fmt.Sprintf(" sess:%s", sessionID)
					}
					if sessionMode != "" {
						line += fmt.Sprintf(" %s", sessionMode)
					}
					if tokenInfo != "" {
						line += fmt.Sprintf(" %s", tokenInfo)
					}
					if watermarkAge != "" {
						line += fmt.Sprintf(" %s", watermarkAge)
					}

					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
			}

			return nil
		},
	}

	return cmd
}
