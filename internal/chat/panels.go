package chat

import "github.com/adamavenir/fray/internal/types"

type channelEntry struct {
	ID   string
	Name string
	Path string
}

type threadEntryKind int

const (
	threadEntryMain threadEntryKind = iota
	threadEntryThread
	threadEntrySeparator
	threadEntryMessageCollection
	threadEntryThreadCollection
	threadEntrySectionHeader // For grouping labels like "agents", "roles", "topics"
)

type messageCollectionView string

const (
	messageCollectionOpenQuestions   messageCollectionView = "open-qs"
	messageCollectionClosedQuestions messageCollectionView = "closed-qs"
	messageCollectionWondering       messageCollectionView = "wondering"
	messageCollectionStaleQuestions  messageCollectionView = "stale-qs"
)

type threadCollectionView string

const (
	threadCollectionMuted          threadCollectionView = "muted"
	threadCollectionUnreadMentions threadCollectionView = "unread-mentions"
)

type threadEntry struct {
	Kind              threadEntryKind
	Thread            *types.Thread
	MessageCollection messageCollectionView
	ThreadCollection  threadCollectionView
	Label             string
	Indent            int
	HasChildren       bool
	Collapsed         bool
	Faved             bool
	Avatar            string // Agent avatar for display in meta view
}
