package db

// RoleHoldJSONLRecord represents a role hold (persistent assignment) event.
type RoleHoldJSONLRecord struct {
	Type       string `json:"type"` // "role_hold"
	AgentID    string `json:"agent_id"`
	RoleName   string `json:"role_name"`
	AssignedAt int64  `json:"assigned_at"`
}

// RoleDropJSONLRecord represents a role drop (removal) event.
type RoleDropJSONLRecord struct {
	Type      string `json:"type"` // "role_drop"
	AgentID   string `json:"agent_id"`
	RoleName  string `json:"role_name"`
	DroppedAt int64  `json:"dropped_at"`
}

// RoleReleaseJSONLRecord represents releasing a held role.
type RoleReleaseJSONLRecord struct {
	Type     string `json:"type"` // "role_release"
	AgentID  string `json:"agent_id"`
	RoleName string `json:"role_name"`
	Seq      int64  `json:"seq,omitempty"`
	TS       int64  `json:"ts"`
}

// RolePlayJSONLRecord represents a session-scoped role play event.
type RolePlayJSONLRecord struct {
	Type      string  `json:"type"` // "role_play"
	AgentID   string  `json:"agent_id"`
	RoleName  string  `json:"role_name"`
	SessionID *string `json:"session_id,omitempty"`
	StartedAt int64   `json:"started_at"`
}

// RoleStopJSONLRecord represents stopping a session role.
type RoleStopJSONLRecord struct {
	Type      string `json:"type"` // "role_stop"
	AgentID   string `json:"agent_id"`
	RoleName  string `json:"role_name"`
	StoppedAt int64  `json:"stopped_at"`
}
