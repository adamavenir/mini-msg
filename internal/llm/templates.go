package llm

import (
	"embed"
)

//go:embed slash/*.mld prompts/*.mld routers/*.mld
var Templates embed.FS

// Template paths for easy reference
const (
	// Slash commands (session lifecycle)
	FlyTemplate  = "slash/fly.mld"
	LandTemplate = "slash/land.mld"
	HandTemplate = "slash/hand.mld"
	HopTemplate  = "slash/hop.mld"

	// Prompts (daemon triggers)
	MentionFreshTemplate  = "prompts/mention-fresh.mld"
	MentionResumeTemplate = "prompts/mention-resume.mld"

	// Routers
	MentionsRouterTemplate  = "routers/mentions.mld"
	StatusTemplate          = "routers/status.mld"
	StdoutRepairTemplate    = "routers/stdout-repair.mld"
	WakeRouterTemplate      = "routers/wake-router.mld"
	WakePromptTemplate      = "routers/wake-prompt.mld"
)

// ReadTemplate reads a template file from the embedded filesystem.
func ReadTemplate(path string) ([]byte, error) {
	return Templates.ReadFile(path)
}
