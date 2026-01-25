package db

import (
	"sort"
	"time"

	"github.com/adamavenir/fray/internal/core"
)

type agentStats struct {
	firstSeen int64
	lastSeen  int64
}

func mergeAgentsFromMessages(agents []AgentJSONLRecord, messages []MessageJSONLRecord) ([]AgentJSONLRecord, error) {
	existing := make(map[string]AgentJSONLRecord, len(agents))
	for _, agent := range agents {
		if agent.AgentID == "" {
			continue
		}
		existing[agent.AgentID] = agent
	}

	stats := make(map[string]agentStats)
	for _, msg := range messages {
		if msg.FromAgent == "" {
			continue
		}
		ts := normalizeTimestamp(msg.TS)
		entry, ok := stats[msg.FromAgent]
		if !ok {
			stats[msg.FromAgent] = agentStats{firstSeen: ts, lastSeen: ts}
			continue
		}
		if ts < entry.firstSeen {
			entry.firstSeen = ts
		}
		if ts > entry.lastSeen {
			entry.lastSeen = ts
		}
		stats[msg.FromAgent] = entry
	}

	missing := make([]string, 0)
	for agentID := range stats {
		if _, ok := existing[agentID]; !ok {
			missing = append(missing, agentID)
		}
	}
	sort.Strings(missing)

	for _, agentID := range missing {
		guid, err := core.GenerateGUID("usr")
		if err != nil {
			return nil, err
		}
		stat := stats[agentID]
		agents = append(agents, AgentJSONLRecord{
			Type:         "agent",
			ID:           guid,
			Name:         agentID,
			AgentID:      agentID,
			RegisteredAt: stat.firstSeen,
			LastSeen:     stat.lastSeen,
		})
	}

	return agents, nil
}

func mergeAgentsFromDescriptors(agents []AgentJSONLRecord, descriptors []AgentDescriptor) ([]AgentJSONLRecord, error) {
	if len(descriptors) == 0 {
		return agents, nil
	}
	existing := make(map[string]bool, len(agents))
	for _, agent := range agents {
		if agent.AgentID == "" {
			continue
		}
		existing[agent.AgentID] = true
	}

	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].AgentID < descriptors[j].AgentID
	})

	for _, descriptor := range descriptors {
		if descriptor.AgentID == "" {
			continue
		}
		if existing[descriptor.AgentID] {
			continue
		}
		guid, err := core.GenerateGUID("usr")
		if err != nil {
			return nil, err
		}
		ts := normalizeTimestamp(descriptor.TS)
		if ts == 0 {
			ts = time.Now().Unix()
		}
		agents = append(agents, AgentJSONLRecord{
			Type:         "agent",
			ID:           guid,
			Name:         descriptor.AgentID,
			AgentID:      descriptor.AgentID,
			RegisteredAt: ts,
			LastSeen:     ts,
		})
		existing[descriptor.AgentID] = true
	}

	return agents, nil
}
