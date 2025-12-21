package command

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/spf13/cobra"
)

type initResult struct {
	Initialized    bool   `json:"initialized"`
	AlreadyExisted bool   `json:"already_existed"`
	ChannelID      string `json:"channel_id"`
	ChannelName    string `json:"channel_name"`
	Path           string `json:"path"`
	Error          string `json:"error,omitempty"`
}

// NewInitCmd creates the init command.
func NewInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize mm in current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			useDefaults, _ := cmd.Flags().GetBool("defaults")
			jsonMode, _ := cmd.Flags().GetBool("json")

			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			if useDefaults && !force {
				if existing, err := core.DiscoverProject(""); err == nil {
					configPath := filepath.Join(existing.Root, ".mm", "mm-config.json")
					if _, err := os.Stat(configPath); err == nil {
						config, err := db.ReadProjectConfig(existing.DBPath)
						if err == nil && config != nil && config.ChannelID != "" && config.ChannelName != "" {
							result := initResult{
								Initialized:    true,
								AlreadyExisted: true,
								ChannelID:      config.ChannelID,
								ChannelName:    config.ChannelName,
								Path:           existing.Root,
							}
							if jsonMode {
								_ = json.NewEncoder(out).Encode(result)
								return nil
							}
							fmt.Fprintf(out, "Already initialized: %s (%s)\n", config.ChannelName, config.ChannelID)
							return nil
						}
					}
				}
			}

			project, err := core.InitProject("", force)
			if err != nil {
				return writeInitError(errOut, jsonMode, err)
			}

			existingConfig, err := db.ReadProjectConfig(project.DBPath)
			if err != nil {
				return writeInitError(errOut, jsonMode, err)
			}
			channelID := ""
			channelName := ""
			if existingConfig != nil {
				channelID = existingConfig.ChannelID
				channelName = existingConfig.ChannelName
			}
			alreadyExisted := channelID != "" && channelName != ""

			if channelID == "" {
				defaultName := filepath.Base(project.Root)
				channelName = defaultName
				if !useDefaults {
					channelName = promptChannelName(defaultName)
				}
				generated, genErr := core.GenerateGUID("ch")
				if genErr != nil {
					return writeInitError(errOut, jsonMode, genErr)
				}
				channelID = generated

				update := db.ProjectConfig{
					Version:     1,
					ChannelID:   channelID,
					ChannelName: channelName,
					CreatedAt:   time.Now().UTC().Format(time.RFC3339),
					KnownAgents: map[string]db.ProjectKnownAgent{},
				}
				if existingConfig != nil {
					if existingConfig.Version != 0 {
						update.Version = existingConfig.Version
					}
					update.KnownAgents = existingConfig.KnownAgents
				}
				if _, err := db.UpdateProjectConfig(project.DBPath, update); err != nil {
					return writeInitError(errOut, jsonMode, err)
				}
			} else if channelName == "" {
				channelName = filepath.Base(project.Root)
				if _, err := db.UpdateProjectConfig(project.DBPath, db.ProjectConfig{ChannelName: channelName}); err != nil {
					return writeInitError(errOut, jsonMode, err)
				}
			}

			dbConn, err := db.OpenDatabase(project)
			if err != nil {
				return writeInitError(errOut, jsonMode, err)
			}
			if err := db.InitSchema(dbConn); err != nil {
				_ = dbConn.Close()
				return writeInitError(errOut, jsonMode, err)
			}
			if channelID != "" {
				if err := db.SetConfig(dbConn, "channel_id", channelID); err != nil {
					_ = dbConn.Close()
					return writeInitError(errOut, jsonMode, err)
				}
				if channelName != "" {
					if err := db.SetConfig(dbConn, "channel_name", channelName); err != nil {
						_ = dbConn.Close()
						return writeInitError(errOut, jsonMode, err)
					}
				}
			}
			_ = dbConn.Close()

			if channelID != "" && channelName != "" {
				if _, err := core.RegisterChannel(channelID, channelName, project.Root); err != nil {
					return writeInitError(errOut, jsonMode, err)
				}

				result := initResult{
					Initialized:    true,
					AlreadyExisted: alreadyExisted,
					ChannelID:      channelID,
					ChannelName:    channelName,
					Path:           project.Root,
				}

				if jsonMode {
					_ = json.NewEncoder(out).Encode(result)
					return nil
				}
				if !alreadyExisted {
					fmt.Fprintf(out, "✓ Registered channel %s as '%s'\n", channelID, channelName)
				}
				fmt.Fprintln(out, "Initialized .mm/")
				fmt.Fprintln(out, "")
				fmt.Fprintln(out, "Next steps:")
				fmt.Fprintln(out, "  mm new <name>                # Join as an agent")
				fmt.Fprintln(out, "  mm hook-install              # Install Claude Code hooks")
				fmt.Fprintln(out, "  mm hook-install --precommit  # Add git pre-commit hook for claims")
			}

			return nil
		},
	}

	cmd.Flags().Bool("force", false, "reinitialize even if already exists")
	cmd.Flags().Bool("defaults", false, "use default values without prompting (idempotent)")

	return cmd
}

func promptChannelName(defaultName string) string {
	if !isTTY(os.Stdin) {
		return defaultName
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stdout, "Channel name for this project? [%s]: ", defaultName)
	text, _ := reader.ReadString('\n')
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return defaultName
	}
	return trimmed
}

func writeInitError(errOut io.Writer, jsonMode bool, err error) error {
	if jsonMode {
		payload := initResult{Initialized: false, Error: err.Error()}
		data, _ := json.Marshal(payload)
		fmt.Fprintln(errOut, string(data))
		return err
	}
	fmt.Fprintf(errOut, "Error: %s\n", err.Error())
	return err
}

func isTTY(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
