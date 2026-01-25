package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

type initResult struct {
	Initialized    bool     `json:"initialized"`
	AlreadyExisted bool     `json:"already_existed"`
	ChannelID      string   `json:"channel_id"`
	ChannelName    string   `json:"channel_name"`
	Path           string   `json:"path"`
	IssueTracker   string   `json:"issue_tracker,omitempty"`
	AgentsCreated  []string `json:"agents_created,omitempty"`
	Error          string   `json:"error,omitempty"`
}

// NewInitCmd creates the init command.
func NewInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize fray in current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			useDefaults, _ := cmd.Flags().GetBool("defaults")
			jsonMode, _ := cmd.Flags().GetBool("json")

			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			if useDefaults && !force {
				if existing, err := core.DiscoverProject(""); err == nil {
					configPath := filepath.Join(existing.Root, ".fray", "fray-config.json")
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

			projectRoot, err := os.Getwd()
			if err != nil {
				return writeInitError(errOut, jsonMode, err)
			}
			projectRoot, err = filepath.Abs(projectRoot)
			if err != nil {
				return writeInitError(errOut, jsonMode, err)
			}

			if !force && shouldJoinExistingProject(projectRoot) {
				return joinExistingProject(projectRoot, useDefaults, jsonMode, out, errOut)
			}

			project, err := core.InitProject(projectRoot, force)
			if err != nil {
				return writeInitError(errOut, jsonMode, err)
			}

			// Create llm/ directory and router.mld template
			if err := ensureLLMRouter(project.Root); err != nil {
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

				// Create agents (interactive or default)
				var agentsCreated []string
				if !useDefaults && isTTY(os.Stdin) {
					// Interactive: let user select agents
					agentsCreated = promptAndCreateAgents(project.DBPath)
				} else {
					// Non-interactive: create all stock agents
					for _, agent := range stockAgents {
						if err := createManagedAgent(project.DBPath, agent.Name, agent.Driver); err != nil {
							fmt.Fprintf(errOut, "Warning: failed to create agent %s: %v\n", agent.Name, err)
							continue
						}
						agentsCreated = append(agentsCreated, agent.Name)
					}
				}
				if len(agentsCreated) > 0 {
					result.AgentsCreated = agentsCreated
				}

				// JSON output (after agents created)
				if jsonMode {
					_ = json.NewEncoder(out).Encode(result)
					return nil
				}

				// Human-readable output
				if !alreadyExisted {
					fmt.Fprintf(out, "✓ Registered channel %s as '%s'\n", channelID, channelName)
				}
				fmt.Fprintln(out, "Initialized .fray/")

				if len(agentsCreated) > 0 {
					fmt.Fprintf(out, "✓ Created %d managed agents: %s\n", len(agentsCreated), strings.Join(agentsCreated, ", "))
				}

				// Interactive: offer to install hooks
				if !useDefaults && isTTY(os.Stdin) {
					fmt.Fprintln(out, "")
					if promptYesNo("Install Claude Code hooks?", true) {
						fmt.Fprintln(out, "")
						fmt.Fprintln(out, "Run: fray hook-install --safety")
						fmt.Fprintln(out, "  (restart Claude Code after installing)")
					}
				} else {
					fmt.Fprintln(out, "")
					fmt.Fprintln(out, "Next steps:")
					fmt.Fprintln(out, "  fray hook-install --safety     # Install hooks with safety guards")
					fmt.Fprintln(out, "  fray hook-install --precommit  # Add git pre-commit hook for claims")
				}
			}

			return nil
		},
	}

	cmd.Flags().Bool("defaults", false, "skip prompts and use defaults")
	cmd.Flags().Bool("json", false, "output JSON")

	return cmd
}
