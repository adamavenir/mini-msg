package chat

import "database/sql"

func countMessages(dbConn *sql.DB, includeArchived bool) (int, error) {
	query := "SELECT COUNT(*) FROM fray_messages"
	if !includeArchived {
		query += " WHERE archived_at IS NULL"
	}
	row := dbConn.QueryRow(query)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
