package db

// ReactionJSONLRecord represents a reaction event in JSONL.
type ReactionJSONLRecord struct {
	Type        string `json:"type"` // "reaction"
	MessageGUID string `json:"message_guid"`
	AgentID     string `json:"agent_id"`
	Emoji       string `json:"emoji"`
	ReactedAt   int64  `json:"reacted_at"`
}
