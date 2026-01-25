package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
)

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
