package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"github.com/adamavenir/fray/internal/core"
)

// OpenDatabase opens the SQLite database for a project.
func OpenDatabase(project core.Project) (*sql.DB, error) {
	frayDir := filepath.Dir(project.DBPath)
	core.EnsureFrayGitignore(frayDir)

	dbExists := true
	if _, err := os.Stat(project.DBPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			dbExists = false
		} else {
			return nil, err
		}
	}

	jsonlMtime := getJSONLMtime(frayDir)
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

func getJSONLMtime(projectPath string) int64 {
	if GetStorageVersion(projectPath) >= 2 {
		latest := int64(0)
		files := []string{messagesFile, threadsFile, questionsFile, agentStateFile}
		for _, dir := range GetSharedMachinesDirs(projectPath) {
			for _, name := range files {
				path := filepath.Join(dir, name)
				info, err := os.Stat(path)
				if err != nil {
					continue
				}
				mt := info.ModTime().UnixMilli()
				if mt > latest {
					latest = mt
				}
			}
		}
		runtimePath := GetLocalRuntimePath(projectPath)
		if info, err := os.Stat(runtimePath); err == nil {
			mt := info.ModTime().UnixMilli()
			if mt > latest {
				latest = mt
			}
		}
		return latest
	}

	frayDir := resolveFrayDir(projectPath)
	files := []string{messagesFile, agentsFile, questionsFile, threadsFile}
	latest := int64(0)
	for _, name := range files {
		path := filepath.Join(frayDir, name)
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
