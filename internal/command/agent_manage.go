package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/daemon"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

func NewAgentCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a managed agent configuration",
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

			driver, _ := cmd.Flags().GetString("driver")
			if driver == "" {
				driver = "claude"
			}
			if daemon.GetDriver(driver) == nil {
				return writeCommandError(cmd, fmt.Errorf("unknown driver: %s (valid: claude, codex, opencode)", driver))
			}

			promptDelivery, _ := cmd.Flags().GetString("prompt-delivery")
			if promptDelivery == "" {
				switch driver {
				case "claude":
					promptDelivery = string(types.PromptDeliveryStdin)
				case "codex":
					promptDelivery = string(types.PromptDeliveryArgs)
				case "opencode":
					promptDelivery = string(types.PromptDeliveryTempfile)
				}
			}

			spawnTimeout, _ := cmd.Flags().GetInt64("spawn-timeout")
			idleAfter, _ := cmd.Flags().GetInt64("idle-after")
			minCheckin, _ := cmd.Flags().GetInt64("min-checkin")
			maxRuntime, _ := cmd.Flags().GetInt64("max-runtime")
			model, _ := cmd.Flags().GetString("model")
			trust, _ := cmd.Flags().GetStringSlice("trust")

			existing, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			now := time.Now().Unix()
			invoke := &types.InvokeConfig{
				Driver:         driver,
				Model:          model,
				Trust:          trust,
				PromptDelivery: types.PromptDelivery(promptDelivery),
				SpawnTimeoutMs: spawnTimeout,
				IdleAfterMs:    idleAfter,
				MinCheckinMs:   minCheckin,
				MaxRuntimeMs:   maxRuntime,
			}

			if existing != nil {
				if err := updateManagedAgentConfig(ctx.DB, agentID, true, invoke); err != nil {
					return writeCommandError(cmd, err)
				}
				managed := true
				if err := db.AppendAgentUpdate(ctx.Project.DBPath, db.AgentUpdateJSONLRecord{
					AgentID: agentID,
					Managed: &managed,
					Invoke:  invoke,
				}); err != nil {
					return writeCommandError(cmd, err)
				}
				// Ensure agent thread hierarchy exists (backfills for existing agents)
				if err := ensureAgentHierarchy(ctx, agentID); err != nil {
					return writeCommandError(cmd, err)
				}
			} else {
				agentGUID, err := core.GenerateGUID("usr")
				if err != nil {
					return writeCommandError(cmd, err)
				}

				// Create AAP identity (without key by default for daemon agents)
				var aapGUID *string
				noAAP, _ := cmd.Flags().GetBool("no-aap")
				if !noAAP {
					identity, err := createAAPIdentity(agentID, false)
					if err != nil {
						// Non-fatal - continue without AAP identity
						fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to create AAP identity: %v\n", err)
					} else if identity != nil {
						aapGUID = &identity.Record.GUID
					}
				}

				agent := types.Agent{
					GUID:         agentGUID,
					AgentID:      agentID,
					AAPGUID:      aapGUID,
					RegisteredAt: now,
					LastSeen:     now,
					Managed:      true,
					Invoke:       invoke,
					Presence:     types.PresenceOffline,
				}
				if err := db.CreateAgent(ctx.DB, agent); err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendAgent(ctx.Project.DBPath, agent); err != nil {
					return writeCommandError(cmd, err)
				}
				// Create agent thread hierarchy for new agents
				if err := ensureAgentHierarchy(ctx, agentID); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"driver":   driver,
					"managed":  true,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created managed agent @%s (driver: %s)\n", agentID, driver)
			return nil
		},
	}

	cmd.Flags().String("driver", "claude", "CLI driver (claude, codex, opencode)")
	cmd.Flags().String("model", "", "model to use (e.g., sonnet-1m for 1M context)")
	cmd.Flags().StringSlice("trust", nil, "trust capabilities (e.g., wake allows agent to wake others)")
	cmd.Flags().String("prompt-delivery", "", "how prompts are passed (args, stdin, tempfile)")
	cmd.Flags().Int64("spawn-timeout", 30000, "max time in 'spawning' state (ms)")
	cmd.Flags().Int64("idle-after", 5000, "time since activity before 'idle' (ms)")
	cmd.Flags().Int64("min-checkin", 0, "done-detection: idle + no fray posts = kill (ms, 0 = disabled)")
	cmd.Flags().Int64("max-runtime", 0, "zombie safety net: forced termination (ms, 0 = unlimited)")
	cmd.Flags().Bool("no-aap", false, "skip AAP identity creation")

	return cmd
}

