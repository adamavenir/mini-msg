package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// uninstallSafetyHooks removes the safety guard.
func uninstallSafetyHooks(projectDir string, out io.Writer, global bool) error {
	var claudeDir, settingsPath string

	if global {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		claudeDir = filepath.Join(homeDir, ".claude")
		settingsPath = filepath.Join(claudeDir, "settings.json")
	} else {
		claudeDir = filepath.Join(projectDir, ".claude")
		settingsPath = filepath.Join(claudeDir, "settings.local.json")
	}

	hooksDir := filepath.Join(claudeDir, "hooks")
	guardPath := filepath.Join(hooksDir, "fray_safety_guard.py")

	// Remove Python script
	if err := os.Remove(guardPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove safety guard: %w", err)
	}

	// Remove from settings
	settings := hookSettings{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		_ = json.Unmarshal(data, &settings)
	}

	if settings.Hooks != nil {
		if existing, ok := settings.Hooks["PreToolUse"].([]any); ok {
			var filtered []any
			for _, hook := range existing {
				keep := true
				if hookMap, ok := hook.(map[string]any); ok {
					if hooks, ok := hookMap["hooks"].([]any); ok {
						for _, h := range hooks {
							if hm, ok := h.(map[string]any); ok {
								if cmd, ok := hm["command"].(string); ok {
									if strings.Contains(cmd, "fray_safety_guard") {
										keep = false
										break
									}
								}
							}
						}
					}
				}
				if keep {
					filtered = append(filtered, hook)
				}
			}
			if len(filtered) > 0 {
				settings.Hooks["PreToolUse"] = filtered
			} else {
				delete(settings.Hooks, "PreToolUse")
			}
		}

		merged, err := json.MarshalIndent(settings, "", "  ")
		if err == nil {
			merged = append(merged, '\n')
			_ = os.WriteFile(settingsPath, merged, 0o644)
		}
	}

	scope := "project"
	if global {
		scope = "global"
	}
	fmt.Fprintf(out, "Safety hooks removed (%s)\n", scope)

	return nil
}

// uninstallIntegrationHooks removes fray integration hooks from settings.
func uninstallIntegrationHooks(projectDir string, out io.Writer, dryRun bool) error {
	claudeDir := filepath.Join(projectDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	// Check if settings file exists
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		fmt.Fprintln(out, "No integration hooks to uninstall (settings.local.json not found)")
		return nil
	}

	// Read existing settings
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	settings := hookSettings{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parse settings: %w", err)
	}

	if settings.Hooks == nil {
		fmt.Fprintln(out, "No integration hooks to uninstall")
		return nil
	}

	// Remove fray hooks - only remove if they contain fray commands
	hooksToCheck := []string{"SessionStart", "UserPromptSubmit", "PreCompact", "SessionEnd"}
	removed := 0
	for _, hookName := range hooksToCheck {
		if hookConfig, exists := settings.Hooks[hookName]; exists {
			if isFrayHook(hookConfig) {
				delete(settings.Hooks, hookName)
				removed++
			}
		}
	}

	if removed == 0 {
		fmt.Fprintln(out, "No fray integration hooks to uninstall")
		return nil
	}

	if dryRun {
		fmt.Fprintf(out, "Would remove %d hook(s) from: %s\n", removed, settingsPath)
		return nil
	}

	// Write updated settings or delete if empty
	if len(settings.Hooks) == 0 {
		if err := os.Remove(settingsPath); err != nil {
			return fmt.Errorf("remove settings file: %w", err)
		}
		fmt.Fprintf(out, "Removed %d hook(s) and deleted %s\n", removed, settingsPath)
	} else {
		updated, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal settings: %w", err)
		}
		updated = append(updated, '\n')
		if err := os.WriteFile(settingsPath, updated, 0o644); err != nil {
			return fmt.Errorf("write settings: %w", err)
		}
		fmt.Fprintf(out, "Removed %d hook(s) from %s\n", removed, settingsPath)
	}

	fmt.Fprintln(out, "\nRemoved hooks:")
	fmt.Fprintln(out, "  SessionStart, UserPromptSubmit, PreCompact, SessionEnd")

	return nil
}

// isFrayHook checks if a hook configuration was installed by fray
func isFrayHook(hook any) bool {
	hookBytes, _ := json.Marshal(hook)
	return strings.Contains(string(hookBytes), "fray hook-")
}

// uninstallPrecommitHook removes fray integration from git pre-commit hook
func uninstallPrecommitHook(projectDir string, dryRun bool, out io.Writer) {
	gitRoot, err := gitRootDir(projectDir)
	if err != nil {
		fmt.Fprintln(out, "\nWARNING: Not in a git repository, skipping pre-commit hook removal")
		return
	}

	precommitPath := filepath.Join(gitRoot, ".git", "hooks", "pre-commit")

	data, err := os.ReadFile(precommitPath)
	if err != nil {
		fmt.Fprintln(out, "\nNo pre-commit hook found")
		return
	}

	hookContent := string(data)
	if !strings.Contains(hookContent, "fray hook-precommit") {
		fmt.Fprintln(out, "\nPre-commit hook does not contain fray integration")
		return
	}

	// Check if it's our standalone hook or part of a larger hook
	if strings.Contains(hookContent, "# fray pre-commit hook") && strings.Count(hookContent, "\n") < 10 {
		// It's our standalone hook, delete the whole file
		if dryRun {
			fmt.Fprintf(out, "\nWould delete pre-commit hook: %s\n", precommitPath)
			return
		}
		if err := os.Remove(precommitPath); err != nil {
			fmt.Fprintf(out, "\nFailed to remove pre-commit hook: %v\n", err)
			return
		}
		fmt.Fprintf(out, "\nRemoved pre-commit hook: %s\n", precommitPath)
	} else {
		// It's part of a larger hook, remove just our lines
		lines := strings.Split(hookContent, "\n")
		var newLines []string
		skipNext := false
		for _, line := range lines {
			if skipNext {
				skipNext = false
				continue
			}
			if strings.Contains(line, "fray file claim conflict detection") {
				skipNext = true // Skip the next line (the fray command)
				continue
			}
			if strings.Contains(line, "fray hook-precommit") {
				continue
			}
			newLines = append(newLines, line)
		}
		newContent := strings.Join(newLines, "\n")

		if dryRun {
			fmt.Fprintf(out, "\nWould remove fray lines from pre-commit hook: %s\n", precommitPath)
			return
		}
		if err := os.WriteFile(precommitPath, []byte(newContent), 0o755); err != nil {
			fmt.Fprintf(out, "\nFailed to update pre-commit hook: %v\n", err)
			return
		}
		fmt.Fprintf(out, "\nRemoved fray integration from pre-commit hook: %s\n", precommitPath)
	}
}
