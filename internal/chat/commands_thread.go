package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) setThreadNickname(args []string) (tea.Cmd, error) {
	var guid string
	var threadName string

	// Check if we have a pending nickname target from Ctrl-N
	if m.pendingNicknameGUID != "" {
		guid = m.pendingNicknameGUID
		// Find thread name for status message
		for _, t := range m.threads {
			if t.GUID == guid {
				threadName = t.Name
				break
			}
		}
		m.pendingNicknameGUID = "" // Clear after use
	} else if m.threadPanelFocus {
		// Fall back to focused thread in sidebar
		entries := m.threadEntries()
		if m.threadIndex < 0 || m.threadIndex >= len(entries) {
			return nil, fmt.Errorf("no thread selected")
		}
		entry := entries[m.threadIndex]
		if entry.Kind != threadEntryThread || entry.Thread == nil {
			return nil, fmt.Errorf("selected item is not a thread")
		}
		guid = entry.Thread.GUID
		threadName = entry.Thread.Name
	} else if m.currentThread != nil {
		// Fall back to currently viewed thread
		guid = m.currentThread.GUID
		threadName = m.currentThread.Name
	} else {
		return nil, fmt.Errorf("no thread selected (navigate to a thread or use Tab to focus sidebar)")
	}

	nickname := strings.Join(args, " ")

	if nickname == "" {
		// Clear nickname
		if err := db.SetNickname(m.db, m.username, "thread", guid, ""); err != nil {
			return nil, fmt.Errorf("failed to clear nickname: %w", err)
		}
		m.status = fmt.Sprintf("Cleared nickname for %s", threadName)
	} else {
		// Set nickname
		if err := db.SetNickname(m.db, m.username, "thread", guid, nickname); err != nil {
			return nil, fmt.Errorf("failed to set nickname: %w", err)
		}
		m.status = fmt.Sprintf("Set nickname '%s' for %s", nickname, threadName)
	}

	m.refreshThreadNicknames()
	m.input.SetValue("")
	m.input.CursorEnd()
	return nil, nil
}

// getTargetThread returns the thread to operate on: either from explicit arg or current thread.
func (m *Model) getTargetThread(args []string) (*types.Thread, error) {
	if len(args) > 0 {
		thread, err := m.resolveThreadRef(args[0])
		if err != nil {
			return nil, err
		}
		return thread, nil
	}
	if m.currentThread == nil {
		return nil, fmt.Errorf("no thread selected (use main to navigate, or specify thread)")
	}
	return m.currentThread, nil
}

// resolveThreadRef finds a thread by GUID, prefix, or name.
func (m *Model) resolveThreadRef(ref string) (*types.Thread, error) {
	value := strings.TrimSpace(strings.TrimPrefix(ref, "#"))
	if value == "" {
		return nil, fmt.Errorf("thread reference is required")
	}

	// Try exact GUID match
	thread, err := db.GetThread(m.db, value)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}

	// Try prefix match
	thread, err = db.GetThreadByPrefix(m.db, value)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}

	// Try name match (root-level first for backward compat)
	thread, err = db.GetThreadByName(m.db, value, nil)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}

	// Try name match (any level)
	thread, err = db.GetThreadByNameAny(m.db, value)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}

	return nil, fmt.Errorf("thread not found: %s", ref)
}

