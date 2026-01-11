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

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
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

// stockAgent represents a suggested agent for interactive init.
type stockAgent struct {
	Name        string
	Description string
	Driver      string // default driver
}

// stockAgents is the default set of agents to suggest during init.
var stockAgents = []stockAgent{
	{Name: "dev", Description: "development work", Driver: "claude"},
	{Name: "desi", Description: "design review", Driver: "claude"},
	{Name: "arch", Description: "architecture review/plans", Driver: "codex"},
	{Name: "qa", Description: "testing and quality checks", Driver: "codex"},
	{Name: "pm", Description: "project coordination", Driver: "claude"},
	{Name: "knit", Description: "knowledge organization", Driver: "claude"},
	{Name: "party", Description: "workparty coordinator, fray advisor", Driver: "claude"},
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

			project, err := core.InitProject("", force)
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

				if jsonMode {
					_ = json.NewEncoder(out).Encode(result)
					return nil
				}
				if !alreadyExisted {
					fmt.Fprintf(out, "✓ Registered channel %s as '%s'\n", channelID, channelName)
				}
				fmt.Fprintln(out, "Initialized .fray/")

				// Interactive setup (only if TTY and not using defaults)
				if !useDefaults && isTTY(os.Stdin) {
					fmt.Fprintln(out, "")

					// Issue tracker selection
					issueTracker := promptIssueTracker()
					if issueTracker != "" && issueTracker != "none" {
						result.IssueTracker = issueTracker
						fmt.Fprintf(out, "✓ Issue tracker: %s\n", issueTracker)
					}

					// Agent selection
					agentsCreated := promptAndCreateAgents(project.DBPath)
					if len(agentsCreated) > 0 {
						result.AgentsCreated = agentsCreated
						fmt.Fprintf(out, "✓ Created %d managed agents\n", len(agentsCreated))
					}

					// Offer to install hooks
					if promptYesNo("Install Claude Code hooks?", true) {
						fmt.Fprintln(out, "")
						fmt.Fprintln(out, "Run: fray hook-install")
						fmt.Fprintln(out, "  (restart Claude Code after installing)")
					}
				} else {
					fmt.Fprintln(out, "")
					fmt.Fprintln(out, "Next steps:")
					fmt.Fprintln(out, "  fray new <name>                # Join as an agent")
					fmt.Fprintln(out, "  fray hook-install              # Install Claude Code hooks")
					fmt.Fprintln(out, "  fray hook-install --precommit  # Add git pre-commit hook for claims")
				}
			}

			return nil
		},
	}

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

// promptIssueTracker asks the user to select an issue tracker.
func promptIssueTracker() string {
	if !isTTY(os.Stdin) {
		return ""
	}

	fmt.Println("Issue tracker:")
	fmt.Println("  1. bd (beads - built-in)")
	fmt.Println("  2. gh (GitHub Issues)")
	fmt.Println("  3. tk (tickets)")
	fmt.Println("  4. md (markdown files in todo/)")
	fmt.Println("  5. none")
	fmt.Print("Select [1-5, default=1]: ")

	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	trimmed := strings.TrimSpace(text)

	switch trimmed {
	case "", "1":
		return "bd"
	case "2":
		return "gh"
	case "3":
		return "tk"
	case "4":
		return "md"
	case "5":
		return "none"
	default:
		return "bd"
	}
}

// promptYesNo asks a yes/no question with a default.
func promptYesNo(question string, defaultYes bool) bool {
	if !isTTY(os.Stdin) {
		return defaultYes
	}

	suffix := "[Y/n]"
	if !defaultYes {
		suffix = "[y/N]"
	}
	fmt.Printf("%s %s: ", question, suffix)

	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	trimmed := strings.ToLower(strings.TrimSpace(text))

	if trimmed == "" {
		return defaultYes
	}
	return trimmed == "y" || trimmed == "yes"
}

// promptAndCreateAgents shows stock agents and creates selected ones.
func promptAndCreateAgents(dbPath string) []string {
	if !isTTY(os.Stdin) {
		return nil
	}

	fmt.Println("")
	fmt.Println("Suggested agents (select with numbers, e.g., 1,2,5 or 'all' or 'none'):")
	for i, agent := range stockAgents {
		driverNote := ""
		if agent.Driver == "codex" {
			driverNote = " [codex]"
		}
		fmt.Printf("  %d. %s - %s%s\n", i+1, agent.Name, agent.Description, driverNote)
	}
	fmt.Print("Select [default=all]: ")

	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	trimmed := strings.TrimSpace(strings.ToLower(text))

	var selectedIndices []int
	if trimmed == "" || trimmed == "all" {
		for i := range stockAgents {
			selectedIndices = append(selectedIndices, i)
		}
	} else if trimmed == "none" {
		return nil
	} else {
		parts := strings.Split(trimmed, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			var idx int
			if _, err := fmt.Sscanf(part, "%d", &idx); err == nil && idx >= 1 && idx <= len(stockAgents) {
				selectedIndices = append(selectedIndices, idx-1)
			}
		}
	}

	if len(selectedIndices) == 0 {
		return nil
	}

	// Ask about driver customization
	fmt.Println("")
	fmt.Println("Default drivers: claude for most, codex for arch/qa")
	if !promptYesNo("Use defaults?", true) {
		// Let user customize per-agent
		for _, idx := range selectedIndices {
			agent := &stockAgents[idx]
			fmt.Printf("Driver for %s [claude/codex, default=%s]: ", agent.Name, agent.Driver)
			driverText, _ := reader.ReadString('\n')
			driverTrimmed := strings.TrimSpace(strings.ToLower(driverText))
			if driverTrimmed == "claude" || driverTrimmed == "codex" {
				agent.Driver = driverTrimmed
			}
		}
	}

	// Create the agents
	var created []string
	for _, idx := range selectedIndices {
		agent := stockAgents[idx]
		if err := createManagedAgent(dbPath, agent.Name, agent.Driver); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create agent %s: %v\n", agent.Name, err)
			continue
		}
		created = append(created, agent.Name)
	}

	return created
}

