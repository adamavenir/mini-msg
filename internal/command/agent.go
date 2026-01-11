package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/daemon"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewAgentCmd creates the parent agent command.
func NewAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage daemon-controlled agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		NewAgentCreateCmd(),
		NewAgentUpdateCmd(),
		NewAgentStartCmd(),
		NewAgentRefreshCmd(),
		NewAgentEndCmd(),
		NewAgentListCmd(),
		NewAgentStatusCmd(),
		NewAgentCheckCmd(),
		NewAgentAvatarCmd(),
	)

	return cmd
}

// NewAgentCreateCmd creates a managed agent configuration.
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
				agent := types.Agent{
					GUID:         agentGUID,
					AgentID:      agentID,
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
func NewAgentStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start a fresh session for a managed agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			agent, err := resolveAgentByRef(cmdCtx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if !agent.Managed {
				return writeCommandError(cmd, fmt.Errorf("agent @%s is not managed (use 'fray agent create' first)", agent.AgentID))
			}

			if agent.Invoke == nil || agent.Invoke.Driver == "" {
				return writeCommandError(cmd, fmt.Errorf("agent @%s has no driver configured", agent.AgentID))
			}

			driver := daemon.GetDriver(agent.Invoke.Driver)
			if driver == nil {
				return writeCommandError(cmd, fmt.Errorf("unknown driver: %s", agent.Invoke.Driver))
			}

			customPrompt, _ := cmd.Flags().GetString("prompt")
			prompt := customPrompt
			if prompt == "" {
				prompt = buildFlyPrompt(agent.AgentID)
			}

			ctx := context.Background()
			proc, err := driver.Spawn(ctx, *agent, prompt)
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("spawn failed: %w", err))
			}

			// Drain pipes in background to prevent blocking
			drainProcessPipes(proc)

			if err := db.UpdateAgentPresence(cmdCtx.DB, agent.AgentID, types.PresenceSpawning); err != nil {
				driver.Cleanup(proc)
				return writeCommandError(cmd, err)
			}

			sessionStart := types.SessionStart{
				AgentID:   agent.AgentID,
				SessionID: proc.SessionID,
				StartedAt: time.Now().Unix(),
			}
			db.AppendSessionStart(cmdCtx.Project.DBPath, sessionStart)

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id":   agent.AgentID,
					"session_id": proc.SessionID,
					"driver":     agent.Invoke.Driver,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Started @%s (session: %s)\n", agent.AgentID, proc.SessionID)
			return nil
		},
	}

	cmd.Flags().String("prompt", "", "custom prompt (default: /fly equivalent)")

	return cmd
}

// NewAgentRefreshCmd ends the current session and starts a new one.
func NewAgentRefreshCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refresh <name>",
		Short: "End current session and start a new one",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			agent, err := resolveAgentByRef(cmdCtx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if !agent.Managed {
				return writeCommandError(cmd, fmt.Errorf("agent @%s is not managed", agent.AgentID))
			}

			if agent.Invoke == nil || agent.Invoke.Driver == "" {
				return writeCommandError(cmd, fmt.Errorf("agent @%s has no driver configured", agent.AgentID))
			}

			// Skip session_end recording - we don't track session_id for manual refreshes
			// The daemon handles session lifecycle properly via monitorProcess
			db.UpdateAgentPresence(cmdCtx.DB, agent.AgentID, types.PresenceOffline)

			driver := daemon.GetDriver(agent.Invoke.Driver)
			if driver == nil {
				return writeCommandError(cmd, fmt.Errorf("unknown driver: %s", agent.Invoke.Driver))
			}

			prompt := buildFlyPrompt(agent.AgentID)
			ctx := context.Background()
			proc, err := driver.Spawn(ctx, *agent, prompt)
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("spawn failed: %w", err))
			}

			// Drain pipes in background to prevent blocking
			drainProcessPipes(proc)

			db.UpdateAgentPresence(cmdCtx.DB, agent.AgentID, types.PresenceSpawning)

			sessionStart := types.SessionStart{
				AgentID:   agent.AgentID,
				SessionID: proc.SessionID,
				StartedAt: time.Now().Unix(),
			}
			db.AppendSessionStart(cmdCtx.Project.DBPath, sessionStart)

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id":   agent.AgentID,
					"session_id": proc.SessionID,
					"refreshed":  true,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Refreshed @%s (session: %s)\n", agent.AgentID, proc.SessionID)
			return nil
		},
	}

	return cmd
}

