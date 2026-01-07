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

// isAncestorOf checks if potentialAncestor is an ancestor of thread.
// Returns true if moving potentialAncestor under thread would create a cycle.
func isAncestorOf(dbConn *sql.DB, threadGUID, potentialAncestorGUID string) (bool, error) {
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
		thread, err := db.GetThread(dbConn, current)
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

// CheckMetaPathCollision checks if a path would collide with an existing meta/ equivalent.
// For example, creating "opus/notes" when "meta/opus/notes" exists is likely an error.
// Returns the suggested meta path if collision detected, empty string otherwise.
func CheckMetaPathCollision(dbConn *sql.DB, path string) (string, error) {
	// Skip if path already starts with meta
	if strings.HasPrefix(path, "meta/") || path == "meta" {
		return "", nil
	}

	// Check if meta/<path> exists
	metaPath := "meta/" + path
	thread, err := resolveThreadPath(dbConn, metaPath)
	if err != nil {
		// Path doesn't exist, no collision
		return "", nil
	}
	if thread != nil {
		return metaPath, nil
	}
	return "", nil
}

// CheckMetaPathCollisionForCreate checks before creating a thread at the given path.
// parentGUID is the parent thread (nil for root), name is the new thread name.
// Returns error if a meta/ equivalent exists.
func CheckMetaPathCollisionForCreate(dbConn *sql.DB, parentGUID *string, name string) error {
	// Build the full path that would be created
	var fullPath string
	if parentGUID == nil {
		fullPath = name
	} else {
		parentThread, err := db.GetThread(dbConn, *parentGUID)
		if err != nil {
			return err
		}
		if parentThread == nil {
			return nil // Parent doesn't exist, let normal validation handle it
		}
		parentPath, err := buildThreadPath(dbConn, parentThread)
		if err != nil {
			return err
		}
		fullPath = parentPath + "/" + name
	}

	// Check for meta collision
	suggestedPath, err := CheckMetaPathCollision(dbConn, fullPath)
	if err != nil {
		return err
	}
	if suggestedPath != "" {
		return fmt.Errorf("thread exists at %s - use that path instead", suggestedPath)
	}
	return nil
}
