package command

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewWakeCmd creates the wake command.
func NewWakeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wake [agent]",
		Short: "Set wake conditions for an agent",
		Long: `Set conditions that will wake an agent when met.

By default, sets wake conditions for yourself (using FRAY_AGENT_ID).
Specify an agent to set wake conditions for another agent (requires trust).

Examples:
  # Wake self when specific users post
  fray wake --on @user1 @user2

  # Wake self after a delay
  fray wake --after 30m

  # Wake self on regex pattern match
  fray wake --pattern "build (failed|succeeded)"

  # Wake self on pattern with haiku assessment
  fray wake --pattern "error" --router

  # Wake another agent (requires trust)
  fray wake @other-agent --on @user1

  # Wake with context prompt
  fray wake --after 1h "Check on build status"`,
		Args: cobra.MaximumNArgs(2), // [agent] [prompt]
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			// Resolve target agent
			var targetAgentID string
			var prompt *string
			asFlag, _ := cmd.Flags().GetString("as")

			if len(args) > 0 && args[0][0] == '@' {
				// First arg is an agent
				targetAgentID, err = resolveAgentRef(ctx, args[0])
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if len(args) > 1 {
					prompt = &args[1]
				}
			} else {
				// No agent arg, use self
				if asFlag != "" {
					targetAgentID, err = resolveAgentRef(ctx, asFlag)
					if err != nil {
						return writeCommandError(cmd, err)
					}
				} else {
					targetAgentID = os.Getenv("FRAY_AGENT_ID")
				}
				if targetAgentID == "" {
					return writeCommandError(cmd, fmt.Errorf("--as flag or FRAY_AGENT_ID env var required"))
				}
				if len(args) > 0 {
					prompt = &args[0]
				}
			}

			// Verify target agent exists
			targetAgent, err := db.GetAgent(ctx.DB, targetAgentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if targetAgent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", targetAgentID))
			}

			// Resolve setter identity
			var setterID string
			if asFlag != "" {
				setterID, err = resolveAgentRef(ctx, asFlag)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			} else {
				setterID = os.Getenv("FRAY_AGENT_ID")
			}
			if setterID == "" {
				return writeCommandError(cmd, fmt.Errorf("--as flag or FRAY_AGENT_ID env var required"))
			}

			// Check trust if setting wake for another agent
			if targetAgentID != setterID {
				setter, err := db.GetAgent(ctx.DB, setterID)
				if err != nil || setter == nil {
					return writeCommandError(cmd, fmt.Errorf("setter agent not found: @%s", setterID))
				}
				if !hasTrust(setter, "wake") {
					return writeCommandError(cmd, fmt.Errorf("@%s does not have trust to set wake conditions for @%s", setterID, targetAgentID))
				}
			}

			// Parse flags
			onAgents, _ := cmd.Flags().GetStringSlice("on")
			afterDuration, _ := cmd.Flags().GetString("after")
			pattern, _ := cmd.Flags().GetString("pattern")
			inThread, _ := cmd.Flags().GetString("in")
			useRouter, _ := cmd.Flags().GetBool("router")

			// Determine wake type
			var wakeType types.WakeConditionType
			var afterMs *int64
			var expiresAt *int64

			switch {
			case len(onAgents) > 0:
				wakeType = types.WakeConditionOnMention
			case afterDuration != "":
				wakeType = types.WakeConditionAfter
				seconds, err := parseDuration(afterDuration)
				if err != nil {
					return writeCommandError(cmd, fmt.Errorf("invalid duration: %w", err))
				}
				ms := seconds * 1000
				afterMs = &ms
				expiry := time.Now().Unix() + seconds
				expiresAt = &expiry
			case pattern != "":
				wakeType = types.WakeConditionPattern
				// Validate regex
				if _, err := regexp.Compile(pattern); err != nil {
					return writeCommandError(cmd, fmt.Errorf("invalid regex pattern: %w", err))
				}
			default:
				return writeCommandError(cmd, fmt.Errorf("must specify --on, --after, or --pattern"))
			}

			// Resolve thread if specified
			var threadGUID *string
			if inThread != "" {
				thread, err := db.GetThreadByNameAny(ctx.DB, inThread)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if thread == nil {
					return writeCommandError(cmd, fmt.Errorf("thread not found: %s", inThread))
				}
				threadGUID = &thread.GUID
			}

			// Normalize on-agents (strip @ prefix)
			normalizedOnAgents := make([]string, 0, len(onAgents))
			for _, a := range onAgents {
				if len(a) > 0 && a[0] == '@' {
					normalizedOnAgents = append(normalizedOnAgents, a[1:])
				} else {
					normalizedOnAgents = append(normalizedOnAgents, a)
				}
			}

			// Create wake condition
			input := types.WakeConditionInput{
				AgentID:   targetAgentID,
				SetBy:     setterID,
				Type:      wakeType,
				OnAgents:  normalizedOnAgents,
				InThread:  threadGUID,
				AfterMs:   afterMs,
				UseRouter: useRouter,
				Prompt:    prompt,
			}

			if pattern != "" {
				input.Pattern = &pattern
			}

			condition, err := db.CreateWakeCondition(ctx.DB, ctx.Project.DBPath, input)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			// Set expiration for timer-based conditions
			if expiresAt != nil {
				condition.ExpiresAt = expiresAt
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(condition)
			}

			// Human-readable output
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Wake condition set for @%s\n", targetAgentID)
			switch wakeType {
			case types.WakeConditionOnMention:
				fmt.Fprintf(out, "  Trigger: when %s posts\n", formatAgentIDs(normalizedOnAgents))
			case types.WakeConditionAfter:
				fmt.Fprintf(out, "  Trigger: in %s\n", afterDuration)
			case types.WakeConditionPattern:
				fmt.Fprintf(out, "  Trigger: pattern /%s/\n", pattern)
				if useRouter {
					fmt.Fprintf(out, "  Router: enabled (haiku assessment)\n")
				}
			}
			if inThread != "" {
				fmt.Fprintf(out, "  Scope: %s\n", inThread)
			}
			if prompt != nil {
				fmt.Fprintf(out, "  Context: %s\n", *prompt)
			}

			return nil
		},
	}

	cmd.Flags().StringSlice("on", nil, "wake when these agents post (e.g., --on @user1 --on @user2)")
	cmd.Flags().String("after", "", "wake after duration (e.g., 30m, 2h, 1d)")
	cmd.Flags().String("pattern", "", "wake on regex pattern match")
	cmd.Flags().Bool("router", false, "use haiku router for pattern assessment")
	cmd.Flags().String("in", "", "scope to specific thread")
	cmd.Flags().String("as", "", "agent identity (uses FRAY_AGENT_ID if not set)")

	return cmd
}

