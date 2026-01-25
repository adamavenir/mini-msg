package chat

// TokenUsage holds token usage data (for activity panel).
type TokenUsage struct {
	SessionID   string            `json:"sessionId"`
	TotalTokens int64             `json:"totalTokens"`
	Entries     []TokenUsageEntry `json:"entries"`
}

// TokenUsageEntry represents a single API call.
type TokenUsageEntry struct {
	Timestamp       string `json:"timestamp"`
	InputTokens     int64  `json:"inputTokens"`
	OutputTokens    int64  `json:"outputTokens"`
	CacheReadTokens int64  `json:"cacheReadTokens"`
}

// ContextTokens returns an estimate of current context window usage.
// InputTokens already includes cache_read_tokens (via effectiveInputTokens in claude.go),
// so we only return InputTokens to avoid double-counting cache.
// OutputTokens are not part of context (they're what the model generates).
func (t *TokenUsage) ContextTokens() int64 {
	if t == nil || len(t.Entries) == 0 {
		return 0
	}
	lastEntry := t.Entries[len(t.Entries)-1]
	return lastEntry.InputTokens
}
