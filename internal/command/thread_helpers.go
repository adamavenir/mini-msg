package command

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
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