func (m *Model) runFaveCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if m.favedThreads[thread.GUID] {
		m.status = fmt.Sprintf("%s is already faved", thread.Name)
		return nil, nil
	}

	favedAt, err := db.AddFave(m.db, m.username, "thread", thread.GUID)
	if err != nil {
		return nil, fmt.Errorf("failed to fave: %w", err)
	}
	if err := db.AppendAgentFave(m.projectDBPath, m.username, "thread", thread.GUID, favedAt); err != nil {
		return nil, fmt.Errorf("failed to persist fave: %w", err)
	}

	// Also subscribe to the thread (faving implies following)
	now := time.Now().Unix()
	if !m.subscribedThreads[thread.GUID] {
		if err := db.SubscribeThread(m.db, thread.GUID, m.username, now); err == nil {
			_ = db.AppendThreadSubscribe(m.projectDBPath, db.ThreadSubscribeJSONLRecord{
				ThreadGUID:   thread.GUID,
				AgentID:      m.username,
				SubscribedAt: now,
			})
			m.refreshSubscribedThreads()
		}
	}

	m.refreshFavedThreads()
	m.status = fmt.Sprintf("Faved %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runUnfaveCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if !m.favedThreads[thread.GUID] {
		m.status = fmt.Sprintf("%s is not faved", thread.Name)
		return nil, nil
	}

	if err := db.RemoveFave(m.db, m.username, "thread", thread.GUID); err != nil {
		return nil, fmt.Errorf("failed to unfave: %w", err)
	}
	if err := db.AppendFaveRemove(m.projectDBPath, m.username, "thread", thread.GUID, time.Now().Unix()); err != nil {
		return nil, fmt.Errorf("failed to persist unfave: %w", err)
	}

	m.refreshFavedThreads()
	m.status = fmt.Sprintf("Unfaved %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runMuteCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if m.mutedThreads[thread.GUID] {
		m.status = fmt.Sprintf("%s is already muted", thread.Name)
		return nil, nil
	}

	if err := db.MuteThread(m.db, thread.GUID, m.username, 0, nil); err != nil {
		return nil, fmt.Errorf("failed to mute: %w", err)
	}

	m.refreshMutedThreads()
	m.status = fmt.Sprintf("Muted %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runUnmuteCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if !m.mutedThreads[thread.GUID] {
		m.status = fmt.Sprintf("%s is not muted", thread.Name)
		return nil, nil
	}

	if err := db.UnmuteThread(m.db, thread.GUID, m.username); err != nil {
		return nil, fmt.Errorf("failed to unmute: %w", err)
	}

	m.refreshMutedThreads()
	m.status = fmt.Sprintf("Unmuted %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runFollowCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if m.subscribedThreads[thread.GUID] {
		m.status = fmt.Sprintf("Already following %s", thread.Name)
		return nil, nil
	}

	now := time.Now().Unix()
	if err := db.SubscribeThread(m.db, thread.GUID, m.username, now); err != nil {
		return nil, fmt.Errorf("failed to follow: %w", err)
	}
	if err := db.AppendThreadSubscribe(m.projectDBPath, db.ThreadSubscribeJSONLRecord{
		ThreadGUID:   thread.GUID,
		AgentID:      m.username,
		SubscribedAt: now,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist follow: %w", err)
	}

	m.refreshSubscribedThreads()
	m.status = fmt.Sprintf("Following %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runUnfollowCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if !m.subscribedThreads[thread.GUID] {
		m.status = fmt.Sprintf("Not following %s", thread.Name)
		return nil, nil
	}

	if err := db.UnsubscribeThread(m.db, thread.GUID, m.username); err != nil {
		return nil, fmt.Errorf("failed to unfollow: %w", err)
	}
	if err := db.AppendThreadUnsubscribe(m.projectDBPath, db.ThreadUnsubscribeJSONLRecord{
		ThreadGUID:     thread.GUID,
		AgentID:        m.username,
		UnsubscribedAt: time.Now().Unix(),
	}); err != nil {
		return nil, fmt.Errorf("failed to persist unfollow: %w", err)
	}

	m.refreshSubscribedThreads()
	m.status = fmt.Sprintf("Unfollowed %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runArchiveCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if thread.Status == types.ThreadStatusArchived {
		m.status = fmt.Sprintf("%s is already archived", thread.Name)
		return nil, nil
	}

	statusValue := string(types.ThreadStatusArchived)
	_, err = db.UpdateThread(m.db, thread.GUID, db.ThreadUpdates{
		Status: types.OptionalString{Set: true, Value: &statusValue},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to archive: %w", err)
	}
	if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
		GUID:   thread.GUID,
		Status: &statusValue,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist archive: %w", err)
	}

	// If archived current thread, navigate away
	if m.currentThread != nil && m.currentThread.GUID == thread.GUID {
		m.currentThread = nil
		m.threadMessages = nil
		m.refreshViewport(false)
	}

	m.status = fmt.Sprintf("Archived %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runRestoreCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if thread.Status != types.ThreadStatusArchived {
		m.status = fmt.Sprintf("%s is not archived", thread.Name)
		return nil, nil
	}

	statusValue := string(types.ThreadStatusOpen)
	_, err = db.UpdateThread(m.db, thread.GUID, db.ThreadUpdates{
		Status: types.OptionalString{Set: true, Value: &statusValue},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to restore: %w", err)
	}
	if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
		GUID:   thread.GUID,
		Status: &statusValue,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist restore: %w", err)
	}

	m.status = fmt.Sprintf("Restored %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runRenameCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /rename <new-name>")
	}

	if m.currentThread == nil {
		return nil, fmt.Errorf("no thread selected (navigate to a thread first)")
	}

	newName := strings.Join(args, " ")

	// Check for duplicate name
	var parentGUID *string
	if m.currentThread.ParentThread != nil {
		parentGUID = m.currentThread.ParentThread
	}
	existing, err := db.GetThreadByName(m.db, newName, parentGUID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.GUID != m.currentThread.GUID {
		return nil, fmt.Errorf("thread already exists: %s", newName)
	}

	_, err = db.UpdateThread(m.db, m.currentThread.GUID, db.ThreadUpdates{
		Name: types.OptionalString{Set: true, Value: &newName},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to rename: %w", err)
	}
	if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
		GUID: m.currentThread.GUID,
		Name: &newName,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist rename: %w", err)
	}

	oldName := m.currentThread.Name
	m.currentThread.Name = newName

	// Create event for thread rename (shows name, not ID)
	eventBody := fmt.Sprintf("edited %s → %s", oldName, newName)
	eventMessage, err := db.CreateMessage(m.db, types.Message{
		TS:        time.Now().Unix(),
		FromAgent: m.username,
		Body:      eventBody,
		Type:      types.MessageTypeEvent,
		Home:      "room",
	})
	if err != nil {
		return nil, err
	}
	if err := db.AppendMessage(m.projectDBPath, eventMessage); err != nil {
		return nil, err
	}
	m.messageCount++
	if m.showUpdates {
		m.messages = append(m.messages, eventMessage)
		m.refreshViewport(false)
	}

	m.status = fmt.Sprintf("Renamed %s to %s", oldName, newName)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runMvCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /mv [#msg-id] <destination>")
	}

	// Check if first arg is a message reference
	firstArg := args[0]
	if strings.HasPrefix(firstArg, "#") || strings.HasPrefix(strings.ToLower(firstArg), "msg-") {
		// Moving a message
		return m.runMvMessageCommand(args)
	}

	// Moving current thread
	return m.runMvThreadCommand(args)
}

