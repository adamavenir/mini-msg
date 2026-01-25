package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.pollCmd(), m.activityPollCmd())
}

func (m *Model) Close() {
	if m.stopSyncChecker != nil {
		close(m.stopSyncChecker)
	}
	if m.db != nil {
		_ = m.db.Close()
	}
}

// runSyncChecker polls DB every 5s and logs when in-memory state diverges from reality.
// Used to debug #fray-rvuu (chat session stops showing new messages/threads).
func (m *Model) runSyncChecker() {
	logPath := filepath.Join(m.projectRoot, ".fray", "sync-debug.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[debug-sync] failed to open log: %v\n", err)
		return
	}
	defer logFile.Close()

	fmt.Fprintf(logFile, "[%s] sync checker started\n", time.Now().Format(time.RFC3339))

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopSyncChecker:
			fmt.Fprintf(logFile, "[%s] sync checker stopped\n", time.Now().Format(time.RFC3339))
			return
		case <-ticker.C:
			m.checkSync(logFile)
		}
	}
}

// checkSync compares in-memory state to DB and logs discrepancies.
func (m *Model) checkSync(logFile *os.File) {
	now := time.Now().Format(time.RFC3339)

	// Get recent messages from DB (last 100)
	dbMsgs, err := db.GetMessages(m.db, &types.MessageQueryOptions{
		Limit:           100,
		IncludeArchived: m.includeArchived,
	})
	if err != nil {
		fmt.Fprintf(logFile, "[%s] ERROR: failed to query messages: %v\n", now, err)
		return
	}

	// Build map of in-memory message IDs
	memMsgIDs := make(map[string]bool)
	for _, msg := range m.messages {
		memMsgIDs[msg.ID] = true
	}

	// Find messages in DB but not in memory
	var missingMsgs []string
	for _, dbMsg := range dbMsgs {
		if !memMsgIDs[dbMsg.ID] {
			missingMsgs = append(missingMsgs, dbMsg.ID)
		}
	}

	if len(missingMsgs) > 0 {
		fmt.Fprintf(logFile, "[%s] DESYNC: %d messages in DB not in chat (mem=%d, db=%d): %v\n",
			now, len(missingMsgs), len(m.messages), len(dbMsgs), missingMsgs)
	}

	// Check threads
	dbThreads, err := db.GetThreads(m.db, nil)
	if err != nil {
		fmt.Fprintf(logFile, "[%s] ERROR: failed to query threads: %v\n", now, err)
		return
	}

	memThreadIDs := make(map[string]bool)
	for _, t := range m.threads {
		memThreadIDs[t.GUID] = true
	}

	var missingThreads []string
	for _, dbThread := range dbThreads {
		if !memThreadIDs[dbThread.GUID] {
			missingThreads = append(missingThreads, dbThread.GUID)
		}
	}

	if len(missingThreads) > 0 {
		fmt.Fprintf(logFile, "[%s] DESYNC: %d threads in DB not in chat (mem=%d, db=%d): %v\n",
			now, len(missingThreads), len(m.threads), len(dbThreads), missingThreads)
	}
}