// NewAgentEndCmd gracefully ends an agent session.
func NewAgentEndCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "end <name>",
		Short: "Gracefully end an agent session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			agent, err := resolveAgentByRef(cmdCtx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if !agent.Managed {
				return writeCommandError(cmd, fmt.Errorf("agent @%s is not managed", agent.AgentID))
			}

			// Skip session_end recording - we don't track session_id for manual ends
			// The daemon handles session lifecycle properly via monitorProcess
			db.UpdateAgentPresence(cmdCtx.DB, agent.AgentID, types.PresenceOffline)

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agent.AgentID,
					"ended":    true,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Ended session for @%s\n", agent.AgentID)
			return nil
		},
	}

	return cmd
}

// NewAgentListCmd lists all agents with their managed status.
func NewAgentListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents with status and driver info",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			managedOnly, _ := cmd.Flags().GetBool("managed")

			agents, err := db.GetAllAgents(cmdCtx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if managedOnly {
				filtered := make([]types.Agent, 0)
				for _, a := range agents {
					if a.Managed {
						filtered = append(filtered, a)
					}
				}
				agents = filtered
			}

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(agents)
			}

			out := cmd.OutOrStdout()
			if len(agents) == 0 {
				fmt.Fprintln(out, "No agents found")
				return nil
			}

			for _, agent := range agents {
				driver := "-"
				if agent.Invoke != nil && agent.Invoke.Driver != "" {
					driver = agent.Invoke.Driver
				}

				presence := string(agent.Presence)
				if presence == "" {
					presence = "offline"
				}

				managed := ""
				if agent.Managed {
					managed = " [managed]"
				}

				fmt.Fprintf(out, "@%s: %s (driver: %s)%s\n", agent.AgentID, presence, driver, managed)
			}

			return nil
		},
	}

	cmd.Flags().Bool("managed", false, "show only managed agents")

	return cmd
}

// AgentStatusEntry represents a single agent in the status output.
type AgentStatusEntry struct {
	Name        string `json:"name"`
	Presence    string `json:"presence"`
	Status      string `json:"status"`
	IdleSeconds int64  `json:"idle_seconds"`
}

// AgentStatusOutput is the JSON output format for fray agent status.
type AgentStatusOutput struct {
	Agents []AgentStatusEntry `json:"agents"`
}

// NewAgentStatusCmd shows agent status for LLM polling.
func NewAgentStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show agent status for LLM polling",
		Long: `Output agent status in JSON format for LLM polling.

This command is designed to be called by haiku when evaluating
wake conditions with --prompt. Returns presence, status, and idle time.

Example output:
{
  "agents": [
    {"name": "dev", "presence": "active", "status": "fixing auth", "idle_seconds": 0},
    {"name": "designer", "presence": "idle", "status": "reviewing PR", "idle_seconds": 634}
  ]
}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			managedOnly, _ := cmd.Flags().GetBool("managed")

			agents, err := db.GetAllAgents(cmdCtx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			now := time.Now().Unix()
			var entries []AgentStatusEntry

			for _, agent := range agents {
				if managedOnly && !agent.Managed {
					continue
				}

				presence := string(agent.Presence)
				if presence == "" {
					presence = "offline"
				}

				status := ""
				if agent.Status != nil {
					status = *agent.Status
				}

				// Calculate idle time from last_seen
				idleSeconds := int64(0)
				if agent.LastSeen > 0 {
					idleSeconds = now - agent.LastSeen
					if idleSeconds < 0 {
						idleSeconds = 0
					}
				}

				entries = append(entries, AgentStatusEntry{
					Name:        agent.AgentID,
					Presence:    presence,
					Status:      status,
					IdleSeconds: idleSeconds,
				})
			}

			output := AgentStatusOutput{Agents: entries}

			// Always output JSON (this command is for LLM consumption)
			return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
		},
	}

	cmd.Flags().Bool("managed", false, "show only managed agents")

	return cmd
}

// NewAgentCheckCmd performs a daemon-less mention check and spawn.
func NewAgentCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <name>",
		Short: "Check for mentions and spawn if needed (daemon-less mode)",
		Long: `Check for @mentions of a managed agent and spawn a new session if needed.
