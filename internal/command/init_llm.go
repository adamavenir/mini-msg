package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adamavenir/fray/internal/llm"
)

// ensureLLMRouter creates the .fray/llm/ directory and stock mlld templates.
// Only creates files that don't exist (preserves user customizations).
func ensureLLMRouter(projectRoot string) error {
	llmDir := filepath.Join(projectRoot, ".fray", "llm")

	// Create llm/ directory
	if err := os.MkdirAll(llmDir, 0o755); err != nil {
		return fmt.Errorf("create llm directory: %w", err)
	}

	// Create llm/run/ directory for user scripts
	runDir := filepath.Join(llmDir, "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("create llm/run directory: %w", err)
	}

	// Create llm/routers/ directory for mlld routers
	routersDir := filepath.Join(llmDir, "routers")
	if err := os.MkdirAll(routersDir, 0o755); err != nil {
		return fmt.Errorf("create llm/routers directory: %w", err)
	}

	// Create llm/slash/ directory for session lifecycle commands
	slashDir := filepath.Join(llmDir, "slash")
	if err := os.MkdirAll(slashDir, 0o755); err != nil {
		return fmt.Errorf("create llm/slash directory: %w", err)
	}

	// Write router templates
	routerTemplates := []string{
		llm.MentionsRouterTemplate,
		llm.StdoutRepairTemplate,
	}
	for _, templatePath := range routerTemplates {
		content, err := llm.ReadTemplate(templatePath)
		if err != nil {
			return fmt.Errorf("read %s: %w", templatePath, err)
		}
		destPath := filepath.Join(routersDir, filepath.Base(templatePath))
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			if err := os.WriteFile(destPath, content, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", templatePath, err)
			}
		}
	}

	// Write status template (lives in llm/ root)
	statusContent, err := llm.ReadTemplate(llm.StatusTemplate)
	if err != nil {
		return fmt.Errorf("read status template: %w", err)
	}
	statusPath := filepath.Join(llmDir, "status.mld")
	if _, err := os.Stat(statusPath); os.IsNotExist(err) {
		if err := os.WriteFile(statusPath, statusContent, 0o644); err != nil {
			return fmt.Errorf("write status template: %w", err)
		}
	}

	// Write slash command templates (session lifecycle)
	slashTemplates := []string{
		llm.FlyTemplate,
		llm.LandTemplate,
		llm.HandTemplate,
		llm.HopTemplate,
	}
	for _, templatePath := range slashTemplates {
		content, err := llm.ReadTemplate(templatePath)
		if err != nil {
			return fmt.Errorf("read %s: %w", templatePath, err)
		}
		destPath := filepath.Join(slashDir, filepath.Base(templatePath))
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			if err := os.WriteFile(destPath, content, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", templatePath, err)
			}
		}
	}

	// Create llm/prompts/ directory for daemon prompts
	promptsDir := filepath.Join(llmDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		return fmt.Errorf("create llm/prompts directory: %w", err)
	}

	// Write prompt templates (used by daemon for @mentions)
	promptTemplates := []string{
		llm.MentionFreshTemplate,
		llm.MentionResumeTemplate,
	}
	for _, templatePath := range promptTemplates {
		content, err := llm.ReadTemplate(templatePath)
		if err != nil {
			return fmt.Errorf("read %s: %w", templatePath, err)
		}
		destPath := filepath.Join(promptsDir, filepath.Base(templatePath))
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			if err := os.WriteFile(destPath, content, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", templatePath, err)
			}
		}
	}

	// Create mlld-config.json (if not exists)
	// @proj resolver points to project root (absolute path)
	mlldConfigPath := filepath.Join(projectRoot, ".fray", "mlld-config.json")
	if _, err := os.Stat(mlldConfigPath); os.IsNotExist(err) {
		mlldConfig := map[string]any{
			"scriptDir": "llm/run",
			"resolvers": map[string]any{
				"prefixes": []map[string]any{
					{
						"prefix":   "@proj/",
						"resolver": "LOCAL",
						"config": map[string]any{
							"basePath": projectRoot,
						},
					},
				},
			},
		}
		configData, err := json.MarshalIndent(mlldConfig, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal mlld config: %w", err)
		}
		if err := os.WriteFile(mlldConfigPath, configData, 0o644); err != nil {
			return fmt.Errorf("write mlld config: %w", err)
		}
	}

	return nil
}
