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