// NewWakeListCmd creates the wake-list subcommand.
func NewWakeListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [agent]",
		Short: "List wake conditions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			var agentID string
			if len(args) > 0 {
				agentID, err = resolveAgentRef(ctx, args[0])
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			conditions, err := db.GetWakeConditions(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(conditions)
			}

			out := cmd.OutOrStdout()
			if len(conditions) == 0 {
				if agentID != "" {
					fmt.Fprintf(out, "No wake conditions for @%s\n", agentID)
				} else {
					fmt.Fprintln(out, "No wake conditions set")
				}
				return nil
			}

			for _, c := range conditions {
				fmt.Fprintf(out, "@%s:\n", c.AgentID)
				switch c.Type {
				case types.WakeConditionOnMention:
					fmt.Fprintf(out, "  when: %s posts\n", formatAgentIDs(c.OnAgents))
				case types.WakeConditionAfter:
					if c.ExpiresAt != nil {
						remaining := time.Until(time.Unix(*c.ExpiresAt, 0)).Round(time.Second)
						fmt.Fprintf(out, "  when: in %s\n", remaining)
					}
				case types.WakeConditionPattern:
					fmt.Fprintf(out, "  when: /%s/\n", *c.Pattern)
					if c.UseRouter {
						fmt.Fprintf(out, "  router: enabled\n")
					}
				}
				if c.InThread != nil {
					fmt.Fprintf(out, "  scope: %s\n", *c.InThread)
				}
				if c.Prompt != nil {
					fmt.Fprintf(out, "  context: %s\n", *c.Prompt)
				}
			}

			return nil
		},
	}

	return cmd
}

// NewWakeClearCmd creates the wake-clear subcommand.
func NewWakeClearCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clear [agent]",
		Short: "Clear wake conditions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			asFlag, _ := cmd.Flags().GetString("as")
			var agentID string

			if len(args) > 0 {
				agentID, err = resolveAgentRef(ctx, args[0])
				if err != nil {
					return writeCommandError(cmd, err)
				}
			} else {
				if asFlag != "" {
					agentID, err = resolveAgentRef(ctx, asFlag)
					if err != nil {
						return writeCommandError(cmd, err)
					}
				} else {
					agentID = os.Getenv("FRAY_AGENT_ID")
				}
				if agentID == "" {
					return writeCommandError(cmd, fmt.Errorf("--as flag or FRAY_AGENT_ID env var required"))
				}
			}

			count, err := db.ClearWakeConditions(ctx.DB, ctx.Project.DBPath, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"cleared":  count,
				})
			}

			out := cmd.OutOrStdout()
			if count == 0 {
				fmt.Fprintf(out, "No wake conditions to clear for @%s\n", agentID)
			} else {
				fmt.Fprintf(out, "Cleared %d wake condition(s) for @%s\n", count, agentID)
			}

			return nil
		},
	}

	cmd.Flags().String("as", "", "agent identity (uses FRAY_AGENT_ID if not set)")

	return cmd
}

// hasTrust checks if an agent has a specific trust capability.
func hasTrust(agent *types.Agent, capability string) bool {
	if agent == nil || agent.Invoke == nil {
		return false
	}
	for _, t := range agent.Invoke.Trust {
		if t == capability {
			return true
		}
	}
	return false
}

// formatAgentIDs formats a list of agent ID strings for display.
func formatAgentIDs(agents []string) string {
	if len(agents) == 0 {
		return "(none)"
	}
	result := ""
	for i, a := range agents {
		if i > 0 {
			if i == len(agents)-1 {
				result += " or "
			} else {
				result += ", "
			}
		}
		result += "@" + a
	}
	return result
}
