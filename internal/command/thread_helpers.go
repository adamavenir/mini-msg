package command

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func resolveThreadRef(dbConn *sql.DB, ref string) (*types.Thread, error) {
	value := strings.TrimSpace(strings.TrimPrefix(ref, "#"))
	if value == "" {
		return nil, fmt.Errorf("thread reference is required")
	}
	if strings.Contains(value, "/") {
		return resolveThreadPath(dbConn, value)
	}

	thread, err := db.GetThread(dbConn, value)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}

	thread, err = db.GetThreadByPrefix(dbConn, value)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}

	thread, err = db.GetThreadByName(dbConn, value, nil)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}

	return nil, fmt.Errorf("thread not found: %s", ref)
}

func resolveThreadPath(dbConn *sql.DB, path string) (*types.Thread, error) {
	parts := strings.Split(path, "/")
	var parent *types.Thread
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			return nil, fmt.Errorf("invalid thread path: %s", path)
		}
		var parentGUID *string
		if parent != nil {
			parentGUID = &parent.GUID
		}
		thread, err := db.GetThreadByName(dbConn, name, parentGUID)
		if err != nil {
			return nil, err
		}
		if thread == nil {
			return nil, fmt.Errorf("thread not found: %s", path)
		}
		parent = thread
	}
	if parent == nil {
		return nil, fmt.Errorf("thread not found: %s", path)
	}
	return parent, nil
}

func buildThreadPath(dbConn *sql.DB, thread *types.Thread) (string, error) {
	if thread == nil {
		return "", nil
	}
	names := []string{thread.Name}
	parent := thread.ParentThread
	seen := map[string]struct{}{thread.GUID: {}}
	for parent != nil && *parent != "" {
		if _, ok := seen[*parent]; ok {
			return "", fmt.Errorf("thread path loop detected")
		}
		seen[*parent] = struct{}{}
		parentThread, err := db.GetThread(dbConn, *parent)
		if err != nil {
			return "", err
		}
		if parentThread == nil {
			break
		}
		names = append([]string{parentThread.Name}, names...)
		parent = parentThread.ParentThread
	}
	return strings.Join(names, "/"), nil
}

func validateThreadName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("thread name is required")
	}
	if strings.Contains(trimmed, "/") {
		return fmt.Errorf("thread name cannot contain '/'")
	}
	return nil
}

func collectParticipants(messages []types.Message) []string {
	seen := make(map[string]struct{})
	var participants []string
	for _, msg := range messages {
		if _, ok := seen[msg.FromAgent]; !ok {
			seen[msg.FromAgent] = struct{}{}
			participants = append(participants, msg.FromAgent)
		}
	}
	return participants
}

func filterMessage(messages []types.Message, excludeID string) []types.Message {
	result := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.ID != excludeID {
			result = append(result, msg)
		}
	}
	return result
}

func formatLastActivity(ts *int64) string {
	if ts == nil {
		return "unknown"
	}
	return formatRelative(*ts)
}

// MaxThreadNestingDepth is the maximum allowed depth for thread nesting.
// Room is level 0, first-level threads are level 1, etc.
const MaxThreadNestingDepth = 4

// getThreadDepth returns the nesting depth of a thread.
// A root thread (no parent) has depth 1.
func getThreadDepth(dbConn *sql.DB, thread *types.Thread) (int, error) {
	if thread == nil {
		return 0, nil
	}
	depth := 1
	parent := thread.ParentThread
	seen := map[string]struct{}{thread.GUID: {}}
	for parent != nil && *parent != "" {
		if _, ok := seen[*parent]; ok {
			return 0, fmt.Errorf("thread parent loop detected")
		}
		seen[*parent] = struct{}{}
		parentThread, err := db.GetThread(dbConn, *parent)
		if err != nil {
			return 0, err
		}
		if parentThread == nil {
			break
		}
		depth++
		parent = parentThread.ParentThread
	}
	return depth, nil
}
