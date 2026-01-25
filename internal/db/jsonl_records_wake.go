package db

// WakeConditionJSONLRecord represents a wake condition in JSONL.
type WakeConditionJSONLRecord struct {
	Type           string   `json:"type"` // "wake_condition"
	GUID           string   `json:"guid"`
	AgentID        string   `json:"agent_id"`
	SetBy          string   `json:"set_by"`
	CondType       string   `json:"cond_type"` // on_mention, after, pattern, prompt
	Pattern        *string  `json:"pattern,omitempty"`
	OnAgents       []string `json:"on_agents,omitempty"`
	InThread       *string  `json:"in_thread,omitempty"`
	AfterMs        *int64   `json:"after_ms,omitempty"`
	UseRouter      bool     `json:"use_router,omitempty"`
	Prompt         *string  `json:"prompt,omitempty"`
	PromptText     *string  `json:"prompt_text,omitempty"`
	PollIntervalMs *int64   `json:"poll_interval_ms,omitempty"`
	PersistMode    string   `json:"persist_mode,omitempty"` // "", "persist", "persist_until_bye", "persist_restore_back"
	Paused         bool     `json:"paused,omitempty"`
	CreatedAt      int64    `json:"created_at"`
	ExpiresAt      *int64   `json:"expires_at,omitempty"`
}

// WakeConditionClearJSONLRecord represents clearing wake conditions.
type WakeConditionClearJSONLRecord struct {
	Type      string `json:"type"` // "wake_condition_clear"
	AgentID   string `json:"agent_id"`
	ClearedAt int64  `json:"cleared_at"`
}

// WakeConditionDeleteJSONLRecord represents deleting a specific wake condition.
type WakeConditionDeleteJSONLRecord struct {
	Type      string `json:"type"` // "wake_condition_delete"
	GUID      string `json:"guid"`
	DeletedAt int64  `json:"deleted_at"`
}

// WakeConditionPauseJSONLRecord represents pausing wake conditions (for restore-on-back).
type WakeConditionPauseJSONLRecord struct {
	Type     string `json:"type"` // "wake_condition_pause"
	AgentID  string `json:"agent_id"`
	PausedAt int64  `json:"paused_at"`
}

// WakeConditionResumeJSONLRecord represents resuming wake conditions.
type WakeConditionResumeJSONLRecord struct {
	Type      string `json:"type"` // "wake_condition_resume"
	AgentID   string `json:"agent_id"`
	ResumedAt int64  `json:"resumed_at"`
}

// WakeConditionClearByByeJSONLRecord represents clearing persist-until-bye conditions.
type WakeConditionClearByByeJSONLRecord struct {
	Type      string `json:"type"` // "wake_condition_clear_by_bye"
	AgentID   string `json:"agent_id"`
	ClearedAt int64  `json:"cleared_at"`
}

// WakeConditionResetJSONLRecord represents resetting a timer condition.
type WakeConditionResetJSONLRecord struct {
	Type      string `json:"type"` // "wake_condition_reset"
	GUID      string `json:"guid"`
	ExpiresAt int64  `json:"expires_at"`
	ResetAt   int64  `json:"reset_at"`
}
