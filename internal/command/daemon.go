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

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"running": isLocked,
				})
			}

			if isLocked {
				fmt.Fprintln(cmd.OutOrStdout(), "Daemon is running")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Daemon is not running")
			}
			return nil
		},
	}

	return cmd
}
