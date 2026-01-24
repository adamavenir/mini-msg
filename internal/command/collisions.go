package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

// NewCollisionsCmd creates the collisions command.
func NewCollisionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collisions",
		Short: "List GUID collisions detected during rebuild",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			clearLog, _ := cmd.Flags().GetBool("clear")
			if clearLog {
				if err := db.ClearCollisionLog(ctx.Project.DBPath); err != nil {
					return writeCommandError(cmd, err)
				}
				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"cleared": true})
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Cleared collision log.")
				return nil
			}

			logData, err := db.ReadCollisionLog(ctx.Project.DBPath)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(logData)
			}

			if logData == nil || len(logData.Collisions) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No GUID collisions recorded.")
				return nil
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "GUID collisions (%d):\n", len(logData.Collisions))
			for _, collision := range logData.Collisions {
				fmt.Fprintf(out, "- %s %s\n", collision.Type, collision.GUID)
				for _, entry := range collision.Entries {
					preview := strings.TrimSpace(entry.Preview)
					if preview != "" {
						preview = " — " + preview
					}
					fmt.Fprintf(out, "  %s @%d%s\n", entry.Machine, entry.TS, preview)
				}
			}
			return nil
		},
	}

	cmd.Flags().Bool("clear", false, "clear collision log after review")

	return cmd
}
