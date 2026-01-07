package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewRoleCmd creates the role command tree.
func NewRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role <agent> [role]",
		Short: "Manage agent roles",
		Long: `Manage agent roles. Roles provide knowledge domain context.

Examples:
  fray role opus                    # List opus's roles
  fray role opus add architect      # opus now holds architect role
  fray role opus play reviewer      # opus plays reviewer this session
  fray role opus drop architect     # opus no longer holds architect
`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])
			if agentID == "" {
				return writeCommandError(cmd, fmt.Errorf("invalid agent: %s", args[0]))
			}

			roles, err := db.GetAgentRoles(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(roles)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "@%s roles:\n", agentID)
			if len(roles.Held) == 0 && len(roles.Playing) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "  (none)")
				return nil
			}
			if len(roles.Held) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  holds: %s\n", strings.Join(roles.Held, ", "))
			}
			if len(roles.Playing) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  plays: %s (session)\n", strings.Join(roles.Playing, ", "))
			}
			return nil
		},
	}

	cmd.AddCommand(newRoleAddCmd())
	cmd.AddCommand(newRoleDropCmd())
	cmd.AddCommand(newRolePlayCmd())
	cmd.AddCommand(newRoleStopCmd())

	return cmd
}

func newRoleAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <agent> <role>",
		Short: "Agent now holds a role (persistent)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])
			if agentID == "" {
				return writeCommandError(cmd, fmt.Errorf("invalid agent: %s", args[0]))
			}

			roleName := strings.ToLower(strings.TrimSpace(args[1]))
			if roleName == "" {
				return writeCommandError(cmd, fmt.Errorf("role name required"))
			}

			// Check if already held
			has, err := db.HasRoleAssignment(ctx.DB, agentID, roleName)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if has {
				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"agent_id": agentID,
						"role":     roleName,
						"action":   "already_held",
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "@%s already holds %s\n", agentID, roleName)
				return nil
			}

			// Ensure role thread hierarchy exists
			if err := ensureRoleHierarchy(ctx, roleName); err != nil {
				return writeCommandError(cmd, err)
			}

			// Add to DB
			if err := db.AddRoleAssignment(ctx.DB, agentID, roleName); err != nil {
				return writeCommandError(cmd, err)
			}

			// Append to JSONL
			now := time.Now().UnixMilli()
			if err := db.AppendRoleHold(ctx.Project.DBPath, agentID, roleName, now); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"role":     roleName,
					"action":   "hold",
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "@%s now holds %s\n", agentID, roleName)
			return nil
		},
	}
}

func newRoleDropCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drop <agent> <role>",
		Short: "Agent no longer holds a role",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])
			if agentID == "" {
				return writeCommandError(cmd, fmt.Errorf("invalid agent: %s", args[0]))
			}

			roleName := strings.ToLower(strings.TrimSpace(args[1]))
			if roleName == "" {
				return writeCommandError(cmd, fmt.Errorf("role name required"))
			}

			// Remove from DB
			removed, err := db.RemoveRoleAssignment(ctx.DB, agentID, roleName)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if !removed {
				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"agent_id": agentID,
						"role":     roleName,
						"action":   "not_held",
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "@%s doesn't hold %s\n", agentID, roleName)
				return nil
			}

			// Append to JSONL
			now := time.Now().UnixMilli()
			if err := db.AppendRoleDrop(ctx.Project.DBPath, agentID, roleName, now); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"role":     roleName,
					"action":   "drop",
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "@%s dropped %s\n", agentID, roleName)
			return nil
		},
	}
}

func newRolePlayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "play <agent> <role>",
		Short: "Agent plays a role this session (session-scoped)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])
			if agentID == "" {
				return writeCommandError(cmd, fmt.Errorf("invalid agent: %s", args[0]))
			}

			roleName := strings.ToLower(strings.TrimSpace(args[1]))
			if roleName == "" {
				return writeCommandError(cmd, fmt.Errorf("role name required"))
			}

			// Check if already playing
			has, err := db.HasSessionRole(ctx.DB, agentID, roleName)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if has {
				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"agent_id": agentID,
						"role":     roleName,
						"action":   "already_playing",
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "@%s already playing %s\n", agentID, roleName)
				return nil
			}

			// Ensure role thread hierarchy exists
			if err := ensureRoleHierarchy(ctx, roleName); err != nil {
				return writeCommandError(cmd, err)
			}

			// Add to DB
			var sessionID *string
			if err := db.AddSessionRole(ctx.DB, agentID, roleName, sessionID); err != nil {
				return writeCommandError(cmd, err)
			}

			// Append to JSONL
			now := time.Now().UnixMilli()
			if err := db.AppendRolePlay(ctx.Project.DBPath, agentID, roleName, sessionID, now); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"role":     roleName,
					"action":   "play",
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "@%s now playing %s (session)\n", agentID, roleName)
			return nil
		},
	}
}

func newRoleStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <agent> <role>",
		Short: "Agent stops playing a session role",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := core.NormalizeAgentRef(args[0])
			if agentID == "" {
				return writeCommandError(cmd, fmt.Errorf("invalid agent: %s", args[0]))
			}

			roleName := strings.ToLower(strings.TrimSpace(args[1]))
			if roleName == "" {
				return writeCommandError(cmd, fmt.Errorf("role name required"))
			}

			// Remove from DB
			removed, err := db.RemoveSessionRole(ctx.DB, agentID, roleName)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if !removed {
				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"agent_id": agentID,
						"role":     roleName,
						"action":   "not_playing",
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "@%s wasn't playing %s\n", agentID, roleName)
				return nil
			}

			// Append to JSONL
			now := time.Now().UnixMilli()
			if err := db.AppendRoleStop(ctx.Project.DBPath, agentID, roleName, now); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"role":     roleName,
					"action":   "stop",
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "@%s stopped playing %s\n", agentID, roleName)
			return nil
		},
	}
}

// NewRolesCmd creates the roles list command.
func NewRolesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "roles",
		Short: "List all agents with their roles",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			allRoles, err := db.GetAllAgentRoles(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(allRoles)
			}

			if len(allRoles) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No agents have roles")
				return nil
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Agents with roles:")
			for agentID, roles := range allRoles {
				parts := []string{}
				if len(roles.Held) > 0 {
					parts = append(parts, fmt.Sprintf("holds: %s", strings.Join(roles.Held, ", ")))
				}
				if len(roles.Playing) > 0 {
					parts = append(parts, fmt.Sprintf("plays: %s", strings.Join(roles.Playing, ", ")))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  @%s  %s\n", agentID, strings.Join(parts, "; "))
			}
			return nil
		},
	}
}

// ensureRoleHierarchy creates the role thread hierarchy if it doesn't exist.
// Creates: meta/role-{role}/ (knowledge), meta/role-{role}/keys (system)
func ensureRoleHierarchy(ctx *CommandContext, roleName string) error {
	// First ensure meta/ root thread exists
	metaThread, err := ensureMetaThread(ctx)
	if err != nil {
		return err
	}

	// Role thread uses role-{name} format under meta/
	roleThreadName := fmt.Sprintf("role-%s", roleName)

	// Check if role thread exists under meta/
	roleThread, err := db.GetThreadByName(ctx.DB, roleThreadName, &metaThread.GUID)
	if err != nil {
		return err
	}

	if roleThread == nil {
		// Create role thread as child of meta/
		roleThread, err = createKnowledgeThread(ctx, roleThreadName, &metaThread.GUID)
		if err != nil {
			return err
		}
	}

	// Check and create keys subthread
	keysThread, err := db.GetThreadByName(ctx.DB, "keys", &roleThread.GUID)
	if err != nil {
		return err
	}
	if keysThread == nil {
		if _, err := createSystemThread(ctx, "keys", &roleThread.GUID); err != nil {
			return err
		}
	}

	return nil
}

// createKnowledgeThread creates a thread with type=knowledge.
func createKnowledgeThread(ctx *CommandContext, name string, parent *string) (*types.Thread, error) {
	thread, err := db.CreateThread(ctx.DB, types.Thread{
		Name:         name,
		ParentThread: parent,
		Type:         types.ThreadTypeKnowledge,
	})
	if err != nil {
		return nil, err
	}

	if err := db.AppendThread(ctx.Project.DBPath, thread, nil); err != nil {
		return nil, err
	}

	return &thread, nil
}

// createSystemThread creates a thread with type=system.
func createSystemThread(ctx *CommandContext, name string, parent *string) (*types.Thread, error) {
	thread, err := db.CreateThread(ctx.DB, types.Thread{
		Name:         name,
		ParentThread: parent,
		Type:         types.ThreadTypeSystem,
	})
	if err != nil {
		return nil, err
	}

	if err := db.AppendThread(ctx.Project.DBPath, thread, nil); err != nil {
		return nil, err
	}

	return &thread, nil
}
