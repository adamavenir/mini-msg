package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/adamavenir/fray/internal/core"
	"github.com/spf13/cobra"
)

type channelSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	HasLocal bool   `json:"has_local"`
}

// NewLsCmd creates the ls command.
func NewLsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List known channels",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, _ := cmd.Flags().GetBool("json")
			config, err := core.ReadGlobalConfig()
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if config == nil || len(config.Channels) == 0 {
				if jsonMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"channels": []channelSummary{}})
				}
				fmt.Fprintln(cmd.OutOrStdout(), "No channels registered")
				return nil
			}

			channels := make([]channelSummary, 0, len(config.Channels))
			for id, channel := range config.Channels {
				frayDir := filepath.Join(channel.Path, ".fray")
				_, err := os.Stat(frayDir)
				channels = append(channels, channelSummary{
					ID:       id,
					Name:     channel.Name,
					Path:     channel.Path,
					HasLocal: err == nil,
				})
			}

			sort.Slice(channels, func(i, j int) bool {
				if channels[i].Name == channels[j].Name {
					return channels[i].ID < channels[j].ID
				}
				return channels[i].Name < channels[j].Name
			})

			if jsonMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"channels": channels})
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Channels (%d):\n", len(channels))
			for _, channel := range channels {
				status := "missing"
				if channel.HasLocal {
					status = "local"
				}
				fmt.Fprintf(out, "  %s  %s  %s (%s)\n", channel.ID, channel.Name, channel.Path, status)
			}
			return nil
		},
	}

	return cmd
}