Respects daemon lock - will not spawn if daemon is running.
Useful for CI/cron-based polling when daemon is not available.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			agent, err := resolveAgentByRef(cmdCtx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if !agent.Managed {
				return writeCommandError(cmd, fmt.Errorf("agent @%s is not managed", agent.AgentID))
			}

			frayDir := filepath.Dir(cmdCtx.Project.DBPath)
			if daemon.IsLocked(frayDir) {
				if cmdCtx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"agent_id":      agent.AgentID,
						"daemon_locked": true,
						"spawned":       false,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Daemon is running - skipping check for @%s\n", agent.AgentID)
				return nil
			}

			if agent.Presence == types.PresenceActive || agent.Presence == types.PresenceSpawning {
				if cmdCtx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"agent_id": agent.AgentID,
						"presence": agent.Presence,
						"spawned":  false,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "@%s is already %s\n", agent.AgentID, agent.Presence)
				return nil
			}

			opts := &types.MessageQueryOptions{Limit: 10}
			if agent.MentionWatermark != nil {
				opts.SinceID = *agent.MentionWatermark
			}

			mentions, err := db.GetMessagesWithMention(cmdCtx.DB, agent.AgentID, opts)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			var nonSelf []types.Message
			for _, msg := range mentions {
				if !daemon.IsSelfMention(msg, agent.AgentID) {
					nonSelf = append(nonSelf, msg)
				}
			}

			if len(nonSelf) == 0 {
				if cmdCtx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"agent_id": agent.AgentID,
						"mentions": 0,
						"spawned":  false,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "No new mentions for @%s\n", agent.AgentID)
				return nil
			}

			if agent.Invoke == nil || agent.Invoke.Driver == "" {
				return writeCommandError(cmd, fmt.Errorf("agent @%s has no driver configured", agent.AgentID))
			}

			driver := daemon.GetDriver(agent.Invoke.Driver)
			if driver == nil {
				return writeCommandError(cmd, fmt.Errorf("unknown driver: %s", agent.Invoke.Driver))
			}

			triggerMsg := nonSelf[0]
			prompt := buildResumePrompt(agent.AgentID, triggerMsg.ID)

			ctx := context.Background()
			proc, err := driver.Spawn(ctx, *agent, prompt)
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("spawn failed: %w", err))
			}

			// Drain pipes in background to prevent blocking
			drainProcessPipes(proc)

			db.UpdateAgentPresence(cmdCtx.DB, agent.AgentID, types.PresenceSpawning)

			// Update watermark in both SQLite and JSONL
			lastMsgID := nonSelf[len(nonSelf)-1].ID
			db.UpdateAgentWatermark(cmdCtx.DB, agent.AgentID, lastMsgID)
			db.AppendAgentUpdate(cmdCtx.Project.DBPath, db.AgentUpdateJSONLRecord{
				AgentID:          agent.AgentID,
				MentionWatermark: &lastMsgID,
			})

			sessionStart := types.SessionStart{
				AgentID:     agent.AgentID,
				SessionID:   proc.SessionID,
				TriggeredBy: &triggerMsg.ID,
				StartedAt:   time.Now().Unix(),
			}
			db.AppendSessionStart(cmdCtx.Project.DBPath, sessionStart)

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id":     agent.AgentID,
					"session_id":   proc.SessionID,
					"mentions":     len(nonSelf),
					"triggered_by": triggerMsg.ID,
					"spawned":      true,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Started @%s (session: %s, triggered by: %s)\n",
				agent.AgentID, proc.SessionID, triggerMsg.ID)
			return nil
		},
	}

	return cmd
}