// NewAgentUpdateCmd updates an existing managed agent's configuration.
func NewAgentUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a managed agent's configuration",
		Long: `Update configuration for an existing managed agent.

Examples:
  fray agent update opus --model sonnet-1m
  fray agent update pm --trust wake
  fray agent update dev --driver claude --min-checkin 600000`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])

			agent, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", agentID))
			}

			// Start with existing config or create new one
			invoke := agent.Invoke
			if invoke == nil {
				invoke = &types.InvokeConfig{}
			}

			// Update fields if flags are set
			if cmd.Flags().Changed("driver") {
				driver, _ := cmd.Flags().GetString("driver")
				invoke.Driver = driver
			}
			if cmd.Flags().Changed("model") {
				model, _ := cmd.Flags().GetString("model")
				invoke.Model = model
			}
			if cmd.Flags().Changed("trust") {
				trust, _ := cmd.Flags().GetStringSlice("trust")
				invoke.Trust = trust
			}
			if cmd.Flags().Changed("prompt-delivery") {
				pd, _ := cmd.Flags().GetString("prompt-delivery")
				invoke.PromptDelivery = types.PromptDelivery(pd)
			}
			if cmd.Flags().Changed("spawn-timeout") {
				v, _ := cmd.Flags().GetInt64("spawn-timeout")
				invoke.SpawnTimeoutMs = v
			}
			if cmd.Flags().Changed("idle-after") {
				v, _ := cmd.Flags().GetInt64("idle-after")
				invoke.IdleAfterMs = v
			}
			if cmd.Flags().Changed("min-checkin") {
				v, _ := cmd.Flags().GetInt64("min-checkin")
				invoke.MinCheckinMs = v
			}
			if cmd.Flags().Changed("max-runtime") {
				v, _ := cmd.Flags().GetInt64("max-runtime")
				invoke.MaxRuntimeMs = v
			}

			// Update in database
			if err := updateManagedAgentConfig(ctx.DB, agentID, agent.Managed, invoke); err != nil {
				return writeCommandError(cmd, err)
			}

			// Persist to JSONL
			if err := db.AppendAgentUpdate(ctx.Project.DBPath, db.AgentUpdateJSONLRecord{
				AgentID: agentID,
				Invoke:  invoke,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"invoke":   invoke,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Updated @%s configuration\n", agentID)
			return nil
		},
	}

	cmd.Flags().String("driver", "", "CLI driver (claude, codex, opencode)")
	cmd.Flags().String("model", "", "model to use (e.g., sonnet-1m for 1M context)")
	cmd.Flags().StringSlice("trust", nil, "trust capabilities (e.g., wake allows agent to wake others)")
	cmd.Flags().String("prompt-delivery", "", "how prompts are passed (args, stdin, tempfile)")
	cmd.Flags().Int64("spawn-timeout", 0, "max time in 'spawning' state (ms)")
	cmd.Flags().Int64("idle-after", 0, "time since activity before 'idle' (ms)")
	cmd.Flags().Int64("min-checkin", 0, "done-detection: idle + no fray posts = kill (ms, 0 = disabled)")
	cmd.Flags().Int64("max-runtime", 0, "zombie safety net: forced termination (ms, 0 = unlimited)")

	return cmd
}

// NewAgentStartCmd starts a fresh session for a managed agent.

func updateManagedAgentConfig(dbConn *sql.DB, agentID string, managed bool, invoke *types.InvokeConfig) error {
	managedInt := 0
	if managed {
		managedInt = 1
	}

	var invokeJSON *string
	if invoke != nil {
		data, err := json.Marshal(invoke)
		if err != nil {
			return err
		}
		s := string(data)
		invokeJSON = &s
	}

	_, err := dbConn.Exec(`UPDATE fray_agents SET managed = ?, invoke = ? WHERE agent_id = ?`,
		managedInt, invokeJSON, agentID)
	return err
}

// drainProcessPipes starts goroutines to drain stdout/stderr to prevent blocking.
// The CLI commands spawn processes and return immediately; without draining,
// long-running sessions can block once the pipe buffers fill.
