package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/spf13/cobra"
)

// NewConfigCmd creates the config command.
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config [key] [value]",
		Short: "Get or set configuration",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			if len(args) == 0 {
				entries, err := db.GetAllConfig(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(entries)
				}
				out := cmd.OutOrStdout()
				if len(entries) == 0 {
					fmt.Fprintln(out, "No configuration set")
					return nil
				}
				fmt.Fprintln(out, "Configuration:")
				for _, entry := range entries {
					fmt.Fprintf(out, "  %s: %s\n", entry.Key, entry.Value)
				}
				return nil
			}

			key := normalizeConfigKey(args[0])
			if len(args) == 1 {
				value, err := db.GetConfig(ctx.DB, key)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if value == "" {
					return writeCommandError(cmd, fmt.Errorf("config key '%s' not found", args[0]))
				}
				if ctx.JSONMode {
					payload := map[string]string{args[0]: value}
					return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", args[0], value)
				return nil
			}

			if err := db.SetConfig(ctx.DB, key, args[1]); err != nil {
				return writeCommandError(cmd, err)
			}
			if ctx.JSONMode {
				payload := map[string]string{args[0]: args[1]}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", args[0], args[1])
			return nil
		},
	}

	return cmd
}

func normalizeConfigKey(value string) string {
	return strings.ReplaceAll(value, "-", "_")
}
