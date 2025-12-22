package command

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/spf13/cobra"
)

type destroyResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Removed bool   `json:"removed"`
}

// NewDestroyCmd creates the destroy command.
func NewDestroyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy <channel>",
		Short: "Delete a channel and all stored history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, _ := cmd.Flags().GetBool("json")
			force, _ := cmd.Flags().GetBool("force")

			config, err := core.ReadGlobalConfig()
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if config == nil || len(config.Channels) == 0 {
				return writeCommandError(cmd, fmt.Errorf("no channels registered"))
			}

			id, channel, ok := core.FindChannelByRef(args[0], config)
			if !ok {
				return writeCommandError(cmd, fmt.Errorf("channel not found: %s", args[0]))
			}

			mmDir := filepath.Join(channel.Path, ".mm")
			if !force {
				prompt := fmt.Sprintf("Destroy channel %s (%s) at %s? This deletes all history and cannot be undone. [y/N]: ", channel.Name, id, mmDir)
				confirmed, err := confirmPrompt(cmd.InOrStdin(), cmd.OutOrStdout(), prompt)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if !confirmed {
					if jsonMode {
						return json.NewEncoder(cmd.OutOrStdout()).Encode(destroyResult{
							ID:      id,
							Name:    channel.Name,
							Path:    channel.Path,
							Removed: false,
						})
					}
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}

			delete(config.Channels, id)
			if err := core.WriteGlobalConfig(*config); err != nil {
				return writeCommandError(cmd, err)
			}

			if err := os.RemoveAll(mmDir); err != nil {
				return writeCommandError(cmd, err)
			}

			if jsonMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(destroyResult{
					ID:      id,
					Name:    channel.Name,
					Path:    channel.Path,
					Removed: true,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Destroyed channel %s (%s) and removed %s\n", channel.Name, id, mmDir)
			return nil
		},
	}

	return cmd
}

func confirmPrompt(input io.Reader, output io.Writer, prompt string) (bool, error) {
	fmt.Fprint(output, prompt)
	reader := bufio.NewReader(input)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	response := strings.TrimSpace(strings.ToLower(line))
	return response == "y" || response == "yes", nil
}
