package main

import (
	"database/sql"
	"sync"

	"github.com/adamavenir/fray/internal/core"
)

// Handle management
var (
	handleMu   sync.RWMutex
	handles           = make(map[uint64]*handleEntry)
	nextHandle uint64 = 1
)

type handleEntry struct {
	db          *sql.DB
	project     core.Project
	projectPath string
}

func registerHandle(database *sql.DB, project core.Project, projectPath string) uint64 {
	handleMu.Lock()
	defer handleMu.Unlock()
	id := nextHandle
	nextHandle++
	handles[id] = &handleEntry{
		db:          database,
		project:     project,
		projectPath: projectPath,
	}
	return id
}

func getHandle(id uint64) (*handleEntry, bool) {
	handleMu.RLock()
	defer handleMu.RUnlock()
	entry, ok := handles[id]
	return entry, ok
}

func closeHandle(id uint64) {
	handleMu.Lock()
	defer handleMu.Unlock()
	if entry, ok := handles[id]; ok {
		if entry.db != nil {
			_ = entry.db.Close()
		}
		delete(handles, id)
	}
}
