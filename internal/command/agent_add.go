package command

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/daemon"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

func NewAgentAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a shared agent locally",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])
			if !core.IsValidAgentID(agentID) {
				return writeCommandError(cmd, fmt.Errorf("invalid agent name: %s", agentID))
			}
			if !db.IsMultiMachineMode(ctx.Project.DBPath) {
				return writeCommandError(cmd, fmt.Errorf("agent add is only supported in multi-machine projects"))
			}

			exists, err := agentExistsInShared(ctx.Project.DBPath, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if !exists {
				return writeCommandError(cmd, fmt.Errorf("agent not found in shared data: @%s", agentID))
			}

			invoke, err := resolveAgentAddConfig(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			localAgent, err := findLocalAgent(ctx.Project.DBPath, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			now := time.Now().Unix()
			if localAgent == nil {
				guid, err := core.GenerateGUID("usr")
				if err != nil {
					return writeCommandError(cmd, err)
				}
				agent := types.Agent{
					GUID:         guid,
					AgentID:      agentID,
					RegisteredAt: now,
					LastSeen:     now,
					Managed:      true,
					Presence:     types.PresenceOffline,
					Invoke:       invoke,
				}
				if err := db.AppendAgent(ctx.Project.DBPath, agent); err != nil {
					return writeCommandError(cmd, err)
				}
				existing, err := db.GetAgent(ctx.DB, agentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if existing == nil {
					if err := db.CreateAgent(ctx.DB, agent); err != nil {
						return writeCommandError(cmd, err)
					}
				}
			} else {
				managed := true
				update := db.AgentUpdateJSONLRecord{
					AgentID: agentID,
					Managed: &managed,
					Invoke:  invoke,
				}
				if err := db.AppendAgentUpdate(ctx.Project.DBPath, update); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if err := updateManagedAgentConfig(ctx.DB, agentID, true, invoke); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"managed":  true,
					"driver":   invoke.Driver,
					"model":    invoke.Model,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Registered @%s locally\n", agentID)
			return nil
		},
	}

	cmd.Flags().String("driver", "", "CLI driver (claude, codex, opencode)")
	cmd.Flags().String("model", "", "model to use (e.g., sonnet-1m)")

	return cmd
}

// NewAgentRemoveCmd removes a locally registered agent.
func NewAgentRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Stop running an agent locally",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])
			if !core.IsValidAgentID(agentID) {
				return writeCommandError(cmd, fmt.Errorf("invalid agent name: %s", agentID))
			}
			if !db.IsMultiMachineMode(ctx.Project.DBPath) {
				return writeCommandError(cmd, fmt.Errorf("agent remove is only supported in multi-machine projects"))
			}

			localAgent, err := findLocalAgent(ctx.Project.DBPath, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if localAgent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not registered locally: @%s", agentID))
			}

			managed := false
			offline := string(types.PresenceOffline)
			update := db.AgentUpdateJSONLRecord{
				AgentID:  agentID,
				Managed:  &managed,
				Presence: &offline,
			}
			if err := db.AppendAgentUpdate(ctx.Project.DBPath, update); err != nil {
				return writeCommandError(cmd, err)
			}
			if err := updateManagedAgentConfig(ctx.DB, agentID, false, nil); err != nil {
				return writeCommandError(cmd, err)
			}
			_ = db.UpdateAgentPresence(ctx.DB, agentID, types.PresenceOffline)

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"managed":  false,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed @%s from local runtime\n", agentID)
			return nil
		},
	}

	return cmd
}

func resolveAgentAddConfig(cmd *cobra.Command) (*types.InvokeConfig, error) {
	driver, _ := cmd.Flags().GetString("driver")
	model, _ := cmd.Flags().GetString("model")

	if driver == "" {
		driver = "claude"
	}

	if isTTY(os.Stdin) {
		reader := bufio.NewReader(os.Stdin)
		if !cmd.Flags().Changed("driver") {
			fmt.Fprint(cmd.OutOrStdout(), "Driver [claude/codex/opencode, default=claude]: ")
			text, _ := reader.ReadString('\n')
			trimmed := strings.ToLower(strings.TrimSpace(text))
			if trimmed != "" {
				driver = trimmed
			}
		}
		if !cmd.Flags().Changed("model") {
			fmt.Fprint(cmd.OutOrStdout(), "Model [default=unset]: ")
			text, _ := reader.ReadString('\n')
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				model = trimmed
			}
		}
	}

	if daemon.GetDriver(driver) == nil {
		return nil, fmt.Errorf("unknown driver: %s (valid: claude, codex, opencode)", driver)
	}

	return &types.InvokeConfig{
		Driver: driver,
		Model:  model,
	}, nil
}

func agentExistsInShared(projectPath, agentID string) (bool, error) {
	descriptors, err := db.ReadAgentDescriptors(projectPath)
	if err != nil {
		return false, err
	}
	for _, descriptor := range descriptors {
		if descriptor.AgentID == agentID {
			return true, nil
		}
	}

	messages, err := db.ReadMessages(projectPath)
	if err != nil {
		return false, err
	}
	for _, message := range messages {
		if message.FromAgent == agentID {
			return true, nil
		}
	}
	return false, nil
}

func findLocalAgent(projectPath, agentID string) (*db.AgentJSONLRecord, error) {
	agents, err := db.ReadAgents(projectPath)
	if err != nil {
		return nil, err
	}
	for i := range agents {
		if agents[i].AgentID == agentID {
			return &agents[i], nil
		}
	}
	return nil, nil
}

// NewAgentCreateCmd creates a managed agent configuration.
