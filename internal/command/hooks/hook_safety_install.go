package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// installSafetyHooks installs the Python safety guard and configures Claude hooks.
func installSafetyHooks(projectDir string, dryRun bool, out io.Writer, global bool) error {
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

	if dryRun {
		fmt.Fprintf(out, "\nWould install safety guard to: %s\n", guardPath)
		fmt.Fprintf(out, "Would update settings at: %s\n", settingsPath)
		fmt.Fprintln(out, "\nBlocked commands:")
		fmt.Fprintln(out, "  - git stash (when .fray/ has uncommitted changes)")
		fmt.Fprintln(out, "  - git checkout/restore <files> (when .fray/ dirty)")
		fmt.Fprintln(out, "  - git reset --hard (when .fray/ dirty)")
		fmt.Fprintln(out, "  - git clean -f (always)")
		fmt.Fprintln(out, "  - rm -rf .fray or rm .fray/*.jsonl")
		fmt.Fprintln(out, "  - git push --force to main/master")
		return nil
	}

	// Create hooks directory
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	// Write Python script
	if err := os.WriteFile(guardPath, []byte(safetyGuardScript), 0o755); err != nil {
		return fmt.Errorf("write safety guard: %w", err)
	}

	// Read existing settings
	settings := hookSettings{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		_ = json.Unmarshal(data, &settings)
	}
	if settings.Hooks == nil {
		settings.Hooks = map[string]any{}
	}

	// Add PreToolUse hook for safety guard
	// Use relative path for project-local, absolute for global
	var guardCmd string
	if global {
		guardCmd = fmt.Sprintf("python3 %s", guardPath)
	} else {
		guardCmd = "python3 $CLAUDE_PROJECT_DIR/.claude/hooks/fray_safety_guard.py"
	}

	preToolUseHooks := []any{
		map[string]any{
			"matcher": "Bash",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": guardCmd,
					"timeout": 10,
				},
			},
		},
	}

	// Merge with existing PreToolUse hooks if any
	if existing, ok := settings.Hooks["PreToolUse"].([]any); ok {
		// Check if safety guard already installed
		alreadyInstalled := false
		for _, hook := range existing {
			if hookMap, ok := hook.(map[string]any); ok {
				if hooks, ok := hookMap["hooks"].([]any); ok {
					for _, h := range hooks {
						if hm, ok := h.(map[string]any); ok {
							if cmd, ok := hm["command"].(string); ok {
								if strings.Contains(cmd, "fray_safety_guard") {
									alreadyInstalled = true
									break
								}
							}
						}
					}
				}
			}
		}
		if !alreadyInstalled {
			settings.Hooks["PreToolUse"] = append(existing, preToolUseHooks...)
		}
	} else {
		settings.Hooks["PreToolUse"] = preToolUseHooks
	}

	// Write updated settings
	merged, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	merged = append(merged, '\n')
	if err := os.WriteFile(settingsPath, merged, 0o644); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	scope := "project"
	if global {
		scope = "global"
	}

	fmt.Fprintf(out, "\nSafety hooks installed (%s):\n", scope)
	fmt.Fprintf(out, "  Guard script: %s\n", guardPath)
	fmt.Fprintf(out, "  Settings: %s\n", settingsPath)
	fmt.Fprintln(out, "\nProtections enabled:")
	fmt.Fprintln(out, "  - git stash blocked when .fray/ has uncommitted changes")
	fmt.Fprintln(out, "  - git checkout/restore <files> blocked when .fray/ dirty")
	fmt.Fprintln(out, "  - git reset --hard blocked when .fray/ dirty")
	fmt.Fprintln(out, "  - git clean -f always blocked")
	fmt.Fprintln(out, "  - rm .fray/ or .fray/*.jsonl blocked")
	fmt.Fprintln(out, "  - git push --force to main/master blocked")

	return nil
}
