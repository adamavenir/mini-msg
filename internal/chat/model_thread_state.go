package chat

import (
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func (m *Model) refreshFavedThreads() {
	if m.db == nil || m.username == "" {
		return
	}
	guids, err := db.GetFavedThreads(m.db, m.username)
	if err != nil {
		return
	}
	m.favedThreads = make(map[string]bool)
	for _, guid := range guids {
		m.favedThreads[guid] = true
	}
}

func (m *Model) refreshSubscribedThreads() {
	if m.db == nil || m.username == "" {
		return
	}
	threads, err := db.GetThreads(m.db, &types.ThreadQueryOptions{
		SubscribedAgent: &m.username,
	})
	if err != nil {
		return
	}
	m.subscribedThreads = make(map[string]bool)
	for _, t := range threads {
		m.subscribedThreads[t.GUID] = true
	}
}

func (m *Model) refreshMutedThreads() {
	if m.db == nil || m.username == "" {
		return
	}
	guids, err := db.GetMutedThreadGUIDs(m.db, m.username)
	if err != nil {
		return
	}
	m.mutedThreads = guids
}

func (m *Model) refreshThreadNicknames() {
	if m.db == nil || m.username == "" {
		return
	}
	nicknames, err := db.GetThreadNicknames(m.db, m.username)
	if err != nil {
		return
	}
	m.threadNicknames = nicknames
}

func (m *Model) refreshAvatars() {
	if m.db == nil {
		return
	}
	agents, err := db.GetAgents(m.db)
	if err != nil {
		return
	}
	m.avatarMap = make(map[string]string)
	for _, agent := range agents {
		if agent.Avatar != nil && *agent.Avatar != "" {
			m.avatarMap[agent.AgentID] = *agent.Avatar
		}
	}
}
