package command

import (
	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
)

func findKnownAgent(config *db.ProjectConfig, ref string) *knownAgentMatch {
	if config == nil || len(config.KnownAgents) == 0 {
		return nil
	}
	normalized := core.NormalizeAgentRef(ref)

	if entry, ok := config.KnownAgents[normalized]; ok {
		return &knownAgentMatch{GUID: normalized, Entry: entry}
	}

	for guid, entry := range config.KnownAgents {
		if entry.Name != nil && *entry.Name == normalized {
			return &knownAgentMatch{GUID: guid, Entry: entry}
		}
		if entry.GlobalName != nil && *entry.GlobalName == normalized {
			return &knownAgentMatch{GUID: guid, Entry: entry}
		}
		for _, nick := range entry.Nicks {
			if nick == normalized {
				return &knownAgentMatch{GUID: guid, Entry: entry}
			}
		}
	}

	return nil
}
