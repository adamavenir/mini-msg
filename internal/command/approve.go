package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewApproveCmd creates the approve command.
func NewApproveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve <perm-guid> <option>",
		Short: "Approve a permission request",
		Long: `Approve a permission request with a specific option.

Options are numbered 1-3:
  1 - Allow once (just this invocation)
  2 - Allow this pattern (for the session)
  3 - Allow for all agents (persists in project settings)

Examples:
  fray approve perm-abc123 1    # Allow once
  fray approve perm-abc123 2    # Allow for session
  fray approve perm-abc123 3    # Allow for project`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			permGUID := args[0]
			optionStr := args[1]
			jsonMode, _ := cmd.Flags().GetBool("json")

			optionIdx, err := strconv.Atoi(optionStr)
			if err != nil || optionIdx < 1 || optionIdx > 3 {
				return fmt.Errorf("option must be 1, 2, or 3")
			}
			optionIdx-- // Convert to 0-indexed

			project, err := core.DiscoverProject("")
			if err != nil {
				return err
			}

			// Read the permission request
			req, err := db.ReadPermissionByGUID(project.Root, permGUID)
			if err != nil {
				return fmt.Errorf("read permission request: %w", err)
			}

			if req.Status != types.PermissionStatusPending {
				return fmt.Errorf("permission request already %s", req.Status)
			}

			if optionIdx >= len(req.Options) {
				return fmt.Errorf("invalid option %d, only %d options available", optionIdx+1, len(req.Options))
			}

			// Update the permission request
			respondedBy := "user"
			now := time.Now().Unix()
			update := db.PermissionUpdateJSONLRecord{
				GUID:        permGUID,
				Status:      string(types.PermissionStatusApproved),
				ChosenIndex: &optionIdx,
				RespondedBy: respondedBy,
				RespondedAt: now,
			}

			if err := db.AppendPermissionUpdate(project.Root, update); err != nil {
				return fmt.Errorf("update permission: %w", err)
			}

			chosenOption := req.Options[optionIdx]

			// If scope is project, add to settings
			if chosenOption.Scope == types.PermissionScopeProject {
				if err := addPermissionsToSettings(project.Root, chosenOption.Patterns); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update settings: %v\n", err)
				}
			}

			out := cmd.OutOrStdout()
			if jsonMode {
				result := map[string]any{
					"approved":    true,
					"guid":        permGUID,
					"option":      optionIdx + 1,
					"scope":       chosenOption.Scope,
					"patterns":    chosenOption.Patterns,
					"responded_at": now,
				}
				return json.NewEncoder(out).Encode(result)
			}

			fmt.Fprintf(out, "Approved: %s\n", chosenOption.Label)
			fmt.Fprintf(out, "Patterns: %v\n", chosenOption.Patterns)
			fmt.Fprintf(out, "Scope: %s\n", chosenOption.Scope)
			return nil
		},
	}

	return cmd
}

// NewDenyCmd creates the deny command.
func NewDenyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deny <perm-guid> [reason]",
		Short: "Deny a permission request",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			permGUID := args[0]
			reason := ""
			if len(args) > 1 {
				reason = args[1]
			}
			jsonMode, _ := cmd.Flags().GetBool("json")

			project, err := core.DiscoverProject("")
			if err != nil {
				return err
			}

			// Read the permission request
			req, err := db.ReadPermissionByGUID(project.Root, permGUID)
			if err != nil {
				return fmt.Errorf("read permission request: %w", err)
			}

			if req.Status != types.PermissionStatusPending {
				return fmt.Errorf("permission request already %s", req.Status)
			}

			// Update the permission request
			respondedBy := "user"
			now := time.Now().Unix()
			update := db.PermissionUpdateJSONLRecord{
				GUID:        permGUID,
				Status:      string(types.PermissionStatusDenied),
				RespondedBy: respondedBy,
				RespondedAt: now,
			}

			if err := db.AppendPermissionUpdate(project.Root, update); err != nil {
				return fmt.Errorf("update permission: %w", err)
			}

			out := cmd.OutOrStdout()
			if jsonMode {
				result := map[string]any{
					"denied":       true,
					"guid":         permGUID,
					"reason":       reason,
					"responded_at": now,
				}
				return json.NewEncoder(out).Encode(result)
			}

			fmt.Fprintf(out, "Denied: %s\n", permGUID)
			if reason != "" {
				fmt.Fprintf(out, "Reason: %s\n", reason)
			}
			return nil
		},
	}

	return cmd
}

// addPermissionsToSettings adds permission patterns to .claude/settings.local.json.
func addPermissionsToSettings(projectRoot string, patterns []string) error {
	claudeDir := filepath.Join(projectRoot, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	// Ensure .claude directory exists
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}

	// Read existing settings
	var settings map[string]any
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			settings = make(map[string]any)
		}
	} else {
		settings = make(map[string]any)
	}

	// Get or create permissions.allow array
	permissions, ok := settings["permissions"].(map[string]any)
	if !ok {
		permissions = make(map[string]any)
		settings["permissions"] = permissions
	}

	allowList, ok := permissions["allow"].([]any)
	if !ok {
		allowList = make([]any, 0)
	}

	// Add new patterns (avoid duplicates)
	existingPatterns := make(map[string]bool)
	for _, p := range allowList {
		if s, ok := p.(string); ok {
			existingPatterns[s] = true
		}
	}

	for _, pattern := range patterns {
		if !existingPatterns[pattern] {
			allowList = append(allowList, pattern)
		}
	}

	permissions["allow"] = allowList

	// Write back
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	return nil
}
