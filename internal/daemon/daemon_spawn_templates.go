package daemon

import (
	"fmt"
	"os"
	"path/filepath"
)

// PromptPayload is the input to prompt template execution.
type PromptPayload struct {
	Agent         string   `json:"agent"`
	TriggerMsgIDs []string `json:"triggerMsgIDs"`
	UserTask      string   `json:"userTask,omitempty"`
}

// executePromptTemplate runs an mlld template and returns the prompt string.
// Falls back to empty string if template doesn't exist or execution fails.
// Template location:
//   - prompts/: mention-fresh, mention-resume (daemon spawn context)
//   - slash/: fly, hop, land (skill templates)
func (d *Daemon) executePromptTemplate(templateName string, payload PromptPayload) (string, error) {
	// Determine template directory based on template type
	var templateDir string
	switch templateName {
	case "mention-fresh", "mention-resume":
		templateDir = "prompts"
	default:
		templateDir = "slash"
	}
	templatePath := filepath.Join(d.project.Root, ".fray", "llm", templateDir, templateName+".mld")

	// Check if template exists
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		d.debugf("  template %s not found at %s", templateName, templatePath)
		return "", fmt.Errorf("template not found: %s", templateName)
	}

	// Execute the template
	result, err := d.mlldClient.Execute(templatePath, payload, nil)
	if err != nil {
		d.debugf("  mlld execute error for %s: %v", templateName, err)
		return "", fmt.Errorf("mlld execute failed: %w", err)
	}

	return result.Output, nil
}
