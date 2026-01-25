package command

import "database/sql"

type agentRow struct {
	GUID         *string
	AgentID      string
	Goal         *string
	Bio          *string
	RegisteredAt int64
	LastSeen     int64
	LeftAt       *int64
}

func loadAgents(dbConn *sql.DB, hasGUID bool) ([]agentRow, error) {
	if hasGUID {
		rows, err := dbConn.Query(`
			SELECT guid, agent_id, goal, bio, registered_at, last_seen, left_at
			FROM fray_agents
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanAgents(rows, true)
	}

	rows, err := dbConn.Query(`
		SELECT agent_id, goal, bio, registered_at, last_seen, left_at
		FROM fray_agents
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgents(rows, false)
}

func scanAgents(rows *sql.Rows, hasGUID bool) ([]agentRow, error) {
	var agents []agentRow
	for rows.Next() {
		var (
			guid         sql.NullString
			agentID      string
			goal         sql.NullString
			bio          sql.NullString
			registeredAt int64
			lastSeen     int64
			leftAt       sql.NullInt64
		)
		if hasGUID {
			if err := rows.Scan(&guid, &agentID, &goal, &bio, &registeredAt, &lastSeen, &leftAt); err != nil {
				return nil, err
			}
		} else {
			if err := rows.Scan(&agentID, &goal, &bio, &registeredAt, &lastSeen, &leftAt); err != nil {
				return nil, err
			}
		}

		agents = append(agents, agentRow{
			GUID:         nullStringPtr(guid),
			AgentID:      agentID,
			Goal:         nullStringPtr(goal),
			Bio:          nullStringPtr(bio),
			RegisteredAt: registeredAt,
			LastSeen:     lastSeen,
			LeftAt:       nullInt64Ptr(leftAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}
