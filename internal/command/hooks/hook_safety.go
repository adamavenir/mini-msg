package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// safetyGuardScript is the Python script that blocks dangerous git commands
// when .fray/ has uncommitted changes.
const safetyGuardScript = `#!/usr/bin/env python3
"""
Fray Safety Guard - Protects .fray/ data from destructive git operations.

This hook blocks dangerous git commands when .fray/ has uncommitted changes,
preventing accidental loss of chat history and agent context.

Blocked commands:
- git stash (when .fray/ dirty) - could lose uncommitted fray data
- git checkout/restore <files> (when .fray/ dirty) - discards changes
- git reset --hard (when .fray/ dirty) - destroys uncommitted work
- git clean -f (always) - deletes untracked files
- rm -rf .fray or rm .fray/*.jsonl - protects source of truth
- git push --force to main/master - prevents history destruction

Install with: fray hook-install --safety
"""

import json
import re
import subprocess
import sys


def has_uncommitted_fray():
    """Check if .fray/ has uncommitted changes."""
    try:
        result = subprocess.run(
            ["git", "status", "--porcelain", ".fray/"],
            capture_output=True,
            text=True,
            timeout=5
        )
        return bool(result.stdout.strip())
    except Exception:
        return False


def get_uncommitted_fray_files():
    """Get list of uncommitted .fray/ files."""
    try:
        result = subprocess.run(
            ["git", "status", "--porcelain", ".fray/"],
            capture_output=True,
            text=True,
            timeout=5
        )
        return result.stdout.strip()
    except Exception:
        return ""


def deny(reason):
    """Return a denial response."""
    print(json.dumps({
        "hookSpecificOutput": {
            "permissionDecision": "deny",
            "permissionDecisionReason": reason
        }
    }))
    sys.exit(0)


def check_command(cmd):
    """Check if command should be blocked."""
    if not cmd or not isinstance(cmd, str):
        return

    # Normalize: handle /usr/bin/git, /bin/rm, etc.
    words = cmd.split()
    if not words:
        return

    # Get base command name
    base_cmd = os.path.basename(words[0])

    # === ALWAYS BLOCKED ===

    # Block rm -rf .fray or rm on .fray/*.jsonl
    if base_cmd == "rm":
        if ".fray" in cmd:
            if re.search(r'\.fray\b', cmd):
                deny(
                    "BLOCKED: Deleting .fray/ would destroy chat history.\n\n"
                    "The .fray/ directory contains your message history and agent data.\n"
                    "If you really need to remove it, do so manually outside of Claude Code."
                )

    # Block git clean -f (always dangerous for untracked files)
    if base_cmd == "git" and "clean" in cmd:
        # Allow dry-run: git clean -n, git clean --dry-run
        if "-n" in words or "--dry-run" in words:
            return
        # Block forced clean
        if "-f" in words or "--force" in words:
            deny(
                "BLOCKED: 'git clean -f' permanently deletes untracked files.\n\n"
                "This could delete .fray/ data or other important untracked files.\n"
                "Use 'git clean -n' to preview what would be deleted first."
            )

    # Block git push --force to main/master
    if base_cmd == "git" and "push" in cmd:
        has_force = "--force" in words or "-f" in words
        targets_main = "main" in words or "master" in words or "origin main" in cmd or "origin master" in cmd
        # Also catch push -f without explicit branch (pushes to current)
        if has_force and (targets_main or len([w for w in words if not w.startswith("-")]) <= 2):
            deny(
                "BLOCKED: Force pushing can destroy remote history.\n\n"
                "This is especially dangerous for main/master branches.\n"
                "If you need to force push, do so manually with explicit confirmation."
            )

    # === BLOCKED WHEN .fray/ IS DIRTY ===

    if not has_uncommitted_fray():
        return  # .fray/ is clean, allow command

    dirty_files = get_uncommitted_fray_files()
    fray_warning = (
        f"\n\nUncommitted .fray/ changes:\n{dirty_files}\n\n"
        "To proceed safely:\n"
        "  git add .fray/ && git commit -m 'fray: checkpoint'"
    )

    # Block git stash when .fray/ is dirty
    if base_cmd == "git" and "stash" in words:
        # Allow stash pop, stash apply, stash list, stash show
        safe_stash = any(w in words for w in ["pop", "apply", "list", "show", "branch"])
        if not safe_stash:
            deny(
                "BLOCKED: 'git stash' with uncommitted .fray/ changes.\n\n"
                "Stashing could lose your fray chat history if you forget to pop it "
                "or switch branches." + fray_warning
            )

    # Block git checkout <files> when .fray/ is dirty (but allow branch checkout)
    if base_cmd == "git" and "checkout" in words:
        # Allow: git checkout -b <branch>, git checkout <branch>
        # Block: git checkout -- <files>, git checkout <files>
        if "-b" in words or "--orphan" in words:
            return  # Creating new branch is safe
        if "--" in words:
            deny(
                "BLOCKED: 'git checkout --' discards uncommitted changes.\n\n"
                "This would permanently lose your .fray/ changes." + fray_warning
            )
        # Check if checking out files (not branches) - heuristic: has file-like args
        for word in words[2:]:  # Skip 'git checkout'
            if word.startswith("-"):
                continue
            if "." in word or "/" in word:
                deny(
                    "BLOCKED: 'git checkout <files>' with uncommitted .fray/ changes.\n\n"
                    "This could discard your fray data." + fray_warning
                )

    # Block git restore when .fray/ is dirty
    if base_cmd == "git" and "restore" in words:
        # Allow: git restore --staged (just unstages)
        if "--staged" in words and "--worktree" not in words:
            return
        deny(
            "BLOCKED: 'git restore' with uncommitted .fray/ changes.\n\n"
            "This could discard your fray data." + fray_warning
        )

    # Block git reset --hard when .fray/ is dirty
    if base_cmd == "git" and "reset" in words:
        if "--hard" in words or "--merge" in words:
            deny(
                "BLOCKED: 'git reset --hard' with uncommitted .fray/ changes.\n\n"
                "This would permanently destroy your uncommitted work." + fray_warning
            )


def main():
    try:
        input_data = sys.stdin.read()
        if not input_data.strip():
            return

        data = json.loads(input_data)
        tool_name = data.get("tool_name", "")

        if tool_name != "Bash":
            return

        tool_input = data.get("tool_input", {})
        if isinstance(tool_input, dict):
            command = tool_input.get("command", "")
        else:
            return

        check_command(command)

    except json.JSONDecodeError:
        pass
    except Exception:
        pass


if __name__ == "__main__":
    main()
`

