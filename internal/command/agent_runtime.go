package command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/daemon"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

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

				tags := []string{}
				if agent.Managed {
					tags = append(tags, "managed")
				}
				if agent.AAPGUID != nil {
					tags = append(tags, "AAP")
				}

				tagStr := ""
				if len(tags) > 0 {
					tagStr = " [" + strings.Join(tags, ", ") + "]"
				}

				fmt.Fprintf(out, "@%s: %s (driver: %s)%s\n", agent.AgentID, presence, driver, tagStr)
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
