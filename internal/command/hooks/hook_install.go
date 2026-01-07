package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type hookSettings struct {
	Hooks map[string]any `json:"hooks,omitempty"`
}

// NewHookInstallCmd installs Claude Code hooks for fray integration.
func NewHookInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook-install",
		Short: "Install Claude Code hooks for fray integration",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			precommit, _ := cmd.Flags().GetBool("precommit")

			projectDir := os.Getenv("CLAUDE_PROJECT_DIR")
			if projectDir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return writeCommandError(cmd, err)
				}
				projectDir = cwd
			}

			claudeDir := filepath.Join(projectDir, ".claude")
			settingsPath := filepath.Join(claudeDir, "settings.local.json")

			hooksConfig := hookSettings{
				Hooks: map[string]any{
					"SessionStart": []any{
						map[string]any{
							"matcher": "startup",
							"hooks": []any{map[string]any{
								"type":    "command",
								"command": "fray hook-session startup",
								"timeout": 10,
							}},
						},
						map[string]any{
							"matcher": "resume",
							"hooks": []any{map[string]any{
								"type":    "command",
								"command": "fray hook-session resume",
								"timeout": 10,
							}},
						},
					},
					"UserPromptSubmit": []any{
						map[string]any{
							"hooks": []any{map[string]any{
								"type":    "command",
								"command": "fray hook-prompt",
								"timeout": 5,
							}},
						},
					},
					"PreCompact": []any{
						map[string]any{
							"hooks": []any{map[string]any{
								"type":    "command",
								"command": "fray hook-precompact",
								"timeout": 5,
							}},
						},
					},
				},
			}

			out := cmd.OutOrStdout()

			if dryRun {
				fmt.Fprintf(out, "Would write to: %s\n", settingsPath)
				data, _ := json.MarshalIndent(hooksConfig, "", "  ")
				fmt.Fprintln(out, string(data))
				if precommit {
					installPrecommitHook(projectDir, true, out)
				}
				return nil
			}

			if err := os.MkdirAll(claudeDir, 0o755); err != nil {
				return writeCommandError(cmd, err)
			}

			settings := hookSettings{}
			if data, err := os.ReadFile(settingsPath); err == nil {
				_ = json.Unmarshal(data, &settings)
			}
			if settings.Hooks == nil {
				settings.Hooks = map[string]any{}
			}

			for key, value := range hooksConfig.Hooks {
				settings.Hooks[key] = value
			}

			merged, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return writeCommandError(cmd, err)
			}
			merged = append(merged, '\n')
			if err := os.WriteFile(settingsPath, merged, 0o644); err != nil {
				return writeCommandError(cmd, err)
			}

			fmt.Fprintf(out, "Hooks installed to %s\n\n", settingsPath)
			fmt.Fprintln(out, "Installed hooks:")
			fmt.Fprintln(out, "  SessionStart (startup/resume) - prompts agent registration or injects context")
			fmt.Fprintln(out, "  UserPromptSubmit - injects room messages and @mentions before each prompt")
			fmt.Fprintln(out, "  PreCompact - reminds to preserve work before context compaction")

			if precommit {
				installPrecommitHook(projectDir, false, out)
			}

			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "Restart Claude Code to activate hooks.")
			return nil
		},
	}

	cmd.Flags().Bool("dry-run", false, "show what would be written without writing")
	cmd.Flags().Bool("precommit", false, "also install git pre-commit hook for claim conflict detection")

	return cmd
}

func installPrecommitHook(projectDir string, dryRun bool, outWriter io.Writer) {
	gitRoot, err := gitRootDir(projectDir)
	if err != nil {
		fmt.Fprintln(outWriter, "")
		fmt.Fprintln(outWriter, "WARNING: Not in a git repository, skipping pre-commit hook installation")
		return
	}

	hooksDir := filepath.Join(gitRoot, ".git", "hooks")
	precommitPath := filepath.Join(hooksDir, "pre-commit")

	hookScript := strings.Join([]string{
		"#!/bin/sh",
		"# fray pre-commit hook - detects file claim conflicts",
		"# Installed by: fray hook-install --precommit",
		"",
		"fray hook-precommit",
		"",
	}, "\n")

	if dryRun {
		fmt.Fprintln(outWriter, "")
		fmt.Fprintf(outWriter, "Would write git pre-commit hook to: %s\n", precommitPath)
		fmt.Fprintln(outWriter, hookScript)
		return
	}

	if data, err := os.ReadFile(precommitPath); err == nil {
		if strings.Contains(string(data), "fray hook-precommit") {
			fmt.Fprintln(outWriter, "")
			fmt.Fprintln(outWriter, "Git pre-commit hook already installed")
			return
		}
		updated := strings.TrimRight(string(data), "\n") + "\n\n# fray file claim conflict detection\nmm hook-precommit\n"
		if err := os.WriteFile(precommitPath, []byte(updated), 0o755); err != nil {
			fmt.Fprintln(outWriter, "")
			fmt.Fprintf(outWriter, "Failed to update pre-commit hook: %v\n", err)
			return
		}
		fmt.Fprintln(outWriter, "")
		fmt.Fprintf(outWriter, "Added fray hook to existing pre-commit hook at %s\n", precommitPath)
		return
	}

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		fmt.Fprintln(outWriter, "")
		fmt.Fprintf(outWriter, "Failed to create hooks directory: %v\n", err)
		return
	}

	if err := os.WriteFile(precommitPath, []byte(hookScript), 0o755); err != nil {
		fmt.Fprintln(outWriter, "")
		fmt.Fprintf(outWriter, "Failed to write pre-commit hook: %v\n", err)
		return
	}
	_ = os.Chmod(precommitPath, 0o755)

	fmt.Fprintln(outWriter, "")
	fmt.Fprintf(outWriter, "Git pre-commit hook installed at %s\n", precommitPath)
	fmt.Fprintln(outWriter, "  Warns on file claim conflicts when committing")
}

func gitRootDir(startDir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = startDir
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