// isRoomDestination checks if the destination refers to the main room.
func (m *Model) isRoomDestination(dest string) bool {
	destLower := strings.ToLower(dest)
	return destLower == "room" || destLower == "main" || destLower == m.projectName
}

// runMvMessageCommand moves a message to a thread or room.
func (m *Model) runMvMessageCommand(args []string) (tea.Cmd, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("usage: /mv #msg-id <destination>")
	}

	msgRef := strings.TrimPrefix(args[0], "#")
	destRef := args[1]

	// Resolve message
	msg, err := m.resolveMessageInput(msgRef)
	if err != nil {
		return nil, fmt.Errorf("message not found: %s", msgRef)
	}

	// Resolve destination
	var newHome string
	if m.isRoomDestination(destRef) {
		newHome = "room"
	} else {
		destThread, err := m.resolveThreadRef(destRef)
		if err != nil {
			return nil, fmt.Errorf("thread not found: %s", destRef)
		}
		newHome = destThread.GUID
	}

	// Check if already there
	if msg.Home == newHome {
		return nil, fmt.Errorf("message is already in %s", destRef)
	}

	now := time.Now().Unix()
	oldHome := msg.Home

	// Move the message
	if err := db.MoveMessage(m.db, msg.ID, newHome); err != nil {
		return nil, fmt.Errorf("failed to move: %w", err)
	}

	if err := db.AppendMessageMove(m.projectDBPath, db.MessageMoveJSONLRecord{
		MessageGUID: msg.ID,
		OldHome:     oldHome,
		NewHome:     newHome,
		MovedBy:     m.username,
		MovedAt:     now,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist move: %w", err)
	}

	// Update thread activity if moving to a thread
	if newHome != "room" {
		if err := db.UpdateThreadActivity(m.db, newHome, now); err != nil {
			return nil, err
		}
		if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
			GUID:           newHome,
			LastActivityAt: &now,
		}); err != nil {
			return nil, err
		}
	}

	// Update display
	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	msgPrefix := core.GetGUIDPrefix(msg.ID, prefixLength)
	destName := destRef
	if newHome == "room" {
		destName = "main"
	}
	m.status = fmt.Sprintf("Moved #%s to %s", msgPrefix, destName)
	m.input.SetValue("")
	return nil, nil
}

