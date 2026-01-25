package chat

import (
	"sort"

	"github.com/adamavenir/fray/internal/types"
)

func (m *Model) threadEntries() []threadEntry {
	entries := make([]threadEntry, 0, len(m.threads)+10)

	// Special case: viewing muted collection
	if m.viewingMutedCollection {
		// Show "back" entry for muted collection
		entries = append(entries, threadEntry{
			Kind:             threadEntryThreadCollection,
			ThreadCollection: threadCollectionMuted,
			Label:            "muted",
		})
		// Show all muted threads
		for guid := range m.mutedThreads {
			for _, thread := range m.threads {
				if thread.GUID == guid {
					t := thread
					entries = append(entries, threadEntry{
						Kind:   threadEntryThread,
						Thread: &t,
						Label:  thread.Name,
						Indent: 0,
					})
					break
				}
			}
		}
		return entries
	}

	// Build parent-child relationships
	children := make(map[string][]types.Thread)
	roots := make([]types.Thread, 0)
	for _, thread := range m.threads {
		if thread.ParentThread == nil || *thread.ParentThread == "" {
			roots = append(roots, thread)
			continue
		}
		children[*thread.ParentThread] = append(children[*thread.ParentThread], thread)
	}

	// Check if we're drilled into a thread
	drilledThread := m.currentDrillThread()
	inMetaView := false
	if drilledThread != nil {
		// When drilled in: show "back" entry + immediate children only
		entries = append(entries, threadEntry{
			Kind:   threadEntryThread,
			Thread: drilledThread,
			Label:  drilledThread.Name,
			Indent: 0,
		})
		// Filter roots to only children of drilled thread
		if kids, hasKids := children[drilledThread.GUID]; hasKids {
			roots = kids
		} else {
			roots = nil // No children
		}
		// Check if we're drilled into the meta thread
		inMetaView = drilledThread.Name == "meta" && drilledThread.ParentThread == nil
	} else {
		// At top level: show #main
		entries = append(entries, threadEntry{Kind: threadEntryMain, Label: "#main"})
	}

	// Sort children alphabetically
	for key := range children {
		slice := children[key]
		sort.Slice(slice, func(i, j int) bool {
			return slice[i].Name < slice[j].Name
		})
		children[key] = slice
	}

	// Special handling for meta view: group children into topics, agents, roles
	if inMetaView && len(roots) > 0 {
		return m.appendMetaViewEntries(entries, roots, children)
	}

	// Helper to add thread entry (no recursion - children shown via drill-in)
	var walk func(thread types.Thread, indent int)
	walk = func(thread types.Thread, indent int) {
		t := thread
		kids, hasKids := children[thread.GUID]
		faved := m.favedThreads[thread.GUID]
		entries = append(entries, threadEntry{
			Kind:        threadEntryThread,
			Thread:      &t,
			Label:       thread.Name,
			Indent:      indent,
			HasChildren: hasKids && len(kids) > 0,
			Collapsed:   false, // Not used - drill handles children
			Faved:       faved,
		})
		// No recursion - children are shown when user drills in
	}

	// Helper to check if thread has unreads with mentions
	hasUnreadMentions := func(guid string) bool {
		return false // TODO: implement mention detection
	}

	// Tracking for shown GUIDs (start early so meta is tracked)
	shownGUIDs := make(map[string]bool)

	// 1b. Meta thread always appears first (if it exists and at top level)
	if drilledThread == nil {
		for _, thread := range roots {
			if thread.Name == "meta" && (thread.ParentThread == nil || *thread.ParentThread == "") {
				t := thread
				walk(t, 0)
				shownGUIDs[thread.GUID] = true
				break
			}
		}
	}

	// 2. Threads with unread mentions/replies (special indicator)
	var unreadMentionThreads []types.Thread
	for _, thread := range roots {
		if hasUnreadMentions(thread.GUID) && !shownGUIDs[thread.GUID] {
			unreadMentionThreads = append(unreadMentionThreads, thread)
		}
	}
	for _, thread := range unreadMentionThreads {
		walk(thread, 0)
	}

	// 3. Faved threads (excluding those already shown)
	for _, t := range unreadMentionThreads {
		shownGUIDs[t.GUID] = true
	}

	var favedThreads []types.Thread
	for _, thread := range roots {
		if m.favedThreads[thread.GUID] && !shownGUIDs[thread.GUID] {
			favedThreads = append(favedThreads, thread)
		}
	}
	// Sort faved by name
	sort.Slice(favedThreads, func(i, j int) bool {
		return favedThreads[i].Name < favedThreads[j].Name
	})
	for _, thread := range favedThreads {
		walk(thread, 0)
		shownGUIDs[thread.GUID] = true
	}

	// 3b. Message collections with content (open-qs, stale-qs) - only at top level
	if m.drillDepth() == 0 {
		if m.questionCounts[pseudoThreadKind(messageCollectionOpenQuestions)] > 0 {
			entries = append(entries, threadEntry{
				Kind:              threadEntryMessageCollection,
				MessageCollection: messageCollectionOpenQuestions,
				Label:             string(messageCollectionOpenQuestions),
			})
		}
		if m.questionCounts[pseudoThreadKind(messageCollectionStaleQuestions)] > 0 {
			entries = append(entries, threadEntry{
				Kind:              threadEntryMessageCollection,
				MessageCollection: messageCollectionStaleQuestions,
				Label:             string(messageCollectionStaleQuestions),
			})
		}
	}

	// 4. Subscribed threads sorted by recency (last_activity_at)
	var subscribedThreads []types.Thread
	for _, thread := range roots {
		if m.subscribedThreads[thread.GUID] && !shownGUIDs[thread.GUID] && !m.mutedThreads[thread.GUID] {
			subscribedThreads = append(subscribedThreads, thread)
		}
	}
	sort.Slice(subscribedThreads, func(i, j int) bool {
		iActivity := subscribedThreads[i].CreatedAt
		if subscribedThreads[i].LastActivityAt != nil {
			iActivity = *subscribedThreads[i].LastActivityAt
		}
		jActivity := subscribedThreads[j].CreatedAt
		if subscribedThreads[j].LastActivityAt != nil {
			jActivity = *subscribedThreads[j].LastActivityAt
		}
		return iActivity > jActivity // most recent first
	})
	for _, thread := range subscribedThreads {
		walk(thread, 0)
		shownGUIDs[thread.GUID] = true
	}

	// 5. All other threads collapsed to top-level (non-subscribed, non-muted)
	var otherThreads []types.Thread
	for _, thread := range roots {
		if !shownGUIDs[thread.GUID] && !m.mutedThreads[thread.GUID] {
			otherThreads = append(otherThreads, thread)
		}
	}
	// Sort by name
	sort.Slice(otherThreads, func(i, j int) bool {
		return otherThreads[i].Name < otherThreads[j].Name
	})
	for _, thread := range otherThreads {
		// Calculate max depth for collapsed display
		maxDepth := m.calculateMaxDepth(thread.GUID, children, 0)
		t := thread
		entries = append(entries, threadEntry{
			Kind:        threadEntryThread,
			Thread:      &t,
			Label:       thread.Name,
			Indent:      maxDepth, // Store depth in Indent field for collapsed display
			HasChildren: false,    // Don't allow expanding in this mode
			Collapsed:   true,     // Mark as collapsed
			Faved:       false,
		})
	}

	// Muted collection at bottom - only show at top level (not when drilled in)
	if len(m.mutedThreads) > 0 && m.drillDepth() == 0 {
		entries = append(entries, threadEntry{
			Kind:             threadEntryThreadCollection,
			ThreadCollection: threadCollectionMuted,
			Label:            "muted",
		})
	}

	// Add search results from database
	if len(m.threadSearchResults) > 0 {
		entries = append(entries, threadEntry{Kind: threadEntrySeparator, Label: "search"})
		for _, thread := range m.threadSearchResults {
			t := thread
			entries = append(entries, threadEntry{
				Kind:   threadEntryThread,
				Thread: &t,
				Label:  thread.Name,
				Indent: 0,
			})
		}
	}

	return entries
}
