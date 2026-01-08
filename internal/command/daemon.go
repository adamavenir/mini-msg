package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
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

			cfg := daemon.Config{
				PollInterval: pollInterval,
				Debug:        debug,
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

	cmd.AddCommand(NewDaemonStatusCmd())

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