// runMvThreadCommand moves the current thread to a new parent.
func (m *Model) runMvThreadCommand(args []string) (tea.Cmd, error) {
	if m.currentThread == nil {
		return nil, fmt.Errorf("no thread selected (navigate to a thread first, or specify a message: /mv #msg-id destination)")
	}

	destRef := args[0]
	var anchorText string
	if len(args) > 1 {
		anchorText = strings.Join(args[1:], " ")
	}

	// Resolve destination (new parent)
	var newParentGUID *string
	destLower := strings.ToLower(destRef)
	if destLower == "root" || destLower == "/" {
		// Move to root (no parent)
		newParentGUID = nil
	} else {
		destThread, err := m.resolveThreadRef(destRef)
		if err != nil {
			return nil, fmt.Errorf("thread not found: %s", destRef)
		}
		newParentGUID = &destThread.GUID

		// Check for cycles: can't move thread under one of its descendants
		isDescendant, err := m.isAncestorOf(destThread.GUID, m.currentThread.GUID)
		if err != nil {
			return nil, err
		}
		if isDescendant {
			return nil, fmt.Errorf("cannot move %s under %s: would create a cycle", m.currentThread.Name, destThread.Name)
		}
	}

	// Check if already at target parent
	currentParent := ""
	if m.currentThread.ParentThread != nil {
		currentParent = *m.currentThread.ParentThread
	}
	targetParent := ""
	if newParentGUID != nil {
		targetParent = *newParentGUID
	}
	if currentParent == targetParent {
		return nil, fmt.Errorf("%s is already under that parent", m.currentThread.Name)
	}

	now := time.Now().Unix()

	// Update thread parent
	_, err := db.UpdateThread(m.db, m.currentThread.GUID, db.ThreadUpdates{
		ParentThread: types.OptionalString{Set: true, Value: newParentGUID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to move: %w", err)
	}

	// Persist to JSONL
	if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
		GUID:         m.currentThread.GUID,
		ParentThread: newParentGUID,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist move: %w", err)
	}

	// If anchor text provided, create anchor message
	if anchorText != "" {
		agentBases, err := db.GetAgentBases(m.db)
		if err != nil {
			return nil, err
		}
		mentions := core.ExtractMentions(anchorText, agentBases)
		mentions = core.ExpandAllMention(mentions, agentBases)

		newMsg := types.Message{
			TS:        now,
			Home:      m.currentThread.GUID,
			FromAgent: m.username,
			Body:      anchorText,
			Mentions:  mentions,
			Type:      types.MessageTypeUser,
		}

		created, err := db.CreateMessage(m.db, newMsg)
		if err != nil {
			return nil, err
		}

		if err := db.AppendMessage(m.projectDBPath, created); err != nil {
			return nil, err
		}

		// Set as anchor
		_, err = db.UpdateThread(m.db, m.currentThread.GUID, db.ThreadUpdates{
			AnchorMessageGUID: types.OptionalString{Set: true, Value: &created.ID},
		})
		if err != nil {
			return nil, err
		}

		if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
			GUID:              m.currentThread.GUID,
			AnchorMessageGUID: &created.ID,
		}); err != nil {
			return nil, err
		}
	}

	// Update thread activity
	if err := db.UpdateThreadActivity(m.db, m.currentThread.GUID, now); err != nil {
		return nil, err
	}
	if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
		GUID:           m.currentThread.GUID,
		LastActivityAt: &now,
	}); err != nil {
		return nil, err
	}

	// Update local state
	m.currentThread.ParentThread = newParentGUID

	destName := "root"
	if newParentGUID != nil {
		destThread, _ := db.GetThread(m.db, *newParentGUID)
		if destThread != nil {
			destName = destThread.Name
		}
	}

	if anchorText != "" {
		m.status = fmt.Sprintf("Moved %s under %s (with anchor)", m.currentThread.Name, destName)
	} else {
		m.status = fmt.Sprintf("Moved %s under %s", m.currentThread.Name, destName)
	}
	m.input.SetValue("")
	return nil, nil
}

