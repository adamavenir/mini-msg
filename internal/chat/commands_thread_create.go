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
