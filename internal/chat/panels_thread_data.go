package chat

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func loadThreads(dbConn *sql.DB, username string) ([]types.Thread, int) {
	if dbConn == nil || username == "" {
		return nil, 0
	}
	threads, err := db.GetThreads(dbConn, &types.ThreadQueryOptions{})
	if err != nil {
		return nil, 0
	}
	return threads, 0
}

func threadPath(dbConn *sql.DB, thread *types.Thread) (string, error) {
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
