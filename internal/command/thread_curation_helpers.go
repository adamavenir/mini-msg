package command

import (
	"database/sql"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// getAllReplies recursively gets all replies to a message.
func getAllReplies(database *sql.DB, messageGUID string) ([]types.Message, error) {
	seen := make(map[string]struct{})
	return getAllRepliesWithGuard(database, messageGUID, seen)
}

func getAllRepliesWithGuard(database *sql.DB, messageGUID string, seen map[string]struct{}) ([]types.Message, error) {
	if _, ok := seen[messageGUID]; ok {
		return nil, nil // already visited, break cycle
	}
	seen[messageGUID] = struct{}{}

	var result []types.Message

	replies, err := db.GetReplies(database, messageGUID)
	if err != nil {
		return nil, err
	}

	for _, reply := range replies {
		if _, ok := seen[reply.ID]; ok {
			continue // skip already visited
		}
		result = append(result, reply)
		nested, err := getAllRepliesWithGuard(database, reply.ID, seen)
		if err != nil {
			return nil, err
		}
		result = append(result, nested...)
	}

	return result, nil
}
