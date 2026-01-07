package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

// readToRecord holds read state to preserve across rebuild.
type readToRecord struct {
	AgentID     string
	Home        string
	MessageGUID string
	MessageTS   int64
	SetAt       int64
}

// NewRebuildCmd creates the rebuild command.
func NewRebuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild database from JSONL files",
		Long: `Rebuild the SQLite cache from the authoritative JSONL files.

Use this command when:
- You see schema errors (e.g., "no such column")
- The database is corrupted
- After manually editing JSONL files
- After a git pull with JSONL changes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Don't use GetContext - it tries to open the DB which may fail
			// Just discover the project and delete/rebuild the DB directly
			project, err := core.DiscoverProject("")
			if err != nil {
				return writeCommandError(cmd, err)
			}

			dbPath := project.DBPath

			// Shelve read state before deleting DB (local state we want to preserve)
			readState := shelveReadState(dbPath)

			// Delete existing db files
			os.Remove(dbPath)
			os.Remove(dbPath + "-wal")
			os.Remove(dbPath + "-shm")

			// Open fresh - this will trigger rebuild from JSONL
			newDB, err := db.OpenDatabase(project)
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("rebuild: %w", err))
			}
			defer newDB.Close()

			// Restore read state
			restoreReadState(newDB, readState)

			jsonMode, _ := cmd.Flags().GetBool("json")
			if jsonMode {
				json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"status": "rebuilt"})
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Database rebuilt from JSONL")
			}
			return nil
		},
	}

	return cmd
}

// shelveReadState extracts read state from the old database before deletion.
func shelveReadState(dbPath string) []readToRecord {
	oldDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil
	}
	defer oldDB.Close()

	rows, err := oldDB.Query(`SELECT agent_id, home, message_guid, message_ts, set_at FROM fray_read_to`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []readToRecord
	for rows.Next() {
		var r readToRecord
		if err := rows.Scan(&r.AgentID, &r.Home, &r.MessageGUID, &r.MessageTS, &r.SetAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records
}

// restoreReadState inserts preserved read state into the new database.
func restoreReadState(newDB *sql.DB, records []readToRecord) {
	for _, r := range records {
		_, _ = newDB.Exec(`
			INSERT OR REPLACE INTO fray_read_to (agent_id, home, message_guid, message_ts, set_at)
			VALUES (?, ?, ?, ?, ?)
		`, r.AgentID, r.Home, r.MessageGUID, r.MessageTS, r.SetAt)
	}
}