// updateManagedAgentConfig updates the managed and invoke fields for an existing agent.
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
func drainProcessPipes(proc *daemon.Process) {
	if proc == nil {
		return
	}
	if proc.Stdout != nil {
		go io.Copy(io.Discard, proc.Stdout)
	}
	if proc.Stderr != nil {
		go io.Copy(io.Discard, proc.Stderr)
	}
}

// buildFlyPrompt creates a fresh-start prompt equivalent to /fly.
func buildFlyPrompt(agentID string) string {
	return fmt.Sprintf(`# Session Start

Your name for this session: **%s**

Use --as %s and @%s throughout.

## First: Learn the Tools

1. Invoke the fray-beads skill for coordination patterns
2. bd quickstart - issue tracking reference
3. fray quickstart - messaging reference

## Then: Join and Orient

fray new %s              # or fray back %s if rejoining
fray get meta/%s/notes   # prior session handoffs
fray get meta            # project-wide shared context

**Read any instructions left for you in the notes.**

## Check What's Ready

bd ready                   # unblocked issues
fray @%s           # direct mentions
fray questions             # open questions you might answer

## As You Work

- **Claim files before editing**: fray claim @%s --file <path>
- **Track work in beads**: bd update <id> --status in_progress when starting an issue
- **Close issues when done**: bd close <id> --reason "..." with what you implemented
- **Create issues for discovered work**: bd create "..." --type task

Claims auto-clear when you fray bye, or clear manually with fray clear @%s.
`, agentID, agentID, agentID, agentID, agentID, agentID, agentID, agentID, agentID)
}

// buildResumePrompt creates a minimal resume prompt for @mention wakeups.
func buildResumePrompt(agentID, triggerMsgID string) string {
	return fmt.Sprintf(`You've been @mentioned. Check fray for context.

Trigger: %s

Run: fray get %s
`, triggerMsgID, agentID)
}

// NewAgentAvatarCmd updates an agent's avatar.
func NewAgentAvatarCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "avatar <name> <avatar>",
		Short: "Set agent avatar character",
		Long: `Set the avatar character for an agent. The avatar is displayed in chat bylines.

Examples:
  fray agent avatar opus 🅾
  fray agent avatar designer 🅳
  fray agent avatar helper ✿`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmdCtx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer cmdCtx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])
			avatar := args[1]

			agent, err := db.GetAgent(cmdCtx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", agentID))
			}

			// Validate avatar
			if !core.IsValidAvatar(avatar) {
				return writeCommandError(cmd, fmt.Errorf("invalid avatar: %s (use a single character or emoji)", avatar))
			}

			// Update in database
			updates := db.AgentUpdates{
				Avatar: types.OptionalString{Set: true, Value: &avatar},
			}
			if err := db.UpdateAgent(cmdCtx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}

			// Append to JSONL
			if err := db.AppendAgentUpdate(cmdCtx.Project.DBPath, db.AgentUpdateJSONLRecord{
				AgentID: agentID,
				Avatar:  &avatar,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if cmdCtx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"avatar":   avatar,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Updated @%s avatar to %s\n", agentID, avatar)
			return nil
		},
	}

	return cmd
}

func init() {
	_ = os.Getenv("FRAY_AGENT_ID")
}