// isAncestorOf checks if potentialAncestor is an ancestor of threadGUID.
func (m *Model) isAncestorOf(threadGUID, potentialAncestorGUID string) (bool, error) {
	if threadGUID == potentialAncestorGUID {
		return true, nil
	}
	current := threadGUID
	seen := map[string]struct{}{}
	for {
		if _, ok := seen[current]; ok {
			return false, fmt.Errorf("thread parent loop detected")
		}
		seen[current] = struct{}{}
		thread, err := db.GetThread(m.db, current)
		if err != nil {
			return false, err
		}
		if thread == nil || thread.ParentThread == nil || *thread.ParentThread == "" {
			return false, nil
		}
		if *thread.ParentThread == potentialAncestorGUID {
			return true, nil
		}
		current = *thread.ParentThread
	}
}

// runThreadCommand creates a new root-level thread.
// Syntax: /thread name "anchor" or /t name "anchor"
func (m *Model) runThreadCommand(input string) (tea.Cmd, error) {
	name, anchor, err := parseThreadArgs(input)
	if err != nil {
		return nil, err
	}

	return m.createThread(name, nil, anchor)
}

// runSubthreadCommand creates a subthread under the current thread.
// Syntax: /subthread name "anchor" or /st name "anchor"
func (m *Model) runSubthreadCommand(input string) (tea.Cmd, error) {
	if m.currentThread == nil {
		return nil, fmt.Errorf("navigate to a thread first to create a subthread")
	}

	name, anchor, err := parseThreadArgs(input)
	if err != nil {
		return nil, err
	}

	return m.createThread(name, &m.currentThread.GUID, anchor)
}

// createThread creates a thread with optional parent and anchor.
func (m *Model) createThread(name string, parentGUID *string, anchorText string) (tea.Cmd, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("thread name is required")
	}
	if strings.Contains(name, "/") {
		return nil, fmt.Errorf("thread name cannot contain '/'")
	}

	// Check if thread already exists at this level
	existing, err := db.GetThreadByName(m.db, name, parentGUID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("thread already exists: %s", name)
	}

	// Check for meta/ path collision (e.g., creating "opus/notes" when "meta/opus/notes" exists)
	if err := m.checkMetaPathCollision(parentGUID, name); err != nil {
		return nil, err
	}

	// Create the thread
	thread, err := db.CreateThread(m.db, types.Thread{
		Name:         name,
		ParentThread: parentGUID,
		Status:       types.ThreadStatusOpen,
	})
	if err != nil {
		return nil, err
	}

	// Persist to JSONL with current user as subscriber
	subscribers := []string{m.username}
	if err := db.AppendThread(m.projectDBPath, thread, subscribers); err != nil {
		return nil, err
	}

	now := time.Now().Unix()

	// Subscribe creator to the thread
	if err := db.SubscribeThread(m.db, thread.GUID, m.username, now); err != nil {
		return nil, err
	}

	// Create anchor message if provided
	if anchorText != "" {
		agentBases, err := db.GetAgentBases(m.db)
		if err != nil {
			return nil, err
		}
		mentions := core.ExtractMentions(anchorText, agentBases)
		mentions = core.ExpandAllMention(mentions, agentBases)

		newMsg := types.Message{
			TS:        now,
			Home:      thread.GUID,
			FromAgent: m.username,
			Body:      anchorText,
			Mentions:  mentions,
			Type:      types.MessageTypeUser,
		}

		created, err := db.CreateMessage(m.db, newMsg)
		if err != nil {
			return nil, err
		}

		if err := db.AppendMessage(m.projectDBPath, created); err != nil {
			return nil, err
		}

		// Set as anchor
		_, err = db.UpdateThread(m.db, thread.GUID, db.ThreadUpdates{
			AnchorMessageGUID: types.OptionalString{Set: true, Value: &created.ID},
			LastActivityAt:    types.OptionalInt64{Set: true, Value: &now},
		})
		if err != nil {
			return nil, err
		}

		if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
			GUID:              thread.GUID,
			AnchorMessageGUID: &created.ID,
			LastActivityAt:    &now,
		}); err != nil {
			return nil, err
		}
	}

	// Navigate to the new thread
	m.currentThread = &thread
	m.currentPseudo = ""
	m.threadMessages = nil
	m.refreshSubscribedThreads()

	// Build path for status message
	path := name
	if parentGUID != nil {
		parentThread, _ := db.GetThread(m.db, *parentGUID)
		if parentThread != nil {
			parentPath, _ := threadPath(m.db, parentThread)
			if parentPath != "" {
				path = parentPath + "/" + name
			}
		}
	}

	if anchorText != "" {
		m.status = fmt.Sprintf("Created %s with anchor", path)
	} else {
		m.status = fmt.Sprintf("Created %s", path)
	}
	m.input.SetValue("")
	m.refreshViewport(false)
	return nil, nil
}

