package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	mlld "github.com/mlld-lang/mlld/sdk/go"
)

// StatusDisplay holds display overrides parsed from status.mld.
// All fields are optional - unset fields use defaults.
type StatusDisplay struct {
	Icon           *string `json:"icon,omitempty"`           // Custom icon character
	IconColor      *string `json:"iconcolor,omitempty"`      // Override icon color
	UsrColor       *string `json:"usrcolor,omitempty"`       // Override agent name color
	Message        *string `json:"message,omitempty"`        // Transformed status message
	MsgColor       *string `json:"msgcolor,omitempty"`       // Override message color
	UsedTokColor   *string `json:"usedtokcolor,omitempty"`   // Token progress bar used portion
	UnusedTokColor *string `json:"unusedtokcolor,omitempty"` // Token progress bar unused portion
	BgColor        *string `json:"bgcolor,omitempty"`        // Row background color
}

// StatusPayload is the input to status.mld.
type StatusPayload struct {
	Status string `json:"status"` // The agent's status message
}

// StatusInvoker wraps the mlld client for status display customization.
type StatusInvoker struct {
	client     *mlld.Client
	statusPath string
	available  bool

	mu    sync.RWMutex
	cache map[string]*StatusDisplay // status string -> display override
}

// NewStatusInvoker creates a StatusInvoker for the given fray project.
// Returns an invoker that gracefully degrades if status.mld is unavailable.
func NewStatusInvoker(frayDir string) *StatusInvoker {
	statusPath := filepath.Join(frayDir, "llm", "status.mld")

	// Check if status.mld file exists
	if _, err := os.Stat(statusPath); os.IsNotExist(err) {
		return &StatusInvoker{
			available: false,
			cache:     make(map[string]*StatusDisplay),
		}
	}

	client := mlld.New()
	client.Timeout = 5 * time.Second
	client.WorkingDir = filepath.Dir(frayDir) // Set to project root

	return &StatusInvoker{
		client:     client,
		statusPath: statusPath,
		available:  true,
		cache:      make(map[string]*StatusDisplay),
	}
}

// Available returns true if status.mld is available for use.
func (s *StatusInvoker) Available() bool {
	return s.available
}

// GetDisplay returns the StatusDisplay for a given status string.
// Results are cached - only invokes mlld when status changes.
// Returns nil if no overrides are needed.
func (s *StatusInvoker) GetDisplay(status string) *StatusDisplay {
	if status == "" {
		return nil
	}

	// Check cache first
	s.mu.RLock()
	if display, ok := s.cache[status]; ok {
		s.mu.RUnlock()
		return display
	}
	s.mu.RUnlock()

	// Invoke mlld
	display := s.invoke(status)

	// Cache result (even nil results to avoid re-invoking)
	s.mu.Lock()
	s.cache[status] = display
	s.mu.Unlock()

	return display
}

// ClearCache removes all cached results.
// Call this when status.mld file changes.
func (s *StatusInvoker) ClearCache() {
	s.mu.Lock()
	s.cache = make(map[string]*StatusDisplay)
	s.mu.Unlock()
}

// invoke executes status.mld with the given status string.
func (s *StatusInvoker) invoke(status string) *StatusDisplay {
	if !s.available {
		return nil
	}

	payload := StatusPayload{Status: status}

	result, err := s.client.Execute(s.statusPath, payload, nil)
	if err != nil {
		// Log error but don't fail - return nil (use defaults)
		fmt.Fprintf(os.Stderr, "[status] execute error: %v\n", err)
		return nil
	}

	// Parse JSON output
	var display StatusDisplay
	if err := json.Unmarshal([]byte(result.Output), &display); err != nil {
		// Not an error if output is empty or not JSON - just means no overrides
		if result.Output != "" && result.Output != "{}" {
			fmt.Fprintf(os.Stderr, "[status] parse error: %v (output: %s)\n", err, result.Output)
		}
		return nil
	}

	// Return nil if all fields are empty (no overrides)
	if display.Icon == nil && display.IconColor == nil && display.UsrColor == nil &&
		display.Message == nil && display.MsgColor == nil &&
		display.UsedTokColor == nil && display.UnusedTokColor == nil && display.BgColor == nil {
		return nil
	}

	return &display
}