// NewHookSafetyCmd runs the safety guard check (for testing).
func NewHookSafetyCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "hook-safety",
		Short:  "Run fray safety guard (internal)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// This command exists for testing; actual guard runs via Python
			fmt.Fprintln(cmd.OutOrStdout(), "Safety guard is implemented in Python.")
			fmt.Fprintln(cmd.OutOrStdout(), "Install with: fray hook-install --safety")
			return nil
		},
	}
}

// NewHookUninstallCmd removes installed hooks.
func NewHookUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook-uninstall",
		Short: "Remove fray Claude Code hooks",
		Long: `Remove fray Claude Code hooks.

By default, removes integration hooks (SessionStart, UserPromptSubmit, etc).
Use --safety to also/only remove safety guards.

Examples:
  fray hook-uninstall                   # Remove integration hooks
  fray hook-uninstall --safety          # Also remove safety guards
  fray hook-uninstall --safety --global # Remove safety guards from all projects
  fray hook-uninstall --precommit       # Also remove git pre-commit hook`,
		RunE: func(cmd *cobra.Command, args []string) error {
			safety, _ := cmd.Flags().GetBool("safety")
			global, _ := cmd.Flags().GetBool("global")
			precommit, _ := cmd.Flags().GetBool("precommit")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			projectDir := os.Getenv("CLAUDE_PROJECT_DIR")
			if projectDir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				projectDir = cwd
			}

			out := cmd.OutOrStdout()

			// Handle --safety flag
			if safety {
				if err := uninstallSafetyHooks(projectDir, out, global); err != nil {
					return err
				}
				if global {
					fmt.Fprintln(out, "\nRestart Claude Code to apply changes.")
					return nil
				}
			}

			// Remove integration hooks (unless --global --safety only)
			if !global || !safety {
				if err := uninstallIntegrationHooks(projectDir, out, dryRun); err != nil {
					return err
				}
			}

			if precommit {
				uninstallPrecommitHook(projectDir, dryRun, out)
			}

			fmt.Fprintln(out, "\nRestart Claude Code to apply changes.")
			return nil
		},
	}

	cmd.Flags().Bool("safety", false, "remove safety guards")
	cmd.Flags().Bool("global", false, "remove from ~/.claude instead of .claude")
	cmd.Flags().Bool("precommit", false, "also remove git pre-commit hook")
	cmd.Flags().Bool("dry-run", false, "show what would be removed without removing")

	return cmd
}

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

