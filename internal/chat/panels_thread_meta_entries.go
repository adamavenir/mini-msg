package chat

import (
	"sort"
	"strings"

	"github.com/adamavenir/fray/internal/types"
)

func (m *Model) appendMetaViewEntries(entries []threadEntry, roots []types.Thread, children map[string][]types.Thread) []threadEntry {
	var topics, agents, roles []types.Thread
	for _, thread := range roots {
		if strings.HasPrefix(thread.Name, "role-") {
			roles = append(roles, thread)
			continue
		}

		// Check if this is an agent thread (has agent-like substructure)
		_, hasNotes := children[thread.GUID]
		if hasNotes {
			agents = append(agents, thread)
		} else {
			topics = append(topics, thread)
		}
	}

	sort.Slice(topics, func(i, j int) bool { return topics[i].Name < topics[j].Name })
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })

	if len(topics) > 0 {
		entries = append(entries, threadEntry{Kind: threadEntrySectionHeader, Label: "topics"})
		for _, thread := range topics {
			t := thread
			entries = append(entries, threadEntry{
				Kind:        threadEntryThread,
				Thread:      &t,
				Label:       thread.Name,
				Indent:      0,
				HasChildren: len(children[thread.GUID]) > 0,
				Collapsed:   m.collapsedThreads[thread.GUID],
				Faved:       m.favedThreads[thread.GUID],
			})
		}
	}

	if len(agents) > 0 {
		entries = append(entries, threadEntry{Kind: threadEntrySectionHeader, Label: "agents"})
		for _, thread := range agents {
			t := thread
			avatar := m.avatarMap[thread.Name] // thread.Name is the agent_id under meta/
			entries = append(entries, threadEntry{
				Kind:        threadEntryThread,
				Thread:      &t,
				Label:       thread.Name,
				Indent:      0,
				HasChildren: len(children[thread.GUID]) > 0,
				Collapsed:   m.collapsedThreads[thread.GUID],
				Faved:       m.favedThreads[thread.GUID],
				Avatar:      avatar,
			})
		}
	}

	if len(roles) > 0 {
		entries = append(entries, threadEntry{Kind: threadEntrySectionHeader, Label: "roles"})
		for _, thread := range roles {
			t := thread
			entries = append(entries, threadEntry{
				Kind:        threadEntryThread,
				Thread:      &t,
				Label:       thread.Name,
				Indent:      0,
				HasChildren: len(children[thread.GUID]) > 0,
				Collapsed:   m.collapsedThreads[thread.GUID],
				Faved:       m.favedThreads[thread.GUID],
			})
		}
	}

	return entries
}