// createManagedAgent creates a managed agent configuration.
func createManagedAgent(dbPath string, name string, driver string) error {
	project, err := core.DiscoverProject("")
	if err != nil {
		return err
	}

	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		return err
	}
	defer dbConn.Close()

	// Check if agent already exists
	existing, _ := db.GetAgent(dbConn, name)
	if existing != nil {
		return nil // Already exists, skip
	}

	// Create the managed agent
	agentGUID, err := core.GenerateGUID("usr")
	if err != nil {
		return err
	}

	config, err := db.ReadProjectConfig(dbPath)
	if err != nil {
		return err
	}

	channelID := ""
	if config != nil {
		channelID = config.ChannelID
	}

	now := time.Now().Unix()
	agent := types.Agent{
		GUID:         agentGUID,
		AgentID:      name,
		RegisteredAt: now,
		LastSeen:     now,
		Managed:      true,
		Presence:     types.PresenceOffline,
		Invoke: &types.InvokeConfig{
			Driver: driver,
		},
	}
	_ = channelID // used by AppendAgent internally

	return db.AppendAgent(dbPath, agent)
}

// ensureLLMRouter creates the .fray/llm/ directory and stock mlld templates.
// Only creates files that don't exist (preserves user customizations).
func ensureLLMRouter(projectRoot string) error {
	llmDir := filepath.Join(projectRoot, ".fray", "llm")

	// Create llm/ directory
	if err := os.MkdirAll(llmDir, 0o755); err != nil {
		return fmt.Errorf("create llm directory: %w", err)
	}

	// Create llm/run/ directory for user scripts
	runDir := filepath.Join(llmDir, "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("create llm/run directory: %w", err)
	}

	// Create llm/routers/ directory for mlld routers
	routersDir := filepath.Join(llmDir, "routers")
	if err := os.MkdirAll(routersDir, 0o755); err != nil {
		return fmt.Errorf("create llm/routers directory: %w", err)
	}

	// Write mentions router template (if not exists)
	mentionsPath := filepath.Join(routersDir, "mentions.mld")
	if _, err := os.Stat(mentionsPath); os.IsNotExist(err) {
		if err := os.WriteFile(mentionsPath, db.MentionsRouterTemplate, 0o644); err != nil {
			return fmt.Errorf("write mentions router template: %w", err)
		}
	}

	// Write stdout-repair router template (if not exists)
	stdoutRepairPath := filepath.Join(routersDir, "stdout-repair.mld")
	if _, err := os.Stat(stdoutRepairPath); os.IsNotExist(err) {
		if err := os.WriteFile(stdoutRepairPath, db.StdoutRepairTemplate, 0o644); err != nil {
			return fmt.Errorf("write stdout-repair router template: %w", err)
		}
	}

	// Write stock status template (if not exists)
	statusPath := filepath.Join(llmDir, "status.mld")
	if _, err := os.Stat(statusPath); os.IsNotExist(err) {
		if err := os.WriteFile(statusPath, db.StatusTemplate, 0o644); err != nil {
			return fmt.Errorf("write status template: %w", err)
		}
	}

	// Create mlld-config.json (if not exists)
	// @proj resolver points to project root (absolute path)
	mlldConfigPath := filepath.Join(projectRoot, ".fray", "mlld-config.json")
	if _, err := os.Stat(mlldConfigPath); os.IsNotExist(err) {
		mlldConfig := map[string]any{
			"scriptDir": "llm/run",
			"resolvers": map[string]any{
				"prefixes": []map[string]any{
					{
						"prefix":   "@proj/",
						"resolver": "LOCAL",
						"config": map[string]any{
							"basePath": projectRoot,
						},
					},
				},
			},
		}
		configData, err := json.MarshalIndent(mlldConfig, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal mlld config: %w", err)
		}
		if err := os.WriteFile(mlldConfigPath, configData, 0o644); err != nil {
			return fmt.Errorf("write mlld config: %w", err)
		}
	}

	return nil
}
