package chat

import (
	"fmt"
	"strings"

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
