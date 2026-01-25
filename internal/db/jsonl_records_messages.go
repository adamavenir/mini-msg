package db

import "github.com/adamavenir/fray/internal/types"

// MessageJSONLRecord represents a message entry in JSONL.
type MessageJSONLRecord struct {
	Type             string              `json:"type"`
	ID               string              `json:"id"`
	ChannelID        *string             `json:"channel_id"`
	Home             string              `json:"home,omitempty"`
	FromAgent        string              `json:"from_agent"`
	Origin           string              `json:"origin,omitempty"`
	Seq              int64               `json:"seq,omitempty"`
	SessionID        *string             `json:"session_id,omitempty"`
	Body             string              `json:"body"`
	Mentions         []string            `json:"mentions"`
	ForkSessions     map[string]string   `json:"fork_sessions,omitempty"`
	Reactions        map[string][]string `json:"reactions,omitempty"`
	MsgType          types.MessageType   `json:"message_type"`
	References       *string             `json:"references,omitempty"`
	SurfaceMessage   *string             `json:"surface_message,omitempty"`
	ReplyTo          *string             `json:"reply_to"`
	QuoteMessageGUID *string             `json:"quote_message_guid,omitempty"`
	TS               int64               `json:"ts"`
	EditedAt         *int64              `json:"edited_at"`
	ArchivedAt       *int64              `json:"archived_at"`
}

// MessageUpdateJSONLRecord represents a message update entry in JSONL.
type MessageUpdateJSONLRecord struct {
	Type       string               `json:"type"`
	ID         string               `json:"id"`
	Body       *string              `json:"body,omitempty"`
	EditedAt   *int64               `json:"edited_at,omitempty"`
	ArchivedAt *int64               `json:"archived_at,omitempty"`
	Reactions  *map[string][]string `json:"reactions,omitempty"`
	Reason     *string              `json:"reason,omitempty"`
}

// MessageDeleteJSONLRecord represents a message deletion tombstone.
type MessageDeleteJSONLRecord struct {
	Type      string  `json:"type"` // "message_delete"
	ID        string  `json:"id"`
	DeletedBy *string `json:"deleted_by,omitempty"`
	Seq       int64   `json:"seq,omitempty"`
	TS        int64   `json:"ts"`
}
