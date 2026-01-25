package command

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/adamavenir/fray/internal/core"
)

func generateUniqueGUID(prefix string, used map[string]struct{}) (string, error) {
	for {
		guid, err := core.GenerateGUID(prefix)
		if err != nil {
			return "", err
		}
		if _, exists := used[guid]; exists {
			continue
		}
		used[guid] = struct{}{}
		return guid, nil
	}
}

func containsGUID(used map[string]struct{}, guid string) bool {
	_, exists := used[guid]
	return exists
}

func resolveReplyTo(value any, hasMessageGUID bool, idToGuid map[int64]string) *string {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case int64:
		if guid, ok := idToGuid[v]; ok {
			return &guid
		}
		return nil
	case []byte:
		text := strings.TrimSpace(string(v))
		return resolveReplyString(text, hasMessageGUID, idToGuid)
	case string:
		return resolveReplyString(strings.TrimSpace(v), hasMessageGUID, idToGuid)
	default:
		text := strings.TrimSpace(fmt.Sprintf("%v", v))
		return resolveReplyString(text, hasMessageGUID, idToGuid)
	}
}

func resolveReplyString(value string, hasMessageGUID bool, idToGuid map[int64]string) *string {
	if value == "" {
		return nil
	}
	if !hasMessageGUID {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			if guid, ok := idToGuid[parsed]; ok {
				return &guid
			}
			return nil
		}
	}
	return &value
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
