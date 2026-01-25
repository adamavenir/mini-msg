package command

import (
	"database/sql"
	"fmt"
)

type tableColumn struct {
	Name    string
	Type    string
	NotNull int
	PK      int
}

func tableExists(dbConn *sql.DB, table string) bool {
	row := dbConn.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name=?
	`, table)
	var name string
	if err := row.Scan(&name); err != nil {
		return false
	}
	return name != ""
}

func getColumns(dbConn *sql.DB, table string) ([]tableColumn, error) {
	rows, err := dbConn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []tableColumn
	for rows.Next() {
		var col tableColumn
		var cid int
		var defaultValue any
		if err := rows.Scan(&cid, &col.Name, &col.Type, &col.NotNull, &defaultValue, &col.PK); err != nil {
			return nil, err
		}
		columns = append(columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func columnsInclude(columns []tableColumn, name string) bool {
	for _, col := range columns {
		if col.Name == name {
			return true
		}
	}
	return false
}
