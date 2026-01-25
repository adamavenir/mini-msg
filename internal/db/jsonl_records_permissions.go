package db

import "github.com/adamavenir/fray/internal/types"

// PermissionJSONLRecord represents a permission request entry in JSONL.
type PermissionJSONLRecord struct {
	Type        string                   `json:"type"`
	GUID        string                   `json:"guid"`
	FromAgent   string                   `json:"from_agent"`
	SessionID   string                   `json:"session_id,omitempty"`
	Tool        string                   `json:"tool"`
	Action      string                   `json:"action"`
	Rationale   string                   `json:"rationale"`
	Options     []types.PermissionOption `json:"options"`
	Status      string                   `json:"status"`
	ChosenIndex *int                     `json:"chosen_index,omitempty"`
	RespondedBy *string                  `json:"responded_by,omitempty"`
	CreatedAt   int64                    `json:"created_at"`
	RespondedAt *int64                   `json:"responded_at,omitempty"`
}

// PermissionUpdateJSONLRecord represents a permission response entry in JSONL.
type PermissionUpdateJSONLRecord struct {
	Type        string `json:"type"`
	GUID        string `json:"guid"`
	Status      string `json:"status"`
	ChosenIndex *int   `json:"chosen_index,omitempty"`
	RespondedBy string `json:"responded_by"`
	RespondedAt int64  `json:"responded_at"`
}
