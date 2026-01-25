package db

// AgentFaveJSONLRecord represents a fave event in JSONL.
type AgentFaveJSONLRecord struct {
	Type     string `json:"type"` // "agent_fave"
	AgentID  string `json:"agent_id"`
	ItemType string `json:"item_type"` // "thread" | "message"
	ItemGUID string `json:"item_guid"`
	FavedAt  int64  `json:"faved_at"`
}

// AgentUnfaveJSONLRecord represents an unfave event in JSONL.
type AgentUnfaveJSONLRecord struct {
	Type      string `json:"type"` // "agent_unfave"
	AgentID   string `json:"agent_id"`
	ItemType  string `json:"item_type"` // "thread" | "message"
	ItemGUID  string `json:"item_guid"`
	UnfavedAt int64  `json:"unfaved_at"`
}

// FaveRemoveJSONLRecord represents a fave removal tombstone.
type FaveRemoveJSONLRecord struct {
	Type     string `json:"type"` // "fave_remove"
	AgentID  string `json:"agent_id"`
	ItemType string `json:"item_type"`
	ItemGUID string `json:"item_guid"`
	Seq      int64  `json:"seq,omitempty"`
	TS       int64  `json:"ts"`
}
