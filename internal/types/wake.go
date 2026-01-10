package types

import "regexp"

// WakeConditionType represents the type of wake condition.
type WakeConditionType string

const (
	WakeConditionOnMention WakeConditionType = "on_mention" // Wake when specific users post
	WakeConditionAfter     WakeConditionType = "after"      // Wake after time delay
	WakeConditionPattern   WakeConditionType = "pattern"    // Wake on regex pattern match
)

// WakeCondition represents a condition that can trigger an agent wake.
type WakeCondition struct {
	GUID      string            `json:"guid"`
	AgentID   string            `json:"agent_id"`             // Agent to wake
	SetBy     string            `json:"set_by"`               // Agent who set this condition
	Type      WakeConditionType `json:"type"`                 // on_mention, after, pattern
	Pattern   *string           `json:"pattern,omitempty"`    // Regex pattern for pattern type
	OnAgents  []string          `json:"on_agents,omitempty"`  // Agents to watch for on_mention type
	InThread  *string           `json:"in_thread,omitempty"`  // Scope to specific thread (nil = anywhere except meta/)
	AfterMs   *int64            `json:"after_ms,omitempty"`   // Delay for after type
	UseRouter bool              `json:"use_router,omitempty"` // Use haiku router for ambiguous patterns
	Prompt    *string           `json:"prompt,omitempty"`     // Context passed on wake
	CreatedAt int64             `json:"created_at"`
	ExpiresAt *int64            `json:"expires_at,omitempty"` // For after type
}

// WakeConditionInput represents new wake condition data.
type WakeConditionInput struct {
	AgentID   string            `json:"agent_id"`
	SetBy     string            `json:"set_by"`
	Type      WakeConditionType `json:"type"`
	Pattern   *string           `json:"pattern,omitempty"`
	OnAgents  []string          `json:"on_agents,omitempty"`
	InThread  *string           `json:"in_thread,omitempty"`
	AfterMs   *int64            `json:"after_ms,omitempty"`
	UseRouter bool              `json:"use_router,omitempty"`
	Prompt    *string           `json:"prompt,omitempty"`
}

// CompiledPattern holds a pre-compiled regex for efficient matching.
type CompiledPattern struct {
	Condition *WakeCondition
	Regex     *regexp.Regexp
}

// CompilePattern compiles the pattern regex.
// Returns nil if compilation fails or pattern is nil.
func (wc *WakeCondition) CompilePattern() *CompiledPattern {
	if wc.Pattern == nil || wc.Type != WakeConditionPattern {
		return nil
	}

	re, err := regexp.Compile(*wc.Pattern)
	if err != nil {
		return nil
	}

	return &CompiledPattern{
		Condition: wc,
		Regex:     re,
	}
}

// MatchesMessage checks if a message body matches the pattern.
func (cp *CompiledPattern) MatchesMessage(body string) bool {
	if cp == nil || cp.Regex == nil {
		return false
	}
	return cp.Regex.MatchString(body)
}

// MatchesThread checks if the message is in a valid scope for this condition.
// Returns false for meta/ threads unless explicitly scoped there.
func (wc *WakeCondition) MatchesThread(home string) bool {
	// If scoped to specific thread, only match that thread
	if wc.InThread != nil {
		return home == *wc.InThread
	}

	// Default: exclude meta/ hierarchy unless explicitly scoped
	if len(home) >= 5 && home[:5] == "meta/" {
		return false
	}

	return true
}

// WakeRouterPayload is the input to the wake router mlld script.
type WakeRouterPayload struct {
	Message string  `json:"message"` // Message body
	From    string  `json:"from"`    // Who sent it
	Agent   string  `json:"agent"`   // Agent to potentially wake
	Pattern string  `json:"pattern"` // The matched pattern
	Thread  *string `json:"thread"`  // Thread context
}

// WakeRouterResult is the output from the wake router.
type WakeRouterResult struct {
	ShouldWake bool    `json:"shouldWake"`
	Reason     string  `json:"reason,omitempty"`
	Confidence float64 `json:"confidence"`
}
