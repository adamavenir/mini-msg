package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/hostedsync"
	"github.com/spf13/cobra"
)

// NewSyncDaemonCmd creates the hosted sync daemon command.
func NewSyncDaemonCmd() *cobra.Command {
	var interval time.Duration
	var runOnce bool
	var batchSize int

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run hosted sync daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			config := ctx.ProjectConfig
			hostedURL := ""
			if config != nil && config.Sync != nil {
				hostedURL = config.Sync.HostedURL
			}

			auth, err := hostedsync.LoadAuth(ctx.Project.Root)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if auth == nil || auth.Token == "" {
				return writeCommandError(cmd, fmt.Errorf("hosted sync not registered; run `fray sync setup --hosted <url>`"))
			}
			if auth.HostedURL != "" {
				hostedURL = auth.HostedURL
			}
			if hostedURL == "" {
				return writeCommandError(cmd, fmt.Errorf("hosted url missing; run `fray sync setup --hosted <url>`"))
			}

			channelID := ctx.ChannelID
			if channelID == "" && auth.ChannelID != "" {
				channelID = auth.ChannelID
			}
			if channelID == "" && config != nil {
				channelID = config.ChannelID
			}
			if channelID == "" {
				return writeCommandError(cmd, fmt.Errorf("channel_id missing; run `fray init` first"))
			}

			machineID := auth.MachineID
			if machineID == "" {
				machineID = db.GetLocalMachineID(ctx.Project.Root)
			}
			if machineID == "" {
				return writeCommandError(cmd, fmt.Errorf("local machine id missing; run `fray init` first"))
			}

			client, err := hostedsync.NewClient(hostedURL, auth.Token)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			runner := &hostedsync.Runner{
				ProjectRoot: ctx.Project.Root,
				ChannelID:   channelID,
				MachineID:   machineID,
				Client:      client,
				BatchSize:   batchSize,
			}

			if runOnce {
				result, err := runner.SyncOnce(cmd.Context())
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"status": "ok",
						"result": result,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Synced (pushed %d, pulled %d)\n", result.PushedLines, result.PulledLines)
				return nil
			}

			if interval == 0 {
				interval = 5 * time.Second
			}

			runCtx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				cancel()
			}()

			if ctx.JSONMode {
				_ = json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"status":   "started",
					"interval": interval.String(),
				})
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Hosted sync started (interval: %s)\n", interval)
				fmt.Fprintln(cmd.OutOrStdout(), "Press Ctrl+C to stop")
			}

			err = runner.Run(runCtx, interval)
			if err != nil && !errors.Is(err, context.Canceled) {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"status": "stopped",
				})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Hosted sync stopped")
			return nil
		},
	}

	cmd.Flags().DurationVar(&interval, "interval", 5*time.Second, "sync interval")
	cmd.Flags().BoolVar(&runOnce, "once", false, "run a single sync pass and exit")
	cmd.Flags().IntVar(&batchSize, "batch", 200, "max lines per push batch")

	return cmd
}
