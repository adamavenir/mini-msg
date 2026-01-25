package db

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/adamavenir/fray/internal/types"
)

// ReadPermissions reads permission requests from JSONL and applies updates.
func ReadPermissions(projectPath string) ([]types.PermissionRequest, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, permissionsFile))
	if err != nil {
		return nil, err
	}

	permMap := make(map[string]types.PermissionRequest)
	order := make([]string, 0)

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "permission_request":
			var record PermissionJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			req := types.PermissionRequest{
				GUID:        record.GUID,
				FromAgent:   record.FromAgent,
				SessionID:   record.SessionID,
				Tool:        record.Tool,
				Action:      record.Action,
				Rationale:   record.Rationale,
				Options:     record.Options,
				Status:      types.PermissionStatus(record.Status),
				ChosenIndex: record.ChosenIndex,
				RespondedBy: record.RespondedBy,
				CreatedAt:   record.CreatedAt,
				RespondedAt: record.RespondedAt,
			}
			if _, exists := permMap[record.GUID]; !exists {
				order = append(order, record.GUID)
			}
			permMap[record.GUID] = req

		case "permission_update":
			var update PermissionUpdateJSONLRecord
			if err := json.Unmarshal([]byte(line), &update); err != nil {
				continue
			}
			if req, exists := permMap[update.GUID]; exists {
				req.Status = types.PermissionStatus(update.Status)
				req.ChosenIndex = update.ChosenIndex
				req.RespondedBy = &update.RespondedBy
				req.RespondedAt = &update.RespondedAt
				permMap[update.GUID] = req
			}
		}
	}

	result := make([]types.PermissionRequest, 0, len(order))
	for _, guid := range order {
		result = append(result, permMap[guid])
	}
	return result, nil
}

// ReadPermissionByGUID reads a specific permission request by GUID.
func ReadPermissionByGUID(projectPath, guid string) (*types.PermissionRequest, error) {
	perms, err := ReadPermissions(projectPath)
	if err != nil {
		return nil, err
	}
	for _, p := range perms {
		if p.GUID == guid {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("permission request not found: %s", guid)
}
