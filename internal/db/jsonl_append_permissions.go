package db

import (
	"path/filepath"

	"github.com/adamavenir/fray/internal/types"
)

// AppendPermissionRequest appends a permission request record to JSONL.
func AppendPermissionRequest(projectPath string, req types.PermissionRequest) error {
	frayDir := resolveFrayDir(projectPath)
	record := PermissionJSONLRecord{
		Type:        "permission_request",
		GUID:        req.GUID,
		FromAgent:   req.FromAgent,
		SessionID:   req.SessionID,
		Tool:        req.Tool,
		Action:      req.Action,
		Rationale:   req.Rationale,
		Options:     req.Options,
		Status:      string(req.Status),
		ChosenIndex: req.ChosenIndex,
		RespondedBy: req.RespondedBy,
		CreatedAt:   req.CreatedAt,
		RespondedAt: req.RespondedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, permissionsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendPermissionUpdate appends a permission response record to JSONL.
func AppendPermissionUpdate(projectPath string, update PermissionUpdateJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	update.Type = "permission_update"
	if err := appendJSONLine(filepath.Join(frayDir, permissionsFile), update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}
