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
	"github.com/spf13/cobra"
)

// NewDaemonCmd creates the daemon command.
func NewDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the agent orchestration daemon",
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

			cfg := daemon.Config{
				PollInterval: pollInterval,
				Debug:        debug,
				Force:        force,
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
		Short: "Check if daemon is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			frayDir := cmdCtx.Project.Root + "/.fray"
			isLocked := daemon.IsLocked(frayDir)

			// Get agents in error state for debugging
			managedAgents, _ := db.GetManagedAgents(cmdCtx.DB)
			var errorAgents []types.Agent
			for _, agent := range managedAgents {
				if agent.Presence == types.PresenceError {
					errorAgents = append(errorAgents, agent)
				}
			}

			if cmdCtx.JSONMode {
				errorInfo := make([]map[string]any, 0, len(errorAgents))
				for _, agent := range errorAgents {
					info := map[string]any{
						"agent_id": agent.AgentID,
						"presence": string(agent.Presence),
					}
					if agent.LastSessionID != nil {
						info["session_id"] = *agent.LastSessionID
					}
					errorInfo = append(errorInfo, info)
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"running":      isLocked,
					"error_agents": errorInfo,
				})
			}

			if isLocked {
				fmt.Fprintln(cmd.OutOrStdout(), "Daemon is running")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Daemon is not running")
			}

			// Show agents in error state with session UUIDs for debugging
			if len(errorAgents) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintln(cmd.OutOrStdout(), "Agents in error state:")
				for _, agent := range errorAgents {
					sessionID := "(no session)"
					if agent.LastSessionID != nil && *agent.LastSessionID != "" {
						sessionID = *agent.LastSessionID
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  @%s: %s\n", agent.AgentID, sessionID)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintln(cmd.OutOrStdout(), "To recover: fray back <agent>")
			}

			return nil
		},
	}

	return cmd
}