// parseThreadArgs extracts name and anchor from /thread or /subthread input.
// Supports: /thread name "anchor" or /thread name
func parseThreadArgs(input string) (string, string, error) {
	// Remove command prefix
	trimmed := strings.TrimSpace(input)
	rest := ""
	for _, prefix := range []string{"/subthread ", "/st ", "/thread ", "/t "} {
		if strings.HasPrefix(trimmed, prefix) {
			rest = strings.TrimSpace(trimmed[len(prefix):])
			break
		}
	}
	if rest == "" {
		return "", "", fmt.Errorf("usage: /thread name [\"anchor\"] or /t name [\"anchor\"]")
	}

	// Check for quoted anchor
	quoteIdx := strings.Index(rest, "\"")
	if quoteIdx == -1 {
		// No anchor, just name
		return strings.TrimSpace(rest), "", nil
	}

	name := strings.TrimSpace(rest[:quoteIdx])
	if name == "" {
		return "", "", fmt.Errorf("thread name is required")
	}

	// Extract quoted anchor
	anchorPart := rest[quoteIdx:]
	if len(anchorPart) < 2 || !strings.HasPrefix(anchorPart, "\"") {
		return "", "", fmt.Errorf("anchor must be quoted: /thread name \"anchor text\"")
	}

	// Find closing quote
	closingIdx := strings.LastIndex(anchorPart, "\"")
	if closingIdx <= 0 {
		return "", "", fmt.Errorf("missing closing quote for anchor")
	}

	anchor := anchorPart[1:closingIdx]
	return name, anchor, nil
}

// checkMetaPathCollision checks if creating a thread would collide with a meta/ equivalent.
// For example, creating "opus/notes" when "meta/opus/notes" exists is likely an error.
func (m *Model) checkMetaPathCollision(parentGUID *string, name string) error {
	// Build the full path that would be created
	var fullPath string
	if parentGUID == nil {
		fullPath = name
	} else {
		parentThread, err := db.GetThread(m.db, *parentGUID)
		if err != nil {
			return err
		}
		if parentThread == nil {
			return nil
		}
		parentPath, err := threadPath(m.db, parentThread)
		if err != nil {
			return err
		}
		fullPath = parentPath + "/" + name
	}

	// Skip if path already starts with meta
	if strings.HasPrefix(fullPath, "meta/") || fullPath == "meta" {
		return nil
	}

	// Check if meta/<path> exists
	metaPath := "meta/" + fullPath
	parts := strings.Split(metaPath, "/")
	var parent *types.Thread
	for _, part := range parts {
		var parentGUID *string
		if parent != nil {
			parentGUID = &parent.GUID
		}
		thread, err := db.GetThreadByName(m.db, part, parentGUID)
		if err != nil {
			return nil // Error checking = no collision
		}
		if thread == nil {
			return nil // Path doesn't exist = no collision
		}
		parent = thread
	}

	// If we got here, the full meta path exists
	return fmt.Errorf("thread exists at %s - use that path instead", metaPath)
}
