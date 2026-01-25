package daemon

import (
	"path/filepath"
	"strings"

	"github.com/adamavenir/fray/internal/aap"
	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
)

// detectSpawnMode checks the trigger message for /fly, /hop, /land, /hand patterns.
// Returns the spawn mode and optionally a user message that follows the command.
func detectSpawnMode(body string, agentID string) (SpawnMode, string) {
	// Look for patterns like "@agent /fly", "@agent /hop", "@agent /land", "@agent /hand"
	patterns := []struct {
		suffix string
		mode   SpawnMode
	}{
		{" /fly", SpawnModeFly},
		{" /hop", SpawnModeHop},
		{" /land", SpawnModeLand},
		{" /hand", SpawnModeHand},
	}

	agentMention := "@" + agentID
	bodyLower := strings.ToLower(body)
	for _, p := range patterns {
		pattern := strings.ToLower(agentMention + p.suffix)
		if strings.HasPrefix(bodyLower, pattern) {
			// Extract any user message after the command
			remainder := strings.TrimSpace(body[len(agentMention)+len(p.suffix):])
			return p.mode, remainder
		}
	}
	return SpawnModeNormal, ""
}

// getDriver returns the driver for an agent.
func (d *Daemon) getDriver(agentID string) Driver {
	agent, err := db.GetAgent(d.database, agentID)
	if err != nil || agent == nil || agent.Invoke == nil {
		return nil
	}
	return d.drivers[agent.Invoke.Driver]
}

// getAgentResolution resolves an agent address using AAP.
// Returns nil if resolution fails (e.g., agent has no AAP identity).
func (d *Daemon) getAgentResolution(agentID string) *aap.Resolution {
	aapDir, err := core.AAPConfigDir()
	if err != nil {
		return nil
	}

	frayDir := filepath.Dir(d.project.DBPath)
	projectAAPDir := filepath.Join(d.project.Root, ".aap")

	resolver, err := aap.NewResolver(aap.ResolverOpts{
		GlobalRegistry:  aapDir,
		ProjectRegistry: projectAAPDir,
		FrayCompat:      true,
		FrayPath:        frayDir,
	})
	if err != nil {
		return nil
	}

	res, err := resolver.Resolve("@" + agentID)
	if err != nil {
		return nil
	}

	return res
}
