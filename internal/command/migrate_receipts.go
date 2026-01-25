package command

import (
	"database/sql"
	"fmt"
)

type readReceiptRow struct {
	MessageGUID *string
	MessageID   *int64
	AgentPrefix string
	ReadAt      int64
}

func loadReadReceipts(dbConn *sql.DB) ([]readReceiptRow, error) {
	if !tableExists(dbConn, "fray_read_receipts") {
		return nil, nil
	}

	columns, err := getColumns(dbConn, "fray_read_receipts")
	if err != nil {
		return nil, err
	}
	hasGUID := columnsInclude(columns, "message_guid")

	field := "message_id"
	if hasGUID {
		field = "message_guid"
	}

	rows, err := dbConn.Query(fmt.Sprintf(`
		SELECT %s, agent_prefix, read_at
		FROM fray_read_receipts
	`, field))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var receipts []readReceiptRow
	for rows.Next() {
		var (
			messageGUID sql.NullString
			messageID   sql.NullInt64
			agentPrefix string
			readAt      int64
		)
		if hasGUID {
			if err := rows.Scan(&messageGUID, &agentPrefix, &readAt); err != nil {
				return nil, err
			}
		} else {
			if err := rows.Scan(&messageID, &agentPrefix, &readAt); err != nil {
				return nil, err
			}
		}
		receipts = append(receipts, readReceiptRow{
			MessageGUID: nullStringPtr(messageGUID),
			MessageID:   nullInt64Ptr(messageID),
			AgentPrefix: agentPrefix,
			ReadAt:      readAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return receipts, nil
}

func restoreReadReceipts(dbConn *sql.DB, receipts []readReceiptRow, idToGuid map[int64]string) error {
	if len(receipts) == 0 {
		return nil
	}

	tx, err := dbConn.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO fray_read_receipts (message_guid, agent_prefix, read_at)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, receipt := range receipts {
		messageGUID := receipt.MessageGUID
		if messageGUID == nil && receipt.MessageID != nil {
			if guid, ok := idToGuid[*receipt.MessageID]; ok {
				messageGUID = &guid
			}
		}
		if messageGUID == nil {
			continue
		}
		if _, err := stmt.Exec(*messageGUID, receipt.AgentPrefix, receipt.ReadAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}
