package db

import "github.com/adamavenir/fray/internal/types"

// QuestionJSONLRecord represents a question entry in JSONL.
type QuestionJSONLRecord struct {
	Type       string                 `json:"type"`
	GUID       string                 `json:"guid"`
	Re         string                 `json:"re"`
	FromAgent  string                 `json:"from_agent"`
	ToAgent    *string                `json:"to,omitempty"`
	Status     string                 `json:"status"`
	ThreadGUID *string                `json:"thread_guid,omitempty"`
	AskedIn    *string                `json:"asked_in,omitempty"`
	AnsweredIn *string                `json:"answered_in,omitempty"`
	Options    []types.QuestionOption `json:"options,omitempty"`
	CreatedAt  int64                  `json:"created_at"`
}

// QuestionUpdateJSONLRecord represents a question update entry in JSONL.
type QuestionUpdateJSONLRecord struct {
	Type       string  `json:"type"`
	GUID       string  `json:"guid"`
	Status     *string `json:"status,omitempty"`
	ToAgent    *string `json:"to,omitempty"`
	ThreadGUID *string `json:"thread_guid,omitempty"`
	AskedIn    *string `json:"asked_in,omitempty"`
	AnsweredIn *string `json:"answered_in,omitempty"`
}
