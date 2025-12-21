package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"github.com/adamavenir/mini-msg/internal/core"
)

// OpenDatabase opens the SQLite database for a project.
func OpenDatabase(project core.Project) (*sql.DB, error) {
	mmDir := filepath.Dir(project.DBPath)
	core.EnsureMMGitignore(mmDir)

	dbExists := true
	if _, err := os.Stat(project.DBPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			dbExists = false
		} else {
			return nil, err
		}
	}

	jsonlMtime := getJSONLMtime(mmDir)
	var dbMtime int64
	if dbExists {
		info, err := os.Stat(project.DBPath)
		if err != nil {
			return nil, err
		}
		dbMtime = info.ModTime().UnixMilli()
	}

	shouldRebuild := jsonlMtime > 0 && (!dbExists || jsonlMtime > dbMtime)

	conn, err := sql.Open("sqlite", project.DBPath)
	if err != nil {
		return nil, err
	}

	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := conn.Exec("PRAGMA journal_mode = WAL"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := conn.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if shouldRebuild {
		if err := RebuildDatabaseFromJSONL(conn, project.DBPath); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}

	return conn, nil
}

func getJSONLMtime(mmDir string) int64 {
	files := []string{"messages.jsonl", "agents.jsonl"}
	latest := int64(0)
	for _, name := range files {
		path := filepath.Join(mmDir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		mt := info.ModTime().UnixMilli()
		if mt > latest {
			latest = mt
		}
	}
	return latest
}
